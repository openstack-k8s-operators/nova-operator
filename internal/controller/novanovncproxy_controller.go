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
	"fmt"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
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
	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	common "github.com/openstack-k8s-operators/lib-common/modules/common"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/endpoint"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	helper "github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"github.com/openstack-k8s-operators/lib-common/modules/common/labels"
	nad "github.com/openstack-k8s-operators/lib-common/modules/common/networkattachment"
	"github.com/openstack-k8s-operators/lib-common/modules/common/service"
	"github.com/openstack-k8s-operators/lib-common/modules/common/statefulset"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"
	util "github.com/openstack-k8s-operators/lib-common/modules/common/util"
	mariadbv1 "github.com/openstack-k8s-operators/mariadb-operator/api/v1beta1"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/nova-operator/internal/nova"
	"github.com/openstack-k8s-operators/nova-operator/internal/novncproxy"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
)

// NovaNoVNCProxyReconciler reconciles a NovaNoVNCProxy object
type NovaNoVNCProxyReconciler struct {
	ReconcilerBase
}

// GetLogger returns a logger object with a prefix of "controller.name" and additional controller context fields
func (r *NovaNoVNCProxyReconciler) GetLogger(ctx context.Context) logr.Logger {
	return log.FromContext(ctx).WithName("Controllers").WithName("NovaNoVNCProxy")
}

//+kubebuilder:rbac:groups=nova.openstack.org,resources=novanovncproxies,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=nova.openstack.org,resources=novanovncproxies/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=nova.openstack.org,resources=novanovncproxies/finalizers,verbs=update;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneendpoints,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=k8s.cni.cncf.io,resources=network-attachment-definitions,verbs=get;list;watch
// +kubebuilder:rbac:groups=memcached.openstack.org,resources=memcacheds,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=memcached.openstack.org,resources=memcacheds/finalizers,verbs=update;patch
// +kubebuilder:rbac:groups=topology.openstack.org,resources=topologies,verbs=get;list;watch;update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NovaNoVNCProxy object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.2/pkg/reconcile
func (r *NovaNoVNCProxyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, _err error) {
	Log := r.GetLogger(ctx)

	// Fetch the NovaNoVNCProxy instance that needs to be reconciled
	instance := &novav1.NovaNoVNCProxy{}
	err := r.Client.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected.
			// For additional cleanup logic use finalizers. Return and don't requeue.
			Log.Info("NovaNoVNCProxy instance not found, probably deleted before reconciled. Nothing to do.")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		Log.Error(err, "Failed to read the NovaNoVNCProxy instance.")
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

	// We are adding Nova finalizer to Memcached CR later.
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

	hashes := make(map[string]env.Setter)

	// hash the endpoint URLs of the services this depends on
	// By adding the hash to the hash of hashes being added to the deployment
	// allows it to get restarted, in case the endpoint changes and it requires
	// the current cached ones to be updated.
	endpointUrlsHash, err := keystonev1.GetHashforKeystoneEndpointUrlsForServices(
		ctx,
		h,
		instance.Namespace,
		ptr.To(string(endpoint.EndpointInternal)),
		endpointList,
	)
	if err != nil {
		return ctrl.Result{}, err
	}
	hashes["endpointUrlsHash"] = env.SetValue(endpointUrlsHash)

	secretHash, result, secret, err := ensureSecret(
		ctx,
		types.NamespacedName{Namespace: instance.Namespace, Name: instance.Spec.Secret},
		[]string{
			ServicePasswordSelector,
			TransportURLSelector,
		},
		h.GetClient(),
		&instance.Status.Conditions,
		r.RequeueTimeout,
	)
	if (err != nil || result != ctrl.Result{}) {
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
		hash, err := tls.ValidateCACertSecret(
			ctx,
			h.GetClient(),
			types.NamespacedName{
				Name:      instance.Spec.TLS.CaBundleSecretName,
				Namespace: instance.Namespace,
			},
		)
		if err != nil {
			if k8s_errors.IsNotFound(err) {
				// Since the CA cert secret should have been manually created by the user and provided in the spec,
				// we treat this as a warning because it means that the service will not be able to start.
				instance.Status.Conditions.Set(condition.FalseCondition(
					condition.TLSInputReadyCondition,
					condition.ErrorReason,
					condition.SeverityWarning,
					condition.TLSInputReadyWaitingMessage, instance.Spec.TLS.CaBundleSecretName))
				return ctrl.Result{}, nil
			}
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.TLSInputReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.TLSInputErrorMessage,
				err.Error()))
			return ctrl.Result{}, err
		}

		if hash != "" {
			hashes[tls.CABundleKey] = env.SetValue(hash)
		}
	}

	// Validate the service cert secret
	if instance.Spec.TLS.Service.Enabled() {
		hash, err := instance.Spec.TLS.Service.ValidateCertSecret(ctx, h, instance.Namespace)
		if err != nil {
			if k8s_errors.IsNotFound(err) {
				// We treat this as an info since the service cert secret is automatically generated
				// by the encompassing OpenStackControlPlane CR
				instance.Status.Conditions.Set(condition.FalseCondition(
					condition.TLSInputReadyCondition,
					condition.RequestedReason,
					condition.SeverityInfo,
					condition.TLSInputReadyWaitingMessage, err.Error()))
				return ctrl.Result{}, nil
			}
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.TLSInputReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.TLSInputErrorMessage,
				err.Error()))
			return ctrl.Result{}, err
		}
		hashes[tls.TLSHashName] = env.SetValue(hash)
	}

	// Validate the Vencrypt cert secret
	if instance.Spec.TLS.Vencrypt.Enabled() {
		hash, err := instance.Spec.TLS.Vencrypt.ValidateCertSecret(ctx, h, instance.Namespace)
		if err != nil {
			if k8s_errors.IsNotFound(err) {
				// We treat this as an info since the vencrypt cert secret is automatically generated
				// by the encompassing OpenStackControlPlane CR
				instance.Status.Conditions.Set(condition.FalseCondition(
					condition.TLSInputReadyCondition,
					condition.RequestedReason,
					condition.SeverityInfo,
					condition.TLSInputReadyWaitingMessage, err.Error()))
				return ctrl.Result{}, nil
			}
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.TLSInputReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.TLSInputErrorMessage,
				err.Error()))
			return ctrl.Result{}, err
		}
		hashes[novncproxy.VencryptName] = env.SetValue(hash)
	}

	// all cert input checks out so report InputReady
	instance.Status.Conditions.MarkTrue(condition.TLSInputReadyCondition, condition.InputReadyMessage)

	result, err = r.ensureServiceExposed(ctx, h, instance)
	if (err != nil || result != ctrl.Result{}) {
		// We can ignore RequeueAfter as we are watching the Service resource
		// but we have to return while waiting for the service to be exposed
		return ctrl.Result{}, err
	}

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

	result, err = r.ensureDeployment(ctx, h, instance, inputHash, serviceAnnotations, memcached)
	if (err != nil || result != ctrl.Result{}) {
		return result, err
	}

	Log.Info("Successfully reconciled")
	return ctrl.Result{}, nil
}

