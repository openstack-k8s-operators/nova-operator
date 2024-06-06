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

	v1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
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

	"github.com/go-logr/logr"
	memcachedv1 "github.com/openstack-k8s-operators/infra-operator/apis/memcached/v1beta1"
	common "github.com/openstack-k8s-operators/lib-common/modules/common"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/cronjob"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	helper "github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	job "github.com/openstack-k8s-operators/lib-common/modules/common/job"
	"github.com/openstack-k8s-operators/lib-common/modules/common/labels"
	nad "github.com/openstack-k8s-operators/lib-common/modules/common/networkattachment"
	"github.com/openstack-k8s-operators/lib-common/modules/common/statefulset"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"
	util "github.com/openstack-k8s-operators/lib-common/modules/common/util"
	mariadbv1 "github.com/openstack-k8s-operators/mariadb-operator/api/v1beta1"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/nova-operator/pkg/novaconductor"
)

// NovaConductorReconciler reconciles a NovaConductor object
type NovaConductorReconciler struct {
	ReconcilerBase
}

// GetLogger returns a logger object with a prefix of "controller.name" and additional controller context fields
func (r *NovaConductorReconciler) GetLogger(ctx context.Context) logr.Logger {
	return log.FromContext(ctx).WithName("Controllers").WithName("NovaConductor")
}

