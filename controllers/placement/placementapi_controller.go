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

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
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
	networkv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	common "github.com/openstack-k8s-operators/lib-common/modules/common"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	deployment "github.com/openstack-k8s-operators/lib-common/modules/common/deployment"
	endpoint "github.com/openstack-k8s-operators/lib-common/modules/common/endpoint"
	env "github.com/openstack-k8s-operators/lib-common/modules/common/env"
	helper "github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	job "github.com/openstack-k8s-operators/lib-common/modules/common/job"
	labels "github.com/openstack-k8s-operators/lib-common/modules/common/labels"
	nad "github.com/openstack-k8s-operators/lib-common/modules/common/networkattachment"
	common_rbac "github.com/openstack-k8s-operators/lib-common/modules/common/rbac"
	"github.com/openstack-k8s-operators/lib-common/modules/common/secret"
	"github.com/openstack-k8s-operators/lib-common/modules/common/service"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"
	util "github.com/openstack-k8s-operators/lib-common/modules/common/util"

	mariadbv1 "github.com/openstack-k8s-operators/mariadb-operator/api/v1beta1"

	placementv1 "github.com/openstack-k8s-operators/nova-operator/apis/placement/v1beta1"
	placement "github.com/openstack-k8s-operators/nova-operator/pkg/placement"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
)

type conditionUpdater interface {
	Set(c *condition.Condition)
	MarkTrue(t condition.Type, messageFormat string, messageArgs ...interface{})
}

type GetSecret interface {
	GetSecret() string
	client.Object
}

// ensureSecret - ensures that the Secret object exists and the expected fields
// are in the Secret. It returns a hash of the values of the expected fields.
func ensureSecret(
	ctx context.Context,
	secretName types.NamespacedName,
	expectedFields []string,
	reader client.Reader,
	conditionUpdater conditionUpdater,
) (string, ctrl.Result, corev1.Secret, error) {
	secret := &corev1.Secret{}
	err := reader.Get(ctx, secretName, secret)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			conditionUpdater.Set(condition.FalseCondition(
				condition.InputReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				fmt.Sprintf("Input data resources missing: %s", "secret/"+secretName.Name)))
			return "",
				ctrl.Result{},
				*secret,
				fmt.Errorf("%w: Secret %s not found", err, secretName)
		}
		conditionUpdater.Set(condition.FalseCondition(
			condition.InputReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.InputReadyErrorMessage,
			err.Error()))
		return "", ctrl.Result{}, *secret, err
	}

	// collect the secret values the caller expects to exist
	values := [][]byte{}
	for _, field := range expectedFields {
		val, ok := secret.Data[field]
		if !ok {
			err := fmt.Errorf("%w: field '%s' not found in secret/%s", util.ErrFieldNotFound, field, secretName.Name)
			conditionUpdater.Set(condition.FalseCondition(
				condition.InputReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.InputReadyErrorMessage,
				err.Error()))
			return "", ctrl.Result{}, *secret, err
		}
		values = append(values, val)
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
		return "", ctrl.Result{}, *secret, err
	}

	return hash, ctrl.Result{}, *secret, nil
}

// GetLogger returns a logger object with a prefix of "controller.name" and additional controller context fields
func (r *PlacementAPIReconciler) GetLogger(ctx context.Context) logr.Logger {
	return log.FromContext(ctx).WithName("Controllers").WithName("PlacementAPI")
}

// PlacementAPIReconciler reconciles a PlacementAPI object
type PlacementAPIReconciler struct {
	client.Client
	Kclient kubernetes.Interface
	Scheme  *runtime.Scheme
}

// +kubebuilder:rbac:groups=placement.openstack.org,resources=placementapis,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=placement.openstack.org,resources=placementapis/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=placement.openstack.org,resources=placementapis/finalizers,verbs=update;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=mariadb.openstack.org,resources=mariadbdatabases,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=mariadb.openstack.org,resources=mariadbdatabases/finalizers,verbs=update;patch
// +kubebuilder:rbac:groups=mariadb.openstack.org,resources=mariadbaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mariadb.openstack.org,resources=mariadbaccounts/finalizers,verbs=update;patch
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneapis,verbs=get;list;watch;
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneservices,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneendpoints,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=k8s.cni.cncf.io,resources=network-attachment-definitions,verbs=get;list;watch

