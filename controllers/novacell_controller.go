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
	"time"

	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	helper "github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"github.com/openstack-k8s-operators/lib-common/modules/common/labels"
	"github.com/openstack-k8s-operators/lib-common/modules/common/service"
	util "github.com/openstack-k8s-operators/lib-common/modules/common/util"

	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
)

// NovaCellReconciler reconciles a NovaCell object
type NovaCellReconciler struct {
	ReconcilerBase
}

//+kubebuilder:rbac:groups=nova.openstack.org,resources=novacells,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=nova.openstack.org,resources=novacells/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=nova.openstack.org,resources=novacells/finalizers,verbs=update

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
	l := log.FromContext(ctx)

	// Fetch the NovaAPI instance that needs to be reconciled
	instance := &novav1.NovaCell{}
	err := r.Client.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected.
			// For additional cleanup logic use finalizers. Return and don't requeue.
			l.Info("NovaCell instance not found, probably deleted before reconciled. Nothing to do.")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		l.Error(err, "Failed to read the NovaCell instance.")
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

	// For the compute config generation we need to read the input secrets
	_, result, secret, err := ensureSecret(
		ctx,
		types.NamespacedName{Namespace: instance.Namespace, Name: instance.Spec.Secret},
		[]string{
			ServicePasswordSelector,
			TransportURLSelector,
		},
		h.GetClient(),
		&instance.Status.Conditions,
		r.RequeueTimeout,
	)
	if err != nil {
		return result, err
	}

	// all our input checks out so report InputReady
	instance.Status.Conditions.MarkTrue(condition.InputReadyCondition, condition.InputReadyMessage)

	result, err = r.ensureConductor(ctx, h, instance)
	if err != nil {
		return result, err
	}

	if *instance.Spec.MetadataServiceTemplate.Enabled {
		result, err = r.ensureMetadata(ctx, h, instance)
		if err != nil {
			return result, err
		}
	} else {
		// The NovaMetadata is explicitly disable so we delete its deployment
		// if exists
		err = r.ensureMetadataDeleted(ctx, h, instance)
		if err != nil {
			return ctrl.Result{}, err
		}
		instance.Status.Conditions.Remove(novav1.NovaMetadataReadyCondition)
		instance.Status.MetadataServiceReadyCount = 0

	}

	cellHasVNCService := (*instance.Spec.NoVNCProxyServiceTemplate.Enabled)
	if cellHasVNCService {
		result, err = r.ensureNoVNCProxy(ctx, h, instance)
		if err != nil {
			return result, err
		}
	} else {
		// The NoVNCProxy is explicitly disable for this cell so we delete its
		// deployment if exists
		err = r.ensureNoVNCProxyDeleted(ctx, h, instance)
		if err != nil {
			return ctrl.Result{}, err
		}
		instance.Status.Conditions.Remove(novav1.NovaNoVNCProxyReadyCondition)
	}

	for computeName, computeTemplate := range instance.Spec.NovaComputeTemplates {
		novaComputeStatus := novav1.NovaComputeCellStatus{Deployed: false, Errors: false}
		instance.Status.NovaComputesStatuses[computeName] = novaComputeStatus
		result, err = r.ensureNovaCompute(ctx, h, instance, computeTemplate, computeName)
	}

	// We need to delete nova computes based on current templates and statuses from previous runs
	for computeName := range instance.Status.NovaComputesStatuses {
		_, ok := instance.Spec.NovaComputeTemplates[computeName]
		if !ok {
			err = r.ensureNovaComputeDeleted(ctx, h, instance, computeName)
			if err != nil {
				return ctrl.Result{}, err
			}
			delete(instance.Status.NovaComputesStatuses, computeName)
		}

	}

	// We need to check if all computes are deployed
	if len(instance.Spec.NovaComputeTemplates) == 0 {
		instance.Status.Conditions.Remove(novav1.NovaAllComputesReadyCondition)
	} else {
		allComputesReady := false
		failedComputes := []string{}
		readyComputes := []string{}
		for computeName, computeStatus := range instance.Status.NovaComputesStatuses {
			if computeStatus.Deployed {
				readyComputes = append(readyComputes, computeName)
			}
			if computeStatus.Errors {
				failedComputes = append(failedComputes, computeName)
			}
		}
		if len(instance.Spec.NovaComputeTemplates) == len(readyComputes) {
			instance.Status.Conditions.MarkTrue(
				novav1.NovaAllComputesReadyCondition, condition.ServiceConfigReadyMessage,
			)
		}

		util.LogForObject(
			h, "Nova compute statuses", instance,
			"ready", readyComputes,
			"failed", failedComputes,
			"all computes ready", allComputesReady)
	}

	// We need to wait for the NovaNoVNCProxy to become Ready before we can try
	// to generate the compute config secret as that needs the endpoint of the
	// proxy to be included.
	// However NovaNoVNCProxy is never deployed in cell0, and optional in other
	// cells too.
	if cellHasVNCService && !instance.Status.Conditions.IsTrue(novav1.NovaNoVNCProxyReadyCondition) {
		util.LogForObject(
			h,
			"Waiting for the NovaNoVNCProxyService to become Ready before "+
				"generating the compute config", instance,
		)
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

	util.LogForObject(h, "Successfully reconciled", instance)
	return ctrl.Result{}, nil
}