func (r *NovaNoVNCProxyReconciler) initStatus(
	instance *novav1.NovaNoVNCProxy,
) error {
	if err := r.initConditions(instance); err != nil {
		return err
	}

	if instance.Status.Hash == nil {
		instance.Status.Hash = map[string]string{}
	}

	if instance.Status.NetworkAttachments == nil {
		instance.Status.NetworkAttachments = map[string][]string{}
	}

	return nil
}

func (r *NovaNoVNCProxyReconciler) ensureConfigs(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.NovaNoVNCProxy,
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

func (r *NovaNoVNCProxyReconciler) initConditions(
	instance *novav1.NovaNoVNCProxy,
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
			condition.DeploymentReadyCondition,
			condition.InitReason,
			condition.DeploymentReadyInitMessage,
		),
		condition.UnknownCondition(
			condition.CreateServiceReadyCondition,
			condition.InitReason,
			condition.CreateServiceReadyInitMessage,
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
	)
	// Init Topology condition if there's a reference
	if instance.Spec.TopologyRef != nil {
		c := condition.UnknownCondition(
			condition.TopologyReadyCondition,
			condition.InitReason,
			condition.TopologyReadyInitMessage,
		)
		cl.Set(c)
	}
	instance.Status.Conditions.Init(&cl)
	return nil
}

