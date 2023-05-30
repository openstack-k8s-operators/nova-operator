/*
Copyright 2022.

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

package controllers

import (
	"context"
	"fmt"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	common "github.com/openstack-k8s-operators/lib-common/modules/common"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	helper "github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"github.com/openstack-k8s-operators/lib-common/modules/common/labels"
	nad "github.com/openstack-k8s-operators/lib-common/modules/common/networkattachment"
	"github.com/openstack-k8s-operators/lib-common/modules/common/statefulset"
	util "github.com/openstack-k8s-operators/lib-common/modules/common/util"

	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/nova-operator/pkg/novascheduler"
)

// NovaSchedulerReconciler reconciles a NovaScheduler object
type NovaSchedulerReconciler struct {
	ReconcilerBase
}

// +kubebuilder:rbac:groups=nova.openstack.org,resources=novaschedulers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nova.openstack.org,resources=novaschedulers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=nova.openstack.org,resources=novaschedulers/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=k8s.cni.cncf.io,resources=network-attachment-definitions,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NovaScheduler object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.2/pkg/reconcile
func (r *NovaSchedulerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, _err error) {
	l := log.FromContext(ctx)

	// Fetch the NovaScheduler instance that needs to be reconciled
	instance := &novav1.NovaScheduler{}
	err := r.Client.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected.
			// For additional cleanup logic use finalizers. Return and don't requeue.
			l.Info("NovaScheduler instance not found, probably deleted before reconciled. Nothing to do.")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		l.Error(err, "Failed to read the NovaScheduler instance.")
		return ctrl.Result{}, err
	}

	h, err := helper.NewHelper(
		instance,
		r.Client,
		r.Kclient,
		r.Scheme,
		r.Log,
	)
	if err != nil {
		l.Error(err, "Failed to create lib-common Helper")
		return ctrl.Result{}, err
	}
	util.LogForObject(h, "Reconciling", instance)

	// initialize status fields
	if err = r.initStatus(ctx, h, instance); err != nil {
		return ctrl.Result{}, err
	}

	// Always update the instance status when exiting this function so we can
	// persist any changes happened during the current reconciliation.
	defer func() {
		// update the Ready condition based on the sub conditions
		if allSubConditionIsTrue(instance.Status) {
			instance.Status.Conditions.MarkTrue(
				condition.ReadyCondition, condition.ReadyMessage)
		} else {
			// something is not ready so reset the Ready condition
			instance.Status.Conditions.MarkUnknown(
				condition.ReadyCondition, condition.InitReason, condition.ReadyInitMessage)
			// and recalculate it based on the state of the rest of the conditions
			instance.Status.Conditions.Set(
				instance.Status.Conditions.Mirror(condition.ReadyCondition))
		}
		err := h.PatchInstance(ctx, instance)
		if err != nil {
			_err = err
			return
		}
	}()

	// TODO(gibi): Can we use a simple map[string][string] for hashes?
	// Collect hashes of all the input we depend on so that we can easily
	// detect if something is changed.
	hashes := make(map[string]env.Setter)

	secretHash, result, err := ensureSecret(
		ctx,
		types.NamespacedName{Namespace: instance.Namespace, Name: instance.Spec.Secret},
		// TODO(gibi): add keystoneAuthURL here is that is also passed via
		// the Secret. Also add DB and MQ user name here too if those are
		// passed via the Secret
		[]string{
			instance.Spec.PasswordSelectors.APIDatabase,
			instance.Spec.PasswordSelectors.Service,
			instance.Spec.PasswordSelectors.CellDatabase,
		},
		h.GetClient(),
		&instance.Status.Conditions,
		r.RequeueTimeout,
	)
	if err != nil {
		return result, err
	}

	hashes[instance.Spec.Secret] = env.SetValue(secretHash)

	// all our input checks out so report InputReady
	instance.Status.Conditions.MarkTrue(condition.InputReadyCondition, condition.InputReadyMessage)

	err = r.ensureConfigMaps(ctx, h, instance, &hashes)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Create hash over all the different input resources to identify if any of
	// those changed and a restart/recreate is required.
	// We have a special input, the registered cells, as the openstack service
	// needs to be restarted if this changes to refresh the in memory cell caches
	cellHash, err := hashOfStringMap(instance.Spec.RegisteredCells)
	if err != nil {
		return ctrl.Result{}, err
	}
	hashes["cells"] = env.SetValue(cellHash)

	inputHash, err := util.HashOfInputHashes(hashes)
	if err != nil {
		return ctrl.Result{}, err
	}

	instance.Status.Hash[common.InputHashName] = inputHash

	instance.Status.Conditions.MarkTrue(condition.ServiceConfigReadyCondition, condition.ServiceConfigReadyMessage)

	serviceAnnotations, result, err := ensureNetworkAttachments(ctx, h, instance.Spec.NetworkAttachments, &instance.Status.Conditions, r.RequeueTimeout)
	if (err != nil || result != ctrl.Result{}) {
		return result, err
	}

	result, err = r.ensureDeployment(ctx, h, instance, inputHash, serviceAnnotations)
	if (err != nil || result != ctrl.Result{}) {
		return result, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NovaSchedulerReconciler) SetupWithManager(mgr ctrl.Manager) error {

	secretToReconcileReqs := func(o client.Object) []reconcile.Request {
		var namespace string = o.GetNamespace()
		var secretName string = o.GetName()
		result := []reconcile.Request{}

		crs := &novav1.NovaSchedulerList{}
		listOpts := []client.ListOption{
			client.InNamespace(namespace),
		}
		if err := r.Client.List(context.TODO(), crs, listOpts...); err != nil {
			r.Log.Error(err, "Unable to retrieve the list of NovaScheduler")
			return nil
		}
		for _, cr := range crs.Items {
			if secretName == cr.Spec.Secret {
				name := client.ObjectKey{
					Namespace: namespace,
					Name:      cr.Name,
				}
				r.Log.Info(
					"Requesting reconcile due to secret change",
					"Secret", secretName, "NovaScheduler", cr.Name,
				)
				result = append(result, reconcile.Request{NamespacedName: name})
			}
		}
		if len(result) > 0 {
			return result
		}
		return nil
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&novav1.NovaScheduler{}).
		Owns(&v1.StatefulSet{}).
		Owns(&corev1.ConfigMap{}).
		// NOTE(gibi): If watching for every secret is too much then
		// we should consider switching to immutable Secret instead of watching
		// mutable Secrets
		Watches(&source.Kind{Type: &corev1.Secret{}},
			handler.EnqueueRequestsFromMapFunc(secretToReconcileReqs)).
		Complete(r)
}

func (r *NovaSchedulerReconciler) initStatus(
	ctx context.Context, h *helper.Helper, instance *novav1.NovaScheduler,
) error {
	if err := r.initConditions(ctx, h, instance); err != nil {
		return err
	}

	// NOTE(gibi): initialize the rest of the status fields here
	// so that the reconcile loop later can assume they are not nil.
	if instance.Status.Hash == nil {
		instance.Status.Hash = map[string]string{}
	}
	if instance.Status.NetworkAttachments == nil {
		instance.Status.NetworkAttachments = map[string][]string{}
	}

	return nil
}

func (r *NovaSchedulerReconciler) initConditions(
	ctx context.Context, h *helper.Helper, instance *novav1.NovaScheduler,
) error {
	if instance.Status.Conditions == nil {
		instance.Status.Conditions = condition.Conditions{}
		// initialize all conditions to Unknown
		cl := condition.CreateList(
			// TODO(gibi): Initialize each condition the controller reports
			// here to Unknown. By default only the top level Ready condition is
			// created by Conditions.Init()
			condition.UnknownCondition(
				condition.InputReadyCondition,
				condition.InitReason,
				condition.InputReadyInitMessage,
			),
			condition.UnknownCondition(
				condition.ServiceConfigReadyCondition,
				condition.InitReason,
				condition.ServiceConfigReadyInitMessage,
			),
			condition.UnknownCondition(
				condition.DeploymentReadyCondition,
				condition.InitReason,
				condition.DeploymentReadyInitMessage,
			),
			condition.UnknownCondition(
				condition.NetworkAttachmentsReadyCondition,
				condition.InitReason,
				condition.NetworkAttachmentsReadyInitMessage,
			),
		)

		instance.Status.Conditions.Init(&cl)
	}
	return nil
}

func (r *NovaSchedulerReconciler) ensureConfigMaps(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.NovaScheduler,
	hashes *map[string]env.Setter,
) error {
	err := r.generateConfigs(ctx, h, instance, hashes)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ServiceConfigReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.ServiceConfigReadyErrorMessage,
			err.Error()))
		return err
	}
	return nil
}

func (r *NovaSchedulerReconciler) generateConfigs(
	ctx context.Context, h *helper.Helper, instance *novav1.NovaScheduler, hashes *map[string]env.Setter,
) error {
	secret := &corev1.Secret{}
	namespace := instance.GetNamespace()
	secretName := types.NamespacedName{
		Namespace: namespace,
		Name:      instance.Spec.Secret,
	}
	err := h.GetClient().Get(ctx, secretName, secret)
	if err != nil {
		return err
	}

	apiMessageBusSecret := &corev1.Secret{}
	secretName = types.NamespacedName{
		Namespace: instance.Namespace,
		Name:      instance.Spec.APIMessageBusSecretName,
	}
	err = h.GetClient().Get(ctx, secretName, apiMessageBusSecret)
	if err != nil {
		util.LogForObject(
			h, "Failed reading Secret", instance,
			"APIMessageBusSecretName", instance.Spec.APIMessageBusSecretName)
		return err
	}

	templateParameters := map[string]interface{}{
		"service_name":           "nova-scheduler",
		"keystone_internal_url":  instance.Spec.KeystoneAuthURL,
		"nova_keystone_user":     instance.Spec.ServiceUser,
		"nova_keystone_password": string(secret.Data[instance.Spec.PasswordSelectors.Service]),
		"api_db_name":            instance.Spec.APIDatabaseUser, // fixme
		"api_db_user":            instance.Spec.APIDatabaseUser,
		"api_db_password":        string(secret.Data[instance.Spec.PasswordSelectors.APIDatabase]),
		"api_db_address":         instance.Spec.APIDatabaseHostname,
		"api_db_port":            3306,
		"cell_db_name":           instance.Spec.Cell0DatabaseUser, // fixme
		"cell_db_user":           instance.Spec.Cell0DatabaseUser,
		"cell_db_password":       string(secret.Data[instance.Spec.PasswordSelectors.CellDatabase]),
		"cell_db_address":        instance.Spec.Cell0DatabaseHostname,
		"cell_db_port":           3306,
		"openstack_cacert":       "",          // fixme
		"openstack_region_name":  "regionOne", // fixme
		"default_project_domain": "Default",   // fixme
		"default_user_domain":    "Default",   // fixme
		"transport_url":          string(apiMessageBusSecret.Data["transport_url"]),
	}
	extraData := map[string]string{}
	if instance.Spec.CustomServiceConfig != "" {
		extraData["02-nova-override.conf"] = instance.Spec.CustomServiceConfig
	}
	for key, data := range instance.Spec.DefaultConfigOverwrite {
		extraData[key] = data
	}

	cmLabels := labels.GetLabels(
		instance, labels.GetGroupLabel(NovaSchedulerLabelPrefix), map[string]string{},
	)

	return r.GenerateConfigs(
		ctx, h, instance, hashes, templateParameters, extraData, cmLabels, map[string]string{},
	)
}

func (r *NovaSchedulerReconciler) ensureDeployment(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.NovaScheduler,
	inputHash string,
	annotations map[string]string,
) (ctrl.Result, error) {
	serviceLabels := map[string]string{
		common.AppSelector: NovaSchedulerLabelPrefix,
	}

	ss := statefulset.NewStatefulSet(novascheduler.StatefulSet(instance, inputHash, serviceLabels, annotations), r.RequeueTimeout)

	ctrlResult, err := ss.CreateOrPatch(ctx, h)
	if err != nil && !k8s_errors.IsNotFound(err) {
		util.LogErrorForObject(h, err, "Deployment failed", instance)
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DeploymentReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.DeploymentReadyErrorMessage,
			err.Error()))
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{} || k8s_errors.IsNotFound(err)) {
		util.LogForObject(h, "Deployment in progress", instance)
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DeploymentReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DeploymentReadyRunningMessage))
		// It is OK to return success as we are watching for StatefulSet changes
		return ctrlResult, nil
	}

	instance.Status.ReadyCount = ss.GetStatefulSet().Status.ReadyReplicas

	// verify if network attachment matches expectations
	networkReady, networkAttachmentStatus, err := nad.VerifyNetworkStatusFromAnnotation(
		ctx,
		h,
		instance.Spec.NetworkAttachments,
		serviceLabels,
		instance.Status.ReadyCount)
	if err != nil {
		return ctrl.Result{}, err
	}

	instance.Status.NetworkAttachments = networkAttachmentStatus
	if networkReady {
		instance.Status.Conditions.MarkTrue(condition.NetworkAttachmentsReadyCondition, condition.NetworkAttachmentsReadyMessage)
	} else {
		err := fmt.Errorf("not all pods have interfaces with ips as configured in NetworkAttachments: %s", instance.Spec.NetworkAttachments)
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.NetworkAttachmentsReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.NetworkAttachmentsReadyErrorMessage,
			err.Error()))

		return ctrl.Result{}, err
	}

	if instance.Status.ReadyCount > 0 {
		util.LogForObject(h, "Deployment is ready", instance)
		instance.Status.Conditions.MarkTrue(condition.DeploymentReadyCondition, condition.DeploymentReadyMessage)
	} else {
		util.LogForObject(h, "Deployment is not ready", instance, "Status", ss.GetStatefulSet().Status)
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DeploymentReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DeploymentReadyRunningMessage))
		// It is OK to return success as we are watching for StatefulSet changes
		return ctrl.Result{}, nil
	}
	return ctrl.Result{}, nil
}