//+kubebuilder:rbac:groups=nova.openstack.org,resources=novaconductors,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=nova.openstack.org,resources=novaconductors/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=nova.openstack.org,resources=novaconductors/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete;
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete;
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete;
//+kubebuilder:rbac:groups=k8s.cni.cncf.io,resources=network-attachment-definitions,verbs=get;list;watch
//+kubebuilder:rbac:groups=memcached.openstack.org,resources=memcacheds,verbs=get;list;watch;update;
//+kubebuilder:rbac:groups=memcached.openstack.org,resources=memcacheds/finalizers,verbs=update
// +kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=get;list;watch;create;update;patch;delete;

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NovaConductor object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.2/pkg/reconcile
func (r *NovaConductorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, _err error) {
	Log := r.GetLogger(ctx)

	// Fetch our instance that needs to be reconciled
	instance := &novav1.NovaConductor{}
	err := r.Client.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected.
			// For additional cleanup logic use finalizers. Return and don't requeue.
			Log.Info("NovaConductor instance not found, probably deleted before reconciled. Nothing to do.")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		Log.Error(err, "Failed to read the NovaConductor instance.")
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

	// Always update the instance status when exiting this function so we can
	// persist any changes happened during the current reconciliation.
	defer func() {
		condition.RestoreLastTransitionTimes(&instance.Status.Conditions, savedConditions)
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

	// We are adding Nova finalizer to Memcached CR.
	// So we need a finalizer on the ourselves too so that
	// during CR delete we can have a chance to remove the finalizer from
	// the our Memcached so that is also deleted.
	updated := controllerutil.AddFinalizer(instance, h.GetFinalizer())
	if updated {
		Log.Info("Added finalizer to ourselves")
		// we intentionally return immediately to force the deferred function
		// to persist the Instance with the finalizer. We need to have our own
		// finalizer persisted before we try to create the Memcached with
		// our finalizer to avoid orphaning the Memcached.
		return ctrl.Result{}, nil
	}

	// TODO(gibi): Can we use a simple map[string][string] for hashes?
	// Collect hashes of all the input we depend on so that we can easily
	// detect if something is changed.
	hashes := make(map[string]env.Setter)

	requiredSecretFields := []string{
		ServicePasswordSelector,
		TransportURLSelector,
	}

	secretHash, result, secret, err := ensureSecret(
		ctx,
		types.NamespacedName{Namespace: instance.Namespace, Name: instance.Spec.Secret},
		requiredSecretFields,
		h.GetClient(),
		&instance.Status.Conditions,
		r.RequeueTimeout,
	)
	if err != nil {
		return result, err
	}

	hashes[instance.Spec.Secret] = env.SetValue(secretHash)

	// all our input checks out so report InputReady
	instance.Status.Conditions.MarkTrue(condition.InputReadyCondition, condition.InputReadyMessage)

	//
	// TLS input validation
	//
	// Validate the CA cert secret if provided
	if instance.Spec.TLS.CaBundleSecretName != "" {
		hash, ctrlResult, err := tls.ValidateCACertSecret(
			ctx,
			h.GetClient(),
			types.NamespacedName{
				Name:      instance.Spec.TLS.CaBundleSecretName,
				Namespace: instance.Namespace,
			},
		)
		if err != nil {
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.TLSInputReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.TLSInputErrorMessage,
				err.Error()))
			return ctrlResult, err
		} else if (ctrlResult != ctrl.Result{}) {
			return ctrlResult, nil
		}

		if hash != "" {
			hashes[tls.CABundleKey] = env.SetValue(hash)
		}
	}

	// all cert input checks out so report InputReady
	instance.Status.Conditions.MarkTrue(condition.TLSInputReadyCondition, condition.InputReadyMessage)

	memcached, err := ensureMemcached(ctx, h, instance.Namespace, instance.Spec.MemcachedInstance, &instance.Status.Conditions)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Add finalizer to Memcached to prevent it from being deleted now that we're using it
	if controllerutil.AddFinalizer(memcached, h.GetFinalizer()) {
		err := h.GetClient().Update(ctx, memcached)
		if err != nil {
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.MemcachedReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.MemcachedReadyErrorMessage,
				err.Error()))
			return ctrl.Result{}, err
		}
	}

	err = r.ensureConfigs(ctx, h, instance, &hashes, secret, memcached)
	if err != nil {
		return ctrl.Result{}, err
	}

	// create hash over all the different input resources to identify if any of
	// those changed and a restart/recreate is required.
	inputHash, err := util.HashOfInputHashes(hashes)
	if err != nil {
		return ctrl.Result{}, err
	}
	instance.Status.Hash[common.InputHashName] = inputHash

	instance.Status.Conditions.MarkTrue(condition.ServiceConfigReadyCondition, condition.ServiceConfigReadyMessage)

	serviceAnnotations, result, err := ensureNetworkAttachments(ctx, h, instance.Spec.NetworkAttachments, &instance.Status.Conditions, r.RequeueTimeout)
	if (err != nil || result != ctrl.Result{}) {
		return result, err
	}

	result, err = r.ensureCellDBSynced(ctx, h, instance, serviceAnnotations)
	if (err != nil || result != ctrl.Result{}) {
		return result, err
	}

	result, err = r.ensureDeployment(ctx, h, instance, inputHash, serviceAnnotations)
	if (err != nil || result != ctrl.Result{}) {
		return result, err
	}

	err = r.ensureDBPurgeCronJob(ctx, h, instance, serviceAnnotations)
	if err != nil {
		return ctrl.Result{}, err
	}

	if *instance.Spec.Replicas != instance.Status.ReadyCount {
		Log.Info("Waiting for the ReadyCount to become equal to Replicas before doing service cleanup in nova database.")
		return ctrl.Result{}, nil
	}
	// clean up nova services from nova db should be always a last step in reconcile
	err = r.cleanServiceFromNovaDb(ctx, h, instance, secret, Log)
	if err != nil {
		Log.Error(err, "Failed cleaning services from nova db")
	}

	Log.Info("Successfully reconciled")
	return ctrl.Result{}, nil
}

func (r *NovaConductorReconciler) initStatus(
	instance *novav1.NovaConductor,
) error {
	if err := r.initConditions(instance); err != nil {
		return err
	}

	// NOTE(gibi): initialize the rest of the status fields here
	// so that the reconcile loop later can assume they are not nil.
	if instance.Status.Hash == nil {
		instance.Status.Hash = map[string]string{}
	}
	if instance.Status.NetworkAttachments == nil {
		instance.Status.NetworkAttachments = map[string][]string{}
	}

	return nil
}

func (r *NovaConductorReconciler) initConditions(
	instance *novav1.NovaConductor,
) error {
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
			condition.ServiceConfigReadyCondition,
			condition.InitReason,
			condition.ServiceConfigReadyInitMessage,
		),
		condition.UnknownCondition(
			condition.DBSyncReadyCondition,
			condition.InitReason,
			condition.DBSyncReadyInitMessage,
		),
		condition.UnknownCondition(
			condition.DeploymentReadyCondition,
			condition.InitReason,
			condition.DeploymentReadyInitMessage,
		),
		condition.UnknownCondition(
			condition.NetworkAttachmentsReadyCondition,
			condition.InitReason,
			condition.NetworkAttachmentsReadyInitMessage,
		),
		condition.UnknownCondition(
			condition.TLSInputReadyCondition,
			condition.InitReason,
			condition.InputReadyInitMessage,
		),
		condition.UnknownCondition(
			condition.MemcachedReadyCondition,
			condition.InitReason,
			condition.MemcachedReadyInitMessage,
		),
		condition.UnknownCondition(
			condition.CronJobReadyCondition,
			condition.InitReason,
			condition.CronJobReadyInitMessage,
		),
	)

	instance.Status.Conditions.Init(&cl)
	return nil
}

