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

	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	common "github.com/openstack-k8s-operators/lib-common/modules/common"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	helper "github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
)

// NovaExternalComputeReconciler reconciles a NovaExternalCompute object
type NovaExternalComputeReconciler struct {
	ReconcilerBase
}

//+kubebuilder:rbac:groups=nova.openstack.org,resources=novaexternalcomputes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=nova.openstack.org,resources=novaexternalcomputes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=nova.openstack.org,resources=novaexternalcomputes/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete;

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.2/pkg/reconcile
func (r *NovaExternalComputeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, _err error) {
	l := log.FromContext(ctx)

	// Fetch the instance that needs to be reconciled
	instance := &novav1.NovaExternalCompute{}
	err := r.Client.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected.
			// For additional cleanup logic use finalizers. Return and don't requeue.
			l.Info("NovaExternalCompute instance not found, probably deleted before reconciled. Nothing to do.")
			return ctrl.Result{}, nil
		}
		l.Error(err, "Failed to read the NovaExternalCompute instance.")
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
	l.Info("Reconciling")

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

	updated := controllerutil.AddFinalizer(instance, h.GetFinalizer())
	if updated {
		l.Info("Added finalizer to ourselves")
		// we intentionally return imediately to force the deferred function
		// to persist the Instance with the finalizer. We need to have our own
		// finalizer persisted before we deploy any compute to avoid orphaning
		// the compute rows in our database during CR deletion.
		return ctrl.Result{}, nil
	}

	// TODO(gibi): Can we use a simple map[string][string] for hashes?
	// Collect hashes of all the input we depend on so that we can easily
	// detect if something is changed.
	hashes := make(map[string]env.Setter)

	inventoryHash, result, err := ensureConfigMap(
		ctx,

		types.NamespacedName{Namespace: instance.Namespace, Name: instance.Spec.InventoryConfigMapName},
		// NOTE(gibi): Add the fields here we expect to exists in the InventorySecret
		[]string{
			"inventory",
		},
		h.GetClient(),
		&instance.Status.Conditions,
		r.RequeueTimeout,
	)
	if err != nil {
		return result, err
	}
	hashes[instance.Spec.InventoryConfigMapName] = env.SetValue(inventoryHash)

	sshKeyHHash, result, err := ensureSecret(
		ctx,
		types.NamespacedName{Namespace: instance.Namespace, Name: instance.Spec.SSHKeySecretName},
		// NOTE(gibi): Add the fields here we expect to exists in the SSHKeySecret
		// This is based on the structure defined in the schema of kubernetes.io/ssh-auth
		//  https://kubernetes.io/docs/concepts/configuration/secret/#ssh-authentication-secrets
		[]string{
			"ssh-privatekey",
		},
		h.GetClient(),
		&instance.Status.Conditions,
		r.RequeueTimeout,
	)
	if err != nil {
		return result, err
	}
	hashes[instance.Spec.SSHKeySecretName] = env.SetValue(sshKeyHHash)

	_, result, err = r.ensureCellReady(ctx, instance)

	if (err != nil || result != ctrl.Result{}) {
		return result, err
	}

	// TODO(gibi): gather the information from the cell we need for the config
	// and hash that into our input hash

	// all our input checks out so report InputReady
	instance.Status.Conditions.MarkTrue(condition.InputReadyCondition, condition.InputReadyMessage)

	// TODO(gibi): generate service config here and include the hash of that
	// into the hashes

	// create hash over all the different input resources to identify if any of
	// those changed and a restart/recreate is required.
	inputHash, err := hashOfInputHashes(ctx, hashes)
	if err != nil {
		return ctrl.Result{}, err
	}
	instance.Status.Hash[common.InputHashName] = inputHash

	return ctrl.Result{}, nil
}

func (r *NovaExternalComputeReconciler) initStatus(
	ctx context.Context, h *helper.Helper, instance *novav1.NovaExternalCompute,
) error {
	if err := r.initConditions(ctx, h, instance); err != nil {
		return err
	}

	if instance.Status.Hash == nil {
		instance.Status.Hash = map[string]string{}
	}

	return nil
}

func (r *NovaExternalComputeReconciler) initConditions(
	ctx context.Context, h *helper.Helper, instance *novav1.NovaExternalCompute,
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
				novav1.NovaCellReadyCondition,
				condition.InitReason,
				novav1.NovaCellReadyInitMessage,
				instance.Spec.CellName,
			),
		/*
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
		*/
		)
		instance.Status.Conditions.Init(&cl)
	}
	return nil
}

func (r *NovaExternalComputeReconciler) reconcileDelete(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.NovaExternalCompute,
) error {
	l := log.FromContext(ctx)
	l.Info("Reconciling delete")

	// TODO(gibi): A compute is being removed from the system so we might want
	// to clean up the compure Service and the ComputeNode from the cell
	// database

	// Successfully cleaned up everyting. So as the final step let's remove the
	// finalizer from ourselves to allow the deletion of the CR itself
	updated := controllerutil.RemoveFinalizer(instance, h.GetFinalizer())
	if updated {
		l.Info("Removed finalizer from ourselves")
	}

	l.Info("Reconciled delete successfully")
	return nil
}

func (r *NovaExternalComputeReconciler) ensureCellReady(
	ctx context.Context,
	instance *novav1.NovaExternalCompute,
) (*novav1.NovaCell, ctrl.Result, error) {
	cell := &novav1.NovaCell{}
	cellCRName := getNovaCellCRName(instance.Spec.NovaInstance, instance.Spec.CellName)
	err := r.Client.Get(
		ctx, types.NamespacedName{
			Namespace: instance.Namespace,
			Name:      cellCRName,
		},
		cell,
	)

	if err != nil && !k8s_errors.IsNotFound(err) {
		instance.Status.Conditions.Set(
			condition.FalseCondition(
				novav1.NovaCellReadyCondition,
				condition.ErrorReason,
				condition.SeverityError,
				novav1.NovaCellReadyErrorMessage,
				cellCRName,
				err.Error(),
			),
		)
		return nil, ctrl.Result{}, fmt.Errorf("failed to query NovaCells %w", err)
	}

	if k8s_errors.IsNotFound(err) {
		instance.Status.Conditions.Set(
			condition.FalseCondition(
				novav1.NovaCellReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				novav1.NovaCellReadyNotExistsMessage,
				cellCRName,
			),
		)
		// Here we need to wait for the NovaCell to be created so we requeue explicitly
		return nil, ctrl.Result{RequeueAfter: r.RequeueTimeout}, nil
	}

	// We cannot move forward while the cell is not ready as we need to gather
	// information from the NovaCell to generate the compute config
	if !cell.IsReady() {
		instance.Status.Conditions.Set(
			condition.FalseCondition(
				novav1.NovaCellReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				novav1.NovaCellReadyNotReadyMessage,
				cellCRName,
			),
		)
		// TODO(gibi): this should not be an explicit requeue, instead we need
		// to add a watch for the NovaCell and just return without requeue here.
		return cell, ctrl.Result{RequeueAfter: r.RequeueTimeout}, nil
	}

	// our NovaCell is Ready
	instance.Status.Conditions.MarkTrue(
		novav1.NovaCellReadyCondition, novav1.NovaCellReadyMessage, cellCRName)

	return cell, ctrl.Result{}, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *NovaExternalComputeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&novav1.NovaExternalCompute{}).
		Complete(r)
}
