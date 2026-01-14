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

package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/go-logr/logr"
	memcachedv1 "github.com/openstack-k8s-operators/infra-operator/apis/memcached/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	helper "github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"github.com/openstack-k8s-operators/lib-common/modules/common/labels"
	"github.com/openstack-k8s-operators/lib-common/modules/common/service"
	util "github.com/openstack-k8s-operators/lib-common/modules/common/util"

	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
)

// NovaCellReconciler reconciles a NovaCell object
type NovaCellReconciler struct {
	ReconcilerBase
}

// GetLogger returns a logger object with a prefix of "controller.name" and additional controller context fields
func (r *NovaCellReconciler) GetLogger(ctx context.Context) logr.Logger {
	return log.FromContext(ctx).WithName("Controllers").WithName("NovaCell")
}

//+kubebuilder:rbac:groups=nova.openstack.org,resources=novacells,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=nova.openstack.org,resources=novacells/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=nova.openstack.org,resources=novacells/finalizers,verbs=update;patch
// +kubebuilder:rbac:groups=memcached.openstack.org,resources=memcacheds,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=memcached.openstack.org,resources=memcacheds/finalizers,verbs=update;patch
// +kubebuilder:rbac:groups=topology.openstack.org,resources=topologies,verbs=get;list;watch;update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NovaCell object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.2/pkg/reconcile
func (r *NovaCellReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, _err error) {
	Log := r.GetLogger(ctx)

	// Fetch the NovaAPI instance that needs to be reconciled
	instance := &novav1.NovaCell{}
	err := r.Client.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected.
			// For additional cleanup logic use finalizers. Return and don't requeue.
			Log.Info("NovaCell instance not found, probably deleted before reconciled. Nothing to do.")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		Log.Error(err, "Failed to read the NovaCell instance.")
		return ctrl.Result{}, err
	}

	h, err := helper.NewHelper(
		instance,
		r.Client,
		r.Kclient,
		r.Scheme,
		Log,
	)
	if err != nil {
		Log.Error(err, "Failed to create lib-common Helper")
		return ctrl.Result{}, err
	}
	Log.Info("Reconciling")

	// Save a copy of the conditions so that we can restore the LastTransitionTime
	// when a condition's state doesn't change.
	savedConditions := instance.Status.Conditions.DeepCopy()
	// initialize status fields
	if err = r.initStatus(instance); err != nil {
		return ctrl.Result{}, err
	}
	instance.Status.ObservedGeneration = instance.Generation

	// Always update the instance status when exiting this function so we can
	// persist any changes happened during the current reconciliation.
	defer func() {
		// Don't update the status, if Reconciler Panics
		if r := recover(); r != nil {
			Log.Info(fmt.Sprintf("Panic during reconcile %v\n", r))
			panic(r)
		}
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
		condition.RestoreLastTransitionTimes(&instance.Status.Conditions, savedConditions)
		err := h.PatchInstance(ctx, instance)
		if err != nil {
			_err = err
			return
		}
	}()

	requiredSecretFields := []string{
		ServicePasswordSelector,
		TransportURLSelector,
		NotificationTransportURLSelector,
	}

	// For the compute config generation we need to read the input secrets
	_, result, secret, err := ensureSecret(
		ctx,
		types.NamespacedName{Namespace: instance.Namespace, Name: instance.Spec.Secret},
		requiredSecretFields,
		h.GetClient(),
		&instance.Status.Conditions,
		r.RequeueTimeout,
	)
	if (err != nil || result != ctrl.Result{}) {
		return result, err
	}

	// all our input checks out so report InputReady
	instance.Status.Conditions.MarkTrue(condition.InputReadyCondition, condition.InputReadyMessage)

	result, err = r.ensureConductor(ctx, instance)
	if err != nil {
		return result, err
	}

	if *instance.Spec.MetadataServiceTemplate.Enabled {
		result, err = r.ensureMetadata(ctx, instance)
		if err != nil {
			return result, err
		}
	} else {
		// The NovaMetadata is explicitly disable so we delete its deployment
		// if exists
		err = r.ensureMetadataDeleted(ctx, instance)
		if err != nil {
			return ctrl.Result{}, err
		}
		instance.Status.Conditions.Remove(novav1.NovaMetadataReadyCondition)
		instance.Status.MetadataServiceReadyCount = 0

	}

	// manage nova-computes with selected driver based on created template
	// we need to keep statuses for deleting and discover action and also create map with hashes
	// to run discover job only when all computes are deployed and never discovered
	computeTemplatesHashMap := make(map[string]string)
	for computeName, computeTemplate := range instance.Spec.NovaComputeTemplates {
		computeStatus := r.ensureNovaCompute(ctx, instance, computeTemplate, computeName)
		instance.Status.NovaComputesStatus[computeName] = computeStatus
		// We hash the entire compute template to keep track of changes in the number of replicas,
		// allowing us to discover nodes accordingly
		computeTemplatesHashMap[computeName], _ = util.ObjectHash(computeTemplate)
	}

	// We need to delete nova computes based on current templates and statuses from previous runs
	for computeName := range instance.Status.NovaComputesStatus {
		_, ok := instance.Spec.NovaComputeTemplates[computeName]
		if !ok {
			err = r.ensureNovaComputeDeleted(ctx, instance, computeName)
			if err != nil {
				return ctrl.Result{}, err
			}
			delete(instance.Status.NovaComputesStatus, computeName)
		}

	}

	// We need to check if all computes are deployed
	if len(instance.Spec.NovaComputeTemplates) == 0 {
		Log.Info("No nova compute ironic/fake driver service definition in cell")
		instance.Status.Conditions.Remove(novav1.NovaAllControlPlaneComputesReadyCondition)
	} else {
		failedComputes := []string{}
		readyComputes := []string{}
		for computeName, computeStatus := range instance.Status.NovaComputesStatus {
			if computeStatus.Deployed {
				readyComputes = append(readyComputes, computeName)
			}
			if computeStatus.Errors {
				failedComputes = append(failedComputes, computeName)
			}
		}
		if len(instance.Spec.NovaComputeTemplates) == len(readyComputes) {
			instance.Status.Conditions.MarkTrue(
				novav1.NovaAllControlPlaneComputesReadyCondition, condition.ServiceConfigReadyMessage,
			)
		}

		Log.Info("Nova compute ironic/fake driver control plane service statuses",
			"ready", readyComputes,
			"failed", failedComputes,
		)
	}

	computeTemplatesHash, err := hashOfStringMap(computeTemplatesHashMap)
	if err != nil {
		return ctrl.Result{}, err
	}
	instance.Status.Hash[novav1.ComputeDiscoverHashKey] = computeTemplatesHash

	cellHasVNCService := (*instance.Spec.NoVNCProxyServiceTemplate.Enabled)
	if cellHasVNCService {
		result, err = r.ensureNoVNCProxy(ctx, instance)
		if err != nil {
			return result, err
		}
	} else {
		// The NoVNCProxy is explicitly disable for this cell so we delete its
		// deployment if exists
		err = r.ensureNoVNCProxyDeleted(ctx, instance)
		if err != nil {
			return ctrl.Result{}, err
		}
		instance.Status.Conditions.Remove(novav1.NovaNoVNCProxyReadyCondition)
	}

	// We need to wait for the NovaNoVNCProxy to become Ready before we can try
	// to generate the compute config secret as that needs the endpoint of the
	// proxy to be included.
	// However NovaNoVNCProxy is never deployed in cell0, and optional in other
	// cells too.
	if cellHasVNCService && !instance.Status.Conditions.IsTrue(novav1.NovaNoVNCProxyReadyCondition) {
		Log.Info("Waiting for the NovaNoVNCProxyService to become Ready before generating the compute config")
		return ctrl.Result{}, nil
	}

	var vncProxyURL *string
	if cellHasVNCService {
		vncProxyURL, err = r.getVNCProxyURL(ctx, h, instance)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	if instance.Spec.CellName != novav1.Cell0Name {
		result, err = r.ensureComputeConfig(ctx, h, instance, secret, vncProxyURL)
		if (err != nil || result != ctrl.Result{}) {
			return result, err
		}

	} else {
		instance.Status.Conditions.Remove(novav1.NovaComputeServiceConfigReady)
	}

	Log.Info("Successfully reconciled")
	return ctrl.Result{}, nil
}

