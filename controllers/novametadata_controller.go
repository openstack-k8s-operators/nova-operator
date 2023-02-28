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

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	common "github.com/openstack-k8s-operators/lib-common/modules/common"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	helper "github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"github.com/openstack-k8s-operators/lib-common/modules/common/labels"
	"github.com/openstack-k8s-operators/lib-common/modules/common/statefulset"
	util "github.com/openstack-k8s-operators/lib-common/modules/common/util"
	novav1beta1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/nova-operator/pkg/novametadata"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
)

// NovaMetadataReconciler reconciles a NovaMetadata object
type NovaMetadataReconciler struct {
	ReconcilerBase
}

//+kubebuilder:rbac:groups=nova.openstack.org,resources=novametadata,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=nova.openstack.org,resources=novametadata/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=nova.openstack.org,resources=novametadata/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NovaMetadata object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.2/pkg/reconcile
func (r *NovaMetadataReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, _err error) {
	l := log.FromContext(ctx)

	// Fetch the NovaMetadata instance that needs to be reconciled
	instance := &novav1beta1.NovaMetadata{}
	err := r.Client.Get(ctx, req.NamespacedName, instance)

	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected.
			// For additional cleanup logic use finalizers. Return and don't requeue.
			l.Info("NovaMetadata instance not found, probably deleted before reconciled. Nothing to do.")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		l.Error(err, "Failed to read the NovaMetadata instance.")
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
		return ctrl.Result{}, err
	}

	util.LogForObject(h, "Reconciling", instance)

	// initialize status fields
	if err = r.initStatus(ctx, h, instance); err != nil {
		return ctrl.Result{}, err
	}

	// Always update the instance status when exiting this function so we can
	// persist any changes happend during the current reconciliation.
	defer func() {
		// update the overall status condition if service is ready
		if allSubConditionIsTrue(instance.Status) {
			instance.Status.Conditions.MarkTrue(
				condition.ReadyCondition, condition.ReadyMessage)
		}
		err := h.PatchInstance(ctx, instance)
		if err != nil {
			_err = err
			return
		}
	}()

	if !instance.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, r.reconcileDelete(ctx, h, instance)
	}

	hashes := make(map[string]env.Setter)

	secretHash, result, err := ensureSecret(
		ctx,
		types.NamespacedName{Namespace: instance.Namespace, Name: instance.Spec.Secret},
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

	// create hash over all the different input resources to identify if any of
	// those changed and a restart/recreate is required.
	inputHash, err := hashOfInputHashes(ctx, hashes)
	if err != nil {
		return ctrl.Result{}, err
	}

	instance.Status.Hash[common.InputHashName] = inputHash

	instance.Status.Conditions.MarkTrue(condition.ServiceConfigReadyCondition, condition.ServiceConfigReadyMessage)

	result, err = r.ensureDeployment(ctx, h, instance, inputHash)
	if (err != nil || result != ctrl.Result{}) {
		return result, err
	}

	util.LogForObject(h, "Successfully reconciled", instance)
	return ctrl.Result{}, nil
}

func (r *NovaMetadataReconciler) initStatus(
	ctx context.Context, h *helper.Helper, instance *novav1beta1.NovaMetadata,
) error {
	if err := r.initConditions(ctx, h, instance); err != nil {
		return err
	}

	if instance.Status.Hash == nil {
		instance.Status.Hash = map[string]string{}
	}

	return nil
}

func (r *NovaMetadataReconciler) initConditions(
	ctx context.Context, h *helper.Helper, instance *novav1beta1.NovaMetadata,
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
		)

		instance.Status.Conditions.Init(&cl)
	}
	return nil
}

func (r *NovaMetadataReconciler) ensureConfigMaps(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1beta1.NovaMetadata,
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

func (r *NovaMetadataReconciler) generateConfigs(
	ctx context.Context, h *helper.Helper, instance *novav1beta1.NovaMetadata, hashes *map[string]env.Setter,
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
		"service_name":           "nova-metadata",
		"api_db_name":            instance.Spec.APIDatabaseUser, // fixme
		"api_db_user":            instance.Spec.APIDatabaseUser,
		"api_db_password":        string(secret.Data[instance.Spec.PasswordSelectors.APIDatabase]),
		"api_db_address":         instance.Spec.APIDatabaseHostname,
		"api_db_port":            3306,
		"cell_db_name":           instance.Spec.CellDatabaseUser, // fixme
		"cell_db_user":           instance.Spec.CellDatabaseUser,
		"cell_db_password":       string(secret.Data[instance.Spec.PasswordSelectors.CellDatabase]),
		"cell_db_address":        instance.Spec.CellDatabaseHostname,
		"cell_db_port":           3306,
		"openstack_cacert":       "",          // fixme
		"openstack_region_name":  "regionOne", // fixme
		"default_project_domain": "Default",   // fixme
		"default_user_domain":    "Default",   // fixme
		"metadata_secret":        "42",        // fixme
		"log_file":               "/var/log/nova/nova-metadata.log",
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
		instance, labels.GetGroupLabel(NovaMetadataLabelPrefix), map[string]string{},
	)

	err = r.GenerateConfigs(
		ctx, h, instance, hashes, templateParameters, extraData, cmLabels,
	)
	return err
}

func (r *NovaMetadataReconciler) ensureDeployment(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1beta1.NovaMetadata,
	inputHash string,
) (ctrl.Result, error) {
	serviceLabels := map[string]string{
		common.AppSelector: NovaConductorLabelPrefix,
	}
	ss := statefulset.NewStatefulSet(novametadata.StatefulSet(instance, inputHash, serviceLabels), r.RequeueTimeout)
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

func (r *NovaMetadataReconciler) ensureServiceExposed(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1beta1.NovaMetadata,
) (ctrl.Result, error) {
	// TODO (ksambor) add logic to check exposed api 169.254.169.254
	return ctrl.Result{}, nil
}

func (r *NovaMetadataReconciler) reconcileDelete(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1beta1.NovaMetadata,
) error {
	util.LogForObject(h, "Reconciling delete", instance)

	// Successfully cleaned up everyting. So as the final step let's remove the
	// finalizer from ourselves to allow the deletion of NovaMetadata CR itself
	updated := controllerutil.RemoveFinalizer(instance, h.GetFinalizer())
	if updated {
		util.LogForObject(h, "Removed finalizer from ourselves", instance)
	}

	util.LogForObject(h, "Reconciled delete successfully", instance)
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NovaMetadataReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&novav1beta1.NovaMetadata{}).
		Owns(&v1.StatefulSet{}).
		Owns(&corev1.ConfigMap{}).
		Complete(r)
}
