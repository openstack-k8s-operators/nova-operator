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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	helper "github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"github.com/openstack-k8s-operators/lib-common/modules/common/labels"
	"github.com/openstack-k8s-operators/lib-common/modules/common/service"
	util "github.com/openstack-k8s-operators/lib-common/modules/common/util"

	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
)

// NovaCellReconciler reconciles a NovaCell object
type NovaCellReconciler struct {
	ReconcilerBase
}

//+kubebuilder:rbac:groups=nova.openstack.org,resources=novacells,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=nova.openstack.org,resources=novacells/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=nova.openstack.org,resources=novacells/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NovaCell object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.2/pkg/reconcile
func (r *NovaCellReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, _err error) {
	l := log.FromContext(ctx)

	// Fetch the NovaAPI instance that needs to be reconciled
	instance := &novav1.NovaCell{}
	err := r.Client.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected.
			// For additional cleanup logic use finalizers. Return and don't requeue.
			l.Info("NovaCell instance not found, probably deleted before reconciled. Nothing to do.")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		l.Error(err, "Failed to read the NovaCell instance.")
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

	// For the compute config generation we need to read the input secrets
	_, result, secret, err := ensureSecret(
		ctx,
		types.NamespacedName{Namespace: instance.Namespace, Name: instance.Spec.Secret},
		[]string{
			ServicePasswordSelector,
		},
		h.GetClient(),
		&instance.Status.Conditions,
		r.RequeueTimeout,
	)
	if err != nil {
		return result, err
	}

	_, result, messageBusSecret, err := ensureSecret(
		ctx,
		types.NamespacedName{
			Namespace: instance.Namespace,
			Name:      instance.Spec.CellMessageBusSecretName,
		},
		[]string{
			"transport_url",
		},
		h.GetClient(),
		&instance.Status.Conditions,
		r.RequeueTimeout,
	)
	if err != nil {
		return result, err
	}

	// all our input checks out so report InputReady
	instance.Status.Conditions.MarkTrue(condition.InputReadyCondition, condition.InputReadyMessage)

	result, err = r.ensureConductor(ctx, h, instance)
	if err != nil {
		return result, err
	}

	isCell0 := (instance.Spec.CellName == novav1.Cell0Name)
	if *instance.Spec.MetadataServiceTemplate.Replicas != 0 && !isCell0 {
		result, err = r.ensureMetadata(ctx, h, instance)
		if err != nil {
			return result, err
		}
	} else {
		instance.Status.Conditions.Remove(novav1.NovaMetadataReadyCondition)
	}

	cellHasVNCService := (*instance.Spec.NoVNCProxyServiceTemplate.Replicas != 0 && !isCell0)
	if cellHasVNCService {
		result, err = r.ensureNoVNCProxy(ctx, h, instance)
		if err != nil {
			return result, err
		}
	} else {
		instance.Status.Conditions.Remove(novav1.NovaNoVNCProxyReadyCondition)
	}

	// We need to wait for the NovaNoVNCProxy to become Ready before we can try
	// to generate the compute config secret as that needs the endpoint of the
	// proxy to be included.
	// However NovaNoVNCProxy is never deployed in cell0, and optional in other
	// cells too.
	if cellHasVNCService && !instance.Status.Conditions.IsTrue(novav1.NovaNoVNCProxyReadyCondition) {
		util.LogForObject(
			h,
			"Waiting for the NovaNoVNCProxyService to become Ready before "+
				"generating the compute config", instance,
		)
		return ctrl.Result{}, nil
	}

	var vncProxyURL *string
	if cellHasVNCService {
		vncProxyURL, err = r.getVNCHost(ctx, h, instance)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	if !isCell0 {
		result, err = r.ensureComputeConfig(ctx, h, instance, secret, messageBusSecret, vncProxyURL)
		if (err != nil || result != ctrl.Result{}) {
			return result, err
		}

	} else {
		instance.Status.Conditions.Remove(novav1.NovaComputeServiceConfigReady)
	}

	util.LogForObject(h, "Successfully reconciled", instance)
	return ctrl.Result{}, nil
}

func (r *NovaCellReconciler) initStatus(
	ctx context.Context, h *helper.Helper, instance *novav1.NovaCell,
) error {
	if err := r.initConditions(ctx, h, instance); err != nil {
		return err
	}
	if instance.Status.Hash == nil {
		instance.Status.Hash = map[string]string{}
	}

	return nil
}

func (r *NovaCellReconciler) initConditions(
	ctx context.Context, h *helper.Helper, instance *novav1.NovaCell,
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
				novav1.NovaConductorReadyCondition,
				condition.InitReason,
				novav1.NovaConductorReadyInitMessage,
			),
			condition.UnknownCondition(
				novav1.NovaMetadataReadyCondition,
				condition.InitReason,
				novav1.NovaMetadataReadyInitMessage,
			),
			condition.UnknownCondition(
				novav1.NovaNoVNCProxyReadyCondition,
				condition.InitReason,
				novav1.NovaNoVNCProxyReadyInitMessage,
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

func (r *NovaCellReconciler) ensureConductor(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.NovaCell,
) (ctrl.Result, error) {
	conductorSpec := novav1.NewNovaConductorSpec(instance.Spec)
	conductor := &novav1.NovaConductor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + "-conductor",
			Namespace: instance.Namespace,
		},
	}

	op, err := controllerutil.CreateOrPatch(ctx, r.Client, conductor, func() error {
		conductor.Spec = conductorSpec
		if len(conductor.Spec.NodeSelector) == 0 {
			conductor.Spec.NodeSelector = instance.Spec.NodeSelector
		}
		err := controllerutil.SetControllerReference(instance, conductor, r.Scheme)
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		condition.FalseCondition(
			novav1.NovaConductorReadyCondition,
			condition.ErrorReason,
			condition.SeverityError,
			novav1.NovaConductorReadyErrorMessage,
			err.Error(),
		)
		return ctrl.Result{}, err
	}

	if op != controllerutil.OperationResultNone {
		util.LogForObject(h, fmt.Sprintf("NovaConductor %s.", string(op)), instance, "NovaConductor.Name", conductor.Name)
	}

	c := conductor.Status.Conditions.Mirror(novav1.NovaConductorReadyCondition)
	// NOTE(gibi): it can be nil if the NovaConductor CR is created but no
	// reconciliation is run on it to initialize the ReadyCondition yet.
	if c != nil {
		instance.Status.Conditions.Set(c)
	}

	return ctrl.Result{}, nil
}

func (r *NovaCellReconciler) ensureNoVNCProxy(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.NovaCell,
) (ctrl.Result, error) {
	novncproxySpec := novav1.NewNovaNoVNCProxySpec(instance.Spec)
	novncproxy := &novav1.NovaNoVNCProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + "-novncproxy",
			Namespace: instance.Namespace,
		},
	}

	op, err := controllerutil.CreateOrPatch(ctx, r.Client, novncproxy, func() error {
		novncproxy.Spec = novncproxySpec
		if len(novncproxy.Spec.NodeSelector) == 0 {
			novncproxy.Spec.NodeSelector = instance.Spec.NodeSelector
		}
		err := controllerutil.SetControllerReference(instance, novncproxy, r.Scheme)
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		condition.FalseCondition(
			novav1.NovaNoVNCProxyReadyCondition,
			condition.ErrorReason,
			condition.SeverityError,
			novav1.NovaNoVNCProxyReadyErrorMessage,
			err.Error(),
		)
		return ctrl.Result{}, err
	}

	if op != controllerutil.OperationResultNone {
		util.LogForObject(h, fmt.Sprintf("NovaNoVNCProxy %s.", string(op)), instance, "NovaNoVNCProxy.Name", novncproxy.Name)
	}

	c := novncproxy.Status.Conditions.Mirror(novav1.NovaNoVNCProxyReadyCondition)

	if c != nil {
		instance.Status.Conditions.Set(c)
	}

	return ctrl.Result{}, nil
}

