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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/go-logr/logr"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	helper "github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	util "github.com/openstack-k8s-operators/lib-common/modules/common/util"
	database "github.com/openstack-k8s-operators/lib-common/modules/database"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/nova-operator/pkg/nova"

	mariadbv1 "github.com/openstack-k8s-operators/mariadb-operator/api/v1beta1"
)

// NovaReconciler reconciles a Nova object
type NovaReconciler struct {
	client.Client
	Kclient        kubernetes.Interface
	Scheme         *runtime.Scheme
	Log            logr.Logger
	RequeueTimeout time.Duration
}

//+kubebuilder:rbac:groups=nova.openstack.org,resources=nova,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=nova.openstack.org,resources=nova/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=nova.openstack.org,resources=nova/finalizers,verbs=update
//+kubebuilder:rbac:groups=mariadb.openstack.org,resources=mariadbdatabases,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Nova object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.2/pkg/reconcile
func (r *NovaReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)
	l.Info("Reconciling ", "request", req)

	// Fetch the NovaAPI instance that needs to be reconciled
	instance := &novav1.Nova{}
	err := r.Client.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected.
			// For additional cleanup logic use finalizers. Return and don't requeue.
			l.Info("Nova instance not found, probably deleted before reconciled. Nothing to do.", "request", req)
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		l.Error(err, "Failed to read the Nova instance. Requeuing", "request", req)
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

func (r *NovaReconciler) initStatus(
	ctx context.Context, h *helper.Helper, instance *novav1.Nova,
) error {
	if err := r.initConditions(ctx, h, instance); err != nil {
		return err
	}

	return nil
}

