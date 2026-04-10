/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cyborg

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"github.com/openstack-k8s-operators/lib-common/modules/common/labels"
	"github.com/openstack-k8s-operators/lib-common/modules/common/secret"
	"github.com/openstack-k8s-operators/lib-common/modules/common/statefulset"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"

	cyborgv1beta1 "github.com/openstack-k8s-operators/nova-operator/api/cyborg/v1beta1"
	cyborgservice "github.com/openstack-k8s-operators/nova-operator/internal/cyborg"
	cyborgconductor "github.com/openstack-k8s-operators/nova-operator/internal/cyborg/conductor"
)

const (
	conductorConfigSecretField = ".spec.configSecret" //nolint:gosec
	conductorTopologyField     = ".spec.topologyRef.Name"
)

// CyborgConductorReconciler reconciles a CyborgConductor object
//
//nolint:revive
type CyborgConductorReconciler struct {
	ReconcilerBase
}

// GetLogger returns a logger object with a prefix of "controller.name" and additional controller context fields
func (r *CyborgConductorReconciler) GetLogger(ctx context.Context) logr.Logger {
	return log.FromContext(ctx).WithName("Controllers").WithName("CyborgConductor")
}

// +kubebuilder:rbac:groups=cyborg.openstack.org,resources=cyborgconductors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cyborg.openstack.org,resources=cyborgconductors/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cyborg.openstack.org,resources=cyborgconductors/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete;

// Reconcile is part of the main kubernetes reconciliation loop for CyborgConductor resources.
func (r *CyborgConductorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, _err error) {
	Log := r.GetLogger(ctx)

	instance := &cyborgv1beta1.CyborgConductor{}
	err := r.Client.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			Log.Info("CyborgConductor instance not found, probably deleted before reconciled. Nothing to do.")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	Log.Info(fmt.Sprintf("Reconciling CyborgConductor '%s'", instance.Name))

	h, err := helper.NewHelper(instance, r.Client, r.Kclient, r.Scheme, Log)
	if err != nil {
		return ctrl.Result{}, err
	}

	isNewInstance := instance.Status.Conditions == nil
	savedConditions := instance.Status.Conditions.DeepCopy()

	defer func() {
		if instance.Status.Conditions.AllSubConditionIsTrue() {
			instance.Status.Conditions.MarkTrue(
				condition.ReadyCondition, condition.ReadyMessage)
		} else {
			instance.Status.Conditions.MarkUnknown(
				condition.ReadyCondition, condition.InitReason, condition.ReadyInitMessage)
			instance.Status.Conditions.Set(
				instance.Status.Conditions.Mirror(condition.ReadyCondition))
		}
		condition.RestoreLastTransitionTimes(&instance.Status.Conditions, savedConditions)
		err := h.PatchInstance(ctx, instance)
		if err != nil {
			_err = err
		}
	}()

	r.initStatus(instance)

	if !instance.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, r.reconcileDelete(ctx, h, instance)
	}

	if instance.DeletionTimestamp.IsZero() && controllerutil.AddFinalizer(instance, h.GetFinalizer()) || isNewInstance {
		return ctrl.Result{}, nil
	}

	// Read the sub-level secret created by the Cyborg controller
	subSecret := &corev1.Secret{}
	secretName := types.NamespacedName{Namespace: instance.Namespace, Name: instance.Spec.ConfigSecret}
	err = r.Client.Get(ctx, secretName, subSecret)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			Log.Info("Secret not found, waiting", "secret", instance.Spec.ConfigSecret)
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.InputReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				condition.InputReadyWaitingMessage))
			return ctrl.Result{RequeueAfter: r.RequeueTimeout}, nil
		}
		return ctrl.Result{}, err
	}

	// Hash the input secret so we detect changes to passwords, transport URL, etc.
	inputHashes := make(map[string]env.Setter)
	secretHash, err := util.ObjectHash(subSecret.Data)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error calculating input secret hash: %w", err)
	}
	inputHashes["input"] = env.SetValue(secretHash)

	instance.Status.Conditions.MarkTrue(condition.InputReadyCondition, condition.InputReadyMessage)

	// Generate config
	configVars := make(map[string]env.Setter)
	err = r.generateServiceConfig(ctx, instance, subSecret, h, &configVars)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ServiceConfigReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.ServiceConfigReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}

	for key, hashVal := range configVars {
		inputHashes[key] = hashVal
	}

	// Compute a combined hash of all inputs (secret + generated config).
	// This hash is set as CONFIG_HASH env var in the pod template so that
	// any change in the input secret or generated config triggers a rollout.
	inputHash, err := util.HashOfInputHashes(inputHashes)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error calculating combined input hash: %w", err)
	}
	instance.Status.Hash[common.InputHashName] = inputHash

	instance.Status.Conditions.MarkTrue(condition.ServiceConfigReadyCondition, condition.ServiceConfigReadyMessage)

	serviceLabels := map[string]string{
		common.AppSelector: cyborgconductor.ComponentName,
	}

	topology, err := ensureTopology(
		ctx,
		h,
		instance,
		instance.Name,
		&instance.Status.Conditions,
		labels.GetLabelSelector(serviceLabels),
	)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("waiting for Topology requirements: %w", err)
	}

	// Create or update the StatefulSet
	ssDef := cyborgconductor.StatefulSet(instance, inputHash, serviceLabels, topology)

	ss := statefulset.NewStatefulSet(ssDef, r.RequeueTimeout)
	ctrlResult, err := ss.CreateOrPatch(ctx, h)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DeploymentReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.DeploymentReadyErrorMessage,
			err.Error()))
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DeploymentReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DeploymentReadyRunningMessage))
		return ctrlResult, nil
	}

	ssObj := ss.GetStatefulSet()
	instance.Status.ReadyCount = ssObj.Status.ReadyReplicas
	if statefulset.IsReady(ssObj) {
		instance.Status.Conditions.MarkTrue(condition.DeploymentReadyCondition, condition.DeploymentReadyMessage)
	} else {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DeploymentReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DeploymentReadyRunningMessage))
	}

	instance.Status.ObservedGeneration = instance.Generation

	Log.Info("Successfully reconciled")
	return ctrl.Result{}, nil
}