func (r *NovaCellReconciler) initStatus(
	ctx context.Context, h *helper.Helper, instance *novav1.NovaCell,
) error {
	if err := r.initConditions(ctx, h, instance); err != nil {
		return err
	}
	if instance.Status.Hash == nil {
		instance.Status.Hash = map[string]string{}
	}
	if instance.Status.NovaComputesStatuses == nil {
		instance.Status.NovaComputesStatuses = map[string]novav1.NovaComputeCellStatus{}
	}

	return nil
}

func (r *NovaCellReconciler) initConditions(
	ctx context.Context, h *helper.Helper, instance *novav1.NovaCell,
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
				novav1.NovaAllComputesReadyCondition,
				condition.InitReason,
				novav1.NovaComputeReadyInitMessage,
			),
		)
		instance.Status.Conditions.Init(&cl)
	}
	return nil
}

func (r *NovaCellReconciler) ensureConductor(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.NovaCell,
) (ctrl.Result, error) {
	conductorSpec := novav1.NewNovaConductorSpec(instance.Spec)
	conductor := &novav1.NovaConductor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + "-conductor",
			Namespace: instance.Namespace,
		},
	}

	op, err := controllerutil.CreateOrPatch(ctx, r.Client, conductor, func() error {
		conductor.Spec = conductorSpec
		if len(conductor.Spec.NodeSelector) == 0 {
			conductor.Spec.NodeSelector = instance.Spec.NodeSelector
		}
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
		util.LogForObject(h, fmt.Sprintf("NovaConductor %s.", string(op)), instance, "NovaConductor.Name", conductor.Name)
	}

	instance.Status.ConductorServiceReadyCount = conductor.Status.ReadyCount

	c := conductor.Status.Conditions.Mirror(novav1.NovaConductorReadyCondition)
	// NOTE(gibi): it can be nil if the NovaConductor CR is created but no
	// reconciliation is run on it to initialize the ReadyCondition yet.
	if c != nil {
		instance.Status.Conditions.Set(c)
	}

	return ctrl.Result{}, nil
}

func getNoVNCProxyName(instance *novav1.NovaCell) types.NamespacedName {
	return types.NamespacedName{Namespace: instance.Namespace, Name: instance.Name + "-novncproxy"}
}