func (r *NovaConductorReconciler) ensureConfigs(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.NovaConductor,
	hashes *map[string]env.Setter,
	secret corev1.Secret,
	memcachedInstance *memcachedv1.Memcached,
) error {
	err := r.generateConfigs(ctx, h, instance, hashes, secret, memcachedInstance)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ServiceConfigReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.ServiceConfigReadyErrorMessage,
			err.Error()))
		return err
	}
	return nil
}

func (r *NovaConductorReconciler) generateConfigs(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.NovaConductor,
	hashes *map[string]env.Setter,
	secret corev1.Secret,
	memcachedInstance *memcachedv1.Memcached,
) error {

	cellDB, err := mariadbv1.GetDatabaseByNameAndAccount(ctx, h, "nova-"+instance.Spec.CellName, instance.Spec.CellDatabaseAccount, instance.Namespace)
	if err != nil {
		return err
	}
	cellDatabaseAccount := cellDB.GetAccount()
	cellDbSecret := cellDB.GetSecret()

	templateParameters := map[string]interface{}{
		"service_name":             "nova-conductor",
		"keystone_internal_url":    instance.Spec.KeystoneAuthURL,
		"nova_keystone_user":       instance.Spec.ServiceUser,
		"nova_keystone_password":   string(secret.Data[ServicePasswordSelector]),
		"cell_db_name":             getCellDatabaseName(instance.Spec.CellName),
		"cell_db_user":             cellDatabaseAccount.Spec.UserName,
		"cell_db_password":         string(cellDbSecret.Data[mariadbv1.DatabasePasswordSelector]),
		"cell_db_address":          instance.Spec.CellDatabaseHostname,
		"cell_db_port":             3306,
		"openstack_region_name":    "regionOne", // fixme
		"default_project_domain":   "Default",   // fixme
		"default_user_domain":      "Default",   // fixme
		"transport_url":            string(secret.Data[TransportURLSelector]),
		"MemcachedServers":         memcachedInstance.GetMemcachedServerListString(),
		"MemcachedServersWithInet": memcachedInstance.GetMemcachedServerListWithInetString(),
		"MemcachedTLS":             memcachedInstance.GetMemcachedTLSSupport(),
	}
	if len(instance.Spec.APIDatabaseHostname) > 0 {
		apiDatabaseAccount, apiDbSecret, err := mariadbv1.GetAccountAndSecret(ctx, h, instance.Spec.APIDatabaseAccount, instance.Namespace)
		if err != nil {
			return err
		}

		templateParameters["api_db_name"] = NovaAPIDatabaseName
		templateParameters["api_db_user"] = apiDatabaseAccount.Spec.UserName
		templateParameters["api_db_password"] = string(apiDbSecret.Data[mariadbv1.DatabasePasswordSelector])
		templateParameters["api_db_address"] = instance.Spec.APIDatabaseHostname
		templateParameters["api_db_port"] = 3306
	}

	var tlsCfg *tls.Service
	if instance.Spec.TLS.CaBundleSecretName != "" {
		tlsCfg = &tls.Service{}
	}
	extraData := map[string]string{
		"my.cnf": cellDB.GetDatabaseClientConfig(tlsCfg), //(mschuppert) for now just get the default my.cnf
	}
	if instance.Spec.CustomServiceConfig != "" {
		extraData["02-nova-override.conf"] = instance.Spec.CustomServiceConfig
	}

	cmLabels := labels.GetLabels(
		instance, labels.GetGroupLabel(NovaConductorLabelPrefix), map[string]string{})

	return r.GenerateConfigsWithScripts(
		ctx, h, instance, hashes, templateParameters, extraData, cmLabels, map[string]string{},
	)
}

