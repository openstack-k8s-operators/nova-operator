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
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	common "github.com/openstack-k8s-operators/lib-common/modules/common"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/endpoint"
	helper "github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	util "github.com/openstack-k8s-operators/lib-common/modules/common/util"
	database "github.com/openstack-k8s-operators/lib-common/modules/database"

	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/nova-operator/pkg/nova"
	"github.com/openstack-k8s-operators/nova-operator/pkg/novaapi"

	rabbitmqv1 "github.com/openstack-k8s-operators/infra-operator/apis/rabbitmq/v1beta1"
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	mariadbv1 "github.com/openstack-k8s-operators/mariadb-operator/api/v1beta1"
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
// +kubebuilder:rbac:groups=rabbitmq.openstack.org,resources=transporturls,verbs=get;list;watch;create;update;patch;delete

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
		err := h.PatchInstance(ctx, instance)
		if err != nil {
			_err = err
			return
		}
	}()

	if !instance.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, r.reconcileDelete(ctx, h, instance)
	}

	// We create a KeystoneService CR later and that will automatically get the
	// Nova finalizer. So we need a finalizer on the ourselves too so that
	// during Nova CR delete we can have a chance to remove the finalizer from
	// the our KeystoneService so that is also deleted.
	updated := controllerutil.AddFinalizer(instance, h.GetFinalizer())
	if updated {
		util.LogForObject(h, "Added finalizer to ourselves", instance)
		// we intentionally return imediately to force the deferred function
		// to persist the Instance with the finalizer. We need to have our own
		// finalizer persisted before we try to create the KeystoneService with
		// our finalizer to avoid orphaning the KeystoneService.
		return ctrl.Result{}, nil
	}

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
		util.LogForObject(h, "Waiting for the KeystoneService to become Ready", instance)
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
	switch apiDBStatus {
	case nova.DBFailed:
		instance.Status.Conditions.Set(condition.FalseCondition(
			novav1.NovaAPIDBReadyCondition,
			condition.ErrorReason,
			condition.SeverityError,
			condition.DBReadyErrorMessage,
			apiDBError.Error(),
		))
	case nova.DBCreating:
		instance.Status.Conditions.Set(condition.FalseCondition(
			novav1.NovaAPIDBReadyCondition,
			condition.ErrorReason,
			condition.SeverityError,
			condition.DBReadyRunningMessage,
		))
	case nova.DBCompleted:
		instance.Status.Conditions.MarkTrue(novav1.NovaAPIDBReadyCondition, condition.DBReadyMessage)
	default:
		return ctrl.Result{}, fmt.Errorf("Invalid DatabaseStatus from ensureAPIDB: %d", apiDBStatus)
	}

	// We need to create a list of cellNames to iterate on and as the map
	// iteration order is undefined we need to make sure that cell0 is the
	// first to allow dependency handling during ensureCell calls.
	orderedCellNames := []string{Cell0Name}
	for cellName := range instance.Spec.CellTemplates {
		if cellName != Cell0Name {
			orderedCellNames = append(orderedCellNames, cellName)
		}
	}

	// Create the Cell DBs. Note that we are not returning on error or if the
	// DB creation is still in progress. We move forward with whathever we can
	// and relay on the watch to get reconciled if some of the resources change
	// status
	cellDBs := map[string]*nova.Database{}
	var failedDBs []string
	var creatingDBs []string
	for _, cellName := range orderedCellNames {
		cellTemplate := instance.Spec.CellTemplates[cellName]
		cellDB, status, err := r.ensureCellDB(ctx, h, instance, cellName, cellTemplate)
		switch status {
		case nova.DBFailed:
			failedDBs = append(failedDBs, fmt.Sprintf("%s(%v)", cellName, err.Error()))
		case nova.DBCreating:
			creatingDBs = append(creatingDBs, cellName)
		case nova.DBCompleted:
		default:
			return ctrl.Result{}, fmt.Errorf("Invalid DatabaseStatus from ensureCellDB: %d for cell %s", status, cellName)
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
		instance.Status.Conditions.MarkTrue(
			novav1.NovaAllCellsDBReadyCondition, novav1.NovaAllCellsDBReadyMessage)
	}

	// Create TransportURLs to access the message buses of each cell. Cell0
	// message bus is always the same as the top level API message bus so
	// we create API MQ separately first
	apiMQSecretName, apiMQStatus, apiMQError := r.ensureMQ(
		ctx, h, instance, "nova-api-transport", instance.Spec.APIMessageBusInstance)
	switch apiMQStatus {
	case nova.MQFailed:
		instance.Status.Conditions.Set(condition.FalseCondition(
			novav1.NovaAPIMQReadyCondition,
			condition.ErrorReason,
			condition.SeverityError,
			novav1.NovaAPIMQReadyErrorMessage,
			apiDBError.Error(),
		))
	case nova.MQCreating:
		instance.Status.Conditions.Set(condition.FalseCondition(
			novav1.NovaAPIMQReadyCondition,
			condition.ErrorReason,
			condition.SeverityError,
			novav1.NovaAPIMQReadyCreatingMessage,
		))
	case nova.MQCompleted:
		instance.Status.Conditions.MarkTrue(
			novav1.NovaAPIMQReadyCondition, novav1.NovaAPIMQReadyMessage)
	default:
		return ctrl.Result{}, fmt.Errorf("Invalid MessageBusStatus from ensureMQ for the API MQ: %d", apiMQStatus)
	}

	cellMQs := map[string]*nova.MessageBus{}
	var failedMQs []string
	var creatingMQs []string
	for _, cellName := range orderedCellNames {
		var cellMQ string
		var status nova.MessageBusStatus
		var err error
		cellTemplate := instance.Spec.CellTemplates[cellName]
		// cell0 does not need its own cell message bus it uses the
		// API message bus instead
		if cellName == Cell0Name {
			cellMQ = apiMQSecretName
			status = apiMQStatus
			err = apiMQError
		} else {
			cellMQ, status, err = r.ensureMQ(
				ctx, h, instance, cellName+"-transport", cellTemplate.CellMessageBusInstance)
		}
		switch status {
		case nova.MQFailed:
			failedMQs = append(failedMQs, fmt.Sprintf("%s(%v)", cellName, err.Error()))
		case nova.MQCreating:
			creatingMQs = append(creatingMQs, cellName)
		case nova.MQCompleted:
		default:
			return ctrl.Result{}, fmt.Errorf("Invalid MessageBusStatus from ensureMQ: %d for cell %s", status, cellName)
		}
		cellMQs[cellName] = &nova.MessageBus{SecretName: cellMQ, Status: status}
	}
	if len(failedMQs) > 0 {
		instance.Status.Conditions.Set(condition.FalseCondition(
			novav1.NovaAllCellsMQReadyCondition,
			condition.ErrorReason,
			condition.SeverityError,
			novav1.NovaAllCellsMQReadyErrorMessage,
			strings.Join(failedMQs, ",")))
	} else if len(creatingMQs) > 0 {
		instance.Status.Conditions.Set(condition.FalseCondition(
			novav1.NovaAllCellsMQReadyCondition,
			condition.ErrorReason,
			condition.SeverityError,
			novav1.NovaAllCellsMQReadyCreatingMessage,
			strings.Join(creatingMQs, ",")))
	} else { // we have no MQ in failed or creating status so all MQ is ready
		instance.Status.Conditions.MarkTrue(
			novav1.NovaAllCellsMQReadyCondition, novav1.NovaAllCellsMQReadyMessage)
	}

	// Kick of the creation of Cells. We skip over those cells where the cell
	// DB or MQ is not yet created and those which needs API DB access but
	// cell0 is not ready yet
	failedCells := []string{}
	creatingCells := []string{}
	skippedCells := []string{}
	readyCells := []string{}
	cells := map[string]*novav1.NovaCell{}
	allCellsReady := true
	for _, cellName := range orderedCellNames {
		cellTemplate := instance.Spec.CellTemplates[cellName]
		cellDB := cellDBs[cellName]
		cellMQ := cellMQs[cellName]
		if cellDB.Status != nova.DBCompleted {
			allCellsReady = false
			skippedCells = append(skippedCells, cellName)
			util.LogForObject(
				h, "Skipping NovaCell as waiting for the cell DB to be created",
				instance, "CellName", cellName)
			continue
		}
		if cellMQ.Status != nova.MQCompleted {
			allCellsReady = false
			skippedCells = append(skippedCells, cellName)
			util.LogForObject(
				h, "Skipping NovaCell as waiting for the cell MQ to be created",
				instance, "CellName", cellName)
			continue
		}

		// The cell0 is always handled first in the loop as we iterate on
		// orderdCellNames. So for any other cells we can get cell0 from cells
		// as it is already there.
		if cellName != Cell0Name && cellTemplate.HasAPIAccess && !cells[Cell0Name].IsReady() {
			allCellsReady = false
			skippedCells = append(skippedCells, cellName)
			util.LogForObject(
				h, "Skippig NovaCell as cell0 is not ready yet and this cell needs API DB access",
				instance, "CellName", cellName)
			continue
		}

		cell, _, err := r.ensureCell(
			ctx, h, instance, cellName, cellTemplate,
			cellDB.Database, apiDB, cellMQ.SecretName, keystoneAuthURL,
		)
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
		"creating", creatingCells, "waiting", skippedCells,
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
		util.LogForObject(h, "Waiting for cell0 to become Ready before creating the top level services", instance)
		return ctrl.Result{}, nil
	}

	result, err = r.ensureAPI(
		ctx, h, instance, cell0Template,
		cellDBs[Cell0Name].Database, apiDB, apiMQSecretName, keystoneAuthURL,
	)
	if err != nil {
		return result, err
	}

	result, err = r.ensureScheduler(
		ctx, h, instance, cell0Template,
		cellDBs[Cell0Name].Database, apiDB, apiMQSecretName, keystoneAuthURL,
	)
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
			condition.UnknownCondition(
				novav1.NovaAPIMQReadyCondition,
				condition.InitReason,
				novav1.NovaAPIMQReadyInitMessage,
			),
			condition.UnknownCondition(
				novav1.NovaAllCellsMQReadyCondition,
				condition.InitReason,
				novav1.NovaAllCellsMQReadyInitMessage,
			),
			condition.UnknownCondition(
				novav1.NovaSchedulerReadyCondition,
				condition.InitReason,
				novav1.NovaSchedulerReadyInitMessage,
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
	cellMQSecretName string,
	keystoneAuthURL string,
) (*novav1.NovaCell, ctrl.Result, error) {
	// TODO(gibi): Pass down a narrowed secret that only holds
	// specific information but also holds user names
	cellSpec := novav1.NovaCellSpec{
		CellName:                  cellName,
		Secret:                    instance.Spec.Secret,
		CellDatabaseHostname:      cellDB.GetDatabaseHostname(),
		CellDatabaseUser:          cellTemplate.CellDatabaseUser,
		CellMessageBusSecretName:  cellMQSecretName,
		ConductorServiceTemplate:  cellTemplate.ConductorServiceTemplate,
		MetadataServiceTemplate:   cellTemplate.MetadataServiceTemplate,
		NoVNCProxyServiceTemplate: cellTemplate.NoVNCProxyServiceTemplate,
		Debug:                     instance.Spec.Debug,
		// TODO(gibi): this should be part of the secret
		ServiceUser:       instance.Spec.ServiceUser,
		KeystoneAuthURL:   keystoneAuthURL,
		PasswordSelectors: instance.Spec.PasswordSelectors,
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
	apiMQSecretName string,
	keystoneAuthURL string,
) (ctrl.Result, error) {
	// TODO(gibi): Pass down a narroved secret that only hold
	// specific information but also holds user names
	apiSpec := novav1.NovaAPISpec{
		Secret:                  instance.Spec.Secret,
		APIDatabaseHostname:     apiDB.GetDatabaseHostname(),
		APIDatabaseUser:         instance.Spec.APIDatabaseUser,
		Cell0DatabaseHostname:   cell0DB.GetDatabaseHostname(),
		Cell0DatabaseUser:       cell0Template.CellDatabaseUser,
		APIMessageBusSecretName: apiMQSecretName,
		Debug:                   instance.Spec.Debug,
		NovaServiceBase: novav1.NovaServiceBase{
			ContainerImage:         instance.Spec.APIServiceTemplate.ContainerImage,
			Replicas:               instance.Spec.APIServiceTemplate.Replicas,
			NodeSelector:           instance.Spec.APIServiceTemplate.NodeSelector,
			CustomServiceConfig:    instance.Spec.APIServiceTemplate.CustomServiceConfig,
			DefaultConfigOverwrite: instance.Spec.APIServiceTemplate.DefaultConfigOverwrite,
			Resources:              instance.Spec.APIServiceTemplate.Resources,
			NetworkAttachments:     instance.Spec.APIServiceTemplate.NetworkAttachments,
		},
		ExternalEndpoints: instance.Spec.APIServiceTemplate.ExternalEndpoints,
		KeystoneAuthURL:   keystoneAuthURL,
		ServiceUser:       instance.Spec.ServiceUser,
		PasswordSelectors: instance.Spec.PasswordSelectors,
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

func (r *NovaReconciler) ensureScheduler(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.Nova,
	cell0Template novav1.NovaCellTemplate,
	cell0DB *database.Database,
	apiDB *database.Database,
	apiMQSecretName string,
	keystoneAuthURL string,
) (ctrl.Result, error) {
	// TODO(gibi): Pass down a narroved secret that only hold
	// specific information but also holds user names
	spec := novav1.NovaSchedulerSpec{
		Secret:                  instance.Spec.Secret,
		APIDatabaseHostname:     apiDB.GetDatabaseHostname(),
		APIDatabaseUser:         instance.Spec.APIDatabaseUser,
		APIMessageBusSecretName: apiMQSecretName,
		Cell0DatabaseHostname:   cell0DB.GetDatabaseHostname(),
		Cell0DatabaseUser:       cell0Template.CellDatabaseUser,
		Debug:                   instance.Spec.Debug,
		// This is a coincidence that the NovaServiceBase
		// has exactly the same fields as the SchedulerServiceTemplate so we
		// can convert between them directly. As soon as these two structs
		// start to diverge we need to copy fields one by one here.
		NovaServiceBase:   novav1.NovaServiceBase(instance.Spec.SchedulerServiceTemplate),
		KeystoneAuthURL:   keystoneAuthURL,
		ServiceUser:       instance.Spec.ServiceUser,
		PasswordSelectors: instance.Spec.PasswordSelectors,
	}
	scheduler := &novav1.NovaScheduler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + "-scheduler",
			Namespace: instance.Namespace,
		},
	}

	op, err := controllerutil.CreateOrPatch(ctx, r.Client, scheduler, func() error {
		scheduler.Spec = spec
		err := controllerutil.SetControllerReference(instance, scheduler, r.Scheme)
		if err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		condition.FalseCondition(
			novav1.NovaSchedulerReadyCondition,
			condition.ErrorReason,
			condition.SeverityError,
			novav1.NovaSchedulerReadyErrorMessage,
			err.Error(),
		)
		return ctrl.Result{}, err
	}

	if op != controllerutil.OperationResultNone {
		util.LogForObject(
			h, fmt.Sprintf("NovaScheduler %s.", string(op)), instance,
			"NovaScheduler.Name", scheduler.Name,
		)
	}

	c := scheduler.Status.Conditions.Mirror(novav1.NovaSchedulerReadyCondition)
	// NOTE(gibi): it can be nil if the NovaScheduler CR is created but no
	// reconciliation is run on it to initialize the ReadyCondition yet.
	if c != nil {
		instance.Status.Conditions.Set(c)
	}
	instance.Status.SchedulerServiceReadyCount = scheduler.Status.ReadyCount

	return ctrl.Result{}, nil
}

func (r *NovaReconciler) ensureKeystoneServiceUser(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.Nova,
) error {
	// NOTE(sean-k-mooney): the service user project is currently
	// hardcoded to "service" so all service users are created as
	// a member of that shared service project
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

	service := keystonev1.NewKeystoneService(serviceSpec, instance.Namespace, serviceLabels, r.RequeueTimeout)
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

func (r *NovaReconciler) ensureDBDeletion(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.Nova,
) error {
	// remove finalizers from all of our MariaDBDatabases to ensure they
	// are deleted

	// initialize a nova dbs list with default db names and add used cells:
	novaDbs := []string{"nova-api"}
	for cellName := range instance.Spec.CellTemplates {
		novaDbs = append(novaDbs, novaapi.ServiceName+"-"+cellName)
	}
	// iterate over novaDbs and remove finalizers
	for _, dbName := range novaDbs {
		db, err := database.GetDatabaseByName(ctx, h, dbName)
		if err != nil && !k8s_errors.IsNotFound(err) {
			return err
		}
		if !k8s_errors.IsNotFound(err) {
			if err := db.DeleteFinalizer(ctx, h); err != nil {
				return err
			}
		}
	}

	util.LogForObject(h, "Removed finalizer from MariaDBDatabase CRs", instance, "MariaDBDatabase names", novaDbs)
	return nil
}

func (r *NovaReconciler) ensureKeystoneServiceUserDeletion(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.Nova,
) error {
	// Remove the finalizer from our KeystoneService CR
	// This is oddly added automatically when we created KeystoneService but
	// we need to remove it manually
	service, err := keystonev1.GetKeystoneServiceWithName(ctx, h, "nova", instance.Namespace)
	if err != nil && !k8s_errors.IsNotFound(err) {
		return err
	}

	if k8s_errors.IsNotFound(err) {
		// Nothing to do as it was never created
		return nil
	}

	updated := controllerutil.RemoveFinalizer(service, h.GetFinalizer())
	if !updated {
		// No finalizer to remove
		return nil
	}

	if err = h.GetClient().Update(ctx, service); err != nil && !k8s_errors.IsNotFound(err) {
		return err
	}
	util.LogForObject(h, "Removed finalizer from nova KeystoneService", instance)

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

func (r *NovaReconciler) reconcileDelete(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.Nova,
) error {
	util.LogForObject(h, "Reconciling delete", instance)

	err := r.ensureDBDeletion(ctx, h, instance)
	if err != nil {
		return err
	}

	err = r.ensureKeystoneServiceUserDeletion(ctx, h, instance)
	if err != nil {
		return err
	}

	// Successfully cleaned up everyting. So as the final step let's remove the
	// finalizer from ourselves to allow the deletion of Nova CR itself
	updated := controllerutil.RemoveFinalizer(instance, h.GetFinalizer())
	if updated {
		util.LogForObject(h, "Removed finalizer from ourselves", instance)
	}

	util.LogForObject(h, "Reconciled delete successfully", instance)
	return nil
}

func (r *NovaReconciler) ensureMQ(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.Nova,
	transportName string,
	messageBusInstanceName string,
) (string, nova.MessageBusStatus, error) {
	transportURL := &rabbitmqv1.TransportURL{
		ObjectMeta: metav1.ObjectMeta{
			Name:      transportName,
			Namespace: instance.Namespace,
		},
	}

	op, err := controllerutil.CreateOrPatch(ctx, r.Client, transportURL, func() error {
		transportURL.Spec.RabbitmqClusterName = messageBusInstanceName

		err := controllerutil.SetControllerReference(instance, transportURL, r.Scheme)
		return err
	})

	if err != nil && !k8s_errors.IsNotFound(err) {
		return "", nova.MQFailed, util.WrapErrorForObject(
			fmt.Sprintf("Error create or update TransportURL object %s", transportName),
			transportURL,
			err,
		)
	}

	if op != controllerutil.OperationResultNone {
		util.LogForObject(h, fmt.Sprintf("TransportURL object %s created or patched", transportName), transportURL)
		return "", nova.MQCreating, nil
	}

	err = r.Client.Get(ctx, types.NamespacedName{Namespace: instance.Namespace, Name: transportName}, transportURL)
	if err != nil && !k8s_errors.IsNotFound(err) {
		return "", nova.MQFailed, util.WrapErrorForObject(
			fmt.Sprintf("Error reading TransportURL object %s", transportName),
			transportURL,
			err,
		)
	}

	if k8s_errors.IsNotFound(err) || !transportURL.IsReady() || transportURL.Status.SecretName == "" {
		return "", nova.MQCreating, nil
	}

	return transportURL.Status.SecretName, nova.MQCompleted, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NovaReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&novav1.Nova{}).
		Owns(&mariadbv1.MariaDBDatabase{}).
		Owns(&keystonev1.KeystoneService{}).
		Owns(&novav1.NovaAPI{}).
		Owns(&novav1.NovaScheduler{}).
		Owns(&novav1.NovaCell{}).
		Owns(&rabbitmqv1.TransportURL{}).
		Complete(r)
}