func (r *NovaCellReconciler) ensureMetadata(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.NovaCell,
) (ctrl.Result, error) {
	metadataSpec := novav1.NewNovaMetadataSpec(instance.Spec)
	metadata := &novav1.NovaMetadata{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + "-metadata",
			Namespace: instance.Namespace,
		},
	}

	op, err := controllerutil.CreateOrPatch(ctx, r.Client, metadata, func() error {
		metadata.Spec = metadataSpec
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

	return ctrl.Result{}, nil
}

// ensureComputeConfig ensures the the compute config Secret exists and up to
// date. The compute config Secret then can be used to configure a nova-compute
// service to connect to this cell.
func (r *NovaCellReconciler) ensureComputeConfig(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.NovaCell,
	secret corev1.Secret,
	messageBusSecret corev1.Secret,
	vncHost *string,
) (ctrl.Result, error) {

	err := r.generateComputeConfigs(ctx, h, instance, secret, messageBusSecret, vncHost)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			novav1.NovaComputeServiceConfigReady,
			condition.ErrorReason,
			condition.SeverityWarning,
			novav1.NovaComputeServiceConfigErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}
	instance.Status.Conditions.MarkTrue(
		novav1.NovaComputeServiceConfigReady, condition.ServiceConfigReadyMessage,
	)

	return ctrl.Result{}, nil
}