// service account, role, rolebinding
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=roles,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=rolebindings,verbs=get;list;watch;create;update;patch
// service account permissions that are needed to grant permission to the above
// +kubebuilder:rbac:groups="security.openshift.io",resourceNames=anyuid,resources=securitycontextconstraints,verbs=use
// +kubebuilder:rbac:groups="",resources=pods,verbs=create;delete;get;list;patch;update;watch
// +kubebuilder:rbac:groups=topology.openstack.org,resources=topologies,verbs=get;list;watch;update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the PlacementAPI object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *PlacementAPIReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, _err error) {
	Log := r.GetLogger(ctx)

	// Fetch the PlacementAPI instance
	instance := &placementv1.PlacementAPI{}
	err := r.Client.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected.
			// For additional cleanup logic use finalizers. Return and don't requeue.
			Log.Info("Placement instance not found, probably deleted before reconciled. Nothing to do.")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		Log.Error(err, "Failed to read the Placement instance.")
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

	// Save a copy of the condtions so that we can restore the LastTransitionTime
	// when a condition's state doesn't change.
	savedConditions := instance.Status.Conditions.DeepCopy()
	// initialize status fields
	if err = r.initStatus(instance); err != nil {
		return ctrl.Result{}, err
	}
	instance.Status.ObservedGeneration = instance.Generation

	// Always patch the instance status when exiting this function so we can persist any changes.
	defer func() {
		// Don't update the status, if reconciler Panics
		if r := recover(); r != nil {
			Log.Info(fmt.Sprintf("panic during reconcile %v\n", r))
			panic(r)
		}
		// update the Ready condition based on the sub conditions
		if instance.Status.Conditions.AllSubConditionIsTrue() {
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

	// Handle service delete
	if !instance.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, instance, h)
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
	// ConfigMap
	configMapVars := make(map[string]env.Setter)

	//
	// check for required OpenStack secret holding passwords for service/admin user and add hash to the vars map
	//
	hash, result, secret, err := ensureSecret(
		ctx,
		types.NamespacedName{Namespace: instance.Namespace, Name: instance.Spec.Secret},
		[]string{
			instance.Spec.PasswordSelectors.Service,
		},
		h.GetClient(),
		&instance.Status.Conditions)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			Log.Info(fmt.Sprintf("OpenStack secret %s not found", instance.Spec.Secret))
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.InputReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				condition.InputReadyWaitingMessage))
			return ctrl.Result{RequeueAfter: time.Second * 10}, nil
		}
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.InputReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.InputReadyErrorMessage,
			err.Error()))
		return result, err
	}
	configMapVars[instance.Spec.Secret] = env.SetValue(hash)

	// all our input checks out so report InputReady
	instance.Status.Conditions.MarkTrue(condition.InputReadyCondition, condition.InputReadyMessage)

	// ensure MariaDBAccount exists.  This account record may be created by
	// openstack-operator or the cloud operator up front without a specific
	// MariaDBDatabase configured yet.   Otherwise, a MariaDBAccount CR is
	// created here with a generated username as well as a secret with
	// generated password.   The MariaDBAccount is created without being
	// yet associated with any MariaDBDatabase.
	_, _, err = mariadbv1.EnsureMariaDBAccount(
		ctx, h, instance.Spec.DatabaseAccount,
		instance.Namespace, false, placement.DatabaseName,
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

	db, result, err := r.ensureDB(ctx, h, instance)
	if err != nil {
		return ctrl.Result{}, err
	} else if (result != ctrl.Result{}) {
		return result, nil
	}

	err = r.generateServiceConfigMaps(ctx, h, instance, secret, &configMapVars, db)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ServiceConfigReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.ServiceConfigReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}

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
				instance.Status.Conditions.Set(condition.FalseCondition(
					condition.TLSInputReadyCondition,
					condition.RequestedReason,
					condition.SeverityInfo,
					fmt.Sprintf(condition.TLSInputReadyWaitingMessage, instance.Spec.TLS.CaBundleSecretName)))
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
			configMapVars[tls.CABundleKey] = env.SetValue(hash)
		}
	}

	// Validate API service certs secrets
	certsHash, err := instance.Spec.TLS.API.ValidateCertSecrets(ctx, h, instance.Namespace)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.TLSInputReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				fmt.Sprintf(condition.TLSInputReadyWaitingMessage, err.Error())))
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
	configMapVars[tls.TLSHashName] = env.SetValue(certsHash)

	instance.Status.Conditions.MarkTrue(condition.TLSInputReadyCondition, condition.InputReadyMessage)

	// create hash over all the different input resources to identify if any those changed
	// and a restart/recreate is required.
	//
	inputHash, hashChanged, err := r.createHashOfInputHashes(ctx, instance, configMapVars)
	if err != nil {
		return ctrl.Result{}, err
	} else if hashChanged {
		// Hash changed and instance status should be updated (which will be done by main defer func),
		// so we need to return and reconcile again
		return ctrl.Result{}, nil
	}

	instance.Status.Conditions.MarkTrue(condition.ServiceConfigReadyCondition, condition.ServiceConfigReadyMessage)

	serviceAnnotations, result, err := r.ensureNetworkAttachments(ctx, h, instance)
	if (err != nil || result != ctrl.Result{}) {
		return result, err
	}

	apiEndpoints, result, err := r.ensureServiceExposed(ctx, h, instance)

	if (err != nil || result != ctrl.Result{}) {
		// We can ignore RequeueAfter as we are watching the Service resource
		// but we have to return while waiting for the service to be exposed
		return ctrl.Result{}, err
	}

	result, err = r.ensureDbSync(ctx, instance, h, serviceAnnotations)
	if (err != nil || result != ctrl.Result{}) {
		return result, err
	}

	result, err = r.ensureDeployment(ctx, h, instance, inputHash, serviceAnnotations)
	if (err != nil || result != ctrl.Result{}) {
		return result, err
	}

	// Only expose the service is the deployment succeeded
	if !instance.Status.Conditions.IsTrue(condition.DeploymentReadyCondition) {
		Log.Info("Waiting for the Deployment to become Ready before exposing the sevice in Keystone")
		return ctrl.Result{}, nil
	}
	err = r.ensureKeystoneServiceUser(ctx, h, instance)
	if err != nil {
		return ctrl.Result{}, err
	}

	result, err = r.ensureKeystoneEndpoint(ctx, h, instance, apiEndpoints)
	if (err != nil || result != ctrl.Result{}) {
		// We can ignore RequeueAfter as we are watching the KeystoneEndpoint resource
		return ctrl.Result{}, err
	}

	// remove finalizers from unused MariaDBAccount records
	err = mariadbv1.DeleteUnusedMariaDBAccountFinalizers(ctx, h, placement.DatabaseName, instance.Spec.DatabaseAccount, instance.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func getServiceLabels(instance *placementv1.PlacementAPI) map[string]string {
	return map[string]string{
		common.AppSelector:   placement.ServiceName,
		common.OwnerSelector: instance.Name,
	}
}

func (r *PlacementAPIReconciler) ensureServiceExposed(
	ctx context.Context,
	h *helper.Helper,
	instance *placementv1.PlacementAPI,
) (map[string]string, ctrl.Result, error) {
	placementEndpoints := map[service.Endpoint]endpoint.Data{
		service.EndpointPublic:   {Port: placement.PlacementPublicPort},
		service.EndpointInternal: {Port: placement.PlacementInternalPort},
	}
	apiEndpoints := make(map[string]string)

	serviceLabels := getServiceLabels(instance)
	for endpointType, data := range placementEndpoints {
		endpointTypeStr := string(endpointType)
		endpointName := placement.ServiceName + "-" + endpointTypeStr

		svcOverride := instance.Spec.Override.Service[endpointType]
		if svcOverride.EmbeddedLabelsAnnotations == nil {
			svcOverride.EmbeddedLabelsAnnotations = &service.EmbeddedLabelsAnnotations{}
		}

		exportLabels := util.MergeStringMaps(
			serviceLabels,
			map[string]string{
				service.AnnotationEndpointKey: endpointTypeStr,
			},
		)

		// Create the service
		svc, err := service.NewService(
			service.GenericService(&service.GenericServiceDetails{
				Name:      endpointName,
				Namespace: instance.Namespace,
				Labels:    exportLabels,
				Selector:  serviceLabels,
				Port: service.GenericServicePort{
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
				err.Error()))

			return apiEndpoints, ctrl.Result{}, err
		}

		svc.AddAnnotation(map[string]string{
			service.AnnotationEndpointKey: endpointTypeStr,
		})

		// add Annotation to whether creating an ingress is required or not
		if endpointType == service.EndpointPublic && svc.GetServiceType() == corev1.ServiceTypeClusterIP {
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

			return apiEndpoints, ctrlResult, err
		} else if (ctrlResult != ctrl.Result{}) {
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.CreateServiceReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				condition.CreateServiceReadyRunningMessage))
			return apiEndpoints, ctrlResult, nil
		}
		// create service - end

		// if TLS is enabled
		if instance.Spec.TLS.API.Enabled(endpointType) {
			// set endpoint protocol to https
			data.Protocol = ptr.To(service.ProtocolHTTPS)
		}

		apiEndpoints[string(endpointType)], err = svc.GetAPIEndpoint(
			svcOverride.EndpointURL, data.Protocol, data.Path)
		if err != nil {
			return apiEndpoints, ctrl.Result{}, err
		}
	}

	instance.Status.Conditions.MarkTrue(condition.CreateServiceReadyCondition, condition.CreateServiceReadyMessage)
	return apiEndpoints, ctrl.Result{}, nil
}

