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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/go-logr/logr"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	helper "github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	util "github.com/openstack-k8s-operators/lib-common/modules/common/util"

	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"

	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
)

// NovaAPIReconciler reconciles a NovaAPI object
type NovaAPIReconciler struct {
	client.Client
	Kclient kubernetes.Interface
	Scheme  *runtime.Scheme
	Log     logr.Logger
}

//+kubebuilder:rbac:groups=nova.openstack.org,resources=novaapis,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=nova.openstack.org,resources=novaapis/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=nova.openstack.org,resources=novaapis/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NovaAPI object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.2/pkg/reconcile
func (r *NovaAPIReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)
	l.Info("Reconciling ", "request", req)

	// Fetch the NovaAPI instance that needs to be reconciled
	instance := &novav1.NovaAPI{}
	err := r.Client.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected.
			// For additional cleanup logic use finalizers. Return and don't requeue.
			l.Info("NovaAPI instance not found, probably deleted before reconciled. Nothing to do.", "request", req)
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		l.Error(err, "Failed to read the NovaAPI instance. Requeuing", "request", req)
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
		l.Error(err, "Failed to create lib-common Helper", "request", req)
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
		err := r.Status().Update(ctx, instance)
		if err != nil && !k8s_errors.IsNotFound(err) {
			util.LogErrorForObject(
				h, err, "Failed to update status at the end of reconciliation", instance)
		}
		util.LogForObject(
			h, "Updated status at the end of reconciliation", instance)
	}()

	return r.reconcileNormal(ctx, h, instance)
}

func (r *NovaAPIReconciler) initStatus(
	ctx context.Context, h *helper.Helper, instance *novav1.NovaAPI,
) error {
	if err := r.initConditions(ctx, h, instance); err != nil {
		return err
	}

	// NOTE(gibi): initialize the rest of the status fields here
	// so that the reconcile loop later can assume they are not nil.
	if instance.Status.Hash == nil {
		instance.Status.Hash = map[string]string{}
	}
	if instance.Status.APIEndpoints == nil {
		instance.Status.APIEndpoints = map[string]string{}
	}

	return nil
}

func (r *NovaAPIReconciler) initConditions(
	ctx context.Context, h *helper.Helper, instance *novav1.NovaAPI,
) error {
	if instance.Status.Conditions == nil {
		instance.Status.Conditions = condition.Conditions{}
		// initialize all conditions to Unknown
		cl := condition.CreateList(
			// TODO(gibi): Initilaize each condition the controller reports
			// here to Unknown. By default only the top level Ready condition is
			// created by Conditions.Init()
			condition.UnknownCondition(
				condition.InputReadyCondition,
				condition.InitReason,
				condition.InputReadyInitMessage,
			),
		)

		instance.Status.Conditions.Init(&cl)

		// Register overall status immediately to have an early feedback e.g.
		// in the cli
		if err := r.Status().Update(ctx, instance); err != nil {
			util.LogErrorForObject(
				h, err, "Failed to initialize Conditions", instance)
			return err
		}

	}
	return nil
}

type conditionsGetter interface {
	GetConditions() condition.Conditions
}

func allSubConditionIsTrue(conditionsGetter conditionsGetter) bool {
	// It assumes that all of our conditions report success via the True status
	for _, c := range conditionsGetter.GetConditions() {
		if c.Type == condition.ReadyCondition {
			continue
		}
		if c.Status != corev1.ConditionTrue {
			return false
		}
	}
	return true
}

func (r *NovaAPIReconciler) reconcileNormal(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.NovaAPI,
) (ctrl.Result, error) {
	secretHash, result, err := ensureSecret(
		ctx,
		types.NamespacedName{Namespace: instance.Namespace, Name: instance.Spec.Secret},
		// TODO(gibi): add keystoneAuthURL here is that is also passed via
		// the Secret. Also add DB and MQ user name here too if those are
		// passed via the Secret
		[]string{
			instance.Spec.PasswordSelectors.APIDatabase,
			instance.Spec.PasswordSelectors.APIMessageBus,
			instance.Spec.PasswordSelectors.Service,
		},
		h.GetClient(),
		&instance.Status.Conditions,
	)
	if err != nil {
		return result, err
	}

	// all our input checks out so report InputReady
	instance.Status.Conditions.MarkTrue(condition.InputReadyCondition, condition.InputReadyMessage)

	// TODO(gibi): Can we use a simple map[string][string] for hashes?
	// Collect hashes of all the input we depend on so that we can easily
	// detect if something is changed.
	hashes := make(map[string]env.Setter)

	hashes[instance.Spec.Secret] = env.SetValue(secretHash)

	return ctrl.Result{}, nil
}

type conditionUpdater interface {
	Set(c *condition.Condition)
	MarkTrue(t condition.Type, messageFormat string, messageArgs ...interface{})
}

// ensureSecret - ensures that the Secret object exists and the expected fields
// are in the Secret. It returns a hash of the values of the expected fields.
func ensureSecret(
	ctx context.Context,
	secretName types.NamespacedName,
	expectedFields []string,
	reader client.Reader,
	conditionUpdater conditionUpdater,
) (string, ctrl.Result, error) {
	secret := &corev1.Secret{}
	err := reader.Get(ctx, secretName, secret)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// TODO(gibi): Change the message to state which input
			// (i.e. Secret with a given name) is missing
			conditionUpdater.Set(condition.FalseCondition(
				condition.InputReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				condition.InputReadyWaitingMessage))
			return "",
				ctrl.Result{RequeueAfter: time.Second * 10},
				fmt.Errorf("OpenStack secret %s not found", secretName)
		}
		conditionUpdater.Set(condition.FalseCondition(
			condition.InputReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.InputReadyErrorMessage,
			err.Error()))
		return "", ctrl.Result{}, err
	}

	// collect the secret values the caller expects to exist
	values := [][]byte{}
	for _, field := range expectedFields {
		if val, ok := secret.Data[field]; !ok {
			err := fmt.Errorf("field %s not found in Secret %s", field, secretName)
			conditionUpdater.Set(condition.FalseCondition(
				condition.InputReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.InputReadyErrorMessage,
				err.Error()))
			return "", ctrl.Result{}, err
		} else {
			values = append(values, val)
		}
	}

	// TODO(gibi): Do we need to watch the Secret for changes?

	hash, err := util.ObjectHash(values)
	if err != nil {
		conditionUpdater.Set(condition.FalseCondition(
			condition.InputReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.InputReadyErrorMessage,
			err.Error()))
		return "", ctrl.Result{}, err
	}

	return hash, ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NovaAPIReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&novav1.NovaAPI{}).
		Complete(r)
}