func (r *NovaCellReconciler) ensureNoVNCProxy(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.NovaCell,
) (ctrl.Result, error) {
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
			"cannot update NovaNoVNCProxy/%s as the cell is not owning it", novncproxyName.Name)
		util.LogErrorForObject(
			h, err,
			"NovaNoVNCProxy is enabled in this cell, but there is a "+
				"NovaNoVNCProxy CR not owned by the cell. We cannot update it. "+
				"Please delete the NovaNoVNCProxy.",
			instance, "NovaNoVNCProxy", novncproxy)
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
		if len(novncproxy.Spec.NodeSelector) == 0 {
			novncproxy.Spec.NodeSelector = instance.Spec.NodeSelector
		}
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
		util.LogForObject(h, fmt.Sprintf("NovaNoVNCProxy %s.", string(op)), instance, "NovaNoVNCProxy.Name", novncproxy.Name)
	}

	instance.Status.NoVNCPRoxyServiceReadyCount = novncproxy.Status.ReadyCount

	c := novncproxy.Status.Conditions.Mirror(novav1.NovaNoVNCProxyReadyCondition)

	if c != nil {
		instance.Status.Conditions.Set(c)
	}

	return ctrl.Result{}, nil
}

func (r *NovaCellReconciler) ensureNoVNCProxyDeleted(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.NovaCell,
) error {
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
		util.LogForObject(
			h, "NovaNoVNCProxy is disabled in this cell, but there is a "+
				"NovaNoVNCProxy CR not owned by the cell. Not deleting it.",
			instance, "NovaNoVNCProxy", novncproxy)
		return nil
	}

	// OK this was created by us so we go and delete it
	err = r.Client.Delete(ctx, novncproxy)
	if err != nil && k8s_errors.IsNotFound(err) {
		return nil
	}
	util.LogForObject(
		h, "NovaNoVNCProxy is disabled in this cell, so deleted NovaNoVNCProxy",
		instance, "NovaNoVNCProxy", novncproxy)

	return nil
}

func (r *NovaCellReconciler) ensureMetadata(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.NovaCell,
) (ctrl.Result, error) {

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
			"cannot update NovaMetadata/%s as the cell is not owning it", metadata.Name)
		util.LogErrorForObject(
			h, err,
			"NovaMetadata is enabled, but there is a "+
				"NovaMetadata CR not owned by us. We cannot update it. "+
				"Please delete the NovaMetadata.",
			instance, "NovaMetadata", metadataName)
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
		if len(metadata.Spec.NodeSelector) == 0 {
			metadata.Spec.NodeSelector = instance.Spec.NodeSelector
		}
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
		util.LogForObject(h, fmt.Sprintf("NovaMetadata %s.", string(op)), instance, "NovaMetadata.Name", metadata.Name)
	}

	instance.Status.MetadataServiceReadyCount = metadata.Status.ReadyCount

	c := metadata.Status.Conditions.Mirror(novav1.NovaMetadataReadyCondition)
	// NOTE(gibi): it can be nil if the NovaMetadata CR is created but no
	// reconciliation is run on it to initialize the ReadyCondition yet.
	if c != nil {
		instance.Status.Conditions.Set(c)
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
	h *helper.Helper,
	instance client.Object,
	computeName string,
) error {
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
		util.LogForObject(
			h, "NovaCompute isn't defined in the cell, but there is a  "+
				"NovaCompute CR not owned by us. Not deleting it.",
			instance, "NovaCompute", compute)
		return nil
	}

	// OK this was created by us so we go and delete it
	err = r.Client.Delete(ctx, compute)
	if err != nil && k8s_errors.IsNotFound(err) {
		return nil
	}
	util.LogForObject(
		h, "NovaCompute isn't defined in the cell, so deleted NovaCompute",
		instance, "NovaCompute", compute)

	return nil
}