func (r *NovaCellReconciler) initStatus(
	instance *novav1.NovaCell,
) error {
	if err := r.initConditions(instance); err != nil {
		return err
	}
	if instance.Status.Hash == nil {
		instance.Status.Hash = map[string]string{}
	}
	if instance.Status.NovaComputesStatus == nil {
		instance.Status.NovaComputesStatus = map[string]novav1.NovaComputeCellStatus{}
	}

	return nil
}

func (r *NovaCellReconciler) initConditions(
	instance *novav1.NovaCell,
) error {
	if instance.Status.Conditions == nil {
		instance.Status.Conditions = condition.Conditions{}
	}
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
			novav1.NovaConductorReadyCondition,
			condition.InitReason,
			novav1.NovaConductorReadyInitMessage,
		),
		condition.UnknownCondition(
			novav1.NovaMetadataReadyCondition,
			condition.InitReason,
			novav1.NovaMetadataReadyInitMessage,
		),
		condition.UnknownCondition(
			novav1.NovaNoVNCProxyReadyCondition,
			condition.InitReason,
			novav1.NovaNoVNCProxyReadyInitMessage,
		),
		condition.UnknownCondition(
			novav1.NovaComputeServiceConfigReady,
			condition.InitReason,
			novav1.NovaComputeServiceConfigInitMessage,
		),
		condition.UnknownCondition(
			novav1.NovaAllControlPlaneComputesReadyCondition,
			condition.InitReason,
			novav1.NovaComputeReadyInitMessage,
		),
	)
	instance.Status.Conditions.Init(&cl)
	return nil
}