func (r *NovaNoVNCProxyReconciler) generateConfigs(
	ctx context.Context, h *helper.Helper, instance *novav1.NovaNoVNCProxy, hashes *map[string]env.Setter,
	secret corev1.Secret,
	memcachedInstance *memcachedv1.Memcached,
) error {

	cellDB, err := mariadbv1.GetDatabaseByNameAndAccount(ctx, h, "nova-"+instance.Spec.CellName, instance.Spec.CellDatabaseAccount, instance.Namespace)
	if err != nil {
		return err
	}
	cellDatabaseAccount := cellDB.GetAccount()
	cellDbSecret := cellDB.GetSecret()

	templateParameters := map[string]any{
		"service_name":             novncproxy.ServiceName,
		"keystone_internal_url":    instance.Spec.KeystoneAuthURL,
		"nova_keystone_user":       instance.Spec.ServiceUser,
		"nova_keystone_password":   string(secret.Data[ServicePasswordSelector]),
		"cell_db_name":             getCellDatabaseName(instance.Spec.CellName),
		"cell_db_user":             cellDatabaseAccount.Spec.UserName,
		"cell_db_password":         string(cellDbSecret.Data[mariadbv1.DatabasePasswordSelector]),
		"cell_db_address":          instance.Spec.CellDatabaseHostname,
		"cell_db_port":             3306,
		"transport_url":            string(secret.Data[TransportURLSelector]),
		"openstack_region_name":    instance.Spec.Region,
		"default_project_domain":   "Default", // fixme
		"default_user_domain":      "Default", // fixme
		"MemcachedServers":         memcachedInstance.GetMemcachedServerListString(),
		"MemcachedServersWithInet": memcachedInstance.GetMemcachedServerListWithInetString(),
		"MemcachedTLS":             memcachedInstance.GetMemcachedTLSSupport(),
		QuorumQueuesTemplateKey:    parseQuorumQueues(secret.Data[QuorumQueuesTemplateKey]),
	}
	if instance.Spec.TLS.Service.Enabled() {
		templateParameters["SSLCertificateFile"] = fmt.Sprintf("/etc/pki/tls/certs/%s.crt", novncproxy.ServiceName)
		templateParameters["SSLCertificateKeyFile"] = fmt.Sprintf("/etc/pki/tls/private/%s.key", novncproxy.ServiceName)
	}

	if instance.Spec.TLS.Vencrypt.Enabled() {
		templateParameters["VencryptClientKey"] = "/etc/pki/nova-novncproxy/client-key.pem"
		templateParameters["VencryptClientCert"] = "/etc/pki/nova-novncproxy/client-cert.pem"
		templateParameters["VencryptCACerts"] = "/etc/pki/nova-novncproxy/ca-cert.pem"
	}

	var tlsCfg *tls.Service
	if instance.Spec.TLS.CaBundleSecretName != "" {
		tlsCfg = &tls.Service{}
	}

	// MTLS
	if memcachedInstance.GetMemcachedMTLSSecret() != "" {
		templateParameters["MemcachedAuthCert"] = fmt.Sprint(memcachedv1.CertMountPath())
		templateParameters["MemcachedAuthKey"] = fmt.Sprint(memcachedv1.KeyMountPath())
		templateParameters["MemcachedAuthCa"] = fmt.Sprint(memcachedv1.CaMountPath())
	}

	extraData := map[string]string{
		"my.cnf": cellDB.GetDatabaseClientConfig(tlsCfg), //(mschuppert) for now just get the default my.cnf
	}
	if instance.Spec.CustomServiceConfig != "" {
		extraData["02-nova-override.conf"] = instance.Spec.CustomServiceConfig
	}

	cmLabels := labels.GetLabels(
		instance, labels.GetGroupLabel(NovaNoVNCProxyLabelPrefix), map[string]string{},
	)

	err = r.GenerateConfigs(
		ctx, h, instance, nova.GetServiceConfigSecretName(instance.GetName()),
		hashes, templateParameters, extraData, cmLabels, map[string]string{},
	)
	return err
}

