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
	"errors"
	"fmt"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"golang.org/x/exp/maps"

	"github.com/go-logr/logr"
	"github.com/openstack-k8s-operators/lib-common/modules/common"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/endpoint"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	helper "github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	job "github.com/openstack-k8s-operators/lib-common/modules/common/job"
	"github.com/openstack-k8s-operators/lib-common/modules/common/labels"
	common_rbac "github.com/openstack-k8s-operators/lib-common/modules/common/rbac"
	"github.com/openstack-k8s-operators/lib-common/modules/common/secret"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"
	util "github.com/openstack-k8s-operators/lib-common/modules/common/util"

	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/nova-operator/internal/nova"
	"github.com/openstack-k8s-operators/nova-operator/internal/novaapi"

	memcachedv1 "github.com/openstack-k8s-operators/infra-operator/apis/memcached/v1beta1"
	rabbitmqv1 "github.com/openstack-k8s-operators/infra-operator/apis/rabbitmq/v1beta1"
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	mariadbv1 "github.com/openstack-k8s-operators/mariadb-operator/api/v1beta1"
)

// NovaReconciler reconciles a Nova object
type NovaReconciler struct {
	ReconcilerBase
}

// GetLogger returns a logger object with a prefix of "controller.name" and additional controller context fields
func (r *NovaReconciler) GetLogger(ctx context.Context) logr.Logger {
	return log.FromContext(ctx).WithName("Controllers").WithName("Nova")
}

// +kubebuilder:rbac:groups=nova.openstack.org,resources=nova,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nova.openstack.org,resources=nova/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=nova.openstack.org,resources=nova/finalizers,verbs=update;patch
// +kubebuilder:rbac:groups=mariadb.openstack.org,resources=mariadbdatabases,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=mariadb.openstack.org,resources=mariadbdatabases/finalizers,verbs=update;patch
// +kubebuilder:rbac:groups=mariadb.openstack.org,resources=mariadbaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mariadb.openstack.org,resources=mariadbaccounts/finalizers,verbs=update;patch
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneapis,verbs=get;list;watch;
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneservices,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneendpoints,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=rabbitmq.openstack.org,resources=transporturls,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=memcached.openstack.org,resources=memcacheds,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=memcached.openstack.org,resources=memcacheds/finalizers,verbs=update;patch