func (r *NovaCellReconciler) ensureConductor(
	ctx context.Context,
	instance *novav1.NovaCell,
) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)

	conductorSpec := novav1.NewNovaConductorSpec(instance.Spec)
	conductor := &novav1.NovaConductor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + "-conductor",
			Namespace: instance.Namespace,
		},
	}

	op, err := controllerutil.CreateOrPatch(ctx, r.Client, conductor, func() error {
		conductor.Spec = conductorSpec
		err := controllerutil.SetControllerReference(instance, conductor, r.Scheme)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			novav1.NovaConductorReadyCondition,
			condition.ErrorReason,
			condition.SeverityError,
			novav1.NovaConductorReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}

	if op != controllerutil.OperationResultNone {
		Log.Info(fmt.Sprintf("NovaConductor %s.", string(op)))
	}
	if conductor.Generation == conductor.Status.ObservedGeneration {
		instance.Status.ConductorServiceReadyCount = conductor.Status.ReadyCount

		c := conductor.Status.Conditions.Mirror(novav1.NovaConductorReadyCondition)
		// NOTE(gibi): it can be nil if the NovaConductor CR is created but no
		// reconciliation is run on it to initialize the ReadyCondition yet.
		if c != nil {
			instance.Status.Conditions.Set(c)
		}
	}

	return ctrl.Result{}, nil
}

func getNoVNCProxyName(instance *novav1.NovaCell) types.NamespacedName {
	return types.NamespacedName{Namespace: instance.Namespace, Name: instance.Name + "-novncproxy"}
}

func (r *NovaCellReconciler) ensureNoVNCProxy(
	ctx context.Context,
	instance *novav1.NovaCell,
) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	// There is a case when the user manually created a NoVNCProxy while it
	// was disabled in the cell and then tries to enable it in the cell.
	// One can think that this means the cell adopts the NoVNCProxy and
	// starts managing it.
	// However at least the label selector of the StatefulSet spec is
	// immutable, so the cell cannot simply start adopting an existing
	// NoVNCProxy. But instead the Cell CR will be in error state until the
	// human deletes the manually created NoVNCProxy and the the Cell will
	// create its own NoVNCProxy.
	// See https://kubernetes.io/docs/concepts/workloads/controllers/deployment/#label-selector-updates

	novncproxyName := getNoVNCProxyName(instance)
	novncproxy := &novav1.NovaNoVNCProxy{}
	err := r.Client.Get(ctx, novncproxyName, novncproxy)
	if err != nil && !k8s_errors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	// If it is not created by us, we don't touch it
	if !k8s_errors.IsNotFound(err) && !OwnedBy(novncproxy, instance) {
		err := fmt.Errorf(
			"%w NovaNoVNCProxy/%s as the cell is not owning it", util.ErrCannotUpdateObject, novncproxyName.Name)
		Log.Error(err,
			"NovaNoVNCProxy is enabled in this cell, but there is a "+
				"NovaNoVNCProxy CR not owned by the cell. We cannot update it. "+
				"Please delete the NovaNoVNCProxy.",
			"NovaNoVNCProxy", novncproxy)
		instance.Status.Conditions.Set(condition.FalseCondition(
			novav1.NovaNoVNCProxyReadyCondition,
			condition.ErrorReason,
			condition.SeverityError,
			novav1.NovaNoVNCProxyReadyErrorMessage,
			err.Error()))

		return ctrl.Result{}, err
	}

	// NoVNCProxy is either not exists, or it exists but owned by us so we can
	// create or update it
	novncproxySpec := novav1.NewNovaNoVNCProxySpec(instance.Spec)
	novncproxy = &novav1.NovaNoVNCProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      novncproxyName.Name,
			Namespace: instance.Namespace,
		},
	}

	op, err := controllerutil.CreateOrPatch(ctx, r.Client, novncproxy, func() error {
		novncproxy.Spec = novncproxySpec
		err := controllerutil.SetControllerReference(instance, novncproxy, r.Scheme)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			novav1.NovaNoVNCProxyReadyCondition,
			condition.ErrorReason,
			condition.SeverityError,
			novav1.NovaNoVNCProxyReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}

	if op != controllerutil.OperationResultNone {
		Log.Info(fmt.Sprintf("NovaNoVNCProxy %s.", string(op)))
	}

	if novncproxy.Generation == novncproxy.Status.ObservedGeneration {
		instance.Status.NoVNCPRoxyServiceReadyCount = novncproxy.Status.ReadyCount

		c := novncproxy.Status.Conditions.Mirror(novav1.NovaNoVNCProxyReadyCondition)

		if c != nil {
			instance.Status.Conditions.Set(c)
		}
	}

	return ctrl.Result{}, nil
}

