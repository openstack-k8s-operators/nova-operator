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

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	novav1beta1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
)

// NovaPlacementAPIReconciler reconciles a NovaPlacementAPI object
type NovaPlacementAPIReconciler struct {
	ReconcilerBase
}

// GetLog returns a logger object with a prefix of "controller.name" and additional controller context fields
func (r *NovaPlacementAPIReconciler) GetLogger(ctx context.Context) logr.Logger {
	return log.FromContext(ctx).WithName("Controllers").WithName("NovaPlacementAPI")
}

// +kubebuilder:rbac:groups=nova.openstack.org,resources=novaplacementapis,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nova.openstack.org,resources=novaplacementapis/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=nova.openstack.org,resources=novaplacementapis/finalizers,verbs=update;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=mariadb.openstack.org,resources=mariadbdatabases,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=mariadb.openstack.org,resources=mariadbdatabases/finalizers,verbs=update;patch
// +kubebuilder:rbac:groups=mariadb.openstack.org,resources=mariadbaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mariadb.openstack.org,resources=mariadbaccounts/finalizers,verbs=update;patch
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneapis,verbs=get;list;watch;
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneservices,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneendpoints,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=k8s.cni.cncf.io,resources=network-attachment-definitions,verbs=get;list;watch

// service account, role, rolebinding
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=roles,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=rolebindings,verbs=get;list;watch;create;update;patch
// service account permissions that are needed to grant permission to the above
// +kubebuilder:rbac:groups="security.openshift.io",resourceNames=anyuid,resources=securitycontextconstraints,verbs=use
// +kubebuilder:rbac:groups="",resources=pods,verbs=create;delete;get;list;patch;update;watch
// +kubebuilder:rbac:groups=topology.openstack.org,resources=topologies,verbs=get;list;watch;update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NovaPlacementAPI object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *NovaPlacementAPIReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)

	// TODO(user): your logic here
	//Fetch the NovaPlacementAPI instance
	instance := &novav1.NovaPlacementAPI{}
	err := r.Client.Get(ctx, req.NamespacedName, instance)
        if err != nil {
                if k8s_errors.IsNotFound(err) {
                        // Request object not found, could have been deleted after reconcile request.
                        // Owned objects are automatically garbage collected.
                        // For additional cleanup logic use finalizers. Return and don't requeue.
                        Log.Info("Placement instance not found, probably deleted before reconciled. Nothing to do.")
                        return ctrl.Result{}, nil
                }
                // Error reading the object - requeue the request.
                Log.Error(err, "Failed to read the Placement instance.")
                return ctrl.Result{}, err
        }

	h, err := helper.NewHelper(
		instance,
		r.Client,
		r.Kclient,
		r.Scheme,
		Log
	)
	if err != nil {
		Log.Error(err, "Failed to create lib-common Helper")
		return ctrl.Result{}, err
	}

	// Save a copy of the conditions so that we can restore the LastTransitionTime
	// when a condition's status doesn't change.
	savedConditions := instance.Status.Conditions.DeepCopy()
	//initialize status fields
	if err = r.initStatus(instance); err != nil {
		return ctrl.Result{}, err
	}
	instance.Status.ObservedGeneration = instance.Generation

	// Always patch the instance when exiting this function so we can persist any changes.
	defer func() {
                // Don't update the status, if reconciler Panics
                if r := recover(); r != nil {
                        Log.Info(fmt.Sprintf("panic during reconcile %v\n", r))
                        panic(r)
                }
                // update the Ready condition based on the sub conditions
                if instance.Status.Conditions.AllSubConditionIsTrue() {
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

        // Handle service delete
        if !instance.DeletionTimestamp.IsZero() {
                return r.reconcileDelete(ctx, h, instance)
        }

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NovaPlacementAPIReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&novav1beta1.NovaPlacementAPI{}).
		Complete(r)
}