func (r *PlacementAPIReconciler) ensureNetworkAttachments(
	ctx context.Context,
	h *helper.Helper,
	instance *placementv1.PlacementAPI,
) (map[string]string, ctrl.Result, error) {
	var nadAnnotations map[string]string
	var err error

	// networks to attach to
	nadList := []networkv1.NetworkAttachmentDefinition{}
	for _, netAtt := range instance.Spec.NetworkAttachments {
		nad, err := nad.GetNADWithName(ctx, h, netAtt, instance.Namespace)
		if err != nil {
			if k8s_errors.IsNotFound(err) {
				r.GetLogger(ctx).Info(fmt.Sprintf("network-attachment-definition %s not found", netAtt))
				instance.Status.Conditions.Set(condition.FalseCondition(
					condition.NetworkAttachmentsReadyCondition,
					condition.RequestedReason,
					condition.SeverityInfo,
					condition.NetworkAttachmentsReadyWaitingMessage,
					netAtt))
				return nadAnnotations, ctrl.Result{RequeueAfter: time.Second * 10}, nil
			}
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.NetworkAttachmentsReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.NetworkAttachmentsReadyErrorMessage,
				err.Error()))
			return nadAnnotations, ctrl.Result{}, err
		}

		if nad != nil {
			nadList = append(nadList, *nad)
		}
	}

	nadAnnotations, err = nad.EnsureNetworksAnnotation(nadList)
	if err != nil {
		return nadAnnotations, ctrl.Result{}, fmt.Errorf("failed create network annotation from %s: %w",
			instance.Spec.NetworkAttachments, err)
	}
	return nadAnnotations, ctrl.Result{}, nil
}

