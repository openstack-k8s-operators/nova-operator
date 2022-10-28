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
	"strings"

	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	common "github.com/openstack-k8s-operators/lib-common/modules/common"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/endpoint"
	helper "github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	util "github.com/openstack-k8s-operators/lib-common/modules/common/util"
	database "github.com/openstack-k8s-operators/lib-common/modules/database"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/nova-operator/pkg/nova"

	mariadbv1 "github.com/openstack-k8s-operators/mariadb-operator/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// NovaReconciler reconciles a Nova object
type NovaReconciler struct {
	ReconcilerBase
}

//+kubebuilder:rbac:groups=nova.openstack.org,resources=nova,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=nova.openstack.org,resources=nova/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=nova.openstack.org,resources=nova/finalizers,verbs=update
//+kubebuilder:rbac:groups=mariadb.openstack.org,resources=mariadbdatabases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneapis,verbs=get;list;watch;
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneservices,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneendpoints,verbs=get;list;watch;create;update;patch;delete;

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Nova object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.2/pkg/reconcile
func (r *NovaReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, _err error) {
	l := log.FromContext(ctx)

	// Fetch the NovaAPI instance that needs to be reconciled
	instance := &novav1.Nova{}
	err := r.Client.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected.
			// For additional cleanup logic use finalizers. Return and don't requeue.
			l.Info("Nova instance not found, probably deleted before reconciled. Nothing to do.")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		l.Error(err, "Failed to read the Nova instance.")
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

	// TODO(gibi): This should be checked in a webhook and reject the CR
	// creation instead of setting its status.
	cell0Template, err := r.getCell0Template(instance)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.ensureKeystoneServiceUser(ctx, h, instance)
	if err != nil {
		return ctrl.Result{}, err
	}

	// We have to wait until our service is registered to keystone
	if !instance.Status.Conditions.IsTrue(condition.KeystoneServiceReadyCondition) {
		return ctrl.Result{}, nil
	}

	keystoneAuthURL, err := r.getKeystoneAuthURL(ctx, h, instance)
	if err != nil {
		return ctrl.Result{}, err
	}

	// We create the API DB separately from the Cell DBs as we want to report
	// its status separately and we need to pass the API DB around for Cells
	// having upcall support
	// NOTE(gibi): We don't return on error or if the DB is not ready yet. We
	// move forward and kick off the rest of the work we can do (e.g. creating
	// Cell DBs and Cells without upcall support). Eventually we rely on the
	// watch to get reconciled if the status of the API DB resource changes.
	apiDB, apiDBStatus, apiDBError := r.ensureAPIDB(ctx, h, instance)
	if apiDBStatus == nova.DBFailed {
		instance.Status.Conditions.Set(condition.FalseCondition(
			novav1.NovaAPIDBReadyCondition,
			condition.ErrorReason,
			condition.SeverityError,
			condition.DBReadyErrorMessage,
			apiDBError.Error(),
		))
	}
	if apiDBStatus == nova.DBCreating {
		instance.Status.Conditions.Set(condition.FalseCondition(
			novav1.NovaAPIDBReadyCondition,
			condition.ErrorReason,
			condition.SeverityError,
			condition.DBReadyRunningMessage,
		))
	}
	if apiDBStatus == nova.DBCompleted {
		instance.Status.Conditions.MarkTrue(novav1.NovaAPIDBReadyCondition, condition.DBReadyMessage)
	}

	// Create the Cell DBs. Note that we are not returning on error or if the
	// DB creation is still in progress. We move forward with whathever we can
	// and relay on the watch to get reconciled if some of the resources change
	// status
	cellDBs := map[string]*nova.Database{}
	var failedDBs []string
	var creatingDBs []string
	for cellName, cellTemplate := range instance.Spec.CellTemplates {
		cellDB, status, err := r.ensureCellDB(ctx, h, instance, cellName, cellTemplate)
		if err != nil {
			failedDBs = append(failedDBs, fmt.Sprintf("%s(%v)", cellName, err.Error()))

		}
		if status == nova.DBCreating {
			creatingDBs = append(creatingDBs, cellName)
		}
		cellDBs[cellName] = &nova.Database{Database: cellDB, Status: status}
	}
	if len(failedDBs) > 0 {
		instance.Status.Conditions.Set(condition.FalseCondition(
			novav1.NovaAllCellsDBReadyCondition,
			condition.ErrorReason,
			condition.SeverityError,
			novav1.NovaAllCellsDBReadyErrorMessage,
			strings.Join(failedDBs, ",")))
	} else if len(creatingDBs) > 0 {
		instance.Status.Conditions.Set(condition.FalseCondition(
			novav1.NovaAllCellsDBReadyCondition,
			condition.ErrorReason,
			condition.SeverityError,
			novav1.NovaAllCellsDBReadyCreatingMessage,
			strings.Join(creatingDBs, ",")))
	} else { // we have no DB in failed or creating status so all DB is ready
		instance.Status.Conditions.MarkTrue(novav1.NovaAllCellsDBReadyCondition, condition.DBReadyMessage)
	}

	// Kick of the creation of Cells. We skip over those cells where the cell
	// DB is not yet created and those which needs API DB access but cell0 is
	// not ready yet
	failedCells := []string{}
	creatingCells := []string{}
	skippedCells := []string{}
	readyCells := []string{}
	cells := map[string]*novav1.NovaCell{}
	allCellsReady := true
	for cellName, cellTemplate := range instance.Spec.CellTemplates {
		cellDB := cellDBs[cellName]
		if cellDB.Status != nova.DBCompleted {
			allCellsReady = false
			skippedCells = append(skippedCells, cellName)
			util.LogForObject(
				h, "Skipping NovaCell as waiting for the cell DB to be created",
				instance, "CellName", cellName)
			continue
		}
		cell0, ok := cells[Cell0Name]
		if cellName != Cell0Name && cellTemplate.HasAPIAccess && (!ok || !cell0.IsReady()) {
			allCellsReady = false
			skippedCells = append(skippedCells, cellName)
			util.LogForObject(
				h, "Skippig NovaCell as cell0 is not ready yet and this cell needs API DB access",
				instance, "CellName", cellName)
			continue
		}

		cell, _, err := r.ensureCell(ctx, h, instance, cellName, cellTemplate, cellDB.Database, apiDB, keystoneAuthURL)
		cells[cellName] = cell
		if err != nil {
			failedCells = append(failedCells, fmt.Sprintf("%s(%v)", cellName, err.Error()))
		} else if !cell.IsReady() {
			creatingCells = append(creatingCells, cellName)
		} else {
			readyCells = append(readyCells, cellName)
		}

		allCellsReady = allCellsReady && cell.IsReady()
	}
	util.LogForObject(
		h, "Cell statuses", instance, "failed", failedCells,
		"creating", creatingCells, "skipped", skippedCells,
		"ready", readyCells, "all cells ready", allCellsReady)
	if len(failedCells) > 0 {
		instance.Status.Conditions.Set(condition.FalseCondition(
			novav1.NovaAllCellsReadyCondition,
			condition.ErrorReason,
			condition.SeverityError,
			novav1.NovaAllCellsReadyErrorMessage,
			strings.Join(failedCells, ","),
		))
	} else if len(creatingCells) > 0 {
		instance.Status.Conditions.Set(condition.FalseCondition(
			novav1.NovaAllCellsReadyCondition,
			condition.ErrorReason,
			condition.SeverityError,
			novav1.NovaAllCellsReadyCreatingMessage,
			strings.Join(creatingCells, ","),
		))
	} else if len(skippedCells) > 0 {
		instance.Status.Conditions.Set(condition.FalseCondition(
			novav1.NovaAllCellsReadyCondition,
			condition.ErrorReason,
			condition.SeverityError,
			novav1.NovaAllCellsReadyWaitingMessage,
			strings.Join(skippedCells, ","),
		))
	}
	if allCellsReady {
		instance.Status.Conditions.MarkTrue(novav1.NovaAllCellsReadyCondition, novav1.NovaAllCellsReadyMessage)
	}

	// Don't move forward with the top level service creations like NovaAPI
	// until cell0 is ready as top level services need cell0 to register in
	if cell0, ok := cells[Cell0Name]; !ok || !cell0.IsReady() {
		// we need to stop here until cell0 is ready
		return ctrl.Result{}, nil
	}

	result, err = r.ensureAPI(ctx, h, instance, cell0Template, cellDBs[Cell0Name].Database, apiDB)
	if err != nil {
		return result, err
	}

	util.LogForObject(h, "Successfully reconciled", instance)
	return ctrl.Result{}, nil
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
				novav1.NovaAPIReadyCondition,
				condition.InitReason,
				novav1.NovaAPIReadyInitMessage,
			),
			condition.UnknownCondition(
				novav1.NovaAllCellsDBReadyCondition,
				condition.InitReason,
				condition.DBReadyInitMessage,
			),
			condition.UnknownCondition(
				novav1.NovaAllCellsReadyCondition,
				condition.InitReason,
				novav1.NovaAllCellsReadyInitMessage,
			),
			condition.UnknownCondition(
				condition.KeystoneServiceReadyCondition,
				condition.InitReason,
				"Service registration not started",
			),
		)
		instance.Status.Conditions.Init(&cl)
	}
	return nil
}

