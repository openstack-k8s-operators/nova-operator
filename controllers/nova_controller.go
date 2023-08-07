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

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/openstack-k8s-operators/lib-common/modules/common"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/endpoint"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	helper "github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	job "github.com/openstack-k8s-operators/lib-common/modules/common/job"
	"github.com/openstack-k8s-operators/lib-common/modules/common/labels"
	common_rbac "github.com/openstack-k8s-operators/lib-common/modules/common/rbac"
	"github.com/openstack-k8s-operators/lib-common/modules/common/secret"
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

// service account, role, rolebinding
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=roles,verbs=get;list;watch;create;update
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=rolebindings,verbs=get;list;watch;create;update
// service account permissions that are needed to grant permission to the above
// +kubebuilder:rbac:groups="security.openshift.io",resourceNames=anyuid,resources=securitycontextconstraints,verbs=use
// +kubebuilder:rbac:groups="",resources=pods,verbs=create;delete;get;list;patch;update;watch

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
		// we intentionally return immediately to force the deferred function
		// to persist the Instance with the finalizer. We need to have our own
		// finalizer persisted before we try to create the KeystoneService with
		// our finalizer to avoid orphaning the KeystoneService.
		return ctrl.Result{}, nil
	}

	// Service account, role, binding
	rbacRules := []rbacv1.PolicyRule{
		{
			APIGroups:     []string{"security.openshift.io"},
			ResourceNames: []string{"anyuid"},
			Resources:     []string{"securitycontextconstraints"},
			Verbs:         []string{"use"},
		},
		{
			APIGroups: []string{""},
			Resources: []string{"pods"},
			Verbs:     []string{"create", "get", "list", "watch", "update", "patch", "delete"},
		},
	}
	rbacResult, err := common_rbac.ReconcileRbac(ctx, h, instance, rbacRules)
	if err != nil {
		return rbacResult, err
	} else if (rbacResult != ctrl.Result{}) {
		return rbacResult, nil
	}

	// TODO(gibi): This should be checked in a webhook and reject the CR
	// creation instead of setting its status.
	cell0Template, err := r.getCell0Template(instance)
	if err != nil {
		return ctrl.Result{}, err
	}

	expectedSelectors := []string{
		instance.Spec.PasswordSelectors.Service,
		instance.Spec.PasswordSelectors.APIDatabase,
		instance.Spec.PasswordSelectors.MetadataSecret,
	}
	for _, cellTemplate := range instance.Spec.CellTemplates {
		expectedSelectors = append(expectedSelectors, cellTemplate.PasswordSelectors.Database)
	}

	_, result, secret, err := ensureSecret(
		ctx,
		types.NamespacedName{Namespace: instance.Namespace, Name: instance.Spec.Secret},
		expectedSelectors,
		h.GetClient(),
		&instance.Status.Conditions,
		r.RequeueTimeout,
	)
	if (err != nil || result != ctrl.Result{}) {
		return result, err
	}
	instance.Status.Conditions.MarkTrue(condition.InputReadyCondition, condition.InputReadyMessage)

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
	// having up-call support
	// NOTE(gibi): We don't return on error or if the DB is not ready yet. We
	// move forward and kick off the rest of the work we can do (e.g. creating
	// Cell DBs and Cells without up-call support). Eventually we rely on the
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
	orderedCellNames := []string{novav1.Cell0Name}
	for cellName := range instance.Spec.CellTemplates {
		if cellName != novav1.Cell0Name {
			orderedCellNames = append(orderedCellNames, cellName)
		}
	}

	// Create the Cell DBs. Note that we are not returning on error or if the
	// DB creation is still in progress. We move forward with whatever we can
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
		ctx, h, instance, instance.Name+"-api-transport", instance.Spec.APIMessageBusInstance)
	switch apiMQStatus {
	case nova.MQFailed:
		instance.Status.Conditions.Set(condition.FalseCondition(
			novav1.NovaAPIMQReadyCondition,
			condition.ErrorReason,
			condition.SeverityError,
			novav1.NovaAPIMQReadyErrorMessage,
			apiMQError.Error(),
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
		return ctrl.Result{}, fmt.Errorf("Invalid MessageBusStatus from  for the API MQ: %d", apiMQStatus)
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
		if cellName == novav1.Cell0Name {
			cellMQ = apiMQSecretName
			status = apiMQStatus
			err = apiMQError
		} else {
			cellMQ, status, err = r.ensureMQ(
				ctx, h, instance, instance.Name+"-"+cellName+"-transport", cellTemplate.CellMessageBusInstance)
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
	deployingCells := []string{}
	mappingCells := []string{}
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
		// orderedCellNames. So for any other cells we can assume that if cell0
		// is not in the list then cell0 is not ready
		cell0Ready := (cells[novav1.Cell0Name] != nil && cells[novav1.Cell0Name].IsReady())
		if cellName != novav1.Cell0Name && cellTemplate.HasAPIAccess && !cell0Ready {
			allCellsReady = false
			skippedCells = append(skippedCells, cellName)
			util.LogForObject(
				h, "Skip NovaCell as cell0 is not ready yet and this cell needs API DB access",
				instance, "CellName", cellName)
			continue
		}

		cell, status, err := r.ensureCell(
			ctx, h, instance, cellName, cellTemplate,
			cellDB.Database, apiDB, cellMQ.SecretName, keystoneAuthURL, secret,
		)
		cells[cellName] = cell
		switch status {
		case nova.CellDeploying:
			deployingCells = append(deployingCells, cellName)
		case nova.CellMapping:
			mappingCells = append(mappingCells, cellName)
		case nova.CellFailed, nova.CellMappingFailed:
			failedCells = append(failedCells, fmt.Sprintf("%s(%v)", cellName, err.Error()))
		case nova.CellReady:
			readyCells = append(readyCells, cellName)
		}
		allCellsReady = allCellsReady && status == nova.CellReady
	}
	util.LogForObject(
		h, "Cell statuses", instance,
		"waiting", skippedCells,
		"deploying", deployingCells,
		"mapping", mappingCells,
		"ready", readyCells,
		"failed", failedCells,
		"all cells ready", allCellsReady)
	if len(failedCells) > 0 {
		instance.Status.Conditions.Set(condition.FalseCondition(
			novav1.NovaAllCellsReadyCondition,
			condition.ErrorReason,
			condition.SeverityError,
			novav1.NovaAllCellsReadyErrorMessage,
			strings.Join(failedCells, ","),
		))
	} else if len(deployingCells)+len(mappingCells) > 0 {
		instance.Status.Conditions.Set(condition.FalseCondition(
			novav1.NovaAllCellsReadyCondition,
			condition.RequestedReason,
			condition.SeverityWarning,
			novav1.NovaAllCellsReadyNotReadyMessage,
			strings.Join(append(deployingCells, mappingCells...), ","),
		))
	} else if len(skippedCells) > 0 {
		instance.Status.Conditions.Set(condition.FalseCondition(
			novav1.NovaAllCellsReadyCondition,
			condition.InitReason,
			condition.SeverityWarning,
			novav1.NovaAllCellsReadyWaitingMessage,
			strings.Join(skippedCells, ","),
		))
	}
	if allCellsReady {
		instance.Status.Conditions.MarkTrue(novav1.NovaAllCellsReadyCondition, novav1.NovaAllCellsReadyMessage)
	}

	// Don't move forward with the top level service creations like NovaAPI
	// until cell0 is ready as top level services need cell0 to register in
	if cell0, ok := cells[novav1.Cell0Name]; !ok || !cell0.IsReady() {
		// we need to stop here until cell0 is ready
		util.LogForObject(h, "Waiting for cell0 to become Ready before creating the top level services", instance)
		return ctrl.Result{}, nil
	}

	topLevelSecretName, err := r.ensureTopLevelSecret(ctx, h, instance, cell0Template, secret)
	if err != nil {
		return ctrl.Result{}, err
	}

	result, err = r.ensureAPI(
		ctx, h, instance, cell0Template,
		cellDBs[novav1.Cell0Name].Database, apiDB, apiMQSecretName, keystoneAuthURL,
		topLevelSecretName,
	)
	if err != nil {
		return result, err
	}

	result, err = r.ensureScheduler(
		ctx, h, instance, cell0Template,
		cellDBs[novav1.Cell0Name].Database, apiDB, apiMQSecretName, keystoneAuthURL,
		topLevelSecretName,
	)
	if err != nil {
		return result, err
	}

	if *instance.Spec.MetadataServiceTemplate.Replicas != 0 {
		result, err = r.ensureMetadata(
			ctx, h, instance, cell0Template,
			cellDBs[novav1.Cell0Name].Database, apiDB, apiMQSecretName, keystoneAuthURL,
			topLevelSecretName,
		)
		if err != nil {
			return result, err
		}
	} else {
		instance.Status.Conditions.Remove(novav1.NovaMetadataReadyCondition)
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

	if instance.Status.RegisteredCells == nil {
		instance.Status.RegisteredCells = map[string]string{}
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
				condition.InputReadyCondition,
				condition.InitReason,
				condition.InputReadyInitMessage,
			),
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
			condition.UnknownCondition(
				novav1.NovaMetadataReadyCondition,
				condition.InitReason,
				novav1.NovaMetadataReadyInitMessage,
			),
			// service account, role, rolebinding conditions
			condition.UnknownCondition(
				condition.ServiceAccountReadyCondition,
				condition.InitReason,
				condition.ServiceAccountReadyInitMessage,
			),
			condition.UnknownCondition(
				condition.RoleReadyCondition,
				condition.InitReason,
				condition.RoleReadyInitMessage,
			),
			condition.UnknownCondition(
				condition.RoleBindingReadyCondition,
				condition.InitReason,
				condition.RoleBindingReadyInitMessage,
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

	if cell0Template, ok = instance.Spec.CellTemplates[novav1.Cell0Name]; !ok {
		err := fmt.Errorf("missing cell0 specification from Spec.CellTemplates")
		instance.Status.Conditions.Set(condition.FalseCondition(
			novav1.NovaAllCellsReadyCondition,
			condition.ErrorReason,
			condition.SeverityError,
			novav1.NovaAllCellsReadyErrorMessage,
			fmt.Sprintf("%s(%v)", novav1.Cell0Name, err.Error()),
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
	secret corev1.Secret,
) (*novav1.NovaCell, nova.CellDeploymentStatus, error) {

	cellSecretName, err := r.ensureCellSecret(ctx, h, instance, cellName, cellTemplate, secret)
	if err != nil {
		return nil, nova.CellDeploying, err
	}

	cellSpec := novav1.NovaCellSpec{
		CellName:                  cellName,
		Secret:                    cellSecretName,
		CellDatabaseHostname:      cellDB.GetDatabaseHostname(),
		CellDatabaseUser:          cellTemplate.CellDatabaseUser,
		CellMessageBusSecretName:  cellMQSecretName,
		ConductorServiceTemplate:  cellTemplate.ConductorServiceTemplate,
		MetadataServiceTemplate:   cellTemplate.MetadataServiceTemplate,
		NoVNCProxyServiceTemplate: cellTemplate.NoVNCProxyServiceTemplate,
		NodeSelector:              cellTemplate.NodeSelector,
		Debug:                     instance.Spec.Debug,
		// TODO(gibi): this should be part of the secret
		ServiceUser:     instance.Spec.ServiceUser,
		KeystoneAuthURL: keystoneAuthURL,
		ServiceAccount:  instance.RbacResourceName(),
	}
	if cellTemplate.HasAPIAccess {
		cellSpec.APIDatabaseHostname = apiDB.GetDatabaseHostname()
		cellSpec.APIDatabaseUser = instance.Spec.APIDatabaseUser
	}

	cell := &novav1.NovaCell{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getNovaCellCRName(instance.Name, cellSpec.CellName),
			Namespace: instance.Namespace,
		},
	}

	op, err := controllerutil.CreateOrPatch(ctx, r.Client, cell, func() error {
		// TODO(gibi): Pass down a narrowed secret that only hold
		// specific information but also holds user names
		cell.Spec = cellSpec
		if len(cell.Spec.NodeSelector) == 0 {
			cell.Spec.NodeSelector = instance.Spec.NodeSelector
		}

		err := controllerutil.SetControllerReference(instance, cell, r.Scheme)
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return cell, nova.CellFailed, err
	}

	if op != controllerutil.OperationResultNone {
		util.LogForObject(h, fmt.Sprintf("NovaCell %s.", string(op)), instance, "NovaCell.Name", cell.Name)
	}

	if !cell.IsReady() {
		// We wait for the cell to become Ready before we map it in the
		// nova_api DB.
		return cell, nova.CellDeploying, err
	}

	// When the cell is ready we need to create a row in the
	// nova_api.CellMapping table to make this cell accessible from the top
	// level services.
	status, err := r.ensureCellMapped(ctx, h, instance, cell, cellTemplate, apiDB.GetDatabaseHostname())
	if status == nova.CellMappingReady {
		// As mapping is the last step if that is ready then the cell is ready
		status = nova.CellReady
	}

	return cell, status, err
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
	secretName string,
) (ctrl.Result, error) {
	// TODO(gibi): Pass down a narrowed secret that only hold
	// specific information but also holds user names
	apiSpec := novav1.NovaAPISpec{
		Secret:                  secretName,
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
		Override:        instance.Spec.APIServiceTemplate.Override,
		KeystoneAuthURL: keystoneAuthURL,
		ServiceUser:     instance.Spec.ServiceUser,
		ServiceAccount:  instance.RbacResourceName(),
		RegisteredCells: instance.Status.RegisteredCells,
	}
	api := &novav1.NovaAPI{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + "-api",
			Namespace: instance.Namespace,
		},
	}

	op, err := controllerutil.CreateOrPatch(ctx, r.Client, api, func() error {
		api.Spec = apiSpec
		if len(api.Spec.NodeSelector) == 0 {
			api.Spec.NodeSelector = instance.Spec.NodeSelector
		}
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
	secretName string,
) (ctrl.Result, error) {
	// TODO(gibi): Pass down a narrowed secret that only hold
	// specific information but also holds user names
	spec := novav1.NovaSchedulerSpec{
		Secret:                  secretName,
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
		NovaServiceBase: novav1.NovaServiceBase(instance.Spec.SchedulerServiceTemplate),
		KeystoneAuthURL: keystoneAuthURL,
		ServiceUser:     instance.Spec.ServiceUser,
		ServiceAccount:  instance.RbacResourceName(),
		RegisteredCells: instance.Status.RegisteredCells,
	}
	scheduler := &novav1.NovaScheduler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + "-scheduler",
			Namespace: instance.Namespace,
		},
	}

	op, err := controllerutil.CreateOrPatch(ctx, r.Client, scheduler, func() error {
		scheduler.Spec = spec
		if len(scheduler.Spec.NodeSelector) == 0 {
			scheduler.Spec.NodeSelector = instance.Spec.NodeSelector
		}
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
	// NOTE(gibi): we use the internal endpoint as that is expected to be
	// available on the external compute nodes as well and we want to keep
	// thing consistent
	authURL, err := keystoneAPI.GetEndpoint(endpoint.EndpointInternal)
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

	// Successfully cleaned up everything. So as the final step let's remove the
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

func (r *NovaReconciler) ensureMetadata(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.Nova,
	cell0Template novav1.NovaCellTemplate,
	cell0DB *database.Database,
	apiDB *database.Database,
	apiMQSecretName string,
	keystoneAuthURL string,
	secretName string,
) (ctrl.Result, error) {

	// TODO(gibi): Pass down a narrowed secret that only hold
	// specific information but also holds user names
	apiSpec := novav1.NovaMetadataSpec{
		Secret:                  secretName,
		APIDatabaseHostname:     apiDB.GetDatabaseHostname(),
		APIDatabaseUser:         instance.Spec.APIDatabaseUser,
		CellDatabaseHostname:    cell0DB.GetDatabaseHostname(),
		CellDatabaseUser:        cell0Template.CellDatabaseUser,
		APIMessageBusSecretName: apiMQSecretName,
		Debug:                   instance.Spec.Debug,
		NovaServiceBase: novav1.NovaServiceBase{
			ContainerImage:         instance.Spec.MetadataServiceTemplate.ContainerImage,
			Replicas:               instance.Spec.MetadataServiceTemplate.Replicas,
			NodeSelector:           instance.Spec.MetadataServiceTemplate.NodeSelector,
			CustomServiceConfig:    instance.Spec.MetadataServiceTemplate.CustomServiceConfig,
			DefaultConfigOverwrite: instance.Spec.MetadataServiceTemplate.DefaultConfigOverwrite,
			Resources:              instance.Spec.MetadataServiceTemplate.Resources,
			NetworkAttachments:     instance.Spec.MetadataServiceTemplate.NetworkAttachments,
		},
		Override:        instance.Spec.MetadataServiceTemplate.Override,
		ServiceUser:     instance.Spec.ServiceUser,
		KeystoneAuthURL: keystoneAuthURL,
		ServiceAccount:  instance.RbacResourceName(),
		RegisteredCells: instance.Status.RegisteredCells,
	}
	metadata := &novav1.NovaMetadata{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + "-metadata",
			Namespace: instance.Namespace,
		},
	}

	op, err := controllerutil.CreateOrPatch(ctx, r.Client, metadata, func() error {
		metadata.Spec = apiSpec

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
		condition.FalseCondition(
			novav1.NovaMetadataReadyCondition,
			condition.ErrorReason,
			condition.SeverityError,
			novav1.NovaMetadataReadyErrorMessage,
			err.Error(),
		)
		return ctrl.Result{}, err
	}

	if op != controllerutil.OperationResultNone {
		util.LogForObject(h, fmt.Sprintf("NovaMetadata %s.", string(op)), instance, "NovaMetadata.Name", metadata.Name)
	}

	c := metadata.Status.Conditions.Mirror(novav1.NovaMetadataReadyCondition)
	// NOTE(gibi): it can be nil if the NovaMetadata CR is created but no
	// reconciliation is run on it to initialize the ReadyCondition yet.
	if c != nil {
		instance.Status.Conditions.Set(c)
	}
	instance.Status.APIServiceReadyCount = metadata.Status.ReadyCount

	return ctrl.Result{}, nil
}

// ensureCellMapped makes sure that the cell has a row in the
// nova_api.CellMapping table by calling nova-manage cell_v2 CLI commands in a
// Job. When a cell is mapped then the name of the cell and the hash of the
// cell config (DB and MQ URL) is stored in the Nova.Status
// so that each cell is only mapped once or when its config is changed.
func (r *NovaReconciler) ensureCellMapped(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.Nova,
	cell *novav1.NovaCell,
	cellTemplate novav1.NovaCellTemplate,
	apiDBHostname string,
) (nova.CellDeploymentStatus, error) {
	ospSecret, _, err := secret.GetSecret(ctx, h, instance.Spec.Secret, instance.Namespace)
	if err != nil {
		return nova.CellMappingFailed, err
	}

	mqSecret, _, err := secret.GetSecret(ctx, h, cell.Spec.CellMessageBusSecretName, instance.Namespace)
	if err != nil {
		return nova.CellMappingFailed, err
	}

	configMapName := fmt.Sprintf("%s-config-data", cell.Name+"-manage")
	scriptConfigMapName := fmt.Sprintf("%s-scripts", cell.Name+"-manage")

	cmLabels := labels.GetLabels(
		instance, labels.GetGroupLabel(NovaLabelPrefix), map[string]string{},
	)

	extraTemplates := map[string]string{
		"01-nova.conf":    "/nova.conf",
		"nova-blank.conf": "/nova-blank.conf",
	}

	// We configure the Job like it runs in the env of the conductor of the given cell
	// but we ensure that the config always has [api_database] section configure
	// even if the cell has no API access at all.
	templateParameters := map[string]interface{}{
		"service_name":           "nova-conductor",
		"keystone_internal_url":  cell.Spec.KeystoneAuthURL,
		"nova_keystone_user":     cell.Spec.ServiceUser,
		"nova_keystone_password": string(ospSecret.Data[instance.Spec.PasswordSelectors.Service]),
		// cell.Spec.APIDatabaseUser is empty for cells without APIDB access
		"api_db_name":     instance.Spec.APIDatabaseUser, // fixme
		"api_db_user":     instance.Spec.APIDatabaseUser,
		"api_db_password": string(ospSecret.Data[instance.Spec.PasswordSelectors.APIDatabase]),
		// cell.Spec.APIDatabaseHostname is empty for cells without APIDB access
		"api_db_address":         apiDBHostname,
		"api_db_port":            3306,
		"cell_db_name":           cell.Spec.CellDatabaseUser, // fixme
		"cell_db_user":           cell.Spec.CellDatabaseUser,
		"cell_db_password":       string(ospSecret.Data[cellTemplate.PasswordSelectors.Database]),
		"cell_db_address":        cell.Spec.CellDatabaseHostname,
		"cell_db_port":           3306,
		"openstack_cacert":       "",          // fixme
		"openstack_region_name":  "regionOne", // fixme
		"default_project_domain": "Default",   // fixme
		"default_user_domain":    "Default",   // fixme
	}

	// NOTE(gibi): cell mapping for cell0 should not have transport_url
	// configured. As the nova-manage command used to create the mapping
	// uses the transport_url from the nova.conf provided to the job
	// we need to make sure that transport_url is only configured for the job
	// if it is mapping other than cell0.
	if cell.Spec.CellName != novav1.Cell0Name {
		templateParameters["transport_url"] = string(mqSecret.Data["transport_url"])
	}

	cms := []util.Template{
		// ScriptsConfigMap
		{
			Name:         scriptConfigMapName,
			Namespace:    instance.Namespace,
			Type:         util.TemplateTypeScripts,
			InstanceType: "nova-manage",
			Labels:       cmLabels,
		},
		// ConfigMap
		{
			Name:               configMapName,
			Namespace:          instance.Namespace,
			Type:               util.TemplateTypeConfig,
			InstanceType:       "nova-manage",
			ConfigOptions:      templateParameters,
			Labels:             cmLabels,
			CustomData:         map[string]string{},
			Annotations:        map[string]string{},
			AdditionalTemplate: extraTemplates,
		},
	}

	configHash := make(map[string]env.Setter)
	err = secret.EnsureSecrets(ctx, h, instance, cms, &configHash)

	if err != nil {
		return nova.CellMappingFailed, err
	}

	// This defines those input parameters that can trigger the re-run of the
	// job and therefore an update on the cell mapping row of this cell in the
	// DB. Today it is the full nova config of the container. We could trim
	// that a bit in the future if we want as we probably don't need to re-run
	// the Job if only keystone_internal_url changes in the config.
	inputHash, err := util.HashOfInputHashes(configHash)
	if err != nil {
		return nova.CellMappingFailed, err
	}

	labels := map[string]string{
		common.AppSelector: NovaLabelPrefix,
	}
	jobDef := nova.CellMappingJob(instance, cell, configMapName, scriptConfigMapName, inputHash, labels)

	job := job.NewJob(
		jobDef, cell.Name+"-cell-mapping",
		instance.Spec.Debug.PreserveJobs, r.RequeueTimeout,
		instance.Status.RegisteredCells[cell.Name])

	result, err := job.DoJob(ctx, h)
	if err != nil {
		return nova.CellMappingFailed, err
	}

	if (result != ctrl.Result{}) {
		// Job is still running. We can simply return as we will be reconciled
		// when the Job status changes
		return nova.CellMapping, nil
	}

	if !job.HasChanged() {
		// there was no need to run a new job as nothing changed
		return nova.CellMappingReady, nil
	}

	// A new cell mapping job is finished. Let's store the result so we
	// won't run the job with the same inputs again.
	// Also the controller distributes the instance.Status.RegisteredCells
	// information to the top level services so that each service can restart
	// their Pods if a new cell is registered or an existing cell is updated.
	instance.Status.RegisteredCells[cell.Name] = job.GetHash()
	r.Log.Info(fmt.Sprintf("Job %s hash added - %s", jobDef.Name, instance.Status.RegisteredCells[cell.Name]))

	return nova.CellMappingReady, nil
}

// ensureCellSecret makes sure that the internal Cell Secret exists and up to
// date
func (r *NovaReconciler) ensureCellSecret(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.Nova,
	cellName string,
	cellTemplate novav1.NovaCellTemplate,
	externalSecret corev1.Secret,
) (string, error) {

	// NOTE(gibi): We can move other sensitive data to the internal Secret from
	// the NovaCellSpec fields, possibly hostnames or usernames.
	// XXX(gibi): Move the transport_url from from the MQ secret to the internal secret
	data := map[string]string{
		ServicePasswordSelector:      string(externalSecret.Data[instance.Spec.PasswordSelectors.Service]),
		CellDatabasePasswordSelector: string(externalSecret.Data[cellTemplate.PasswordSelectors.Database]),
	}

	if cellTemplate.HasAPIAccess {
		data[APIDatabasePasswordSelector] = string(externalSecret.Data[instance.Spec.PasswordSelectors.APIDatabase])
	}

	// NOTE(gibi): there could be two reasons for 0 replicas in a cell
	// i) cell0 always have 0 replicas from metadata
	// ii) metadata can be deployed on the top level instead of deployed per
	// cell. In this case each cell will have 0 replica from metadta.
	if *cellTemplate.MetadataServiceTemplate.Replicas > 0 {
		data[MetadataSecretSelector] = string(externalSecret.Data[instance.Spec.PasswordSelectors.MetadataSecret])
	}

	// NOTE(gibi): When we switch to immutable secrets then we need to include
	// the hash of the secret data into the name of the secret to avoid
	// deleting and re-creating the Secret with the same name.
	secretName := getNovaCellCRName(instance.Name, cellName)

	labels := labels.GetLabels(
		instance, labels.GetGroupLabel(NovaLabelPrefix), map[string]string{},
	)

	template := util.Template{
		Name:         secretName,
		Namespace:    instance.Namespace,
		Type:         util.TemplateTypeNone,
		InstanceType: instance.GetObjectKind().GroupVersionKind().Kind,
		Labels:       labels,
		CustomData:   data,
	}

	err := secret.EnsureSecrets(ctx, h, instance, []util.Template{template}, nil)

	return secretName, err
}

// ensureCellSecret makes sure that the internal Cell Secret exists and up to
// date
func (r *NovaReconciler) ensureTopLevelSecret(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.Nova,
	cell0Template novav1.NovaCellTemplate,
	externalSecret corev1.Secret,
) (string, error) {

	// NOTE(gibi): We can move other sensitive data to the internal Secret from
	// the subCR fields, possibly hostnames or usernames.
	// XXX(gibi): Move the transport_url from from the MQ secret to the internal secret
	data := map[string]string{
		ServicePasswordSelector:      string(externalSecret.Data[instance.Spec.PasswordSelectors.Service]),
		APIDatabasePasswordSelector:  string(externalSecret.Data[instance.Spec.PasswordSelectors.APIDatabase]),
		CellDatabasePasswordSelector: string(externalSecret.Data[cell0Template.PasswordSelectors.Database]),
		MetadataSecretSelector:       string(externalSecret.Data[instance.Spec.PasswordSelectors.MetadataSecret]),
	}

	// NOTE(gibi): When we switch to immutable secrets then we need to include
	// the hash of the secret data into the name of the secret to avoid
	// deleting and re-creating the Secret with the same name.
	secretName := instance.Name

	labels := labels.GetLabels(
		instance, labels.GetGroupLabel(NovaLabelPrefix), map[string]string{},
	)

	template := util.Template{
		Name:         secretName,
		Namespace:    instance.Namespace,
		Type:         util.TemplateTypeNone,
		InstanceType: instance.GetObjectKind().GroupVersionKind().Kind,
		Labels:       labels,
		CustomData:   data,
	}

	err := secret.EnsureSecrets(ctx, h, instance, []util.Template{template}, nil)

	return secretName, err
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
		Owns(&novav1.NovaMetadata{}).
		Owns(&rabbitmqv1.TransportURL{}).
		Owns(&batchv1.Job{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Watches(&source.Kind{Type: &corev1.Secret{}},
			handler.EnqueueRequestsFromMapFunc(r.GetSecretMapperFor(&novav1.NovaList{}))).
		Complete(r)
}