func (r *PlacementAPIReconciler) ensureKeystoneServiceUser(
	ctx context.Context,
	h *helper.Helper,
	instance *placementv1.PlacementAPI,
) error {
	//
	// create service and user in keystone - https://docs.openstack.org/placement/latest/install/install-rdo.html#configure-user-and-endpoints
	//
	ksSvcSpec := keystonev1.KeystoneServiceSpec{
		ServiceType:        placement.ServiceName,
		ServiceName:        placement.ServiceName,
		ServiceDescription: "Placement Service",
		Enabled:            true,
		ServiceUser:        instance.Spec.ServiceUser,
		Secret:             instance.Spec.Secret,
		PasswordSelector:   instance.Spec.PasswordSelectors.Service,
	}
	serviceLabels := getServiceLabels(instance)
	ksSvc := keystonev1.NewKeystoneService(ksSvcSpec, instance.Namespace, serviceLabels, time.Duration(10)*time.Second)
	_, err := ksSvc.CreateOrPatch(ctx, h)
	if err != nil {
		return err
	}
	// mirror the Status, Reason, Severity and Message of the latest keystoneservice condition
	// into a local condition with the type condition.KeystoneServiceReadyCondition
	c := ksSvc.GetConditions().Mirror(condition.KeystoneServiceReadyCondition)
	if c != nil {
		instance.Status.Conditions.Set(c)
	}

	return nil
}

func (r *PlacementAPIReconciler) ensureKeystoneEndpoint(
	ctx context.Context,
	h *helper.Helper,
	instance *placementv1.PlacementAPI,
	apiEndpoints map[string]string,
) (ctrl.Result, error) {
	ksEndptSpec := keystonev1.KeystoneEndpointSpec{
		ServiceName: placement.ServiceName,
		Endpoints:   apiEndpoints,
	}
	ksEndpt := keystonev1.NewKeystoneEndpoint(
		placement.ServiceName,
		instance.Namespace,
		ksEndptSpec,
		getServiceLabels(instance),
		time.Duration(10)*time.Second,
	)
	ctrlResult, err := ksEndpt.CreateOrPatch(ctx, h)
	if err != nil {
		return ctrlResult, err
	}
	// mirror the Status, Reason, Severity and Message of the latest keystoneendpoint condition
	// into a local condition with the type condition.KeystoneEndpointReadyCondition
	c := ksEndpt.GetConditions().Mirror(condition.KeystoneEndpointReadyCondition)
	if c != nil {
		instance.Status.Conditions.Set(c)
	}

	if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	return ctrlResult, nil
}