// service account, role, rolebinding
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=roles,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=rolebindings,verbs=get;list;watch;create;update;patch
// service account permissions that are needed to grant permission to the above
// +kubebuilder:rbac:groups="security.openshift.io",resourceNames=anyuid,resources=securitycontextconstraints,verbs=use
// +kubebuilder:rbac:groups="",resources=pods,verbs=create;delete;get;list;patch;update;watch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete;

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
	Log := r.GetLogger(ctx)

	// Fetch the NovaAPI instance that needs to be reconciled
	instance := &novav1.Nova{}
	err := r.Client.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected.
			// For additional cleanup logic use finalizers. Return and don't requeue.
			Log.Info("Nova instance not found, probably deleted before reconciled. Nothing to do.")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		Log.Error(err, "Failed to read the Nova instance.")
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

	if !instance.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, r.reconcileDelete(ctx, h, instance)
	}

	// We create a KeystoneService CR later and that will automatically get the
	// Nova finalizer. So we need a finalizer on the ourselves too so that
	// during Nova CR delete we can have a chance to remove the finalizer from
	// the our KeystoneService so that is also deleted.
	updated := controllerutil.AddFinalizer(instance, h.GetFinalizer())
	if updated {
		Log.Info("Added finalizer to ourselves")
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

	// ensure MariaDBAccount exists.  This account record may be created by
	// openstack-operator or the cloud operator up front without a specific
	// MariaDBDatabase configured yet.   Otherwise, a MariaDBAccount CR is
	// created here with a generated username as well as a secret with
	// generated password.   The MariaDBAccount is created without being
	// yet associated with any MariaDBDatabase.
	_, _, err = mariadbv1.EnsureMariaDBAccount(
		ctx, h, instance.Spec.APIDatabaseAccount,
		instance.Namespace, false, "nova_api",
	)

	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			mariadbv1.MariaDBAccountReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			mariadbv1.MariaDBAccountNotReadyMessage,
			err.Error()))

		return ctrl.Result{}, err
	}
	instance.Status.Conditions.MarkTrue(
		mariadbv1.MariaDBAccountReadyCondition,
		mariadbv1.MariaDBAccountReadyMessage,
	)

	// There is a webhook validation that ensures that there is always cell0 in
	// the cellTemplates
	cell0Template := instance.Spec.CellTemplates[novav1.Cell0Name]

	expectedSelectors := []string{
		instance.Spec.PasswordSelectors.Service,
		instance.Spec.PasswordSelectors.MetadataSecret,
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
		Log.Info("Waiting for the KeystoneService to become Ready")
		return ctrl.Result{}, nil
	}

	keystoneInternalAuthURL, keystonePublicAuthURL, region, err := r.getKeystoneAuthURL(
		ctx, h, instance)
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
		return ctrl.Result{}, fmt.Errorf("%w from ensureAPIDB: %d", util.ErrInvalidStatus, apiDBStatus)
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
			return ctrl.Result{}, fmt.Errorf("%w from ensureCellDB: %d for cell %s", util.ErrInvalidStatus, status, cellName)
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
			condition.RequestedReason,
			condition.SeverityInfo,
			novav1.NovaAllCellsDBReadyCreatingMessage,
			strings.Join(creatingDBs, ",")))
	} else { // we have no DB in failed or creating status so all DB is ready
		instance.Status.Conditions.MarkTrue(
			novav1.NovaAllCellsDBReadyCondition, novav1.NovaAllCellsDBReadyMessage)
	}

	_, err = ensureMemcached(ctx, h, instance.Namespace, instance.Spec.MemcachedInstance, &instance.Status.Conditions)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Create TransportURLs to access the message buses of each cell. Cell0
	// message bus is always the same as the top level API message bus so
	// we create API MQ separately first
	apiTransportURL, apiQuorumQueues, apiMQStatus, apiMQError := r.ensureMQ(
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
			condition.RequestedReason,
			condition.SeverityInfo,
			novav1.NovaAPIMQReadyCreatingMessage,
		))
	case nova.MQCompleted:
		instance.Status.Conditions.MarkTrue(
			novav1.NovaAPIMQReadyCondition, novav1.NovaAPIMQReadyMessage)
	default:
		return ctrl.Result{}, fmt.Errorf("%w from  for the API MQ: %d", util.ErrInvalidStatus, apiMQStatus)
	}

	// nova broadcaster rabbit
	notificationBusName := ""
	if instance.Spec.NotificationsBusInstance != nil {
		notificationBusName = *instance.Spec.NotificationsBusInstance
	}

	var notificationTransportURL string
	var notificationMQStatus nova.MessageBusStatus
	var notificationMQError error

	notificationTransportURLName := instance.Name + "-notification-transport"
	if notificationBusName != "" {
		notificationTransportURL, _, notificationMQStatus, notificationMQError = r.ensureMQ(
			ctx, h, instance, notificationTransportURLName, notificationBusName)

		switch notificationMQStatus {
		case nova.MQFailed:
			instance.Status.Conditions.Set(condition.FalseCondition(
				novav1.NovaNotificationMQReadyCondition,
				condition.ErrorReason,
				condition.SeverityError,
				novav1.NovaNotificationMQReadyErrorMessage,
				notificationMQError.Error(),
			))
		case nova.MQCreating:
			instance.Status.Conditions.Set(condition.FalseCondition(
				novav1.NovaNotificationMQReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				novav1.NovaNotificationMQReadyCreatingMessage,
			))
		case nova.MQCompleted:
			instance.Status.Conditions.MarkTrue(
				novav1.NovaNotificationMQReadyCondition, novav1.NovaNotificationMQReadyMessage)
		default:
			return ctrl.Result{}, fmt.Errorf("%w from  for the Notification MQ: %d",
				util.ErrInvalidStatus, notificationMQStatus)
		}
	} else {
		instance.Status.Conditions.Remove(novav1.NovaNotificationMQReadyCondition)

		// Ensure to delete the previous notifications transport url
		transportURLList := &rabbitmqv1.TransportURLList{}
		listOpts := []client.ListOption{
			client.InNamespace(instance.Namespace),
		}
		if err := r.Client.List(ctx, transportURLList, listOpts...); err != nil {
			return ctrl.Result{}, err
		}

		for _, url := range transportURLList.Items {
			if strings.Contains(url.Name, notificationTransportURLName) {
				err = r.ensureMQDeleted(ctx, instance, url.Name)
				if err != nil {
					return ctrl.Result{}, err
				}
			}
		}
	}

	cellMQs := map[string]*nova.MessageBus{}
	var failedMQs []string
	var creatingMQs []string
	for _, cellName := range orderedCellNames {
		var cellTransportURL string
		var status nova.MessageBusStatus
		var err error
		cellTemplate := instance.Spec.CellTemplates[cellName]
		var cellQuorumQueues bool
		// cell0 does not need its own cell message bus it uses the
		// API message bus instead
		if cellName == novav1.Cell0Name {
			cellTransportURL = apiTransportURL
			cellQuorumQueues = apiQuorumQueues
			status = apiMQStatus
			err = apiMQError
		} else {
			cellTransportURL, cellQuorumQueues, status, err = r.ensureMQ(
				ctx, h, instance, instance.Name+"-"+cellName+"-transport", cellTemplate.CellMessageBusInstance)
		}
		switch status {
		case nova.MQFailed:
			failedMQs = append(failedMQs, fmt.Sprintf("%s(%v)", cellName, err.Error()))
		case nova.MQCreating:
			creatingMQs = append(creatingMQs, cellName)
		case nova.MQCompleted:
		default:
			return ctrl.Result{}, fmt.Errorf("%w from ensureMQ: %d for cell %s", util.ErrInvalidStatus, status, cellName)
		}
		cellMQs[cellName] = &nova.MessageBus{TransportURL: cellTransportURL, QuorumQueues: cellQuorumQueues, Status: status}
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
			condition.RequestedReason,
			condition.SeverityInfo,
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
	discoveringCells := []string{}
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
			Log.Info("Skipping NovaCell as waiting for the cell DB to be created",
				"CellName", cellName)
			continue
		}
		if cellMQ.Status != nova.MQCompleted {
			allCellsReady = false
			skippedCells = append(skippedCells, cellName)
			Log.Info("Skipping NovaCell as waiting for the cell MQ to be created",
				"CellName", cellName)
			continue
		}

		// The cell0 is always handled first in the loop as we iterate on
		// orderedCellNames. So for any other cells we can assume that if cell0
		// is not in the list then cell0 is not ready
		cell0Ready := (cells[novav1.Cell0Name] != nil && cells[novav1.Cell0Name].IsReady())
		if cellName != novav1.Cell0Name && cellTemplate.HasAPIAccess && !cell0Ready {
			allCellsReady = false
			skippedCells = append(skippedCells, cellName)
			Log.Info("Skip NovaCell as cell0 is not ready yet and this cell needs API DB access", "CellName", cellName)
			continue
		}
		cell, status, err := r.ensureCell(
			ctx, h, instance, cellName, cellTemplate,
			cellDB.Database, apiDB, cellMQ.TransportURL, cellMQ.QuorumQueues, notificationTransportURL,
			keystoneInternalAuthURL, region, secret,
		)
		cells[cellName] = cell
		switch status {
		case nova.CellDeploying:
			deployingCells = append(deployingCells, cellName)
		case nova.CellMapping:
			mappingCells = append(mappingCells, cellName)
		case nova.CellComputeDiscovering:
			discoveringCells = append(discoveringCells, cellName)
		case nova.CellFailed, nova.CellMappingFailed, nova.CellComputeDiscoveryFailed:
			failedCells = append(failedCells, fmt.Sprintf("%s(%v)", cellName, err.Error()))
		case nova.CellReady:
			readyCells = append(readyCells, cellName)
		}
		allCellsReady = allCellsReady && status == nova.CellReady
	}
	Log.Info("Cell statuses",
		"waiting", skippedCells,
		"deploying", deployingCells,
		"mapping", mappingCells,
		"discovering", discoveringCells,
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
	} else if len(deployingCells)+len(mappingCells)+len(discoveringCells) > 0 {
		instance.Status.Conditions.Set(condition.FalseCondition(
			novav1.NovaAllCellsReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
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
	if cell0, ok := cells[novav1.Cell0Name]; !ok || !cell0.IsReady() ||
		cell0.Generation > cell0.Status.ObservedGeneration {
		// we need to stop here until cell0 is ready
		Log.Info("Waiting for cell0 to become Ready before creating the top level services")
		return ctrl.Result{}, nil
	}

	topLevelSecretName, err := r.ensureTopLevelSecret(
		ctx, h, instance,
		apiTransportURL, apiQuorumQueues,
		notificationTransportURL,
		secret)
	if err != nil {
		return ctrl.Result{}, err
	}

	result, err = r.ensureAPI(
		ctx, instance, cell0Template,
		cellDBs[novav1.Cell0Name].Database, apiDB,
		keystoneInternalAuthURL, keystonePublicAuthURL, region,
		topLevelSecretName,
	)
	if err != nil {
		return result, err
	}

	result, err = r.ensureScheduler(
		ctx, instance, cell0Template,
		cellDBs[novav1.Cell0Name].Database, apiDB, keystoneInternalAuthURL, region,
		topLevelSecretName,
	)
	if err != nil {
		return result, err
	}

	if *instance.Spec.MetadataServiceTemplate.Enabled {
		result, err = r.ensureMetadata(
			ctx, instance, cell0Template,
			cellDBs[novav1.Cell0Name].Database, apiDB, keystoneInternalAuthURL, region,
			topLevelSecretName,
		)
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

	// remove finalizers from unused MariaDBAccount records but ONLY if
	// ensureAPIDB finished
	if apiDBStatus == nova.DBCompleted {
		err = mariadbv1.DeleteUnusedMariaDBAccountFinalizers(ctx, h, "nova-api", instance.Spec.APIDatabaseAccount, instance.Namespace)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	// We need to check and delete cells
	novaCellList := &novav1.NovaCellList{}
	listOpts := []client.ListOption{
		client.InNamespace(instance.Namespace),
	}
	if err := r.Client.List(ctx, novaCellList, listOpts...); err != nil {
		return ctrl.Result{}, err
	}

	SortNovaCellListByName(novaCellList)

	var deleteErrs []error
	toDeletCells := map[string]string{}

	for _, cr := range novaCellList.Items {
		_, ok := instance.Spec.CellTemplates[cr.Spec.CellName]
		if !ok {

			toDeletCells[cr.Spec.CellName] = cr.Spec.CellName
			result, err := r.ensureCellDeleted(ctx, h, instance,
				cr.Spec.CellName, apiTransportURL,
				secret, apiDB, cellDBs[novav1.Cell0Name].Database.GetDatabaseHostname(), cells[novav1.Cell0Name])
			if err != nil {
				deleteErrs = append(deleteErrs, fmt.Errorf("cell '%s' deletion failed, because: %w", cr.Spec.CellName, err))
			}
			if result == nova.CellDeleteComplete {
				Log.Info("Cell deleted", "cell", cr.Spec.CellName)
				delete(instance.Status.RegisteredCells, cr.Name)
				delete(toDeletCells, cr.Spec.CellName)
			}
		}
		if len(toDeletCells) > 0 {
			instance.Status.Conditions.Set(condition.FalseCondition(
				novav1.NovaCellsDeletionCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				novav1.NovaCellsDeletionMessage,
				strings.Join(maps.Keys(toDeletCells), ", "),
			))
		}
	}

	if len(deleteErrs) > 0 {
		delErrs := errors.Join(deleteErrs...)
		return ctrl.Result{}, delErrs
	}

	if len(toDeletCells) == 0 {
		Log.Info("All cells marked for deletion have been successfully deleted.")
		instance.Status.Conditions.MarkTrue(
			novav1.NovaCellsDeletionCondition,
			novav1.NovaCellsDeletionConditionReadyMessage,
		)
	}

	Log.Info("Successfully reconciled")
	return ctrl.Result{}, nil
}

func (r *NovaReconciler) ensureAccountDeletedIfOwned(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.Nova,
	accountName string,
) error {
	Log := r.GetLogger(ctx)

	account, err := mariadbv1.GetAccount(ctx, h, accountName, instance.Namespace)
	if err != nil && !k8s_errors.IsNotFound(err) {
		return err
	}
	if k8s_errors.IsNotFound(err) {
		// Nothing to delete
		return nil
	}

	// If it is not created by us, we don't clean it up
	if !OwnedBy(account, instance) {
		Log.Info("MariaDBAccount in not owned by Nova, not deleting", "account", account)
		return nil
	}

	// NOTE(gibi): We need to delete the Secret first and then the Account
	// otherwise we cannot retry the Secret deletion when the Account is
	// gone as we will not know the name of the Secret. This logic should
	// be moved to the mariadb-operator.
	err = secret.DeleteSecretsWithName(ctx, h, account.Spec.Secret, instance.Namespace)
	if err != nil && !k8s_errors.IsNotFound(err) {
		return err
	}
	err = r.Client.Delete(ctx, account)
	if err != nil && !k8s_errors.IsNotFound(err) {
		return err
	}

	Log.Info("Deleted MariaDBAccount", "account", account)
	return nil
}

func (r *NovaReconciler) ensureCellDeleted(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.Nova,
	cellName string,
	apiTransportURL string,
	topLevelSecret corev1.Secret,
	apiDB *mariadbv1.Database,
	APIDatabaseHostname string,
	cell0 *novav1.NovaCell,
) (nova.CellDeploymentStatus, error) {
	Log := r.GetLogger(ctx)
	cell := &novav1.NovaCell{}
	fullCellName := types.NamespacedName{
		Name:      getNovaCellCRName(instance.Name, cellName),
		Namespace: instance.GetNamespace(),
	}

	err := r.Client.Get(ctx, fullCellName, cell)
	if k8s_errors.IsNotFound(err) {
		// We cannot do further cleanup of the MariaDBDatabase and
		// MariaDBAccount as their name is only available in the NovaCell CR
		// since the cell definition is removed from the Nova CR already.
		return nova.CellDeleteComplete, nil
	}
	if err != nil {
		return nova.CellDeleteFailed, err
	}
	// If it is not created by us, we don't touch it
	if !OwnedBy(cell, instance) {
		Log.Info("Cell isn't defined in the Nova, but there is a  "+
			"Cell CR not owned by us. Not deleting it.",
			"cell", cell)
		return nova.CellDeleteComplete, nil
	}

	dbName, accountName := novaapi.ServiceName+"-"+cell.Spec.CellName, cell.Spec.CellDatabaseAccount

	configHash, scriptName, configName, err := r.ensureNovaManageJobSecret(ctx, h, instance,
		cell0, topLevelSecret, APIDatabaseHostname, apiTransportURL, apiDB)
	if err != nil {
		return nova.CellDeleteFailed, err
	}
	inputHash, err := util.HashOfInputHashes(configHash)
	if err != nil {
		return nova.CellDeleteFailed, err
	}

	labels := map[string]string{
		common.AppSelector: NovaLabelPrefix,
	}
	jobDef := nova.CellDeleteJob(instance, cell, configName, scriptName, inputHash, labels)
	job := job.NewJob(
		jobDef, cell.Name+"-cell-delete",
		instance.Spec.PreserveJobs, r.RequeueTimeout,
		inputHash)

	result, err := job.DoJob(ctx, h)
	if err != nil {
		return nova.CellDeleteFailed, err
	}

	if (result != ctrl.Result{}) {
		// Job is still running. We can simply return as we will be reconciled
		// when the Job status changes
		return nova.CellDeleteInProgress, nil
	}

	secretName := getNovaCellCRName(instance.Name, cellName)
	err = secret.DeleteSecretsWithName(ctx, h, secretName, instance.Namespace)
	if err != nil && !k8s_errors.IsNotFound(err) {
		return nova.CellDeleteFailed, err
	}
	configSecret, scriptSecret := r.getNovaManageJobSecretNames(cell)
	err = secret.DeleteSecretsWithName(ctx, h, configSecret, instance.Namespace)
	if err != nil && !k8s_errors.IsNotFound(err) {
		return nova.CellDeleteFailed, err
	}
	err = secret.DeleteSecretsWithName(ctx, h, scriptSecret, instance.Namespace)
	if err != nil && !k8s_errors.IsNotFound(err) {
		return nova.CellDeleteFailed, err
	}

	// Delete transportURL cr
	err = r.ensureMQDeleted(ctx, instance, instance.Name+"-"+cellName+"-transport")
	if err != nil && !k8s_errors.IsNotFound(err) {
		return nova.CellDeleteFailed, err
	}

	err = mariadbv1.DeleteDatabaseAndAccountFinalizers(
		ctx, h, dbName, accountName, instance.Namespace)
	if err != nil {
		return nova.CellDeleteFailed, err
	}

	err = r.ensureAccountDeletedIfOwned(ctx, h, instance, accountName)
	if err != nil {
		return nova.CellDeleteFailed, err
	}

	database := &mariadbv1.MariaDBDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dbName,
			Namespace: instance.Namespace,
		},
	}
	err = r.Client.Delete(ctx, database)
	if err != nil && !k8s_errors.IsNotFound(err) {
		return nova.CellDeleteFailed, err
	}
	Log.Info("Deleted MariaDBDatabase", "database", database)

	// Finally we delete the NovaCell CR. We need to do it as the last step
	// otherwise we cannot retry the above cleanup as we won't have the data
	// what to clean up.
	err = r.Client.Delete(ctx, cell)
	if err != nil && k8s_errors.IsNotFound(err) {
		return nova.CellDeleteFailed, err
	}

	Log.Info("Cell isn't defined in the Nova CR, so it is deleted", "cell", cell)
	return nova.CellDeleteComplete, nil
}

func (r *NovaReconciler) initStatus(
	instance *novav1.Nova,
) error {
	if err := r.initConditions(instance); err != nil {
		return err
	}

	if instance.Status.RegisteredCells == nil {
		instance.Status.RegisteredCells = map[string]string{}
	}
	if instance.Status.DiscoveredCells == nil {
		instance.Status.DiscoveredCells = map[string]string{}
	}

	return nil
}

func (r *NovaReconciler) initConditions(
	instance *novav1.Nova,
) error {
	//
	// initialize status
	//
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
			novav1.NovaCellsDeletionCondition,
			condition.InitReason,
			novav1.NovaCellsDeletionConditionInitMessage,
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
		condition.UnknownCondition(
			condition.MemcachedReadyCondition,
			condition.InitReason,
			condition.MemcachedReadyInitMessage,
		),
		condition.UnknownCondition(
			novav1.NovaNotificationMQReadyCondition,
			condition.InitReason,
			novav1.NovaNotificationMQReadyInitMessage,
		),
	)
	instance.Status.Conditions.Init(&cl)
	return nil
}

func (r *NovaReconciler) getNovaManageJobSecretNames(
	cell *novav1.NovaCell,
) (configName string, scriptName string) {
	configName = fmt.Sprintf("%s-config-data", cell.Name+"-manage")
	scriptName = fmt.Sprintf("%s-scripts", cell.Name+"-manage")
	return
}

func (r *NovaReconciler) ensureNovaManageJobSecret(
	ctx context.Context, h *helper.Helper, instance *novav1.Nova,
	cell *novav1.NovaCell,
	ospSecret corev1.Secret,
	apiDBHostname string,
	cellTransportURL string,
	cellDB *mariadbv1.Database,
) (map[string]env.Setter, string, string, error) {
	configName, scriptName := r.getNovaManageJobSecretNames(cell)

	cmLabels := labels.GetLabels(
		instance, labels.GetGroupLabel(NovaLabelPrefix), map[string]string{},
	)

	var tlsCfg *tls.Service
	if instance.Spec.APIServiceTemplate.TLS.CaBundleSecretName != "" {
		tlsCfg = &tls.Service{}
	}

	extraData := map[string]string{
		"my.cnf": cellDB.GetDatabaseClientConfig(tlsCfg), //(mschuppert) for now just get the default my.cnf
	}

	extraTemplates := map[string]string{
		"01-nova.conf":    "/nova.conf",
		"nova-blank.conf": "/nova-blank.conf",
	}

	apiDatabaseAccount, apiDbSecret, err := mariadbv1.GetAccountAndSecret(ctx, h, instance.Spec.APIDatabaseAccount, instance.Namespace)
	if err != nil {
		return nil, "", "", err
	}

	cellDatabaseAccount, cellDbSecret, err := mariadbv1.GetAccountAndSecret(ctx, h, cell.Spec.CellDatabaseAccount, instance.Namespace)
	if err != nil {
		return nil, "", "", err
	}

	keystoneAPI, err := keystonev1.GetKeystoneAPI(ctx, h, instance.Namespace, map[string]string{})
	if err != nil {
		return nil, "", "", err
	}

	// We configure the Job like it runs in the env of the conductor of the given cell
	// but we ensure that the config always has [api_database] section configure
	// even if the cell has no API access at all.
	templateParameters := map[string]any{
		"service_name":           "nova-conductor",
		"keystone_internal_url":  cell.Spec.KeystoneAuthURL,
		"nova_keystone_user":     cell.Spec.ServiceUser,
		"nova_keystone_password": string(ospSecret.Data[instance.Spec.PasswordSelectors.Service]),
		// cell.Spec.APIDatabaseAccount is empty for cells without APIDB access
		"api_db_name":     NovaAPIDatabaseName,
		"api_db_user":     apiDatabaseAccount.Spec.UserName,
		"api_db_password": string(apiDbSecret.Data[mariadbv1.DatabasePasswordSelector]),
		// cell.Spec.APIDatabaseHostname is empty for cells without APIDB access
		"api_db_address":         apiDBHostname,
		"api_db_port":            3306,
		"cell_db_name":           getCellDatabaseName(cell.Spec.CellName),
		"cell_db_user":           cellDatabaseAccount.Spec.UserName,
		"cell_db_password":       string(cellDbSecret.Data[mariadbv1.DatabasePasswordSelector]),
		"cell_db_address":        cell.Spec.CellDatabaseHostname,
		"cell_db_port":           3306,
		"openstack_region_name":  keystoneAPI.GetRegion(),
		"default_project_domain": "Default", // fixme
		"default_user_domain":    "Default", // fixme
	}

	// NOTE(gibi): cell mapping for cell0 should not have transport_url
	// configured. As the nova-manage command used to create the mapping
	// uses the transport_url from the nova.conf provided to the job
	// we need to make sure that transport_url is only configured for the job
	// if it is mapping other than cell0.
	if cell.Spec.CellName != novav1.Cell0Name {
		templateParameters["transport_url"] = cellTransportURL
	}

	cms := []util.Template{
		{
			Name:         scriptName,
			Namespace:    instance.Namespace,
			Type:         util.TemplateTypeScripts,
			InstanceType: "nova-manage",
			Labels:       cmLabels,
		},
		{
			Name:               configName,
			Namespace:          instance.Namespace,
			Type:               util.TemplateTypeConfig,
			InstanceType:       "nova-manage",
			ConfigOptions:      templateParameters,
			Labels:             cmLabels,
			CustomData:         extraData,
			Annotations:        map[string]string{},
			AdditionalTemplate: extraTemplates,
		},
	}

	configHash := make(map[string]env.Setter)
	err = secret.EnsureSecrets(ctx, h, instance, cms, &configHash)

	return configHash, scriptName, configName, err
}

func (r *NovaReconciler) ensureDB(
	ctx context.Context,
	h *helper.Helper,
	db *mariadbv1.Database,
) (nova.DatabaseStatus, error) {
	ctrlResult, err := db.CreateOrPatchAll(ctx, h)
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

func (r *NovaReconciler) ensureAPIDB(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.Nova,
) (*mariadbv1.Database, nova.DatabaseStatus, error) {
	apiDB := mariadbv1.NewDatabaseForAccount(
		instance.Spec.APIDatabaseInstance, // mariadb/galera service to target
		NovaAPIDatabaseName,               // name used in CREATE DATABASE in mariadb
		"nova-api",                        // CR name for MariaDBDatabase
		instance.Spec.APIDatabaseAccount,  // CR name for MariaDBAccount
		instance.Namespace,                // namespace
	)

	result, err := r.ensureDB(ctx, h, apiDB)
	return apiDB, result, err
}

func (r *NovaReconciler) ensureCellDB(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.Nova,
	cellName string,
	cellTemplate novav1.NovaCellTemplate,
) (*mariadbv1.Database, nova.DatabaseStatus, error) {

	// ensure MariaDBAccount exists.  This account record may be created by
	// openstack-operator or the cloud operator up front without a specific
	// MariaDBDatabase configured yet.   Otherwise, a MariaDBAccount CR is
	// created here with a generated username as well as a secret with
	// generated password.   The MariaDBAccount is created without being
	// yet associated with any MariaDBDatabase.
	_, _, err := mariadbv1.EnsureMariaDBAccount(
		ctx, h, cellTemplate.CellDatabaseAccount,
		instance.Namespace, false, "nova_"+cellName,
	)

	if err != nil {
		return nil, nova.DBFailed, err
	}

	cellDB := mariadbv1.NewDatabaseForAccount(
		cellTemplate.CellDatabaseInstance, // mariadb/galera service to target
		"nova_"+cellName,                  // name used in CREATE DATABASE in mariadb
		"nova-"+cellName,                  // CR name for MariaDBDatabase
		cellTemplate.CellDatabaseAccount,  // CR name for MariaDBAccount
		instance.Namespace,                // namespace
	)

	result, err := r.ensureDB(ctx, h, cellDB)
	return cellDB, result, err
}

func (r *NovaReconciler) ensureCell(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.Nova,
	cellName string,
	cellTemplate novav1.NovaCellTemplate,
	cellDB *mariadbv1.Database,
	apiDB *mariadbv1.Database,
	cellTransportURL string,
	cellQuorumQueues bool,
	notificationTransportURL string,
	keystoneAuthURL string,
	region string,
	secret corev1.Secret,
) (*novav1.NovaCell, nova.CellDeploymentStatus, error) {
	Log := r.GetLogger(ctx)

	cellSecretName, err := r.ensureCellSecret(
		ctx, h, instance, cellName, cellTemplate,
		cellTransportURL, cellQuorumQueues, notificationTransportURL,
		secret)
	if err != nil {
		return nil, nova.CellDeploying, err
	}

	cellSpec := novav1.NovaCellSpec{
		CellName:                  cellName,
		Secret:                    cellSecretName,
		CellDatabaseHostname:      cellDB.GetDatabaseHostname(),
		CellDatabaseAccount:       cellTemplate.CellDatabaseAccount,
		ConductorServiceTemplate:  cellTemplate.ConductorServiceTemplate,
		MetadataServiceTemplate:   cellTemplate.MetadataServiceTemplate,
		NoVNCProxyServiceTemplate: cellTemplate.NoVNCProxyServiceTemplate,
		NovaComputeTemplates:      cellTemplate.NovaComputeTemplates,
		NodeSelector:              cellTemplate.NodeSelector,
		TopologyRef:               cellTemplate.TopologyRef,
		// TODO(gibi): this should be part of the secret
		ServiceUser:     instance.Spec.ServiceUser,
		KeystoneAuthURL: keystoneAuthURL,
		Region:          region,
		ServiceAccount:  instance.RbacResourceName(),
		APITimeout:      instance.Spec.APITimeout,
		// The assumption is that the CA bundle for ironic compute in the cell
		// and the conductor in the cell always the same as the NovaAPI
		TLS:               instance.Spec.APIServiceTemplate.TLS.Ca,
		PreserveJobs:      instance.Spec.PreserveJobs,
		MemcachedInstance: getMemcachedInstance(instance, cellTemplate),
		DBPurge:           cellTemplate.DBPurge,
		NovaCellImages:    instance.Spec.NovaCellImages,
	}
	if cellTemplate.HasAPIAccess {
		cellSpec.APIDatabaseHostname = apiDB.GetDatabaseHostname()
		cellSpec.APIDatabaseAccount = instance.Spec.APIDatabaseAccount
	}

	cell := &novav1.NovaCell{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getNovaCellCRName(instance.Name, cellSpec.CellName),
			Namespace: instance.Namespace,
		},
	}

	if cellSpec.NodeSelector == nil {
		cellSpec.NodeSelector = instance.Spec.NodeSelector
	}

	if cellSpec.TopologyRef == nil {
		cellSpec.TopologyRef = instance.Spec.TopologyRef
	}

	op, err := controllerutil.CreateOrPatch(ctx, r.Client, cell, func() error {
		// TODO(gibi): Pass down a narrowed secret that only hold
		// specific information but also holds user names
		cell.Spec = cellSpec

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
		Log.Info(fmt.Sprintf("NovaCell %s.", string(op)), "NovaCell.Name", cell.Name)
	}

	if !cell.IsReady() || cell.Generation != cell.Status.ObservedGeneration {
		// We wait for the cell to become Ready before we map it in the
		// nova_api DB.
		return cell, nova.CellDeploying, err
	}
	configHash, scriptName, configName, err := r.ensureNovaManageJobSecret(ctx, h, instance,
		cell, secret, apiDB.GetDatabaseHostname(), cellTransportURL, cellDB)
	if err != nil {
		return cell, nova.CellFailed, err
	}
	// When the cell is ready we need to create a row in the
	// nova_api.CellMapping table to make this cell accessible from the top
	// level services.
	status, err := r.ensureCellMapped(
		ctx, h, instance,
		cell, configHash, scriptName, configName)
	if err != nil {
		return cell, status, err
	}

	// We need to discover computes when cell have compute templates and mapping is done
	status, err = r.ensureNovaComputeDiscover(
		ctx, h, instance, cell, cellTemplate, scriptName, configName)

	if status == nova.CellComputeDiscoveryReady {
		status = nova.CellReady
	}

	if err != nil {
		return cell, status, err
	}

	// remove finalizers from unused MariaDBAccount records
	if status == nova.CellReady {
		err = mariadbv1.DeleteUnusedMariaDBAccountFinalizers(ctx, h, "nova-"+cellName, cellTemplate.CellDatabaseAccount, instance.Namespace)
		if err != nil {
			return cell, status, err
		}
	}

	return cell, status, err
}

func (r *NovaReconciler) ensureNovaComputeDiscover(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.Nova,
	cell *novav1.NovaCell,
	cellTemplate novav1.NovaCellTemplate,
	scriptName string,
	configName string,
) (nova.CellDeploymentStatus, error) {
	Log := r.GetLogger(ctx)
	if len(cellTemplate.NovaComputeTemplates) == 0 {
		return nova.CellComputeDiscoveryReady, nil
	}
	if !cell.Status.Conditions.IsTrue(novav1.NovaAllControlPlaneComputesReadyCondition) {
		return nova.CellComputeDiscovering, nil
	}

	labels := map[string]string{
		common.AppSelector: NovaLabelPrefix,
	}
	jobDef := nova.HostDiscoveryJob(cell, configName, scriptName, cell.Status.Hash[novav1.ComputeDiscoverHashKey], labels)

	job := job.NewJob(
		jobDef, cell.Name+"-host-discover",
		cell.Spec.PreserveJobs, r.RequeueTimeout, instance.Status.DiscoveredCells[cell.Name])

	result, err := job.DoJob(ctx, h)
	if err != nil {
		return nova.CellComputeDiscoveryFailed, err
	}

	if (result != ctrl.Result{}) {
		// Job is still running. We can simply return as we will be reconciled
		// when the Job status changes
		return nova.CellComputeDiscovering, nil
	}

	if !job.HasChanged() {
		// there was no need to run a new job as nothing changed
		return nova.CellComputeDiscoveryReady, nil
	}

	instance.Status.DiscoveredCells[cell.Name] = job.GetHash()
	Log.Info(fmt.Sprintf("Job %s hash added - %s", jobDef.Name, cell.Name))

	return nova.CellComputeDiscoveryReady, nil
}

func (r *NovaReconciler) ensureAPI(
	ctx context.Context,
	instance *novav1.Nova,
	cell0Template novav1.NovaCellTemplate,
	cell0DB *mariadbv1.Database,
	apiDB *mariadbv1.Database,
	keystoneInternalAuthURL string,
	keystonePublicAuthURL string,
	region string,
	secretName string,
) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)

	// TODO(gibi): Pass down a narrowed secret that only hold
	// specific information but also holds user names
	apiSpec := novav1.NovaAPISpec{
		Secret:                secretName,
		APIDatabaseHostname:   apiDB.GetDatabaseHostname(),
		APIDatabaseAccount:    instance.Spec.APIDatabaseAccount,
		Cell0DatabaseHostname: cell0DB.GetDatabaseHostname(),
		Cell0DatabaseAccount:  cell0Template.CellDatabaseAccount,
		NovaServiceBase: novav1.NovaServiceBase{
			ContainerImage:      instance.Spec.APIContainerImageURL,
			Replicas:            instance.Spec.APIServiceTemplate.Replicas,
			NodeSelector:        instance.Spec.APIServiceTemplate.NodeSelector,
			CustomServiceConfig: instance.Spec.APIServiceTemplate.CustomServiceConfig,
			Resources:           instance.Spec.APIServiceTemplate.Resources,
			NetworkAttachments:  instance.Spec.APIServiceTemplate.NetworkAttachments,
			TopologyRef:         instance.Spec.APIServiceTemplate.TopologyRef,
		},
		Override:               instance.Spec.APIServiceTemplate.Override,
		KeystoneAuthURL:        keystoneInternalAuthURL,
		KeystonePublicAuthURL:  keystonePublicAuthURL,
		Region:                 region,
		ServiceUser:            instance.Spec.ServiceUser,
		ServiceAccount:         instance.RbacResourceName(),
		RegisteredCells:        instance.Status.RegisteredCells,
		TLS:                    instance.Spec.APIServiceTemplate.TLS,
		DefaultConfigOverwrite: instance.Spec.APIServiceTemplate.DefaultConfigOverwrite,
		MemcachedInstance:      getMemcachedInstance(instance, cell0Template),
		APITimeout:             instance.Spec.APITimeout,
	}
	api := &novav1.NovaAPI{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + "-api",
			Namespace: instance.Namespace,
		},
	}

	if apiSpec.NodeSelector == nil {
		apiSpec.NodeSelector = instance.Spec.NodeSelector
	}

	if apiSpec.TopologyRef == nil {
		apiSpec.TopologyRef = instance.Spec.TopologyRef
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
		instance.Status.Conditions.Set(condition.FalseCondition(
			novav1.NovaAPIReadyCondition,
			condition.ErrorReason,
			condition.SeverityError,
			novav1.NovaAPIReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}

	if op != controllerutil.OperationResultNone {
		Log.Info(fmt.Sprintf("NovaAPI %s , NovaAPI.Name %s.", string(op), api.Name))
	}

	if api.Generation == api.Status.ObservedGeneration {
		c := api.Status.Conditions.Mirror(novav1.NovaAPIReadyCondition)
		// NOTE(gibi): it can be nil if the NovaAPI CR is created but no
		// reconciliation is run on it to initialize the ReadyCondition yet.
		if c != nil {
			instance.Status.Conditions.Set(c)
		}
		instance.Status.APIServiceReadyCount = api.Status.ReadyCount
	}

	return ctrl.Result{}, nil
}

func (r *NovaReconciler) ensureScheduler(
	ctx context.Context,
	instance *novav1.Nova,
	cell0Template novav1.NovaCellTemplate,
	cell0DB *mariadbv1.Database,
	apiDB *mariadbv1.Database,
	keystoneAuthURL string,
	region string,
	secretName string,
) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	// TODO(gibi): Pass down a narrowed secret that only hold
	// specific information but also holds user names
	schedulerSpec := novav1.NovaSchedulerSpec{
		Secret:                secretName,
		APIDatabaseHostname:   apiDB.GetDatabaseHostname(),
		APIDatabaseAccount:    instance.Spec.APIDatabaseAccount,
		Cell0DatabaseHostname: cell0DB.GetDatabaseHostname(),
		Cell0DatabaseAccount:  cell0Template.CellDatabaseAccount,
		// This is a coincidence that the NovaServiceBase
		// has exactly the same fields as the SchedulerServiceTemplate so we
		// can convert between them directly. As soon as these two structs
		// start to diverge we need to copy fields one by one here.
		NovaServiceBase: novav1.NovaServiceBase{
			ContainerImage:      instance.Spec.SchedulerContainerImageURL,
			Replicas:            instance.Spec.SchedulerServiceTemplate.Replicas,
			NodeSelector:        instance.Spec.SchedulerServiceTemplate.NodeSelector,
			CustomServiceConfig: instance.Spec.SchedulerServiceTemplate.CustomServiceConfig,
			Resources:           instance.Spec.SchedulerServiceTemplate.Resources,
			NetworkAttachments:  instance.Spec.SchedulerServiceTemplate.NetworkAttachments,
			TopologyRef:         instance.Spec.SchedulerServiceTemplate.TopologyRef,
		},
		KeystoneAuthURL: keystoneAuthURL,
		Region:          region,
		ServiceUser:     instance.Spec.ServiceUser,
		ServiceAccount:  instance.RbacResourceName(),
		RegisteredCells: instance.Status.RegisteredCells,
		// The assumption is that the CA bundle for the NovaScheduler is the same as the NovaAPI
		TLS:               instance.Spec.APIServiceTemplate.TLS.Ca,
		MemcachedInstance: getMemcachedInstance(instance, cell0Template),
	}
	scheduler := &novav1.NovaScheduler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + "-scheduler",
			Namespace: instance.Namespace,
		},
	}

	if schedulerSpec.NodeSelector == nil {
		schedulerSpec.NodeSelector = instance.Spec.NodeSelector
	}

	if schedulerSpec.TopologyRef == nil {
		schedulerSpec.TopologyRef = instance.Spec.TopologyRef
	}

	op, err := controllerutil.CreateOrPatch(ctx, r.Client, scheduler, func() error {
		scheduler.Spec = schedulerSpec
		err := controllerutil.SetControllerReference(instance, scheduler, r.Scheme)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			novav1.NovaSchedulerReadyCondition,
			condition.ErrorReason,
			condition.SeverityError,
			novav1.NovaSchedulerReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}

	if op != controllerutil.OperationResultNone {
		Log.Info(fmt.Sprintf("NovaScheduler %s. NovaScheduler.Name %s", string(op),
			scheduler.Name))
	}

	if scheduler.Generation == scheduler.Status.ObservedGeneration {
		c := scheduler.Status.Conditions.Mirror(novav1.NovaSchedulerReadyCondition)
		// NOTE(gibi): it can be nil if the NovaScheduler CR is created but no
		// reconciliation is run on it to initialize the ReadyCondition yet.
		if c != nil {
			instance.Status.Conditions.Set(c)
		}
		instance.Status.SchedulerServiceReadyCount = scheduler.Status.ReadyCount
	}

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
	Log := r.GetLogger(ctx)
	// initialize a nova dbs list with default db names and add used cells:
	novaDbs := [][]string{{"nova-api", instance.Spec.APIDatabaseAccount}}
	for cellName := range instance.Spec.CellTemplates {
		novaDbs = append(novaDbs, []string{novaapi.ServiceName + "-" + cellName, instance.Spec.CellTemplates[cellName].CellDatabaseAccount})
	}
	// iterate over novaDbs and remove finalizers
	for _, novaDb := range novaDbs {
		dbName, accountName := novaDb[0], novaDb[1]

		err := mariadbv1.DeleteDatabaseAndAccountFinalizers(ctx, h, dbName, accountName, instance.Namespace)
		if err != nil {
			return err
		}
	}

	Log.Info("Removed finalizer from MariaDBDatabase CRs", "MariaDBDatabase names", novaDbs)
	return nil
}