func (r *NovaReconciler) initConditions(
	ctx context.Context, h *helper.Helper, instance *novav1.Nova,
) error {
	if instance.Status.Conditions == nil {
		instance.Status.Conditions = condition.Conditions{}
		// initialize all conditions to Unknown
		cl := condition.CreateList(
			// TODO(gibi): Initialize each condition the controller reports
			// here to Unknown. By default only the top level Ready condition is
			// created by Conditions.Init()
			condition.UnknownCondition(
				novav1.NovaAPIDBReadyCondition,
				condition.InitReason,
				condition.DBReadyInitMessage,
			),
			condition.UnknownCondition(
				novav1.NovaCell0DBReadyCondition,
				condition.InitReason,
				condition.DBReadyInitMessage,
			),
			condition.UnknownCondition(
				novav1.NovaAPIReadyCondition,
				condition.InitReason,
				novav1.NovaAPIReadyInitMessage,
			),
			condition.UnknownCondition(
				novav1.NovaCell0ReadyCondition,
				condition.InitReason,
				novav1.NovaCell0ReadyInitMessage,
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

func (r *NovaReconciler) reconcileNormal(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.Nova,
) (ctrl.Result, error) {
	// TODO(gibi): This should be checked in a webhook and reject the CR
	// creation instead of setting its status.
	var cell0Template novav1.NovaCellTemplate
	var ok bool

	if cell0Template, ok = instance.Spec.CellTemplates["cell0"]; !ok {
		err := fmt.Errorf("missing cell0 specification from Spec.CellTemplates")
		instance.Status.Conditions.Set(condition.FalseCondition(
			novav1.NovaCell0ReadyCondition,
			condition.ErrorReason,
			condition.SeverityError,
			novav1.NovaCell0ReadyErrorMessage,
			err.Error()))

		return ctrl.Result{}, err
	}

	apiDB := database.NewDatabaseWithNamespace(
		nova.NovaAPIDatabaseName,
		instance.Spec.APIDatabaseUser,
		instance.Spec.Secret,
		map[string]string{
			"dbName": instance.Spec.APIDatabaseInstance,
		},
		"nova-api",
		instance.Namespace,
	)
	result, err := r.reconcileDB(ctx, h, instance, apiDB, novav1.NovaAPIDBReadyCondition)
	if (err != nil || result != ctrl.Result{}) {
		return result, err
	}

	cell0DB := database.NewDatabaseWithNamespace(
		nova.NovaCell0DatabaseName,
		// TODO(gibi): This should be cell0.CellDatabaseUser or should be
		// embedded into the Secret we passing down to the cell
		instance.Spec.APIDatabaseUser,
		instance.Spec.Secret,
		map[string]string{
			"dbName": cell0Template.CellDatabaseInstance,
		},
		"nova-cell0",
		instance.Namespace,
	)
	result, err = r.reconcileDB(ctx, h, instance, cell0DB, novav1.NovaCell0DBReadyCondition)
	if (err != nil || result != ctrl.Result{}) {
		return result, err
	}

	cell0, result, err := r.reconcileNovaCell0(
		ctx,
		h,
		instance,
		"cell0",
		cell0Template,
		cell0DB.GetDatabaseHostname(),
		// TODO(gibi): this is a limitation of the current MariaDBDatabase
		// implementation, it always assumes that the
		// DatabaseUser == DatabaseName
		nova.NovaCell0DatabaseName,
		apiDB.GetDatabaseHostname(),
		// ditto
		nova.NovaAPIDatabaseName,
	)
	if err != nil {
		return result, err
	}

	// Don't move forward with the other service creations like NovaAPI until
	// cell0 is ready as top level services needs cell0 to register in
	cell0ReadyCond := cell0.Status.Conditions.Get(condition.ReadyCondition)
	if cell0ReadyCond == nil || cell0ReadyCond.Status != corev1.ConditionTrue {
		return ctrl.Result{RequeueAfter: r.RequeueTimeout}, nil
	}

	result, err = r.reconcileNovaAPI(ctx, h, instance, apiDB.GetDatabaseHostname())
	if err != nil {
		return result, err
	}

	return ctrl.Result{}, nil
}

func (r *NovaReconciler) reconcileDB(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.Nova,
	db *database.Database,
	targetCondition condition.Type,
) (ctrl.Result, error) {

	// create or patch the DB
	// TODO(gibi): use db.CreateOrPatchDBByName and passing in
	// instance.Spec.APIDatabaseInstance after
	// https://github.com/openstack-k8s-operators/lib-common/pull/63
	// is merged
	ctrlResult, err := db.CreateOrPatchDB(
		ctx,
		h,
	)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			targetCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.DBReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}
	if (ctrlResult != ctrl.Result{}) {
		instance.Status.Conditions.Set(condition.FalseCondition(
			targetCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DBReadyRunningMessage))
		return ctrlResult, nil
	}
	// wait for the DB to be setup
	ctrlResult, err = db.WaitForDBCreated(ctx, h)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			targetCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.DBReadyErrorMessage,
			err.Error()))
		return ctrlResult, err
	}
	if (ctrlResult != ctrl.Result{}) {
		instance.Status.Conditions.Set(condition.FalseCondition(
			targetCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DBReadyRunningMessage))
		return ctrlResult, nil
	}

	instance.Status.Conditions.MarkTrue(targetCondition, condition.DBReadyMessage)

	return ctrl.Result{}, nil
}

func newNovaAPISpec(
	novaAPITemplate novav1.NovaAPITemplate,
	secretName string,
	apiDatabaseHostname string,
	debug novav1.Debug,
	apiDatabaseUser string,
) novav1.NovaAPISpec {
	apiSpec := novav1.NovaAPISpec{
		// TODO(gibi): Pass down a narroved secret that only hold NovaAPI
		// specific information but also holds user names
		Secret: secretName,
		// TODO(gibi): register service user in Keystone and get auth URL
		// then we can pass those to NovaAPI here
		// KeystoneAuthURL:
		APIDatabaseHostname: apiDatabaseHostname,
		APIDatabaseUser:     apiDatabaseUser,
		// TODO(gibi): initialize API messag bus and pass it forward
		// APIMessageBusHostname:
		Debug: debug,
		// NOTE(gibi): this is a coincidence that the NovaServiceBase
		// has exactly the same fields as the NovaAPITemplate so we can convert
		// between them directly. As soon as these two structs start to diverge
		// we need to copy fields one by one here.
		NovaServiceBase: novav1.NovaServiceBase(novaAPITemplate),
	}
	return apiSpec
}