func (r *PlacementAPIReconciler) initStatus(
	instance *placementv1.PlacementAPI,
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

func (r *PlacementAPIReconciler) initConditions(
	instance *placementv1.PlacementAPI,
) error {
	if instance.Status.Conditions == nil {
		instance.Status.Conditions = condition.Conditions{}
	}
	// initialize conditions used later as Status=Unknown
	cl := condition.CreateList(
		condition.UnknownCondition(
			condition.DBReadyCondition,
			condition.InitReason,
			condition.DBReadyInitMessage,
		),
		condition.UnknownCondition(
			condition.DBSyncReadyCondition,
			condition.InitReason,
			condition.DBSyncReadyInitMessage,
		),
		condition.UnknownCondition(
			condition.CreateServiceReadyCondition,
			condition.InitReason,
			condition.CreateServiceReadyInitMessage,
		),
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
		// right now we have no dedicated KeystoneServiceReadyInitMessage and KeystoneEndpointReadyInitMessage
		condition.UnknownCondition(
			condition.KeystoneServiceReadyCondition,
			condition.InitReason,
			"Service registration not started",
		),
		condition.UnknownCondition(
			condition.KeystoneEndpointReadyCondition,
			condition.InitReason,
			"KeystoneEndpoint not created",
		),
		condition.UnknownCondition(
			condition.NetworkAttachmentsReadyCondition,
			condition.InitReason,
			condition.NetworkAttachmentsReadyInitMessage,
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
			condition.RoleBindingReadyInitMessage),
		condition.UnknownCondition(
			condition.TLSInputReadyCondition,
			condition.InitReason,
			condition.InputReadyInitMessage),
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

// fields to index to reconcile when change
const (
	passwordSecretField     = ".spec.secret"
	caBundleSecretNameField = ".spec.tls.caBundleSecretName" // #nosec G101
	tlsAPIInternalField     = ".spec.tls.api.internal.secretName"
	tlsAPIPublicField       = ".spec.tls.api.public.secretName"
	topologyField           = ".spec.topologyRef.Name"
)

var allWatchFields = []string{
	passwordSecretField,
	caBundleSecretNameField,
	tlsAPIInternalField,
	tlsAPIPublicField,
	topologyField,
}

// SetupWithManager sets up the controller with the Manager.
func (r *PlacementAPIReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// index passwordSecretField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &placementv1.PlacementAPI{}, passwordSecretField, func(rawObj client.Object) []string {
		// Extract the secret name from the spec, if one is provided
		cr := rawObj.(*placementv1.PlacementAPI)
		if cr.Spec.Secret == "" {
			return nil
		}
		return []string{cr.Spec.Secret}
	}); err != nil {
		return err
	}

	// index caBundleSecretNameField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &placementv1.PlacementAPI{}, caBundleSecretNameField, func(rawObj client.Object) []string {
		// Extract the secret name from the spec, if one is provided
		cr := rawObj.(*placementv1.PlacementAPI)
		if cr.Spec.TLS.CaBundleSecretName == "" {
			return nil
		}
		return []string{cr.Spec.TLS.CaBundleSecretName}
	}); err != nil {
		return err
	}

	// index tlsAPIInternalField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &placementv1.PlacementAPI{}, tlsAPIInternalField, func(rawObj client.Object) []string {
		// Extract the secret name from the spec, if one is provided
		cr := rawObj.(*placementv1.PlacementAPI)
		if cr.Spec.TLS.API.Internal.SecretName == nil {
			return nil
		}
		return []string{*cr.Spec.TLS.API.Internal.SecretName}
	}); err != nil {
		return err
	}
	// index tlsAPIPublicField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &placementv1.PlacementAPI{}, tlsAPIPublicField, func(rawObj client.Object) []string {
		// Extract the secret name from the spec, if one is provided
		cr := rawObj.(*placementv1.PlacementAPI)
		if cr.Spec.TLS.API.Public.SecretName == nil {
			return nil
		}
		return []string{*cr.Spec.TLS.API.Public.SecretName}
	}); err != nil {
		return err
	}

	// index topologyField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &placementv1.PlacementAPI{}, topologyField, func(rawObj client.Object) []string {
		// Extract the topology name from the spec, if one is provided
		cr := rawObj.(*placementv1.PlacementAPI)
		if cr.Spec.TopologyRef == nil {
			return nil
		}
		return []string{cr.Spec.TopologyRef.Name}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&placementv1.PlacementAPI{}).
		Owns(&mariadbv1.MariaDBDatabase{}).
		Owns(&mariadbv1.MariaDBAccount{}).
		Owns(&keystonev1.KeystoneService{}).
		Owns(&keystonev1.KeystoneEndpoint{}).
		Owns(&batchv1.Job{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.findObjectsForSrc),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(&topologyv1.Topology{},
			handler.EnqueueRequestsFromMapFunc(r.findObjectsForSrc),
			builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(&keystonev1.KeystoneAPI{},
			handler.EnqueueRequestsFromMapFunc(r.findObjectForSrc),
			builder.WithPredicates(keystonev1.KeystoneAPIStatusChangedPredicate)).
		Complete(r)
}

func (r *PlacementAPIReconciler) findObjectsForSrc(ctx context.Context, src client.Object) []reconcile.Request {
	requests := []reconcile.Request{}

	l := log.FromContext(context.Background()).WithName("Controllers").WithName("PlacementAPI")

	for _, field := range allWatchFields {
		crList := &placementv1.PlacementAPIList{}
		listOps := &client.ListOptions{
			FieldSelector: fields.OneTermEqualSelector(field, src.GetName()),
			Namespace:     src.GetNamespace(),
		}
		err := r.List(ctx, crList, listOps)
		if err != nil {
			l.Error(err, fmt.Sprintf("listing %s for field: %s - %s", crList.GroupVersionKind().Kind, field, src.GetNamespace()))
			return requests
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

func (r *PlacementAPIReconciler) findObjectForSrc(ctx context.Context, src client.Object) []reconcile.Request {
	requests := []reconcile.Request{}

	l := log.FromContext(ctx).WithName("Controllers").WithName("PlacementAPI")

	crList := &placementv1.PlacementAPIList{}
	listOps := &client.ListOptions{
		Namespace: src.GetNamespace(),
	}
	err := r.Client.List(ctx, crList, listOps)
	if err != nil {
		l.Error(err, fmt.Sprintf("listing %s for namespace: %s", crList.GroupVersionKind().Kind, src.GetNamespace()))
		return requests
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

	return requests
}

func (r *PlacementAPIReconciler) reconcileDelete(ctx context.Context, instance *placementv1.PlacementAPI, helper *helper.Helper) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	Log.Info("Reconciling Service delete")

	// Remove finalizer from the referenced Topology CR
	if ctrlResult, err := topologyv1.EnsureDeletedTopologyRef(
		ctx,
		helper,
		instance.Status.LastAppliedTopology,
		instance.Name,
	); err != nil {
		return ctrlResult, err
	}

	// remove db finalizer before the placement one
	db, err := mariadbv1.GetDatabaseByNameAndAccount(ctx, helper, placement.DatabaseName, instance.Spec.DatabaseAccount, instance.Namespace)
	if err != nil && !k8s_errors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	if !k8s_errors.IsNotFound(err) {
		if err := db.DeleteFinalizer(ctx, helper); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Remove the finalizer from our KeystoneEndpoint CR
	keystoneEndpoint, err := keystonev1.GetKeystoneEndpointWithName(ctx, helper, placement.ServiceName, instance.Namespace)
	if err != nil && !k8s_errors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	if err == nil {
		if controllerutil.RemoveFinalizer(keystoneEndpoint, helper.GetFinalizer()) {
			err = r.Update(ctx, keystoneEndpoint)
			if err != nil && !k8s_errors.IsNotFound(err) {
				return ctrl.Result{}, err
			}
			Log.Info("Removed finalizer from our KeystoneEndpoint")
		}
	}

	// Remove the finalizer from our KeystoneService CR
	keystoneService, err := keystonev1.GetKeystoneServiceWithName(ctx, helper, placement.ServiceName, instance.Namespace)
	if err != nil && !k8s_errors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	if err == nil {
		if controllerutil.RemoveFinalizer(keystoneService, helper.GetFinalizer()) {
			err = r.Update(ctx, keystoneService)
			if err != nil && !k8s_errors.IsNotFound(err) {
				return ctrl.Result{}, err
			}
			Log.Info("Removed finalizer from our KeystoneService")
		}
	}

	// We did all the cleanup on the objects we created so we can remove the
	// finalizer from ourselves to allow the deletion
	controllerutil.RemoveFinalizer(instance, helper.GetFinalizer())
	Log.Info("Reconciled Service delete successfully")
	return ctrl.Result{}, nil
}

func (r *PlacementAPIReconciler) ensureDB(
	ctx context.Context,
	h *helper.Helper,
	instance *placementv1.PlacementAPI,
) (*mariadbv1.Database, ctrl.Result, error) {
	db := mariadbv1.NewDatabaseForAccount(
		instance.Spec.DatabaseInstance, // mariadb/galera service to target
		placement.DatabaseName,         // name used in CREATE DATABASE in mariadb
		placement.DatabaseName,         // CR name for MariaDBDatabase
		instance.Spec.DatabaseAccount,  // CR name for MariaDBAccount
		instance.Namespace,             // namespace
	)

	// create or patch the DB
	ctrlResult, err := db.CreateOrPatchAll(ctx, h)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.DBReadyErrorMessage,
			err.Error()))
		return db, ctrl.Result{}, err
	}
	if (ctrlResult != ctrl.Result{}) {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DBReadyRunningMessage))
		return db, ctrlResult, nil
	}
	// wait for the DB to be setup
	// (ksambor) should we use WaitForDBCreatedWithTimeout instead?
	ctrlResult, err = db.WaitForDBCreated(ctx, h)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.DBReadyErrorMessage,
			err.Error()))
		return db, ctrlResult, err
	}
	if (ctrlResult != ctrl.Result{}) {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DBReadyRunningMessage))
		return db, ctrlResult, nil
	}

	// update Status.DatabaseHostname, used to config the service
	instance.Status.DatabaseHostname = db.GetDatabaseHostname()
	instance.Status.Conditions.MarkTrue(condition.DBReadyCondition, condition.DBReadyMessage)
	return db, ctrlResult, nil
}