func (r *NovaReconciler) ensureKeystoneServiceUserDeletion(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.Nova,
) error {
	Log := r.GetLogger(ctx)
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
	Log.Info("Removed finalizer from nova KeystoneService")

	return nil
}

func (r *NovaReconciler) getKeystoneAuthURL(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.Nova,
) (string, string, string, error) {
	// TODO(gibi): change lib-common to take the name of the KeystoneAPI as
	// parameter instead of labels. Then use instance.Spec.KeystoneInstance as
	// the name.
	keystoneAPI, err := keystonev1.GetKeystoneAPI(ctx, h, instance.Namespace, map[string]string{})
	if err != nil {
		return "", "", "", err
	}
	// NOTE(gibi): we use the internal endpoint as that is expected to be
	// available on the external compute nodes as well and we want to keep
	// thing consistent
	internalAuthURL, err := keystoneAPI.GetEndpoint(endpoint.EndpointInternal)
	if err != nil {
		return "", "", "", err
	}
	// NOTE(gibi): but there is one case the www_authenticate_uri of nova-api
	// the we need to configure the public keystone endpoint
	publicAuthURL, err := keystoneAPI.GetEndpoint(endpoint.EndpointInternal)
	if err != nil {
		return "", "", "", err
	}

	region := keystoneAPI.GetRegion()

	return internalAuthURL, publicAuthURL, region, nil
}