func (r *NovaCellReconciler) ensureNoVNCProxyDeleted(
	ctx context.Context,
	instance *novav1.NovaCell,
) error {
	Log := r.GetLogger(ctx)
	novncproxyName := getNoVNCProxyName(instance)
	novncproxy := &novav1.NovaNoVNCProxy{}
	err := r.Client.Get(ctx, novncproxyName, novncproxy)
	if k8s_errors.IsNotFound(err) {
		// Nothing to do as it does not exists
		return nil
	}
	if err != nil {
		return err
	}
	// If it is not created by us, we don't touch it
	if !OwnedBy(novncproxy, instance) {
		Log.Info("NovaNoVNCProxy is disabled in this cell, but there is a "+
			"NovaNoVNCProxy CR not owned by the cell. Not deleting it.",
			"NovaNoVNCProxy", novncproxy)
		return nil
	}

	// OK this was created by us so we go and delete it
	err = r.Client.Delete(ctx, novncproxy)
	if err != nil && k8s_errors.IsNotFound(err) {
		return nil
	}
	Log.Info("NovaNoVNCProxy is disabled in this cell, so deleted NovaNoVNCProxy",
		"NovaNoVNCProxy", novncproxy)

	return nil
}

func (r *NovaCellReconciler) ensureMetadata(
	ctx context.Context,
	instance *novav1.NovaCell,
) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	// There is a case when the user manually created a NovaMetadata while it
	// was disabled in the NovaCell and then tries to enable it in NovaCell.
	// One can think that this means the cell adopts the NovaMetadata and
	// starts managing it.
	// However at least the label selector of the StatefulSet spec is
	// immutable, so the controller cannot simply start adopting an existing
	// NovaMetadata. But instead the our CR will be in error state until the
	// human deletes the manually created NovaMetadata and then Nova will
	// create its own NovaMetadata.
	// See https://kubernetes.io/docs/concepts/workloads/controllers/deployment/#label-selector-updates

	metadataName := getNovaMetadataName(instance)
	metadata := &novav1.NovaMetadata{}
	err := r.Client.Get(ctx, metadataName, metadata)
	if err != nil && !k8s_errors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	// If it is not created by us, we don't touch it
	if !k8s_errors.IsNotFound(err) && !OwnedBy(metadata, instance) {
		err := fmt.Errorf(
			"%w NovaMetadata/%s as the cell is not owning it", util.ErrCannotUpdateObject, metadata.Name)
		Log.Error(err,
			"NovaMetadata is enabled, but there is a "+
				"NovaMetadata CR not owned by us. We cannot update it. "+
				"Please delete the NovaMetadata.",
			"NovaMetadata", metadataName)
		instance.Status.Conditions.Set(condition.FalseCondition(
			novav1.NovaMetadataReadyCondition,
			condition.ErrorReason,
			condition.SeverityError,
			novav1.NovaMetadataReadyErrorMessage,
			err.Error()))

		return ctrl.Result{}, err
	}

	metadataSpec := novav1.NewNovaMetadataSpec(instance.Spec)

	metadata = &novav1.NovaMetadata{
		ObjectMeta: metav1.ObjectMeta{
			Name:      metadataName.Name,
			Namespace: instance.Namespace,
		},
	}

	op, err := controllerutil.CreateOrPatch(ctx, r.Client, metadata, func() error {
		metadata.Spec = metadataSpec
		err := controllerutil.SetControllerReference(instance, metadata, r.Scheme)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			novav1.NovaMetadataReadyCondition,
			condition.ErrorReason,
			condition.SeverityError,
			novav1.NovaMetadataReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}

	if op != controllerutil.OperationResultNone {
		Log.Info(fmt.Sprintf("NovaMetadata %s.", string(op)))
	}
	if metadata.Generation == metadata.Status.ObservedGeneration {
		instance.Status.MetadataServiceReadyCount = metadata.Status.ReadyCount

		c := metadata.Status.Conditions.Mirror(novav1.NovaMetadataReadyCondition)
		// NOTE(gibi): it can be nil if the NovaMetadata CR is created but no
		// reconciliation is run on it to initialize the ReadyCondition yet.
		if c != nil {
			instance.Status.Conditions.Set(c)
		}
	}

	return ctrl.Result{}, nil
}