func (r *NovaReconciler) reconcileNovaAPI(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.Nova,
	apiDatabaseHostname string,
) (ctrl.Result, error) {
	api := &novav1.NovaAPI{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, api, func() error {
		api.Spec = newNovaAPISpec(
			instance.Spec.APIServiceTemplate,
			instance.Spec.Secret,
			apiDatabaseHostname,
			instance.Spec.Debug,
			// TODO(gibi): this is a limitation of the current MariaDBDatabase
			// implementation, it always assumes that the
			// DatabaseUser == DatabaseName
			nova.NovaAPIDatabaseName,
		)

		err := controllerutil.SetControllerReference(instance, api, r.Scheme)
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		condition.FalseCondition(
			novav1.NovaAPIReadyCondition,
			condition.ErrorReason,
			condition.SeverityError,
			novav1.NovaAPIReadyErrorMessage,
			err.Error(),
		)
		return ctrl.Result{}, err
	}

	if op != controllerutil.OperationResultNone {
		util.LogForObject(h, fmt.Sprintf("NovaAPI %s %s.", api.ObjectMeta.Name, string(op)), instance)
	}

	c := api.Status.Conditions.Mirror(novav1.NovaAPIReadyCondition)
	// NOTE(gibi): it can be nil if the NovaAPI CR is created but no
	// reconciliation is run on it to initialize the ReadyCondition yet.
	if c != nil {
		instance.Status.Conditions.Set(c)
	}
	instance.Status.APIServiceReadyCount = api.Status.ReadyCount

	return ctrl.Result{}, nil
}

func newNovaCellSpec(
	cellName string,
	cellTemplate novav1.NovaCellTemplate,
	secretName string,
	cellDatabaseHostname string,
	cellDatabaseUser string,
	apiDatabaseHostname string,
	apiDatabaseUser string,
	debug novav1.Debug,
) novav1.NovaCellSpec {
	cellSpec := novav1.NovaCellSpec{
		CellName: cellName,
		// TODO(gibi): Pass down a narroved secret that only hold
		// specific information but also holds user names
		Secret:                    secretName,
		CellDatabaseHostname:      cellDatabaseHostname,
		CellDatabaseUser:          cellDatabaseUser,
		APIDatabaseHostname:       apiDatabaseHostname,
		APIDatabaseUser:           apiDatabaseUser,
		ConductorServiceTemplate:  cellTemplate.ConductorServiceTemplate,
		MetadataServiceTemplate:   cellTemplate.MetadataServiceTemplate,
		NoVNCProxyServiceTemplate: cellTemplate.NoVNCProxyServiceTemplate,
		Debug:                     debug,
	}
	return cellSpec
}

func (r *NovaReconciler) reconcileNovaCell0(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.Nova,
	cellName string,
	cellTemplate novav1.NovaCellTemplate,
	cellDatabaseHostname string,
	cellDatabaseUser string,
	apiDatabaseHostname string,
	apiDatabaseUser string,
) (*novav1.NovaCell, ctrl.Result, error) {
	cell := &novav1.NovaCell{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + "-" + cellName,
			Namespace: instance.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, cell, func() error {
		cell.Spec = newNovaCellSpec(
			cellName,
			cellTemplate,
			instance.Spec.Secret,
			cellDatabaseHostname,
			cellDatabaseUser,
			apiDatabaseHostname,
			apiDatabaseUser,
			instance.Spec.Debug,
		)

		err := controllerutil.SetControllerReference(instance, cell, r.Scheme)
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		condition.FalseCondition(
			novav1.NovaCell0ReadyCondition,
			condition.ErrorReason,
			condition.SeverityError,
			novav1.NovaCell0ReadyErrorMessage,
			err.Error(),
		)
		return cell, ctrl.Result{}, err
	}

	if op != controllerutil.OperationResultNone {
		util.LogForObject(h, fmt.Sprintf("NovaCell0 %s %s.", cell.ObjectMeta.Name, string(op)), instance)
	}

	c := cell.Status.Conditions.Mirror(novav1.NovaCell0ReadyCondition)
	// NOTE(gibi): it can be nil if the NovaCell CR is created but no
	// reconciliation is run on it to initialize the ReadyCondition yet.
	if c != nil {
		instance.Status.Conditions.Set(c)
	}

	return cell, ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NovaReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&novav1.Nova{}).
		Owns(&mariadbv1.MariaDBDatabase{}).
		Owns(&novav1.NovaAPI{}).
		Complete(r)
}