func (r *NovaNoVNCProxyReconciler) ensureDeployment(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.NovaNoVNCProxy,
	inputHash string,
	annotations map[string]string,
	memcached *memcachedv1.Memcached,
) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	serviceLabels := getNoVNCProxyServiceLabels(instance.Spec.CellName)

	//
	// Handle Topology
	//
	topology, err := ensureTopology(
		ctx,
		h,
		instance,      // topologyHandler
		instance.Name, // finalizer
		&instance.Status.Conditions,
		labels.GetLabelSelector(serviceLabels),
	)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("waiting for Topology requirements: %w", err)
	}

	ssSpec, err := novncproxy.StatefulSet(instance, inputHash, serviceLabels, annotations, topology, memcached)
	if err != nil {
		Log.Info("Deployment failed")
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DeploymentReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.DeploymentReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}
	ss := statefulset.NewStatefulSet(ssSpec, r.RequeueTimeout)
	ctrlResult, err := ss.CreateOrPatch(ctx, h)
	if err != nil && !k8s_errors.IsNotFound(err) {
		Log.Info("Deployment failed")
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

	statefulSet := ss.GetStatefulSet()
	if statefulSet.Generation == statefulSet.Status.ObservedGeneration {
		instance.Status.ReadyCount = statefulSet.Status.ReadyReplicas
	}

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
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.NetworkAttachmentsReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.NetworkAttachmentsErrorMessage,
			instance.Spec.NetworkAttachments))

		return ctrl.Result{}, err
	}

	if instance.Status.ReadyCount == *instance.Spec.Replicas && statefulSet.Generation == statefulSet.Status.ObservedGeneration {
		Log.Info("Deployment is ready")
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

func (r *NovaNoVNCProxyReconciler) ensureServiceExposed(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.NovaNoVNCProxy,
) (ctrl.Result, error) {
	endpointTypeStr := string(service.EndpointPublic)
	serviceName := novncproxy.ServiceName + "-" + instance.Spec.CellName + "-" + endpointTypeStr

	svcOverride := instance.Spec.Override.Service
	if svcOverride == nil {
		svcOverride = &service.RoutedOverrideSpec{}
	}
	if svcOverride.EmbeddedLabelsAnnotations == nil {
		svcOverride.EmbeddedLabelsAnnotations = &service.EmbeddedLabelsAnnotations{}
	}

	exportLabels := util.MergeStringMaps(
		getNoVNCProxyServiceLabels(instance.Spec.CellName),
		map[string]string{
			service.AnnotationEndpointKey: endpointTypeStr,
		},
	)

	// Create the service
	svc, err := service.NewService(
		service.GenericService(&service.GenericServiceDetails{
			Name:      serviceName,
			Namespace: instance.Namespace,
			Labels:    exportLabels,
			Selector:  getNoVNCProxyServiceLabels(instance.Spec.CellName),
			Port: service.GenericServicePort{
				Name:     serviceName,
				Port:     novncproxy.NoVNCProxyPort,
				Protocol: corev1.ProtocolTCP,
			},
		}),
		5,
		&svcOverride.OverrideSpec,
	)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.CreateServiceReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.CreateServiceReadyErrorMessage,
			err.Error()))

		return ctrl.Result{}, err
	}

	svc.AddAnnotation(map[string]string{
		service.AnnotationEndpointKey: endpointTypeStr,
	})

	// add Annotation to whether creating an ingress is required or not
	if svc.GetServiceType() == corev1.ServiceTypeClusterIP {
		svc.AddAnnotation(map[string]string{
			service.AnnotationIngressCreateKey: "true",
		})
	} else {
		svc.AddAnnotation(map[string]string{
			service.AnnotationIngressCreateKey: "false",
		})
		if svc.GetServiceType() == corev1.ServiceTypeLoadBalancer {
			svc.AddAnnotation(map[string]string{
				service.AnnotationHostnameKey: svc.GetServiceHostname(), // add annotation to register service name in dnsmasq
			})
		}
	}

	ctrlResult, err := svc.CreateOrPatch(ctx, h)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.CreateServiceReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.CreateServiceReadyErrorMessage,
			err.Error()))

		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.CreateServiceReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.CreateServiceReadyRunningMessage))
		return ctrlResult, nil
	}
	// create service - end
	instance.Status.Conditions.MarkTrue(condition.CreateServiceReadyCondition, condition.CreateServiceReadyMessage)

	return ctrl.Result{}, nil
}

func (r *NovaNoVNCProxyReconciler) reconcileDelete(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.NovaNoVNCProxy,
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

	// Remove finalizer from the referenced Topology CR
	if _, err := topologyv1.EnsureDeletedTopologyRef(
		ctx,
		h,
		instance.Status.LastAppliedTopology,
		instance.Name,
	); err != nil {
		return err
	}

	// Successfully cleaned up everything. So as the final step let's remove the
	// finalizer from ourselves to allow the deletion of NovaNoVNCProxy CR itself
	updated := controllerutil.RemoveFinalizer(instance, h.GetFinalizer())
	if updated {
		Log.Info("Removed finalizer from ourselves")
	}

	Log.Info("Reconciled delete successfully")
	return nil
}

func getNoVNCProxyServiceLabels(cell string) map[string]string {
	return map[string]string{
		common.AppSelector: NovaNoVNCProxyLabelPrefix,
		CellSelector:       cell,
	}
}

