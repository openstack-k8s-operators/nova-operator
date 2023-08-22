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
	"net/url"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/go-logr/logr"
	common "github.com/openstack-k8s-operators/lib-common/modules/common"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	helper "github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"github.com/openstack-k8s-operators/lib-common/modules/common/labels"
	nad "github.com/openstack-k8s-operators/lib-common/modules/common/networkattachment"
	common_secret "github.com/openstack-k8s-operators/lib-common/modules/common/secret"
	"github.com/openstack-k8s-operators/lib-common/modules/common/service"
	"github.com/openstack-k8s-operators/lib-common/modules/common/statefulset"
	util "github.com/openstack-k8s-operators/lib-common/modules/common/util"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/nova-operator/pkg/nova"
	"github.com/openstack-k8s-operators/nova-operator/pkg/novametadata"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
)

// NovaMetadataReconciler reconciles a NovaMetadata object
type NovaMetadataReconciler struct {
	ReconcilerBase
}

// getlogger returns a logger object with a prefix of "conroller.name" and aditional controller context fields
func (r *NovaMetadataReconciler) GetLogger(ctx context.Context) logr.Logger {
	return log.FromContext(ctx).WithName("Controllers").WithName("NovaMetadata")
}

//+kubebuilder:rbac:groups=nova.openstack.org,resources=novametadata,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=nova.openstack.org,resources=novametadata/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=nova.openstack.org,resources=novametadata/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=k8s.cni.cncf.io,resources=network-attachment-definitions,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NovaMetadata object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.2/pkg/reconcile
func (r *NovaMetadataReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, _err error) {
	Log := r.GetLogger(ctx)

	// Fetch the NovaMetadata instance that needs to be reconciled
	instance := &novav1.NovaMetadata{}
	err := r.Client.Get(ctx, req.NamespacedName, instance)

	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected.
			// For additional cleanup logic use finalizers. Return and don't requeue.
			Log.Info("NovaMetadata instance not found, probably deleted before reconciled. Nothing to do.")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		Log.Error(err, "Failed to read the NovaMetadata instance.")
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
		return ctrl.Result{}, err
	}

	Log.Info("Reconciling")

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

	hashes := make(map[string]env.Setter)

	expectedSelectors := []string{
		ServicePasswordSelector,
		CellDatabasePasswordSelector,
		MetadataSecretSelector,
		TransportURLSelector,
	}
	if instance.Spec.CellName == "" {
		expectedSelectors = append(expectedSelectors, APIDatabasePasswordSelector)
	}
	secretHash, result, secret, err := ensureSecret(
		ctx,
		types.NamespacedName{Namespace: instance.Namespace, Name: instance.Spec.Secret},
		expectedSelectors,
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

	err = r.ensureConfigs(ctx, h, instance, &hashes, secret)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Create hash over all the different input resources to identify if any of
	// those changed and a restart/recreate is required.
	// We have a special input, the registered cells, as the openstack service
	// needs to be restarted if this changes to refresh the in memory cell caches
	cellHash, err := hashOfStringMap(instance.Spec.RegisteredCells)
	if err != nil {
		return ctrl.Result{}, err
	}
	hashes["cells"] = env.SetValue(cellHash)

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

	result, err = r.ensureDeployment(ctx, h, instance, inputHash, serviceAnnotations)
	if (err != nil || result != ctrl.Result{}) {
		return result, err
	}

	apiEndpoint, result, err := r.ensureServiceExposed(ctx, h, instance)
	if (err != nil || result != ctrl.Result{}) {
		// We can ignore RequeueAfter as we are watching the Service resource
		// but we have to return while waiting for the service to be exposed
		return ctrl.Result{}, err
	}

	// We have to wait until our service is fully exposed so that we can
	// generate the compute config containing the metadata host
	if !instance.Status.Conditions.IsTrue(condition.ExposeServiceReadyCondition) {
		Log.Info("Waiting for the service to be exposed before generating compute configuration")
		return ctrl.Result{}, nil
	}

	// TODO(gibi): fix lib-common endpoint.ExposeEndpoints return value to
	// avoid the need for the cast
	err = r.ensureNeutronConfig(ctx, h, instance, apiEndpoint, secret)
	if err != nil {
		return result, err
	}

	Log.Info("Successfully reconciled")
	return ctrl.Result{}, nil
}

func (r *NovaMetadataReconciler) initStatus(
	ctx context.Context, h *helper.Helper, instance *novav1.NovaMetadata,
) error {
	if err := r.initConditions(ctx, h, instance); err != nil {
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

func (r *NovaMetadataReconciler) initConditions(
	ctx context.Context, h *helper.Helper, instance *novav1.NovaMetadata,
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
				condition.ExposeServiceReadyCondition,
				condition.InitReason,
				condition.ExposeServiceReadyInitMessage,
			),
			condition.UnknownCondition(
				condition.NetworkAttachmentsReadyCondition,
				condition.InitReason,
				condition.NetworkAttachmentsReadyInitMessage,
			),
			condition.UnknownCondition(
				novav1.NovaComputeServiceConfigReady,
				condition.InitReason,
				novav1.NovaComputeServiceConfigInitMessage,
			),
		)

		instance.Status.Conditions.Init(&cl)
	}
	return nil
}