// ensureComputeConfig ensures the the compute config Secret exists and up to
// date. The compute config Secret then can be used to configure a nova-compute
// service to connect to this cell.
func (r *NovaCellReconciler) ensureComputeConfig(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.NovaCell,
	secret corev1.Secret,
	vncProxyURL *string,
) (ctrl.Result, error) {
	err := r.generateComputeConfigs(ctx, h, instance, secret, vncProxyURL)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			novav1.NovaComputeServiceConfigReady,
			condition.ErrorReason,
			condition.SeverityWarning,
			novav1.NovaComputeServiceConfigErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}
	instance.Status.Conditions.MarkTrue(
		novav1.NovaComputeServiceConfigReady, condition.ServiceConfigReadyMessage,
	)

	return ctrl.Result{}, nil
}

func getNovaComputeName(instance client.Object, computeName string) types.NamespacedName {
	return types.NamespacedName{Namespace: instance.GetNamespace(), Name: instance.GetName() + "-" + computeName + "-compute"}
}

func (r *NovaCellReconciler) ensureNovaComputeDeleted(
	ctx context.Context,
	instance client.Object,
	computeName string,
) error {
	Log := r.GetLogger(ctx)
	fullComputeName := getNovaComputeName(instance, computeName)
	compute := &novav1.NovaCompute{}
	err := r.Client.Get(ctx, fullComputeName, compute)
	if k8s_errors.IsNotFound(err) {
		// Nothing to do as it does not exists
		return nil
	}
	if err != nil {
		return err
	}
	// If it is not created by us, we don't touch it
	if !OwnedBy(compute, instance) {
		Log.Info("NovaCompute isn't defined in the cell, but there is a  "+
			"NovaCompute CR not owned by us. Not deleting it.",
			"NovaCompute", compute)
		return nil
	}

	// OK this was created by us so we go and delete it
	err = r.Client.Delete(ctx, compute)
	if err != nil && k8s_errors.IsNotFound(err) {
		return err
	}
	Log.Info("NovaCompute isn't defined in the cell, so deleted NovaCompute", "NovaCompute", compute)

	return nil
}