func (r *NovaReconciler) ensureDB(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.Nova,
	db *database.Database,
	databaseServiceName string,
	targetCondition condition.Type,
) (nova.DatabaseStatus, error) {

	ctrlResult, err := db.CreateOrPatchDBByName(
		ctx,
		h,
		databaseServiceName,
	)
	if err != nil {
		return nova.DBFailed, err
	}
	if (ctrlResult != ctrl.Result{}) {
		return nova.DBCreating, nil
	}
	// poll the status of the DB creation
	ctrlResult, err = db.WaitForDBCreatedWithTimeout(ctx, h, r.RequeueTimeout)
	if err != nil {
		return nova.DBFailed, err
	}
	if (ctrlResult != ctrl.Result{}) {
		return nova.DBCreating, nil
	}

	return nova.DBCompleted, nil
}

func (r *NovaReconciler) getCell0Template(instance *novav1.Nova) (novav1.NovaCellTemplate, error) {
	var cell0Template novav1.NovaCellTemplate
	var ok bool

	if cell0Template, ok = instance.Spec.CellTemplates[Cell0Name]; !ok {
		err := fmt.Errorf("missing cell0 specification from Spec.CellTemplates")
		instance.Status.Conditions.Set(condition.FalseCondition(
			novav1.NovaAllCellsReadyCondition,
			condition.ErrorReason,
			condition.SeverityError,
			novav1.NovaAllCellsReadyErrorMessage,
			fmt.Sprintf("%s(%v)", Cell0Name, err.Error()),
		))

		return cell0Template, err
	}

	return cell0Template, nil
}