func (r *CyborgConductorReconciler) initStatus(instance *cyborgv1beta1.CyborgConductor) {
	if instance.Status.Conditions == nil {
		instance.Status.Conditions = condition.Conditions{}
	}

	cl := condition.CreateList(
		condition.UnknownCondition(condition.ReadyCondition, condition.InitReason, condition.ReadyInitMessage),
		condition.UnknownCondition(condition.InputReadyCondition, condition.InitReason, condition.InputReadyInitMessage),
		condition.UnknownCondition(condition.ServiceConfigReadyCondition, condition.InitReason, condition.ServiceConfigReadyInitMessage),
		condition.UnknownCondition(condition.DeploymentReadyCondition, condition.InitReason, condition.DeploymentReadyInitMessage),
	)

	if instance.Spec.TopologyRef != nil {
		cl.Set(condition.UnknownCondition(
			condition.TopologyReadyCondition,
			condition.InitReason,
			condition.TopologyReadyInitMessage,
		))
	}

	instance.Status.Conditions.Init(&cl)

	if instance.Status.Hash == nil {
		instance.Status.Hash = make(map[string]string)
	}
}

func (r *CyborgConductorReconciler) generateServiceConfig(
	ctx context.Context,
	instance *cyborgv1beta1.CyborgConductor,
	subSecret *corev1.Secret,
	h *helper.Helper,
	envVars *map[string]env.Setter,
) error {
	Log := r.GetLogger(ctx)
	Log.Info("generateServiceConfig - reconciling config for CyborgConductor")

	var tlsCfg *tls.Service
	if instance.Spec.TLS.CaBundleSecretName != "" {
		tlsCfg = &tls.Service{}
	}

	databaseConnection := fmt.Sprintf("mysql+pymysql://%s:%s@%s/%s?read_default_file=/etc/my.cnf",
		string(subSecret.Data[DatabaseUsername]),
		string(subSecret.Data[DatabasePassword]),
		string(subSecret.Data[DatabaseHostname]),
		cyborgservice.DatabaseName,
	)

	templateParameters := map[string]any{
		"DatabaseConnection": databaseConnection,
		"TransportURL":       string(subSecret.Data[TransportURLSelector]),
	}

	if quorumQueues, ok := subSecret.Data[QuorumQueuesSelector]; ok && string(quorumQueues) == "true" {
		templateParameters["QuorumQueues"] = true
	}

	if keystoneURL, ok := subSecret.Data["KeystoneAuthURL"]; ok && len(keystoneURL) > 0 {
		templateParameters["KeystoneAuthURL"] = string(keystoneURL)
	}

	if serviceUser, ok := subSecret.Data["ServiceUser"]; ok && len(serviceUser) > 0 {
		templateParameters["ServiceUser"] = string(serviceUser)
	}

	if servicePassword, ok := subSecret.Data["ServicePassword"]; ok && len(servicePassword) > 0 {
		templateParameters["ServicePassword"] = string(servicePassword)
	}

	if region, ok := subSecret.Data["Region"]; ok && len(region) > 0 {
		templateParameters["Region"] = string(region)
	}

	if acid, ok := subSecret.Data["ACID"]; ok && len(acid) > 0 {
		templateParameters["ACID"] = string(acid)
		templateParameters["ACSecret"] = string(subSecret.Data["ACSecret"])
	}

	if instance.Spec.TLS.CaBundleSecretName != "" {
		templateParameters["CaFilePath"] = tls.DownstreamTLSCABundlePath
	}

	customData := map[string]string{
		"my.cnf": generateMyCnf(tlsCfg),
	}

	serviceLabels := labels.GetLabels(instance, labels.GetGroupLabel(cyborgservice.ServiceName), map[string]string{})

	if instance.Spec.CustomServiceConfig != "" {
		customData["01-service-custom.conf"] = instance.Spec.CustomServiceConfig
	}

	cms := []util.Template{
		{
			Name:          fmt.Sprintf("%s-config-data", instance.GetName()),
			Namespace:     instance.GetNamespace(),
			Type:          util.TemplateTypeConfig,
			InstanceType:  instance.GetObjectKind().GroupVersionKind().Kind,
			ConfigOptions: templateParameters,
			CustomData:    customData,
			Labels:        serviceLabels,
			AdditionalTemplate: map[string]string{
				"00-default.conf":              "/cyborg/00-default.conf",
				"cyborg-conductor-config.json": "/cyborg/conductor/cyborg-conductor-config.json",
			},
		},
	}

	return secret.EnsureSecrets(ctx, h, instance, cms, envVars)
}