func (r *NovaCellReconciler) ensureNovaCompute(
	ctx context.Context,
	instance *novav1.NovaCell,
	compute novav1.NovaComputeTemplate,
	computeName string,
) novav1.NovaComputeCellStatus {
	Log := r.GetLogger(ctx)
	// There is a case when the user manually created a NovaCompute with selected name.
	// One can think that this means the cell adopts the NovaCompute and
	// starts managing it.
	// However at least the label selector of the StatefulSet spec is
	// immutable, so the controller cannot simply start adopting an existing
	// NovaCompute. But instead the our CR will be in error state until the
	// human deletes the manually created NovaCompute and then Nova will
	// create its own NovaComputes.
	// See https://kubernetes.io/docs/concepts/workloads/controllers/deployment/#label-selector-updates

	fullComputeName := getNovaComputeName(instance, computeName)
	novacompute := &novav1.NovaCompute{}
	err := r.Client.Get(ctx, fullComputeName, novacompute)
	var computeStatus novav1.NovaComputeCellStatus
	if computeStatus, ok := instance.Status.NovaComputesStatus[computeName]; ok {
		computeStatus.Errors = false
	} else {
		computeStatus = novav1.NovaComputeCellStatus{Deployed: false, Errors: false}
	}
	if err != nil && !k8s_errors.IsNotFound(err) {
		computeStatus.Errors = true
		return computeStatus
	}

	// If it is not created by us, we don't touch it
	if !k8s_errors.IsNotFound(err) && !OwnedBy(novacompute, instance) {
		err := fmt.Errorf(
			"%w NovaCompute/%s as the cell is not owning it", util.ErrCannotUpdateObject, novacompute.Name)
		Log.Error(err,
			"NovaCompute is defined in the cell, but there is a "+
				"NovaCompute CR not owned by us. We cannot update it. "+
				"Please delete the NovaCompute.",
			"NovaCompute", fullComputeName)

		computeStatus.Errors = true

		return computeStatus
	}

	novacomputeSpec := novav1.NewNovaComputeSpec(instance.Spec, compute, computeName)
	novacompute = &novav1.NovaCompute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fullComputeName.Name,
			Namespace: instance.Namespace,
		},
	}

	op, err := controllerutil.CreateOrPatch(ctx, r.Client, novacompute, func() error {
		novacompute.Spec = novacomputeSpec
		err := controllerutil.SetControllerReference(instance, novacompute, r.Scheme)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		Log.Error(err, "Failed to create NovaCompute CR")
		computeStatus.Errors = true
		return computeStatus
	}

	if op != controllerutil.OperationResultNone {
		Log.Info(fmt.Sprintf("NovaCompute %s, NovaCompute.Name %s .", string(op), novacompute.Name))
	}

	if novacompute.Generation == novacompute.Status.ObservedGeneration && novacompute.IsReady() {
		// We wait for the novacompute to become Ready before we map it deployed.
		computeStatus.Deployed = true
	}

	return computeStatus
}

func (r *NovaCellReconciler) generateComputeConfigs(
	ctx context.Context, h *helper.Helper, instance *novav1.NovaCell,
	secret corev1.Secret, vncProxyURL *string,
) error {
	templateParameters := map[string]any{
		"service_name":               "nova-compute",
		"keystone_internal_url":      instance.Spec.KeystoneAuthURL,
		"nova_keystone_user":         instance.Spec.ServiceUser,
		"nova_keystone_password":     string(secret.Data[ServicePasswordSelector]),
		"openstack_region_name":      instance.Spec.Region,
		"default_project_domain":     "Default", // fixme
		"default_user_domain":        "Default", // fixme
		"compute_driver":             "libvirt.LibvirtDriver",
		"transport_url":              string(secret.Data[TransportURLSelector]),
		"notification_transport_url": string(secret.Data[NotificationTransportURLSelector]),
		QuorumQueuesTemplateKey:      parseQuorumQueues(secret.Data[QuorumQueuesTemplateKey]),
	}
	// vnc is optional so we only need to configure it for the compute
	// if the proxy service is deployed in the cell
	if vncProxyURL != nil {
		templateParameters["vnc_enabled"] = true
		templateParameters["novncproxy_base_url"] = *vncProxyURL
	} else {
		templateParameters["vnc_enabled"] = false
	}

	cmLabels := labels.GetLabels(
		instance, labels.GetGroupLabel(NovaCellLabelPrefix), map[string]string{},
	)

	hashes := make(map[string]env.Setter)

	configName := instance.GetName() + "-compute-config"
	err := r.GenerateConfigs(
		ctx, h, instance, configName, &hashes, templateParameters, map[string]string{}, cmLabels, map[string]string{},
	)
	if err != nil {
		return err
	}

	// TODO(gibi): can we make it simpler?
	a := &corev1.EnvVar{}
	hashes[configName](a)
	instance.Status.Hash[configName] = a.Value
	return nil
}