func (r *NovaReconciler) ensureAPIDB(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.Nova,
) (*database.Database, nova.DatabaseStatus, error) {
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
	result, err := r.ensureDB(
		ctx,
		h,
		instance,
		apiDB,
		instance.Spec.APIDatabaseInstance,
		novav1.NovaAPIDBReadyCondition,
	)
	return apiDB, result, err
}

func (r *NovaReconciler) ensureCellDB(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.Nova,
	cellName string,
	cellTemplate novav1.NovaCellTemplate,
) (*database.Database, nova.DatabaseStatus, error) {
	cellDB := database.NewDatabaseWithNamespace(
		"nova_"+cellName,
		cellTemplate.CellDatabaseUser,
		instance.Spec.Secret,
		map[string]string{
			"dbName": cellTemplate.CellDatabaseInstance,
		},
		"nova-"+cellName,
		instance.Namespace,
	)
	result, err := r.ensureDB(
		ctx,
		h,
		instance,
		cellDB,
		cellTemplate.CellDatabaseInstance,
		novav1.NovaAllCellsDBReadyCondition,
	)
	return cellDB, result, err
}

func (r *NovaReconciler) ensureCell(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.Nova,
	cellName string,
	cellTemplate novav1.NovaCellTemplate,
	cellDB *database.Database,
	apiDB *database.Database,
	keystoneAuthURL string,
) (*novav1.NovaCell, ctrl.Result, error) {
	// TODO(gibi): Pass down a narrowed secret that only holds
	// specific information but also holds user names
	cellSpec := novav1.NovaCellSpec{
		CellName:                  cellName,
		Secret:                    instance.Spec.Secret,
		CellDatabaseHostname:      cellDB.GetDatabaseHostname(),
		CellDatabaseUser:          cellTemplate.CellDatabaseUser,
		ConductorServiceTemplate:  cellTemplate.ConductorServiceTemplate,
		MetadataServiceTemplate:   cellTemplate.MetadataServiceTemplate,
		NoVNCProxyServiceTemplate: cellTemplate.NoVNCProxyServiceTemplate,
		Debug:                     instance.Spec.Debug,
		ServiceUser:               instance.Spec.ServiceUser,
		KeystoneAuthURL:           keystoneAuthURL,
	}
	if cellTemplate.HasAPIAccess {
		cellSpec.APIDatabaseHostname = apiDB.GetDatabaseHostname()
		cellSpec.APIDatabaseUser = instance.Spec.APIDatabaseUser
	}

	cell := &novav1.NovaCell{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + "-" + cellSpec.CellName,
			Namespace: instance.Namespace,
		},
	}

	op, err := controllerutil.CreateOrPatch(ctx, r.Client, cell, func() error {
		// TODO(gibi): Pass down a narroved secret that only hold
		// specific information but also holds user names
		cell.Spec = cellSpec

		err := controllerutil.SetControllerReference(instance, cell, r.Scheme)
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return cell, ctrl.Result{}, err
	}

	if op != controllerutil.OperationResultNone {
		util.LogForObject(h, fmt.Sprintf("NovaCell %s.", string(op)), instance, "NovaCell.Name", cell.Name)
	}

	return cell, ctrl.Result{}, nil
}