func (r *PlacementAPIReconciler) ensureDbSync(
	ctx context.Context,
	instance *placementv1.PlacementAPI,
	helper *helper.Helper,
	serviceAnnotations map[string]string,
) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	serviceLabels := getServiceLabels(instance)
	dbSyncHash := instance.Status.Hash[placementv1.DbSyncHash]
	jobDef := placement.DbSyncJob(instance, serviceLabels, serviceAnnotations)
	dbSyncjob := job.NewJob(
		jobDef,
		placementv1.DbSyncHash,
		instance.Spec.PreserveJobs,
		time.Duration(5)*time.Second,
		dbSyncHash,
	)
	ctrlResult, err := dbSyncjob.DoJob(
		ctx,
		helper,
	)
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
	if dbSyncjob.HasChanged() {
		instance.Status.Hash[placementv1.DbSyncHash] = dbSyncjob.GetHash()
		Log.Info(fmt.Sprintf("Job %s hash added - %s", jobDef.Name, instance.Status.Hash[placementv1.DbSyncHash]))
	}
	instance.Status.Conditions.MarkTrue(condition.DBSyncReadyCondition, condition.DBSyncReadyMessage)

	return ctrl.Result{}, nil
}

func (r *PlacementAPIReconciler) ensureDeployment(
	ctx context.Context,
	h *helper.Helper,
	instance *placementv1.PlacementAPI,
	inputHash string,
	serviceAnnotations map[string]string,
) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	Log.Info("Reconciling Service")

	serviceLabels := getServiceLabels(instance)

	//
	// Handle Topology
	//
	topology, err := topologyv1.EnsureServiceTopology(
		ctx,
		h,
		instance.Spec.TopologyRef,
		instance.Status.LastAppliedTopology,
		instance.Name,
		labels.GetLabelSelector(serviceLabels),
	)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.TopologyReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.TopologyReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, fmt.Errorf("waiting for Topology requirements: %w", err)
	}

	// If TopologyRef is present and ensureServiceTopology returned a valid
	// topology object, set .Status.LastAppliedTopology to the referenced one
	// and mark the condition as true
	if instance.Spec.TopologyRef != nil {
		// update the Status with the last retrieved Topology name
		instance.Status.LastAppliedTopology = instance.Spec.TopologyRef
		// update the TopologyRef associated condition
		instance.Status.Conditions.MarkTrue(condition.TopologyReadyCondition, condition.TopologyReadyMessage)
	} else {
		// remove LastAppliedTopology from the .Status
		instance.Status.LastAppliedTopology = nil
	}

	// Define a new Deployment object
	deplDef, err := placement.Deployment(instance, inputHash, serviceLabels, serviceAnnotations, topology)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DeploymentReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.DeploymentReadyErrorMessage,
			err.Error()))
	}

	depl := deployment.NewDeployment(
		deplDef,
		time.Duration(5)*time.Second,
	)

	ctrlResult, err := depl.CreateOrPatch(ctx, h)
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
		return ctrl.Result{}, nil
	}

	deploy := depl.GetDeployment()
	if deploy.Generation == deploy.Status.ObservedGeneration {
		instance.Status.ReadyCount = deploy.Status.ReadyReplicas
	}

	// verify if network attachment matches expectations
	networkReady, networkAttachmentStatus, err := nad.VerifyNetworkStatusFromAnnotation(ctx, h, instance.Spec.NetworkAttachments, serviceLabels, instance.Status.ReadyCount)
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

	// Mark the Deployment as Ready only if the number of Replicas is equals
	// to the Deployed instances (ReadyCount), and the the Status.Replicas
	// match Status.ReadyReplicas. If a deployment update is in progress,
	// Replicas > ReadyReplicas.
	// In addition, make sure the controller sees the last Generation
	// by comparing it with the ObservedGeneration.
	if deployment.IsReady(deploy) {
		instance.Status.Conditions.MarkTrue(condition.DeploymentReadyCondition, condition.DeploymentReadyMessage)
	} else {
		Log.Info("Deployment is not ready")
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DeploymentReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DeploymentReadyRunningMessage))
		// It is OK to return success as we are watching for StatefulSet changes
		return ctrl.Result{}, nil
	}
	// create Deployment - end

	Log.Info("Reconciled Service successfully")
	return ctrl.Result{}, nil

}