func (r *NovaCellReconciler) generateComputeConfigs(
	ctx context.Context, h *helper.Helper, instance *novav1.NovaCell,
	secret corev1.Secret, messageBusSecret corev1.Secret, vncProxyURL *string,
) error {
	templateParameters := map[string]interface{}{
		"service_name":           "nova-compute",
		"keystone_internal_url":  instance.Spec.KeystoneAuthURL,
		"nova_keystone_user":     instance.Spec.ServiceUser,
		"nova_keystone_password": string(secret.Data[ServicePasswordSelector]),
		"openstack_cacert":       "",          // fixme
		"openstack_region_name":  "regionOne", // fixme
		"default_project_domain": "Default",   // fixme
		"default_user_domain":    "Default",   // fixme
		"transport_url":          string(messageBusSecret.Data["transport_url"]),
		"log_file":               "/var/log/containers/nova/nova-compute.log",
	}
	// vnc is optional so we only need to configure it for the compute
	// if the proxy service is deployed in the cell
	if vncProxyURL != nil {
		templateParameters["novncproxy_base_url"] = *vncProxyURL
	}

	cmLabels := labels.GetLabels(
		instance, labels.GetGroupLabel(NovaCellLabelPrefix), map[string]string{},
	)

	hashes := make(map[string]env.Setter)

	configName := instance.GetName() + "-compute-config"
	err := r.GenerateConfigs(
		ctx, h, instance, configName, &hashes, templateParameters, map[string]string{}, cmLabels, map[string]string{},
	)
	// TODO(gibi): can we make it simpler?
	a := &corev1.EnvVar{}
	hashes[configName](a)
	instance.Status.Hash[configName] = a.Value
	return err
}

func (r *NovaCellReconciler) getVNCHost(
	ctx context.Context, h *helper.Helper, instance *novav1.NovaCell,
) (*string, error) {

	// we should restructure this to be in the same order as the reset of the resources
	// <nova_instance>-<cell_name>-<service_name>-<service_type>
	vncRouteName := fmt.Sprintf("nova-novncproxy-%s-public", instance.Spec.CellName)

	svcOverride := ptr.To(instance.Spec.NoVNCProxyServiceTemplate.Override.Service[string(service.EndpointPublic)])
	if svcOverride != nil &&
		svcOverride.EndpointURL != nil {
		return svcOverride.EndpointURL, nil
	}

	vncSvc, err := service.GetServiceWithName(ctx, h, vncRouteName, instance.Namespace)
	if err != nil {
		return nil, err
	}
	svc, err := service.NewService(vncSvc, time.Duration(5)*time.Second, svcOverride)
	if err != nil {
		return nil, err
	}

	// TODO: TLS
	vncProxyURL, err := svc.GetAPIEndpoint(svcOverride, ptr.To(service.ProtocolHTTP), "/vnc_lite.html")
	if err != nil {
		return nil, err
	}

	return ptr.To(vncProxyURL), nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NovaCellReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&novav1.NovaCell{}).
		Owns(&novav1.NovaConductor{}).
		Owns(&novav1.NovaMetadata{}).
		Owns(&novav1.NovaNoVNCProxy{}).
		// It generates and therefor owns the compute config secret
		Owns(&corev1.Secret{}).
		// and it needs to watch the input secrets
		Watches(&source.Kind{Type: &corev1.Secret{}},
			handler.EnqueueRequestsFromMapFunc(r.GetSecretMapperFor(&novav1.NovaCellList{}))).
		Complete(r)
}