func (r *NovaReconciler) ensureAPI(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.Nova,
	cell0Template novav1.NovaCellTemplate,
	cell0DB *database.Database,
	apiDB *database.Database,
) (ctrl.Result, error) {
	// TODO(gibi): Pass down a narroved secret that only hold
	// specific information but also holds user names
	apiSpec := novav1.NovaAPISpec{
		Secret:                instance.Spec.Secret,
		APIDatabaseHostname:   apiDB.GetDatabaseHostname(),
		APIDatabaseUser:       instance.Spec.APIDatabaseUser,
		Cell0DatabaseHostname: cell0DB.GetDatabaseHostname(),
		Cell0DatabaseUser:     cell0Template.CellDatabaseUser,
		Debug:                 instance.Spec.Debug,
		// NOTE(gibi): this is a coincidence that the NovaServiceBase
		// has exactly the same fields as the NovaAPITemplate so we can convert
		// between them directly. As soon as these two structs start to diverge
		// we need to copy fields one by one here.
		NovaServiceBase: novav1.NovaServiceBase(instance.Spec.APIServiceTemplate),
	}
	api := &novav1.NovaAPI{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + "-api",
			Namespace: instance.Namespace,
		},
	}

	op, err := controllerutil.CreateOrPatch(ctx, r.Client, api, func() error {
		api.Spec = apiSpec

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
		util.LogForObject(h, fmt.Sprintf("NovaAPI %s.", string(op)), instance, "NovaAPI.Name", api.Name)
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

func (r *NovaReconciler) ensureKeystoneServiceUser(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.Nova,
) error {
	serviceSpec := keystonev1.KeystoneServiceSpec{
		ServiceType:        "compute",
		ServiceName:        "nova",
		ServiceDescription: "Nova Compute Service",
		Enabled:            true,
		ServiceUser:        instance.Spec.ServiceUser,
		Secret:             instance.Spec.Secret,
		PasswordSelector:   instance.Spec.PasswordSelectors.Service,
	}
	serviceLabels := map[string]string{
		common.AppSelector: "nova",
	}

	service := keystonev1.NewKeystoneService(serviceSpec, instance.Namespace, serviceLabels, 10)
	result, err := service.CreateOrPatch(ctx, h)
	if k8s_errors.IsNotFound(err) {
		return nil
	}
	if (err != nil || result != ctrl.Result{}) {
		return err
	}

	c := service.GetConditions().Mirror(condition.KeystoneServiceReadyCondition)
	if c != nil {
		instance.Status.Conditions.Set(c)
	}

	return nil
}

func (r *NovaReconciler) getKeystoneAuthURL(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.Nova,
) (string, error) {
	// TODO(gibi): change lib-common to take the name of the KeystoneAPI as
	// parameter instead of labels. Then use instance.Spec.KeystoneInstance as
	// the name.
	keystoneAPI, err := keystonev1.GetKeystoneAPI(ctx, h, instance.Namespace, map[string]string{})
	if err != nil {
		return "", err
	}
	authURL, err := keystoneAPI.GetEndpoint(endpoint.EndpointPublic)
	if err != nil {
		return "", err
	}
	return authURL, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NovaReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&novav1.Nova{}).
		Owns(&mariadbv1.MariaDBDatabase{}).
		Owns(&keystonev1.KeystoneService{}).
		Owns(&novav1.NovaAPI{}).
		Owns(&novav1.NovaCell{}).
		Complete(r)
}