func (r *NovaCellReconciler) getVNCProxyURL(
	ctx context.Context, h *helper.Helper, instance *novav1.NovaCell,
) (*string, error) {
	// we should restructure this to be in the same order as the reset of the resources
	// <nova_instance>-<cell_name>-<service_name>-<service_type>
	vncServiceName := fmt.Sprintf("nova-novncproxy-%s-public", instance.Spec.CellName)

	svcOverride := instance.Spec.NoVNCProxyServiceTemplate.Override.Service
	if svcOverride == nil {
		svcOverride = &service.RoutedOverrideSpec{}
	}
	if svcOverride.EmbeddedLabelsAnnotations == nil {
		svcOverride.EmbeddedLabelsAnnotations = &service.EmbeddedLabelsAnnotations{}
	}

	vncSvc, err := service.GetServiceWithName(ctx, h, vncServiceName, instance.Namespace)
	if err != nil {
		return nil, err
	}
	svc, err := service.NewService(
		vncSvc,
		time.Duration(5)*time.Second,
		&svcOverride.OverrideSpec)
	if err != nil {
		return nil, err
	}

	// TODO: TLS
	vncProxyURL, err := svc.GetAPIEndpoint(svcOverride.EndpointURL, ptr.To(service.ProtocolHTTP), "/vnc_lite.html")
	if err != nil {
		return nil, err
	}

	return ptr.To(vncProxyURL), nil
}

func (r *NovaCellReconciler) findObjectsForSrc(ctx context.Context, src client.Object) []reconcile.Request {
	requests := []reconcile.Request{}

	Log := r.GetLogger(ctx)

	for _, field := range cellWatchFields {
		crList := &novav1.NovaCellList{}
		listOps := &client.ListOptions{
			FieldSelector: fields.OneTermEqualSelector(field, src.GetName()),
			Namespace:     src.GetNamespace(),
		}
		err := r.Client.List(ctx, crList, listOps)
		if err != nil {
			Log.Error(err, fmt.Sprintf("listing %s for field: %s - %s", crList.GroupVersionKind().Kind, field, src.GetNamespace()))
			return requests
		}

		for _, item := range crList.Items {
			Log.Info(fmt.Sprintf("input source %s changed, reconcile: %s - %s", src.GetName(), item.GetName(), item.GetNamespace()))

			requests = append(requests,
				reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      item.GetName(),
						Namespace: item.GetNamespace(),
					},
				},
			)
		}
	}

	return requests
}

// fields to index to reconcile when change
var (
	cellWatchFields = []string{
		passwordSecretField,
		topologyField,
	}
)

func (r *NovaCellReconciler) memcachedNamespaceMapFunc(ctx context.Context, src client.Object) []reconcile.Request {

	result := []reconcile.Request{}

	// get all Nova CRs
	novaCellList := &novav1.NovaCellList{}
	listOpts := []client.ListOption{
		client.InNamespace(src.GetNamespace()),
	}
	if err := r.Client.List(ctx, novaCellList, listOpts...); err != nil {
		return nil
	}

	for _, cr := range novaCellList.Items {
		if src.GetName() == cr.Spec.MemcachedInstance {
			name := client.ObjectKey{
				Namespace: src.GetNamespace(),
				Name:      cr.Name,
			}
			result = append(result, reconcile.Request{NamespacedName: name})
		}
	}
	if len(result) > 0 {
		return result
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NovaCellReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// index passwordSecretField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &novav1.NovaCell{}, passwordSecretField, func(rawObj client.Object) []string {
		// Extract the secret name from the spec, if one is provided
		cr := rawObj.(*novav1.NovaCell)
		if cr.Spec.Secret == "" {
			return nil
		}
		return []string{cr.Spec.Secret}
	}); err != nil {
		return err
	}

	// index topologyField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &novav1.NovaCell{}, topologyField, func(rawObj client.Object) []string {
		// Extract the topology name from the spec, if one is provided
		cr := rawObj.(*novav1.NovaCell)
		if cr.Spec.TopologyRef == nil {
			return nil
		}
		return []string{cr.Spec.TopologyRef.Name}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&novav1.NovaCell{}).
		Owns(&novav1.NovaConductor{}).
		Owns(&novav1.NovaMetadata{}).
		Owns(&novav1.NovaNoVNCProxy{}).
		Owns(&novav1.NovaCompute{}).
		// It generates and therefore owns the compute config secret
		Owns(&corev1.Secret{}).
		// watch the input secrets
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.findObjectsForSrc),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(
			&memcachedv1.Memcached{},
			handler.EnqueueRequestsFromMapFunc(r.memcachedNamespaceMapFunc),
		).
		Watches(&topologyv1.Topology{},
			handler.EnqueueRequestsFromMapFunc(r.findObjectsForSrc),
			builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Complete(r)
}