func (r *NovaReconciler) reconcileDelete(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.Nova,
) error {
	Log := r.GetLogger(ctx)
	Log.Info("Reconciling delete")

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
		Log.Info("Removed finalizer from ourselves")
	}

	Log.Info("Reconciled delete successfully")
	return nil
}

func (r *NovaReconciler) ensureMQ(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.Nova,
	transportName string,
	messageBusInstanceName string,
) (string, bool, nova.MessageBusStatus, error) {
	Log := r.GetLogger(ctx)
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
		return "", false, nova.MQFailed, util.WrapErrorForObject(
			fmt.Sprintf("Error create or update TransportURL object %s", transportName),
			transportURL,
			err,
		)
	}

	if op != controllerutil.OperationResultNone {
		Log.Info(fmt.Sprintf("TransportURL object %s created or patched", transportName))
		return "", false, nova.MQCreating, nil
	}

	err = r.Client.Get(ctx, types.NamespacedName{Namespace: instance.Namespace, Name: transportName}, transportURL)
	if err != nil && !k8s_errors.IsNotFound(err) {
		return "", false, nova.MQFailed, util.WrapErrorForObject(
			fmt.Sprintf("Error reading TransportURL object %s", transportName),
			transportURL,
			err,
		)
	}

	if k8s_errors.IsNotFound(err) || !transportURL.IsReady() || transportURL.Status.SecretName == "" {
		return "", false, nova.MQCreating, nil
	}

	secretName := types.NamespacedName{Namespace: instance.Namespace, Name: transportURL.Status.SecretName}

	secret := &corev1.Secret{}
	err = h.GetClient().Get(ctx, secretName, secret)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			return "", false, nova.MQCreating, nil
		}
		return "", false, nova.MQFailed, err
	}

	url, ok := secret.Data[TransportURLSelector]
	if !ok {
		return "", false, nova.MQFailed, fmt.Errorf(
			"%w: the TransportURL secret %s does not have 'transport_url' field", util.ErrFieldNotFound, transportURL.Status.SecretName)
	}

	// Check if quorum queues are enabled
	quorumQueues := false
	if val, ok := secret.Data[QuorumQueuesSelector]; ok {
		quorumQueues = string(val) == "true"
	}

	return string(url), quorumQueues, nova.MQCompleted, nil
}

