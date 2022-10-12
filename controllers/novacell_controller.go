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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	helper "github.com/openstack-k8s-operators/lib-common/modules/common/helper"
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
	// persist any changes happend during the current reconciliation.
	defer func() {
		// update the overall status condition if service is ready
		if allSubConditionIsTrue(instance.Status) {
			instance.Status.Conditions.MarkTrue(
				condition.ReadyCondition, condition.ReadyMessage)
		}
		if err := h.SetAfter(instance); err != nil {
			util.LogErrorForObject(h, err, "Set after and calc patch/diff", instance)
			_err = err
			return
		}

		if changed := h.GetChanges()["status"]; changed {
			patch := client.MergeFrom(h.GetBeforeObject())

			err = r.Client.Status().Patch(ctx, instance, patch)
			if k8s_errors.IsConflict(err) {
				util.LogForObject(h, "Status update conflict", instance)
				_err = err
				return
			} else if err != nil && !k8s_errors.IsNotFound(err) {
				util.LogErrorForObject(h, err, "Status update failed", instance)
				_err = err
				return
			}
		}
	}()

	result, err = r.ensureConductor(ctx, h, instance)
	if err != nil {
		return result, err
	}

	return ctrl.Result{}, nil
}

func (r *NovaCellReconciler) initStatus(
	ctx context.Context, h *helper.Helper, instance *novav1.NovaCell,
) error {
	if err := r.initConditions(ctx, h, instance); err != nil {
		return err
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
				novav1.NovaConductorReadyCondition,
				condition.InitReason,
				novav1.NovaConductorReadyInitMessage,
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

		err := controllerutil.SetControllerReference(instance, conductor, r.Scheme)
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		condition.FalseCondition(
			novav1.NovaConductorReadyCondition,
			condition.ErrorReason,
			condition.SeverityError,
			novav1.NovaConductorReadyErrorMessage,
			err.Error(),
		)
		return ctrl.Result{}, err
	}

	if op != controllerutil.OperationResultNone {
		util.LogForObject(h, fmt.Sprintf("NovaConductor %s.", string(op)), instance, "NovaConductor.Name", conductor.Name)
	}

	c := conductor.Status.Conditions.Mirror(novav1.NovaConductorReadyCondition)
	// NOTE(gibi): it can be nil if the NovaConductor CR is created but no
	// reconciliation is run on it to initialize the ReadyCondition yet.
	if c != nil {
		instance.Status.Conditions.Set(c)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NovaCellReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&novav1.NovaCell{}).
		Owns(&novav1.NovaConductor{}).
		Complete(r)
}