// generateServiceConfigMaps - create create configmaps which hold scripts and service configuration
func (r *PlacementAPIReconciler) generateServiceConfigMaps(
	ctx context.Context,
	h *helper.Helper,
	instance *placementv1.PlacementAPI,
	ospSecret corev1.Secret,
	envVars *map[string]env.Setter,
	db *mariadbv1.Database,
) error {
	//
	// create Secret required for placement input
	// - %-scripts secret holding scripts to e.g. bootstrap the service
	// - %-config secret holding minimal placement config required to get the service up, user can add additional files to be added to the service
	// - parameters which has passwords gets added from the ospSecret via the init container
	//

	cmLabels := labels.GetLabels(instance, labels.GetGroupLabel(placement.ServiceName), map[string]string{})

	var tlsCfg *tls.Service
	if instance.Spec.TLS.Ca.CaBundleSecretName != "" {
		tlsCfg = &tls.Service{}
	}

	// customData hold any customization for the service.
	// custom.conf is going to /etc/<service>/<service>.conf.d
	// my.cnf is going to /etc/my.cnf
	// all other files get placed into /etc/<service> to allow overwrite of e.g. policy.json
	customData := map[string]string{
		common.CustomServiceConfigFileName: instance.Spec.CustomServiceConfig,
		"my.cnf":                           db.GetDatabaseClientConfig(tlsCfg), //(mschuppert) for now just get the default my.cnf
	}
	for key, data := range instance.Spec.DefaultConfigOverwrite {
		customData[key] = data
	}

	keystoneAPI, err := keystonev1.GetKeystoneAPI(ctx, h, instance.Namespace, map[string]string{})
	if err != nil {
		return err
	}
	keystoneInternalURL, err := keystoneAPI.GetEndpoint(endpoint.EndpointInternal)
	if err != nil {
		return err
	}
	keystonePublicURL, err := keystoneAPI.GetEndpoint(endpoint.EndpointPublic)
	if err != nil {
		return err
	}

	databaseAccount := db.GetAccount()
	dbSecret := db.GetSecret()

	templateParameters := map[string]interface{}{
		"ServiceUser":         instance.Spec.ServiceUser,
		"KeystoneInternalURL": keystoneInternalURL,
		"KeystonePublicURL":   keystonePublicURL,
		"PlacementPassword":   string(ospSecret.Data[instance.Spec.PasswordSelectors.Service]),
		"log_file":            "/var/log/placement/placement-api.log",
		"DatabaseConnection": fmt.Sprintf("mysql+pymysql://%s:%s@%s/%s?read_default_file=/etc/my.cnf",
			databaseAccount.Spec.UserName,
			string(dbSecret.Data[mariadbv1.DatabasePasswordSelector]),
			instance.Status.DatabaseHostname,
			placement.DatabaseName,
		),
	}

	// create httpd  vhost template parameters
	httpdVhostConfig := map[string]interface{}{}
	for _, endpt := range []service.Endpoint{service.EndpointInternal, service.EndpointPublic} {
		endptConfig := map[string]interface{}{}
		endptConfig["ServerName"] = fmt.Sprintf("placement-%s.%s.svc", endpt.String(), instance.Namespace)
		endptConfig["TLS"] = false // default TLS to false, and set it bellow to true if enabled
		if instance.Spec.TLS.API.Enabled(endpt) {
			endptConfig["TLS"] = true
			endptConfig["SSLCertificateFile"] = fmt.Sprintf("/etc/pki/tls/certs/%s.crt", endpt.String())
			endptConfig["SSLCertificateKeyFile"] = fmt.Sprintf("/etc/pki/tls/private/%s.key", endpt.String())
		}
		httpdVhostConfig[endpt.String()] = endptConfig
	}
	templateParameters["VHosts"] = httpdVhostConfig
	templateParameters["TimeOut"] = instance.Spec.APITimeout

	extraTemplates := map[string]string{
		"placement.conf": "placementapi/config/placement.conf",
	}

	cms := []util.Template{
		// ScriptsConfigMap
		{
			Name:         fmt.Sprintf("%s-scripts", instance.Name),
			Namespace:    instance.Namespace,
			Type:         util.TemplateTypeScripts,
			InstanceType: instance.Kind,
			Labels:       cmLabels,
		},
		// ConfigMap
		{
			Name:               fmt.Sprintf("%s-config-data", instance.Name),
			Namespace:          instance.Namespace,
			Type:               util.TemplateTypeConfig,
			InstanceType:       instance.Kind,
			CustomData:         customData,
			ConfigOptions:      templateParameters,
			Labels:             cmLabels,
			AdditionalTemplate: extraTemplates,
		},
	}
	return secret.EnsureSecrets(ctx, h, instance, cms, envVars)
}

// createHashOfInputHashes - creates a hash of hashes which gets added to the resources which requires a restart
// if any of the input resources change, like configs, passwords, ...
//
// returns the hash, whether the hash changed (as a bool) and any error
func (r *PlacementAPIReconciler) createHashOfInputHashes(
	ctx context.Context,
	instance *placementv1.PlacementAPI,
	envVars map[string]env.Setter,
) (string, bool, error) {
	Log := r.GetLogger(ctx)
	var hashMap map[string]string
	changed := false
	mergedMapVars := env.MergeEnvs([]corev1.EnvVar{}, envVars)
	hash, err := util.ObjectHash(mergedMapVars)
	if err != nil {
		return hash, changed, err
	}
	if hashMap, changed = util.SetHash(instance.Status.Hash, common.InputHashName, hash); changed {
		instance.Status.Hash = hashMap
		Log.Info(fmt.Sprintf("Input maps hash %s - %s", common.InputHashName, hash))
	}
	return hash, changed, nil
}