func (r *NovaConductorReconciler) ensureCellDBSynced(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.NovaConductor,
	annotations map[string]string,
) (ctrl.Result, error) {
	serviceLabels := map[string]string{
		common.AppSelector: NovaConductorLabelPrefix,
	}
	Log := r.GetLogger(ctx)
	dbSyncHash := instance.Status.Hash[DbSyncHash]
	jobDef := novaconductor.CellDBSyncJob(instance, serviceLabels, annotations)
	dbSyncJob := job.NewJob(jobDef, "dbsync", instance.Spec.PreserveJobs, r.RequeueTimeout, dbSyncHash)
	ctrlResult, err := dbSyncJob.DoJob(ctx, h)
	if (ctrlResult != ctrl.Result{}) {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBSyncReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DBSyncReadyRunningMessage))
		return ctrlResult, nil
	}
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBSyncReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.DBSyncReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}
	if dbSyncJob.HasChanged() {
		instance.Status.Hash[DbSyncHash] = dbSyncJob.GetHash()
		Log.Info(fmt.Sprintf("Job %s ash added %s", jobDef.Name, instance.Status.Hash[DbSyncHash]))
	}
	instance.Status.Conditions.MarkTrue(condition.DBSyncReadyCondition, condition.DBSyncReadyMessage)

	return ctrl.Result{}, nil
}

func (r *NovaConductorReconciler) ensureDeployment(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.NovaConductor,
	inputHash string,
	annotations map[string]string,
) (ctrl.Result, error) {
	serviceLabels := map[string]string{
		common.AppSelector: NovaConductorLabelPrefix,
		CellSelector:       instance.Spec.CellName,
	}
	Log := r.GetLogger(ctx)
	ss := statefulset.NewStatefulSet(novaconductor.StatefulSet(instance, inputHash, serviceLabels, annotations), r.RequeueTimeout)
	ctrlResult, err := ss.CreateOrPatch(ctx, h)
	if err != nil && !k8s_errors.IsNotFound(err) {
		Log.Error(err, "Deployment failed")
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DeploymentReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.DeploymentReadyErrorMessage,
			err.Error()))
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{} || k8s_errors.IsNotFound(err)) {
		Log.Info("Deployment in progress")
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DeploymentReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DeploymentReadyRunningMessage))
		// It is OK to return success as we are watching for StatefulSet changes
		return ctrlResult, nil
	}

	instance.Status.ReadyCount = ss.GetStatefulSet().Status.ReadyReplicas

	// verify if network attachment matches expectations
	networkReady, networkAttachmentStatus, err := nad.VerifyNetworkStatusFromAnnotation(
		ctx,
		h,
		instance.Spec.NetworkAttachments,
		serviceLabels,
		instance.Status.ReadyCount)
	if err != nil {
		return ctrl.Result{}, err
	}

	instance.Status.NetworkAttachments = networkAttachmentStatus
	if networkReady {
		instance.Status.Conditions.MarkTrue(condition.NetworkAttachmentsReadyCondition, condition.NetworkAttachmentsReadyMessage)
	} else {
		err := fmt.Errorf("not all pods have interfaces with ips as configured in NetworkAttachments: %s", instance.Spec.NetworkAttachments)
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.NetworkAttachmentsReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.NetworkAttachmentsReadyErrorMessage,
			err.Error()))

		return ctrl.Result{}, err
	}

	if instance.Status.ReadyCount > 0 {
		Log.Info("Deployment is ready")
		instance.Status.Conditions.MarkTrue(condition.DeploymentReadyCondition, condition.DeploymentReadyMessage)
	} else if *instance.Spec.Replicas == 0 {
		Log.Info("Deployment with 0 replicas is ready")
		instance.Status.Conditions.MarkTrue(condition.DeploymentReadyCondition, condition.DeploymentReadyMessage)
	} else {
		Log.Info("Deployment is not ready", "Status", ss.GetStatefulSet().Status)
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DeploymentReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DeploymentReadyRunningMessage))
		// It is OK to return success as we are watching for StatefulSet changes
		return ctrl.Result{}, nil
	}
	return ctrl.Result{}, nil
}