func (r *NovaCellReconciler) ensureNovaCompute(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.NovaCell,
	compute novav1.NovaComputeTemplate,
	computeName string,
) (ctrl.Result, error) {
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
	if computeStatus, ok := instance.Status.NovaComputesStatuses[computeName]; ok {
		computeStatus.Errors = false
	} else {
		computeStatus = novav1.NovaComputeCellStatus{Deployed: false, Errors: false}
	}
	if err != nil && !k8s_errors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	// If it is not created by us, we don't touch it
	if !k8s_errors.IsNotFound(err) && !OwnedBy(novacompute, instance) {
		err := fmt.Errorf(
			"cannot update NovaCompute/%s as the cell is not owning it", novacompute.Name)
		util.LogErrorForObject(
			h, err,
			"NovaCompute is defined in the cell, but there is a "+
				"NovaCompute CR not owned by us. We cannot update it. "+
				"Please delete the NovaCompute.",
			instance, "NovaCompute", fullComputeName)

		computeStatus.Errors = true
		instance.Status.NovaComputesStatuses[computeName] = computeStatus

		return ctrl.Result{}, err
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
		if len(novacompute.Spec.NodeSelector) == 0 {
			novacompute.Spec.NodeSelector = instance.Spec.NodeSelector
		}
		err := controllerutil.SetControllerReference(instance, novacompute, r.Scheme)
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		computeStatus.Errors = true
		instance.Status.NovaComputesStatuses[computeName] = computeStatus
		return ctrl.Result{}, err
	}

	if op != controllerutil.OperationResultNone {
		util.LogForObject(h, fmt.Sprintf("NovaCompute %s.", string(op)), instance, "NovaCompute.Name", novacompute.Name)
	}

	computeStatus.Errors = false
	// TODO(ksambor) move seting deploy true to job that will run nova-manage cell_v2 discover_hosts
	computeStatus.Deployed = true
	instance.Status.NovaComputesStatuses[computeName] = computeStatus
	return ctrl.Result{}, nil
}

func (r *NovaCellReconciler) generateComputeConfigs(
	ctx context.Context, h *helper.Helper, instance *novav1.NovaCell,
	secret corev1.Secret, vncProxyURL *string,
) error {
	templateParameters := map[string]interface{}{
		"service_name":           "nova-compute",
		"keystone_internal_url":  instance.Spec.KeystoneAuthURL,
		"nova_keystone_user":     instance.Spec.ServiceUser,
		"nova_keystone_password": string(secret.Data[ServicePasswordSelector]),
		"openstack_cacert":       "",          // fixme
		"openstack_region_name":  "regionOne", // fixme
		"default_project_domain": "Default",   // fixme
		"default_user_domain":    "Default",   // fixme
		"compute_driver":         "libvirt.LibvirtDriver",
		"transport_url":          string(secret.Data[TransportURLSelector]),
		"log_file":               "/var/log/nova/nova-compute.log",
	}
	// vnc is optional so we only need to configure it for the compute
	// if the proxy service is deployed in the cell
	if vncProxyURL != nil {
		templateParameters["novncproxy_base_url"] = *vncProxyURL
	}

	cmLabels := labels.GetLabels(
		instance, labels.GetGroupLabel(NovaCellLabelPrefix), map[string]string{},
	)

	hashes := make(map[string]env.Setter)

	configName := instance.GetName() + "-compute-config"
	err := r.GenerateConfigs(
		ctx, h, instance, configName, &hashes, templateParameters, map[string]string{}, cmLabels, map[string]string{},
	)
	// TODO(gibi): can we make it simpler?
	a := &corev1.EnvVar{}
	hashes[configName](a)
	instance.Status.Hash[configName] = a.Value
	return err
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

// SetupWithManager sets up the controller with the Manager.
func (r *NovaCellReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&novav1.NovaCell{}).
		Owns(&novav1.NovaConductor{}).
		Owns(&novav1.NovaMetadata{}).
		Owns(&novav1.NovaNoVNCProxy{}).
		Owns(&novav1.NovaCompute{}).
		// It generates and therefor owns the compute config secret
		Owns(&corev1.Secret{}).
		// and it needs to watch the input secrets
		Watches(&source.Kind{Type: &corev1.Secret{}},
			handler.EnqueueRequestsFromMapFunc(r.GetSecretMapperFor(&novav1.NovaCellList{}))).
		Complete(r)
}