func generateMyCnf(tlsCfg *tls.Service) string {
	if tlsCfg != nil {
		return fmt.Sprintf("[client]\nssl-ca=%s\nssl=1\n", tls.DownstreamTLSCABundlePath)
	}
	return "[client]\n"
}

// SetupWithManager sets up the controller with the Manager.
func (r *CyborgConductorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&cyborgv1beta1.CyborgConductor{},
		conductorConfigSecretField,
		func(rawObj client.Object) []string {
			cr := rawObj.(*cyborgv1beta1.CyborgConductor)
			if cr.Spec.ConfigSecret == "" {
				return nil
			}
			return []string{cr.Spec.ConfigSecret}
		},
	); err != nil {
		return err
	}

	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&cyborgv1beta1.CyborgConductor{},
		conductorTopologyField,
		func(rawObj client.Object) []string {
			cr := rawObj.(*cyborgv1beta1.CyborgConductor)
			if cr.Spec.TopologyRef == nil {
				return nil
			}
			return []string{cr.Spec.TopologyRef.Name}
		},
	); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&cyborgv1beta1.CyborgConductor{}).
		Owns(&corev1.Secret{}).
		Owns(&appsv1.StatefulSet{}).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.findConductorsForSecret),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(
			&topologyv1.Topology{},
			handler.EnqueueRequestsFromMapFunc(r.findConductorsForTopology),
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		Named("cyborg-cyborgconductor").
		Complete(r)
}

func (r *CyborgConductorReconciler) findConductorsForTopology(ctx context.Context, src client.Object) []reconcile.Request {
	Log := r.GetLogger(ctx)
	crList := &cyborgv1beta1.CyborgConductorList{}
	listOps := &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(conductorTopologyField, src.GetName()),
		Namespace:     src.GetNamespace(),
	}
	err := r.Client.List(ctx, crList, listOps)
	if err != nil {
		Log.Error(err, "listing CyborgConductors for topology change")
		return nil
	}

	requests := make([]reconcile.Request, 0, len(crList.Items))
	for _, item := range crList.Items {
		Log.Info(fmt.Sprintf("Topology %s changed, reconciling CyborgConductor %s", src.GetName(), item.GetName()))
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      item.GetName(),
				Namespace: item.GetNamespace(),
			},
		})
	}
	return requests
}

func (r *CyborgConductorReconciler) findConductorsForSecret(ctx context.Context, src client.Object) []reconcile.Request {
	Log := r.GetLogger(ctx)
	crList := &cyborgv1beta1.CyborgConductorList{}
	listOps := &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(conductorConfigSecretField, src.GetName()),
		Namespace:     src.GetNamespace(),
	}
	err := r.Client.List(ctx, crList, listOps)
	if err != nil {
		Log.Error(err, "listing CyborgConductors for secret change")
		return nil
	}

	requests := make([]reconcile.Request, 0, len(crList.Items))
	for _, item := range crList.Items {
		Log.Info(fmt.Sprintf("Secret %s changed, reconciling CyborgConductor %s", src.GetName(), item.GetName()))
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      item.GetName(),
				Namespace: item.GetNamespace(),
			},
		})
	}
	return requests
}

func (r *CyborgConductorReconciler) reconcileDelete(
	ctx context.Context,
	h *helper.Helper,
	instance *cyborgv1beta1.CyborgConductor,
) error {
	Log := r.GetLogger(ctx)

	Log.Info("Reconciling delete")

	// Remove finalizer from the referenced Topology CR
	if _, err := topologyv1.EnsureDeletedTopologyRef(
		ctx,
		h,
		instance.Status.LastAppliedTopology,
		instance.Name,
	); err != nil {
		return err
	}

	// Successfully cleaned up everything. So as the final step let's remove the
	// finalizer from ourselves to allow the deletion of CyborgConductor CR itself
	updated := controllerutil.RemoveFinalizer(instance, h.GetFinalizer())
	if updated {
		Log.Info("Removed finalizer from ourselves")
	}

	Log.Info("Reconciled delete successfully")
	return nil
}