func (r *NovaMetadataReconciler) ensureConfigs(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.NovaMetadata,
	hashes *map[string]env.Setter,
	secret corev1.Secret,
) error {
	err := r.generateConfigs(ctx, h, instance, hashes, secret)
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

func (r *NovaMetadataReconciler) generateConfigs(
	ctx context.Context, h *helper.Helper, instance *novav1.NovaMetadata, hashes *map[string]env.Setter,
	secret corev1.Secret,
) error {
	templateParameters := map[string]interface{}{
		"service_name":           novametadata.ServiceName,
		"keystone_internal_url":  instance.Spec.KeystoneAuthURL,
		"nova_keystone_user":     instance.Spec.ServiceUser,
		"nova_keystone_password": string(secret.Data[ServicePasswordSelector]),
		"cell_db_name":           instance.Spec.CellDatabaseUser, // fixme
		"cell_db_user":           instance.Spec.CellDatabaseUser,
		"cell_db_password":       string(secret.Data[CellDatabasePasswordSelector]),
		"cell_db_address":        instance.Spec.CellDatabaseHostname,
		"cell_db_port":           3306,
		"openstack_cacert":       "",          // fixme
		"openstack_region_name":  "regionOne", // fixme
		"default_project_domain": "Default",   // fixme
		"default_user_domain":    "Default",   // fixme
		"metadata_secret":        string(secret.Data[MetadataSecretSelector]),
		"log_file":               "/var/log/nova/nova-metadata.log",
		"transport_url":          string(secret.Data[TransportURLSelector]),
	}

	if instance.Spec.CellName == "" {
		templateParameters["api_db_name"] = instance.Spec.APIDatabaseUser // fixme
		templateParameters["api_db_user"] = instance.Spec.APIDatabaseUser // fixme
		templateParameters["api_db_password"] = string(secret.Data[APIDatabasePasswordSelector])
		templateParameters["api_db_address"] = instance.Spec.APIDatabaseHostname
		templateParameters["api_db_port"] = 3306
		templateParameters["local_metadata_per_cell"] = false
	} else {
		templateParameters["local_metadata_per_cell"] = true
	}

	extraData := map[string]string{}
	if instance.Spec.CustomServiceConfig != "" {
		extraData["02-nova-override.conf"] = instance.Spec.CustomServiceConfig
	}
	for key, data := range instance.Spec.DefaultConfigOverwrite {
		extraData[key] = data
	}

	cmLabels := labels.GetLabels(
		instance, labels.GetGroupLabel(NovaMetadataLabelPrefix), map[string]string{},
	)

	err := r.GenerateConfigs(
		ctx, h, instance, nova.GetServiceConfigSecretName(instance.GetName()),
		hashes, templateParameters, extraData, cmLabels, map[string]string{},
	)
	return err
}

func (r *NovaMetadataReconciler) ensureDeployment(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.NovaMetadata,
	inputHash string,
	annotations map[string]string,
) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)

	serviceLabels := getMetadataServiceLabels(instance.Spec.CellName)
	ss := statefulset.NewStatefulSet(novametadata.StatefulSet(instance, inputHash, serviceLabels, annotations), r.RequeueTimeout)
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

	if instance.Status.ReadyCount > 0 || *instance.Spec.Replicas == 0 {
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

func (r *NovaMetadataReconciler) ensureServiceExposed(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.NovaMetadata,
) (string, ctrl.Result, error) {
	endpointTypeStr := string(service.EndpointInternal)
	serviceName := novametadata.ServiceName
	if instance.Spec.CellName != "" {
		serviceName = novametadata.ServiceName + "-" + instance.Spec.CellName
	}
	serviceName = serviceName + "-" + endpointTypeStr
	svcOverride := instance.Spec.Override.Service
	if svcOverride == nil {
		svcOverride = &service.OverrideSpec{}
	}
	if svcOverride.EmbeddedLabelsAnnotations == nil {
		svcOverride.EmbeddedLabelsAnnotations = &service.EmbeddedLabelsAnnotations{}
	}

	exportLabels := util.MergeStringMaps(
		getMetadataServiceLabels(instance.Spec.CellName),
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
			Selector:  getMetadataServiceLabels(instance.Spec.CellName),
			Port: service.GenericServicePort{
				Name:     serviceName,
				Port:     novametadata.APIServicePort,
				Protocol: corev1.ProtocolTCP,
			},
		}),
		5,
		svcOverride,
	)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ExposeServiceReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.ExposeServiceReadyErrorMessage,
			err.Error()))
		return "", ctrl.Result{}, err
	}

	if svc.GetServiceType() == corev1.ServiceTypeLoadBalancer {
		svc.AddAnnotation(map[string]string{
			service.AnnotationHostnameKey: svc.GetServiceHostname(), // add annotation to register service name in dnsmasq
		})
	}

	ctrlResult, err := svc.CreateOrPatch(ctx, h)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ExposeServiceReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.ExposeServiceReadyErrorMessage,
			err.Error()))

		return "", ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ExposeServiceReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.ExposeServiceReadyRunningMessage))
		return "", ctrlResult, err
	}
	// create service - end

	// TODO: TLS, pass in https as protocol
	apiEndpoint, err := svc.GetAPIEndpoint(
		nil, ptr.To(service.ProtocolHTTP), "")
	if err != nil {
		return "", ctrl.Result{}, err
	}

	instance.Status.Conditions.MarkTrue(condition.ExposeServiceReadyCondition, condition.ExposeServiceReadyMessage)

	return apiEndpoint, ctrl.Result{}, nil
}