func (r *NovaReconciler) ensureMQDeleted(
	ctx context.Context,
	instance *novav1.Nova,
	transportURLName string,
) error {
	Log := r.GetLogger(ctx)
	transportURL := &rabbitmqv1.TransportURL{
		ObjectMeta: metav1.ObjectMeta{
			Name:      transportURLName,
			Namespace: instance.Namespace,
		},
	}

	err := r.Client.Delete(ctx, transportURL)
	if err != nil {
		Log.Info(fmt.Sprintf("Could not delete TransportURL %s err: %s", transportURLName, err))
		return err
	}

	Log.Info("Deleted transportURL", ":", transportURLName)

	return nil
}

func getNovaMetadataName(instance client.Object) types.NamespacedName {
	return types.NamespacedName{Namespace: instance.GetNamespace(), Name: instance.GetName() + "-metadata"}
}

func (r *NovaReconciler) ensureMetadata(
	ctx context.Context,
	instance *novav1.Nova,
	cell0Template novav1.NovaCellTemplate,
	cell0DB *mariadbv1.Database,
	apiDB *mariadbv1.Database,
	keystoneAuthURL string,
	region string,
	secretName string,
) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	// There is a case when the user manually created a NovaMetadata while it
	// was disabled in the Nova and then tries to enable it in Nova.
	// One can think that this means we adopts the NovaMetadata and
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

	// TODO(gibi): Pass down a narrowed secret that only hold
	// specific information but also holds user names
	metadataSpec := novav1.NovaMetadataSpec{
		Secret:               secretName,
		APIDatabaseHostname:  apiDB.GetDatabaseHostname(),
		APIDatabaseAccount:   instance.Spec.APIDatabaseAccount,
		CellDatabaseHostname: cell0DB.GetDatabaseHostname(),
		CellDatabaseAccount:  cell0Template.CellDatabaseAccount,
		NovaServiceBase: novav1.NovaServiceBase{
			ContainerImage:      instance.Spec.MetadataContainerImageURL,
			Replicas:            instance.Spec.MetadataServiceTemplate.Replicas,
			NodeSelector:        instance.Spec.MetadataServiceTemplate.NodeSelector,
			CustomServiceConfig: instance.Spec.MetadataServiceTemplate.CustomServiceConfig,
			Resources:           instance.Spec.MetadataServiceTemplate.Resources,
			NetworkAttachments:  instance.Spec.MetadataServiceTemplate.NetworkAttachments,
			TopologyRef:         instance.Spec.MetadataServiceTemplate.TopologyRef,
		},
		Override:               instance.Spec.MetadataServiceTemplate.Override,
		ServiceUser:            instance.Spec.ServiceUser,
		KeystoneAuthURL:        keystoneAuthURL,
		Region:                 region,
		ServiceAccount:         instance.RbacResourceName(),
		RegisteredCells:        instance.Status.RegisteredCells,
		TLS:                    instance.Spec.MetadataServiceTemplate.TLS,
		DefaultConfigOverwrite: instance.Spec.MetadataServiceTemplate.DefaultConfigOverwrite,
		MemcachedInstance:      getMemcachedInstance(instance, cell0Template),
		APITimeout:             instance.Spec.APITimeout,
	}
	metadata = &novav1.NovaMetadata{
		ObjectMeta: metav1.ObjectMeta{
			Name:      metadataName.Name,
			Namespace: instance.Namespace,
		},
	}

	if metadataSpec.NodeSelector == nil {
		metadataSpec.NodeSelector = instance.Spec.NodeSelector
	}

	if metadataSpec.TopologyRef == nil {
		metadataSpec.TopologyRef = instance.Spec.TopologyRef
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
		Log.Info(fmt.Sprintf("NovaMetadata %s.", string(op)), "NovaMetadata.Name", metadata.Name)
	}

	if metadata.Generation == metadata.Status.ObservedGeneration {
		c := metadata.Status.Conditions.Mirror(novav1.NovaMetadataReadyCondition)
		// NOTE(gibi): it can be nil if the NovaMetadata CR is created but no
		// reconciliation is run on it to initialize the ReadyCondition yet.
		if c != nil {
			instance.Status.Conditions.Set(c)
		}
		instance.Status.MetadataServiceReadyCount = metadata.Status.ReadyCount
	}
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
	configHash map[string]env.Setter,
	scriptName string,
	configName string,
) (nova.CellDeploymentStatus, error) {
	Log := r.GetLogger(ctx)
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
	jobDef := nova.CellMappingJob(instance, cell, configName, scriptName, inputHash, labels)

	job := job.NewJob(
		jobDef, cell.Name+"-cell-mapping",
		instance.Spec.PreserveJobs, r.RequeueTimeout,
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
	Log.Info("Job hash added ", "job", jobDef.Name, "hash", instance.Status.RegisteredCells[cell.Name])

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
	cellTransportURL string,
	cellQuorumQueues bool,
	notificationTransportURL string,
	externalSecret corev1.Secret,
) (string, error) {
	// NOTE(gibi): We can move other sensitive data to the internal Secret from
	// the NovaCellSpec fields, possibly hostnames or usernames.
	quorumQueuesValue := "false"
	if cellQuorumQueues {
		quorumQueuesValue = "true"
	}

	data := map[string]string{
		ServicePasswordSelector:          string(externalSecret.Data[instance.Spec.PasswordSelectors.Service]),
		TransportURLSelector:             cellTransportURL,
		NotificationTransportURLSelector: notificationTransportURL,
		QuorumQueuesTemplateKey:          quorumQueuesValue,
	}

	// If metadata is enabled in the cell then the cell secret needs the
	// metadata shared secret
	if *cellTemplate.MetadataServiceTemplate.Enabled {
		val, ok := externalSecret.Data[instance.Spec.PasswordSelectors.PrefixMetadataCellsSecret+cellName]
		if ok {
			data[MetadataSecretSelector] = string(val)
		} else {
			data[MetadataSecretSelector] = string(externalSecret.Data[instance.Spec.PasswordSelectors.MetadataSecret])
		}
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

// ensureTopLevelSecret makes sure that the internal Cell Secret exists and up to
// date
func (r *NovaReconciler) ensureTopLevelSecret(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.Nova,
	apiTransportURL string,
	apiQuorumQueues bool,
	notificationTransportURL string,
	externalSecret corev1.Secret,
) (string, error) {
	// NOTE(gibi): We can move other sensitive data to the internal Secret from
	// the subCR fields, possibly hostnames or usernames.
	quorumQueuesValue := "false"
	if apiQuorumQueues {
		quorumQueuesValue = "true"
	}

	data := map[string]string{
		ServicePasswordSelector:          string(externalSecret.Data[instance.Spec.PasswordSelectors.Service]),
		MetadataSecretSelector:           string(externalSecret.Data[instance.Spec.PasswordSelectors.MetadataSecret]),
		TransportURLSelector:             apiTransportURL,
		NotificationTransportURLSelector: notificationTransportURL,
		QuorumQueuesTemplateKey:          quorumQueuesValue,
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

func (r *NovaReconciler) findObjectsForSrc(ctx context.Context, src client.Object) []reconcile.Request {
	requests := []reconcile.Request{}

	Log := r.GetLogger(ctx)

	for _, field := range novaWatchFields {
		crList := &novav1.NovaList{}
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

func (r *NovaReconciler) findObjectForSrc(ctx context.Context, src client.Object) []reconcile.Request {
	requests := []reconcile.Request{}

	Log := r.GetLogger(ctx)

	crList := &novav1.NovaList{}
	listOps := &client.ListOptions{
		Namespace: src.GetNamespace(),
	}
	err := r.Client.List(ctx, crList, listOps)
	if err != nil {
		Log.Error(err, fmt.Sprintf("listing %s for namespace: %s", crList.GroupVersionKind().Kind, src.GetNamespace()))
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

	return requests
}

func (r *NovaReconciler) memcachedNamespaceMapFunc(ctx context.Context, src client.Object) []reconcile.Request {

	result := []reconcile.Request{}

	// get all Nova CRs
	novaList := &novav1.NovaList{}
	listOpts := []client.ListOption{
		client.InNamespace(src.GetNamespace()),
	}
	if err := r.Client.List(ctx, novaList, listOpts...); err != nil {
		return nil
	}

	for _, cr := range novaList.Items {
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

// fields to index to reconcile when change
var (
	novaWatchFields = []string{
		passwordSecretField,
	}
)

// SetupWithManager sets up the controller with the Manager.
func (r *NovaReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// index passwordSecretField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &novav1.Nova{}, passwordSecretField, func(rawObj client.Object) []string {
		// Extract the secret name from the spec, if one is provided
		cr := rawObj.(*novav1.Nova)
		if cr.Spec.Secret == "" {
			return nil
		}
		return []string{cr.Spec.Secret}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&novav1.Nova{}).
		Owns(&mariadbv1.MariaDBDatabase{}).
		Owns(&mariadbv1.MariaDBAccount{}).
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
		Watches(&keystonev1.KeystoneAPI{},
			handler.EnqueueRequestsFromMapFunc(r.findObjectForSrc),
			builder.WithPredicates(keystonev1.KeystoneAPIStatusChangedPredicate)).
		Complete(r)
}