func (r *NovaConductorReconciler) ensureDBPurgeCronJob(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.NovaConductor,
	annotations map[string]string,
) error {
	serviceLabels := map[string]string{
		common.AppSelector: NovaConductorLabelPrefix,
	}
	cronDef := novaconductor.DBPurgeCronJob(instance, serviceLabels, annotations)
	cronjob := cronjob.NewCronJob(cronDef, r.RequeueTimeout)

	_, err := cronjob.CreateOrPatch(ctx, h)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.CronJobReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.CronJobReadyErrorMessage,
			err.Error()))
		return err
	}

	instance.Status.Conditions.MarkTrue(
		condition.CronJobReadyCondition, condition.CronJobReadyMessage)
	return nil
}

func (r *NovaConductorReconciler) cleanServiceFromNovaDb(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.NovaConductor,
	secret corev1.Secret,
	l logr.Logger,
) error {
	authPassword := string(secret.Data[ServicePasswordSelector])
	computeClient, err := getNovaClient(ctx, h, instance, authPassword, l)
	if err != nil {
		return err
	}
	replicaCount := instance.Spec.Replicas
	cellName := instance.Spec.CellName

	return cleanNovaServiceFromNovaDb(computeClient, "nova-conductor", l, *replicaCount, cellName)
}

func (r *NovaConductorReconciler) findObjectsForSrc(ctx context.Context, src client.Object) []reconcile.Request {
	requests := []reconcile.Request{}

	l := log.FromContext(ctx).WithName("Controllers").WithName("NovaConductor")

	for _, field := range cdWatchFields {
		crList := &novav1.NovaConductorList{}
		listOps := &client.ListOptions{
			FieldSelector: fields.OneTermEqualSelector(field, src.GetName()),
			Namespace:     src.GetNamespace(),
		}
		err := r.Client.List(ctx, crList, listOps)
		if err != nil {
			return []reconcile.Request{}
		}

		for _, item := range crList.Items {
			l.Info(fmt.Sprintf("input source %s changed, reconcile: %s - %s", src.GetName(), item.GetName(), item.GetNamespace()))

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

// fields to index to reconcile when change
var (
	cdWatchFields = []string{
		passwordSecretField,
		caBundleSecretNameField,
	}
)

func (r *NovaConductorReconciler) memcachedNamespaceMapFunc(ctx context.Context, src client.Object) []reconcile.Request {

	result := []reconcile.Request{}

	// get all Nova CRs
	novaConductorList := &novav1.NovaConductorList{}
	listOpts := []client.ListOption{
		client.InNamespace(src.GetNamespace()),
	}
	if err := r.Client.List(ctx, novaConductorList, listOpts...); err != nil {
		return nil
	}

	for _, cr := range novaConductorList.Items {
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

func (r *NovaConductorReconciler) reconcileDelete(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.NovaConductor,
) error {
	Log := r.GetLogger(ctx)

	Log.Info("Reconciling delete")

	// Remove our finalizer from Memcached

	memcached, _ := memcachedv1.GetMemcachedByName(ctx, h, instance.Spec.MemcachedInstance, instance.Namespace)
	if memcached != nil {
		if controllerutil.RemoveFinalizer(memcached, h.GetFinalizer()) {
			err := h.GetClient().Update(ctx, memcached)
			if err != nil {
				return err
			}
		}
	}

	// Successfully cleaned up everything. So as the final step let's remove the
	// finalizer from ourselves to allow the deletion of NovaConductor CR itself
	updated := controllerutil.RemoveFinalizer(instance, h.GetFinalizer())
	if updated {
		Log.Info("Removed finalizer from ourselves")
	}

	Log.Info("Reconciled delete successfully")
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NovaConductorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// index passwordSecretField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &novav1.NovaConductor{}, passwordSecretField, func(rawObj client.Object) []string {
		// Extract the secret name from the spec, if one is provided
		cr := rawObj.(*novav1.NovaConductor)
		if cr.Spec.Secret == "" {
			return nil
		}
		return []string{cr.Spec.Secret}
	}); err != nil {
		return err
	}

	// index caBundleSecretNameField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &novav1.NovaConductor{}, caBundleSecretNameField, func(rawObj client.Object) []string {
		// Extract the secret name from the spec, if one is provided
		cr := rawObj.(*novav1.NovaConductor)
		if cr.Spec.TLS.CaBundleSecretName == "" {
			return nil
		}
		return []string{cr.Spec.TLS.CaBundleSecretName}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&novav1.NovaConductor{}).
		Owns(&v1.StatefulSet{}).
		Owns(&batchv1.Job{}).
		Owns(&batchv1.CronJob{}).
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
		Complete(r)
}