func (r *NovaMetadataReconciler) reconcileDelete(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.NovaMetadata,
) error {
	Log := r.GetLogger(ctx)
	Log.Info("Reconciling delete")
	// TODO(ksambor): add cleanup for the service rows in the nova DB
	// when the service is scaled in or deleted
	Log.Info("Reconciled delete successfully")
	return nil
}

func getMetadataServiceLabels(cell string) map[string]string {
	if cell != "" {
		return map[string]string{
			common.AppSelector: NovaMetadataLabelPrefix,
			CellSelector:       cell,
		}
	}
	return map[string]string{
		common.AppSelector: NovaMetadataLabelPrefix,
	}
}

// ensureNeutronConfig ensures the metadata config Secret exists and up to
// date. The metadata config Secret then can be used to configure the neutron
// metadata agent on the the EDPM side
func (r *NovaMetadataReconciler) ensureNeutronConfig(
	ctx context.Context, h *helper.Helper,
	instance *novav1.NovaMetadata, endpoint string, secret corev1.Secret,
) error {

	err := r.generateNeutronConfigs(ctx, h, instance, endpoint, secret)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			novav1.NovaComputeServiceConfigReady,
			condition.ErrorReason,
			condition.SeverityWarning,
			novav1.NovaComputeServiceConfigErrorMessage,
			err.Error()))
		return err
	}
	instance.Status.Conditions.MarkTrue(
		novav1.NovaComputeServiceConfigReady, condition.ServiceConfigReadyMessage,
	)

	return nil
}

func (r *NovaMetadataReconciler) generateNeutronConfigs(
	ctx context.Context, h *helper.Helper,
	instance *novav1.NovaMetadata, endpoint string, secret corev1.Secret,
) error {
	configName := instance.GetName() + "-neutron-config"

	templates := map[string]string{
		"05-nova-metadata.conf": "/neutron-metadata.conf",
	}

	endpointURL, err := url.Parse(endpoint)
	if err != nil {
		return err
	}

	// NOTE(gibi): We are generating this data in the nova-operator to:
	// 1. avoid the work needed to teach cells to neutron
	// 2. avoid the need to synchronize the shared secret between nova- and
	//    neutron-operator externally
	templateParameters := map[string]interface{}{
		"nova_metadata_host":           endpointURL.Hostname(),
		"nova_metadata_port":           endpointURL.Port(),
		"nova_metadata_protocol":       endpointURL.Scheme,
		"metadata_proxy_shared_secret": string(secret.Data[MetadataSecretSelector]),
	}

	labels := getMetadataServiceLabels(instance.Spec.CellName)
	hashes := make(map[string]env.Setter)

	cms := []util.Template{
		{
			Name:               configName,
			Namespace:          instance.GetNamespace(),
			Type:               util.TemplateTypeConfig,
			InstanceType:       instance.GetObjectKind().GroupVersionKind().Kind,
			ConfigOptions:      templateParameters,
			Labels:             labels,
			AdditionalTemplate: templates,
		},
	}

	err = common_secret.EnsureSecrets(ctx, h, instance, cms, &hashes)
	if err != nil {
		return err
	}

	// TODO(gibi): can we make it simpler?
	a := &corev1.EnvVar{}
	hashes[configName](a)
	instance.Status.Hash[configName] = a.Value
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NovaMetadataReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&novav1.NovaMetadata{}).
		Owns(&v1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.Secret{}).
		Watches(&source.Kind{Type: &corev1.Secret{}},
			handler.EnqueueRequestsFromMapFunc(r.GetSecretMapperFor(&novav1.NovaMetadataList{}, context.TODO()))).
		Complete(r)
}