func (r *NovaNoVNCProxyReconciler) findObjectsForSrc(ctx context.Context, src client.Object) []reconcile.Request {
	requests := []reconcile.Request{}

	Log := r.GetLogger(ctx)

	for _, field := range noVNCProxyWatchFields {
		crList := &novav1.NovaNoVNCProxyList{}
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

func (r *NovaNoVNCProxyReconciler) findObjectsWithAppSelectorLabelInNamespace(ctx context.Context, src client.Object) []reconcile.Request {
	requests := []reconcile.Request{}

	Log := r.GetLogger(ctx)

	// if the endpoint has the service label and its in our endpointList, reconcile the CR in the namespace
	if svc, ok := src.GetLabels()[common.AppSelector]; ok && util.StringInSlice(svc, endpointList) {
		crList := &novav1.NovaNoVNCProxyList{}
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
	}

	return requests
}

// fields to index to reconcile when change
var (
	noVNCProxyWatchFields = []string{
		passwordSecretField,
		caBundleSecretNameField,
		tlsNoVNCProxyServiceField,
		tlsNoVNCProxyVencryptField,
		topologyField,
	}
)

func (r *NovaNoVNCProxyReconciler) memcachedNamespaceMapFunc(ctx context.Context, src client.Object) []reconcile.Request {

	result := []reconcile.Request{}

	// get all Nova CRs
	novaNoVNCProxyList := &novav1.NovaNoVNCProxyList{}
	listOpts := []client.ListOption{
		client.InNamespace(src.GetNamespace()),
	}
	if err := r.Client.List(ctx, novaNoVNCProxyList, listOpts...); err != nil {
		return nil
	}

	for _, cr := range novaNoVNCProxyList.Items {
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

// SetupWithManager sets up the controller with the Manager.
func (r *NovaNoVNCProxyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// index passwordSecretField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &novav1.NovaNoVNCProxy{}, passwordSecretField, func(rawObj client.Object) []string {
		// Extract the secret name from the spec, if one is provided
		cr := rawObj.(*novav1.NovaNoVNCProxy)
		if cr.Spec.Secret == "" {
			return nil
		}
		return []string{cr.Spec.Secret}
	}); err != nil {
		return err
	}

	// index caBundleSecretNameField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &novav1.NovaNoVNCProxy{}, caBundleSecretNameField, func(rawObj client.Object) []string {
		// Extract the secret name from the spec, if one is provided
		cr := rawObj.(*novav1.NovaNoVNCProxy)
		if cr.Spec.TLS.CaBundleSecretName == "" {
			return nil
		}
		return []string{cr.Spec.TLS.CaBundleSecretName}
	}); err != nil {
		return err
	}

	// index service cert secret tlsNoVNCProxyField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &novav1.NovaNoVNCProxy{}, tlsNoVNCProxyServiceField, func(rawObj client.Object) []string {
		// Extract the secret name from the spec, if one is provided
		cr := rawObj.(*novav1.NovaNoVNCProxy)
		if cr.Spec.TLS.Service.SecretName == nil {
			return nil
		}
		return []string{*cr.Spec.TLS.Service.SecretName}
	}); err != nil {
		return err
	}

	// index vencrypt cert secret tlsNoVNCProxyField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &novav1.NovaNoVNCProxy{}, tlsNoVNCProxyVencryptField, func(rawObj client.Object) []string {
		// Extract the secret name from the spec, if one is provided
		cr := rawObj.(*novav1.NovaNoVNCProxy)
		if cr.Spec.TLS.Vencrypt.SecretName == nil {
			return nil
		}
		return []string{*cr.Spec.TLS.Vencrypt.SecretName}
	}); err != nil {
		return err
	}

	// index topologyField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &novav1.NovaNoVNCProxy{}, topologyField, func(rawObj client.Object) []string {
		// Extract the topology name from the spec, if one is provided
		cr := rawObj.(*novav1.NovaNoVNCProxy)
		if cr.Spec.TopologyRef == nil {
			return nil
		}
		return []string{cr.Spec.TopologyRef.Name}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&novav1.NovaNoVNCProxy{}).
		Owns(&v1.StatefulSet{}).
		Owns(&corev1.Service{}).
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
		Watches(&topologyv1.Topology{},
			handler.EnqueueRequestsFromMapFunc(r.findObjectsForSrc),
			builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(&keystonev1.KeystoneEndpoint{},
			handler.EnqueueRequestsFromMapFunc(r.findObjectsWithAppSelectorLabelInNamespace),
			builder.WithPredicates(keystonev1.KeystoneEndpointStatusChangedPredicate)).
		Complete(r)
}
