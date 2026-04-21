/*
Copyright 2024.

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

package cyborg

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
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

	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/endpoint"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"github.com/openstack-k8s-operators/lib-common/modules/common/labels"
	"github.com/openstack-k8s-operators/lib-common/modules/common/secret"
	libservice "github.com/openstack-k8s-operators/lib-common/modules/common/service"
	"github.com/openstack-k8s-operators/lib-common/modules/common/statefulset"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"

	cyborgv1beta1 "github.com/openstack-k8s-operators/nova-operator/api/cyborg/v1beta1"
	cyborgservice "github.com/openstack-k8s-operators/nova-operator/internal/cyborg"
	cyborgapi "github.com/openstack-k8s-operators/nova-operator/internal/cyborg/api"
)

const (
	apiConfigSecretField = ".spec.configSecret" // #nosec G101
	apiTopologyField     = ".spec.topologyRef.Name"
)

var apiSecretWatchFields = []string{
	apiConfigSecretField,
	caBundleSecretNameField,
	tlsAPIInternalField,
	tlsAPIPublicField,
}

// CyborgAPIReconciler reconciles a CyborgAPI object
//
//nolint:revive
type CyborgAPIReconciler struct {
	ReconcilerBase
}

// GetLogger returns a logger object with a prefix of "controller.name" and additional controller context fields
func (r *CyborgAPIReconciler) GetLogger(ctx context.Context) logr.Logger {
	return log.FromContext(ctx).WithName("Controllers").WithName("CyborgAPI")
}

// +kubebuilder:rbac:groups=cyborg.openstack.org,resources=cyborgapis,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cyborg.openstack.org,resources=cyborgapis/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cyborg.openstack.org,resources=cyborgapis/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneendpoints,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop
func (r *CyborgAPIReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, _err error) {
	Log := r.GetLogger(ctx)

	instance := &cyborgv1beta1.CyborgAPI{}
	err := r.Client.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			Log.Info("CyborgAPI instance not found, probably deleted before reconciled. Nothing to do.")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	Log.Info(fmt.Sprintf("Reconciling CyborgAPI '%s'", instance.Name))

	h, err := helper.NewHelper(instance, r.Client, r.Kclient, r.Scheme, Log)
	if err != nil {
		return ctrl.Result{}, err
	}

	isNewInstance := instance.Status.Conditions == nil
	savedConditions := instance.Status.Conditions.DeepCopy()

	defer func() {
		// Don't update the status, if Reconciler Panics
		if r := recover(); r != nil {
			Log.Info(fmt.Sprintf("Panic during reconcile %v\n", r))
			panic(r)
		}
		// update the Ready condition based on the sub conditions
		if instance.Status.Conditions.AllSubConditionIsTrue() {
			instance.Status.Conditions.MarkTrue(
				condition.ReadyCondition, condition.ReadyMessage)
		} else {
			instance.Status.Conditions.MarkUnknown(
				condition.ReadyCondition, condition.InitReason, condition.ReadyInitMessage)
			instance.Status.Conditions.Set(
				instance.Status.Conditions.Mirror(condition.ReadyCondition))
		}
		condition.RestoreLastTransitionTimes(&instance.Status.Conditions, savedConditions)
		err := h.PatchInstance(ctx, instance)
		if err != nil {
			_err = err
		}
	}()

	r.initStatus(instance)

	if instance.DeletionTimestamp.IsZero() && controllerutil.AddFinalizer(instance, h.GetFinalizer()) || isNewInstance {
		return ctrl.Result{}, nil
	}

	if !instance.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, instance, h)
	}

	// Read the sub-level secret created by the Cyborg controller
	subSecret := &corev1.Secret{}
	secretName := types.NamespacedName{Namespace: instance.Namespace, Name: instance.Spec.ConfigSecret}
	err = r.Client.Get(ctx, secretName, subSecret)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			Log.Info("Secret not found, waiting", "secret", instance.Spec.ConfigSecret)
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.InputReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				condition.InputReadyWaitingMessage))
			return ctrl.Result{RequeueAfter: r.RequeueTimeout}, nil
		}
		return ctrl.Result{}, err
	}

	// Hash the input secret so we detect changes to passwords, transport URL, etc.
	inputHashes := make(map[string]env.Setter)
	secretHash, err := util.ObjectHash(subSecret.Data)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error calculating input secret hash: %w", err)
	}
	inputHashes["input"] = env.SetValue(secretHash)

	instance.Status.Conditions.MarkTrue(condition.InputReadyCondition, condition.InputReadyMessage)

	//
	// TLS input validation
	//
	configVars := make(map[string]env.Setter)

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
				instance.Status.Conditions.Set(condition.FalseCondition(
					condition.TLSInputReadyCondition,
					condition.ErrorReason,
					condition.SeverityWarning,
					condition.TLSInputReadyWaitingMessage, instance.Spec.TLS.CaBundleSecretName,
				))
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
			configVars[tls.CABundleKey] = env.SetValue(hash)
		}
	}

	// Validate API cert secrets
	certsHash, err := instance.Spec.TLS.API.ValidateCertSecrets(ctx, h, instance.Namespace)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.TLSInputReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				condition.TLSInputReadyWaitingMessage, err.Error(),
			))
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
	configVars[tls.TLSHashName] = env.SetValue(certsHash)
	instance.Status.Conditions.MarkTrue(condition.TLSInputReadyCondition, condition.InputReadyMessage)

	// Generate config
	err = r.generateServiceConfig(ctx, instance, subSecret, h, &configVars)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ServiceConfigReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.ServiceConfigReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}

	for key, hashVal := range configVars {
		inputHashes[key] = hashVal
	}

	// Compute a combined hash of all inputs (secret + generated config).
	// This hash is set as CONFIG_HASH env var in the pod template so that
	// any change in the input secret or generated config triggers a rollout.
	inputHash, err := util.HashOfInputHashes(inputHashes)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error calculating combined input hash: %w", err)
	}
	instance.Status.Hash[common.InputHashName] = inputHash

	instance.Status.Conditions.MarkTrue(condition.ServiceConfigReadyCondition, condition.ServiceConfigReadyMessage)

	serviceLabels := map[string]string{
		common.AppSelector: cyborgapi.ComponentName,
	}

	topology, err := ensureTopology(
		ctx,
		h,
		instance,
		instance.Name,
		&instance.Status.Conditions,
		labels.GetLabelSelector(serviceLabels),
	)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("waiting for Topology requirements: %w", err)
	}

	// Create or update the StatefulSet
	ssDef := cyborgapi.StatefulSet(instance, inputHash, serviceLabels, topology)

	ss := statefulset.NewStatefulSet(ssDef, r.RequeueTimeout)
	ctrlResult, err := ss.CreateOrPatch(ctx, h)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DeploymentReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.DeploymentReadyErrorMessage,
			err.Error()))
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DeploymentReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DeploymentReadyRunningMessage))
		return ctrlResult, nil
	}

	ssObj := ss.GetStatefulSet()
	instance.Status.ReadyCount = ssObj.Status.ReadyReplicas
	if statefulset.IsReady(ssObj) {
		instance.Status.Conditions.MarkTrue(condition.DeploymentReadyCondition, condition.DeploymentReadyMessage)
	} else {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DeploymentReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DeploymentReadyRunningMessage))
	}

	if instance.Status.Conditions.IsTrue(condition.DeploymentReadyCondition) {
		apiEndpoints, ctrlResult, err := r.ensureServiceExposed(ctx, h, instance, serviceLabels)
		if err != nil || (ctrlResult != ctrl.Result{}) {
			return ctrl.Result{}, err
		}

		ctrlResult, err = r.ensureKeystoneEndpoint(ctx, h, instance, apiEndpoints, serviceLabels)
		if err != nil || (ctrlResult != ctrl.Result{}) {
			return ctrlResult, err
		}
	}

	instance.Status.ObservedGeneration = instance.Generation

	Log.Info("Successfully reconciled")
	return ctrl.Result{}, nil
}

func (r *CyborgAPIReconciler) initStatus(instance *cyborgv1beta1.CyborgAPI) {
	if instance.Status.Conditions == nil {
		instance.Status.Conditions = condition.Conditions{}
	}

	cl := condition.CreateList(
		condition.UnknownCondition(condition.ReadyCondition, condition.InitReason, condition.ReadyInitMessage),
		condition.UnknownCondition(condition.InputReadyCondition, condition.InitReason, condition.InputReadyInitMessage),
		condition.UnknownCondition(condition.TLSInputReadyCondition, condition.InitReason, condition.InputReadyInitMessage),
		condition.UnknownCondition(condition.ServiceConfigReadyCondition, condition.InitReason, condition.ServiceConfigReadyInitMessage),
		condition.UnknownCondition(condition.DeploymentReadyCondition, condition.InitReason, condition.DeploymentReadyInitMessage),
		condition.UnknownCondition(condition.CreateServiceReadyCondition, condition.InitReason, condition.CreateServiceReadyInitMessage),
		condition.UnknownCondition(condition.KeystoneEndpointReadyCondition, condition.InitReason, "KeystoneEndpoint not created"),
	)

	if instance.Spec.TopologyRef != nil {
		cl.Set(condition.UnknownCondition(
			condition.TopologyReadyCondition,
			condition.InitReason,
			condition.TopologyReadyInitMessage,
		))
	}

	instance.Status.Conditions.Init(&cl)

	if instance.Status.Hash == nil {
		instance.Status.Hash = make(map[string]string)
	}
}

func (r *CyborgAPIReconciler) generateServiceConfig(
	ctx context.Context,
	instance *cyborgv1beta1.CyborgAPI,
	subSecret *corev1.Secret,
	h *helper.Helper,
	envVars *map[string]env.Setter,
) error {
	Log := r.GetLogger(ctx)
	Log.Info("generateServiceConfig - reconciling config for CyborgAPI")

	var tlsCfg *tls.Service
	if instance.Spec.TLS.CaBundleSecretName != "" {
		tlsCfg = &tls.Service{}
	}

	databaseConnection := fmt.Sprintf("mysql+pymysql://%s:%s@%s/%s?read_default_file=/etc/my.cnf",
		string(subSecret.Data[DatabaseUsername]),
		string(subSecret.Data[DatabasePassword]),
		string(subSecret.Data[DatabaseHostname]),
		cyborgservice.DatabaseName,
	)

	templateParameters := map[string]any{
		"DatabaseConnection": databaseConnection,
		"TransportURL":       string(subSecret.Data[TransportURLSelector]),
		"APIPublicPort":      cyborgservice.CyborgPublicPort,
		"LogFile":            cyborgservice.CyborgLogPath + instance.Name + ".log",
	}

	if quorumQueues, ok := subSecret.Data[QuorumQueuesSelector]; ok && string(quorumQueues) == "true" {
		templateParameters["QuorumQueues"] = true
	}

	if keystoneURL, ok := subSecret.Data["KeystoneAuthURL"]; ok && len(keystoneURL) > 0 {
		templateParameters["KeystoneAuthURL"] = string(keystoneURL)
	}

	if serviceUser, ok := subSecret.Data["ServiceUser"]; ok && len(serviceUser) > 0 {
		templateParameters["ServiceUser"] = string(serviceUser)
	}

	if servicePassword, ok := subSecret.Data["ServicePassword"]; ok && len(servicePassword) > 0 {
		templateParameters["ServicePassword"] = string(servicePassword)
	}

	if region, ok := subSecret.Data["Region"]; ok && len(region) > 0 {
		templateParameters["Region"] = string(region)
	}

	if acid, ok := subSecret.Data["ACID"]; ok && len(acid) > 0 {
		templateParameters["ACID"] = string(acid)
		templateParameters["ACSecret"] = string(subSecret.Data["ACSecret"])
	}

	if instance.Spec.TLS.CaBundleSecretName != "" {
		templateParameters["CaFilePath"] = tls.DownstreamTLSCABundlePath
	}

	// Build per-endpoint VirtualHost parameters for the Apache WSGI config
	httpdVhostConfig := map[string]any{}
	for _, endpt := range []libservice.Endpoint{libservice.EndpointInternal, libservice.EndpointPublic} {
		endptConfig := map[string]any{
			"ServerName": fmt.Sprintf("%s-%s.%s.svc", cyborgservice.ServiceName, endpt.String(), instance.Namespace),
			"Port":       cyborgservice.CyborgPublicPort,
			"TLS":        false,
			"TimeOut":    *instance.Spec.APITimeout,
		}
		if instance.Spec.TLS.API.Enabled(endpt) {
			endptConfig["TLS"] = true
			endptConfig["SSLCertificateFile"] = fmt.Sprintf("/etc/pki/tls/certs/%s.crt", endpt.String())
			endptConfig["SSLCertificateKeyFile"] = fmt.Sprintf("/etc/pki/tls/private/%s.key", endpt.String())
		}
		httpdVhostConfig[endpt.String()] = endptConfig
	}
	templateParameters["VHosts"] = httpdVhostConfig

	customData := map[string]string{
		"my.cnf": generateMyCnf(tlsCfg),
	}

	serviceLabels := labels.GetLabels(instance, labels.GetGroupLabel(cyborgservice.ServiceName), map[string]string{})

	if instance.Spec.CustomServiceConfig != "" {
		customData["01-service-custom.conf"] = instance.Spec.CustomServiceConfig
	}

	cms := []util.Template{
		{
			Name:          fmt.Sprintf("%s-config-data", instance.GetName()),
			Namespace:     instance.GetNamespace(),
			Type:          util.TemplateTypeConfig,
			InstanceType:  instance.GetObjectKind().GroupVersionKind().Kind,
			ConfigOptions: templateParameters,
			CustomData:    customData,
			Labels:        serviceLabels,
			AdditionalTemplate: map[string]string{
				"00-default.conf":          "/cyborg/00-default.conf",
				"cyborg-api-config.json":   "/cyborg/api/cyborg-api-config.json",
				"httpd.conf":               "/cyborg/api/httpd.conf",
				"ssl.conf":                 "/cyborg/api/ssl.conf",
				"10-cyborg-wsgi-main.conf": "/cyborg/api/10-cyborg-wsgi-main.conf",
			},
		},
	}

	return secret.EnsureSecrets(ctx, h, instance, cms, envVars)
}

func (r *CyborgAPIReconciler) ensureServiceExposed(
	ctx context.Context,
	h *helper.Helper,
	instance *cyborgv1beta1.CyborgAPI,
	serviceLabels map[string]string,
) (map[string]string, ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	Log.Info(fmt.Sprintf("Exposing CyborgAPI services for '%s'", instance.Name))

	ports := map[libservice.Endpoint]endpoint.Data{
		libservice.EndpointPublic: {
			Port: cyborgservice.CyborgPublicPort,
			Path: "/v2",
		},
		libservice.EndpointInternal: {
			Port: cyborgservice.CyborgInternalPort,
			Path: "/v2",
		},
	}

	apiEndpoints := make(map[string]string)

	for endpointType, data := range ports {
		endpointTypeStr := string(endpointType)
		endpointName := cyborgservice.ServiceName + "-" + endpointTypeStr
		svcOverride := instance.Spec.Override.Service[endpointType]
		if svcOverride.EmbeddedLabelsAnnotations == nil {
			svcOverride.EmbeddedLabelsAnnotations = &libservice.EmbeddedLabelsAnnotations{}
		}

		exportLabels := util.MergeStringMaps(
			serviceLabels,
			map[string]string{
				libservice.AnnotationEndpointKey: endpointTypeStr,
			},
		)

		svc, err := libservice.NewService(
			libservice.GenericService(&libservice.GenericServiceDetails{
				Name:      endpointName,
				Namespace: instance.Namespace,
				Labels:    exportLabels,
				Selector:  serviceLabels,
				Port: libservice.GenericServicePort{
					Name:     endpointName,
					Port:     data.Port,
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
				err.Error(),
			))
			return nil, ctrl.Result{}, err
		}

		svc.AddAnnotation(map[string]string{
			libservice.AnnotationEndpointKey: endpointTypeStr,
		})

		if endpointType == libservice.EndpointPublic && svc.GetServiceType() == corev1.ServiceTypeClusterIP {
			svc.AddAnnotation(map[string]string{
				libservice.AnnotationIngressCreateKey: "true",
			})
		} else {
			svc.AddAnnotation(map[string]string{
				libservice.AnnotationIngressCreateKey: "false",
			})
			if svc.GetServiceType() == corev1.ServiceTypeLoadBalancer {
				svc.AddAnnotation(map[string]string{
					libservice.AnnotationHostnameKey: svc.GetServiceHostname(),
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
				err.Error(),
			))
			return nil, ctrlResult, err
		} else if (ctrlResult != ctrl.Result{}) {
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.CreateServiceReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				condition.CreateServiceReadyRunningMessage,
			))
			return nil, ctrlResult, nil
		}

		if instance.Spec.TLS.API.Enabled(endpointType) {
			data.Protocol = ptr.To(libservice.ProtocolHTTPS)
		}

		apiEndpoints[endpointTypeStr], err = svc.GetAPIEndpoint(
			svcOverride.EndpointURL,
			data.Protocol,
			data.Path,
		)
		if err != nil {
			return nil, ctrl.Result{}, err
		}
	}

	instance.Status.Conditions.MarkTrue(condition.CreateServiceReadyCondition, condition.CreateServiceReadyMessage)

	return apiEndpoints, ctrl.Result{}, nil
}

func (r *CyborgAPIReconciler) ensureKeystoneEndpoint(
	ctx context.Context,
	h *helper.Helper,
	instance *cyborgv1beta1.CyborgAPI,
	apiEndpoints map[string]string,
	serviceLabels map[string]string,
) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	Log.Info(fmt.Sprintf("Reconciling CyborgAPI KeystoneEndpoint for '%s'", instance.Name))

	endpointSpec := keystonev1.KeystoneEndpointSpec{
		ServiceName: cyborgservice.ServiceName,
		Endpoints:   apiEndpoints,
	}

	ksEndpoint := keystonev1.NewKeystoneEndpoint(
		cyborgservice.ServiceName,
		instance.Namespace,
		endpointSpec,
		serviceLabels,
		r.RequeueTimeout,
	)

	ctrlResult, err := ksEndpoint.CreateOrPatch(ctx, h)
	if err != nil {
		return ctrlResult, err
	}

	c := ksEndpoint.GetConditions().Mirror(condition.KeystoneEndpointReadyCondition)
	if c != nil {
		instance.Status.Conditions.Set(c)
	}

	return ctrlResult, nil
}

func (r *CyborgAPIReconciler) reconcileDelete(
	ctx context.Context,
	instance *cyborgv1beta1.CyborgAPI,
	h *helper.Helper,
) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	Log.Info(fmt.Sprintf("Reconcile CyborgAPI '%s' delete started", instance.Name))

	// Remove the finalizer from the KeystoneEndpoint so the keystone operator
	// can clean it up without being blocked by our finalizer.
	ksEndpoint, err := keystonev1.GetKeystoneEndpointWithName(ctx, h, cyborgservice.ServiceName, instance.Namespace)
	if err != nil && !k8s_errors.IsNotFound(err) {
		return ctrl.Result{}, err
	}
	if err == nil {
		if controllerutil.RemoveFinalizer(ksEndpoint, h.GetFinalizer()) {
			err = h.GetClient().Update(ctx, ksEndpoint)
			if err != nil && !k8s_errors.IsNotFound(err) {
				return ctrl.Result{}, err
			}
			Log.Info("Removed finalizer from CyborgAPI KeystoneEndpoint")
		}
	}

	// Remove finalizer from the referenced Topology CR
	if _, err := topologyv1.EnsureDeletedTopologyRef(
		ctx,
		h,
		instance.Status.LastAppliedTopology,
		instance.Name,
	); err != nil {
		return ctrl.Result{}, err
	}

	controllerutil.RemoveFinalizer(instance, h.GetFinalizer())
	Log.Info(fmt.Sprintf("Reconciled CyborgAPI '%s' delete successfully", instance.Name))

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CyborgAPIReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&cyborgv1beta1.CyborgAPI{},
		apiConfigSecretField,
		func(rawObj client.Object) []string {
			cr := rawObj.(*cyborgv1beta1.CyborgAPI)
			if cr.Spec.ConfigSecret == "" {
				return nil
			}
			return []string{cr.Spec.ConfigSecret}
		},
	); err != nil {
		return err
	}

	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&cyborgv1beta1.CyborgAPI{},
		apiTopologyField,
		func(rawObj client.Object) []string {
			cr := rawObj.(*cyborgv1beta1.CyborgAPI)
			if cr.Spec.TopologyRef == nil {
				return nil
			}
			return []string{cr.Spec.TopologyRef.Name}
		},
	); err != nil {
		return err
	}

	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&cyborgv1beta1.CyborgAPI{},
		tlsAPIInternalField,
		func(rawObj client.Object) []string {
			cr := rawObj.(*cyborgv1beta1.CyborgAPI)
			if cr.Spec.TLS.API.Internal.SecretName == nil {
				return nil
			}
			return []string{*cr.Spec.TLS.API.Internal.SecretName}
		},
	); err != nil {
		return err
	}

	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&cyborgv1beta1.CyborgAPI{},
		tlsAPIPublicField,
		func(rawObj client.Object) []string {
			cr := rawObj.(*cyborgv1beta1.CyborgAPI)
			if cr.Spec.TLS.API.Public.SecretName == nil {
				return nil
			}
			return []string{*cr.Spec.TLS.API.Public.SecretName}
		},
	); err != nil {
		return err
	}

	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&cyborgv1beta1.CyborgAPI{},
		caBundleSecretNameField,
		func(rawObj client.Object) []string {
			cr := rawObj.(*cyborgv1beta1.CyborgAPI)
			if cr.Spec.TLS.CaBundleSecretName == "" {
				return nil
			}
			return []string{cr.Spec.TLS.CaBundleSecretName}
		},
	); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&cyborgv1beta1.CyborgAPI{}).
		Owns(&corev1.Secret{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Owns(&keystonev1.KeystoneEndpoint{}).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.findAPIsForSecret),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(
			&topologyv1.Topology{},
			handler.EnqueueRequestsFromMapFunc(r.findAPIsForTopology),
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		Named("cyborg-cyborgapi").
		Complete(r)
}

func (r *CyborgAPIReconciler) findAPIsForTopology(ctx context.Context, src client.Object) []reconcile.Request {
	Log := r.GetLogger(ctx)
	crList := &cyborgv1beta1.CyborgAPIList{}
	listOps := &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(apiTopologyField, src.GetName()),
		Namespace:     src.GetNamespace(),
	}
	err := r.Client.List(ctx, crList, listOps)
	if err != nil {
		Log.Error(err, "listing CyborgAPIs for topology change")
		return nil
	}

	requests := make([]reconcile.Request, 0, len(crList.Items))
	for _, item := range crList.Items {
		Log.Info(fmt.Sprintf("Topology %s changed, reconciling CyborgAPI %s", src.GetName(), item.GetName()))
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      item.GetName(),
				Namespace: item.GetNamespace(),
			},
		})
	}
	return requests
}

func (r *CyborgAPIReconciler) findAPIsForSecret(ctx context.Context, src client.Object) []reconcile.Request {
	Log := r.GetLogger(ctx)
	var requests []reconcile.Request

	for _, field := range apiSecretWatchFields {
		crList := &cyborgv1beta1.CyborgAPIList{}
		listOps := &client.ListOptions{
			FieldSelector: fields.OneTermEqualSelector(field, src.GetName()),
			Namespace:     src.GetNamespace(),
		}
		if err := r.Client.List(ctx, crList, listOps); err != nil {
			Log.Error(err, "listing CyborgAPIs for secret change", "field", field)
			continue
		}
		for _, item := range crList.Items {
			Log.Info(fmt.Sprintf("Secret %s changed, reconciling CyborgAPI %s", src.GetName(), item.GetName()))
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      item.GetName(),
					Namespace: item.GetNamespace(),
				},
			})
		}
	}
	return requests
}
