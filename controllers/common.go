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
	"sort"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/nova-operator/pkg/nova"

	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	helper "github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	nad "github.com/openstack-k8s-operators/lib-common/modules/common/networkattachment"
	"github.com/openstack-k8s-operators/lib-common/modules/common/secret"
	util "github.com/openstack-k8s-operators/lib-common/modules/common/util"
)

const (
	// NovaAPILabelPrefix - a unique, service binary specific prefix for the
	// labels the NovaAPI controller uses on children objects
	NovaAPILabelPrefix = "nova-api"
	// NovaConductorLabelPrefix - a unique, service binary specific prefix for
	// the labels the NovaConductor controller uses on children objects
	NovaConductorLabelPrefix = "nova-conductor"
	// NovaSchedulerLabelPrefix - a unique, service binary specific prefix for
	// the labels the NovaScheduler controller uses on children objects
	NovaSchedulerLabelPrefix = "nova-scheduler"
	// NovaCellLabelPrefix - a unique, prefix used for the compute config
	// Secret
	NovaCellLabelPrefix = "nova-cell"
	// NovaLabelPrefix - a unique, prefix used for labels on Nova CR level jobs
	// and Secrets
	NovaLabelPrefix = "nova"
	// NovaMetadataLabelPrefix - a unique, service binary specific prefix for
	// the labels the NovaMetadata controller uses on children objects
	NovaMetadataLabelPrefix = "nova-metadata"
	// NovaNoVNCProxyLabelPrefix - a unique, service binary specific prefix for
	// the labels the NovaNoVNCProxy controller uses on children objects
	NovaNoVNCProxyLabelPrefix = "nova-novncproxy"
	// NovaComputeIronicLabelPrefix - a unique, service binary specific prefix for
	// the labels the nova-compute-ironic controller uses on children objects
	NovaComputeIronicLabelPrefix = "nova-compute-ironic"
	// DbSyncHash - the field name in Status.Hashes storing the has of the DB
	// sync job
	DbSyncHash = "dbsync"
	// CellSelector is the key name of a cell label
	CellSelector = "cell"

	// ServicePasswordSelector is the name of key in the internal Secret for
	// the nova service password
	ServicePasswordSelector = "ServicePassword"
	// MetadataSecretSelector is the name of key in the internal Secret for
	// the metadata shared secret
	MetadataSecretSelector = "MetadataSecret"
	// APIDatabasePasswordSelector is the name of key in the internal Secret
	// for the API database
	APIDatabasePasswordSelector = "APIDatabasePassword"
	// CellDatabasePasswordSelector is the name of key in the internal cell
	// Secret for the cell database of the given cell
	CellDatabasePasswordSelector = "CellDatabasePassword"
	// TransportURLSelector is the name of key in the internal cell
	// Secret for the cell message bus transport URL
	TransportURLSelector = "transport_url"
)

type conditionsGetter interface {
	GetConditions() condition.Conditions
}

func allSubConditionIsTrue(conditionsGetter conditionsGetter) bool {
	// It assumes that all of our conditions report success via the True status
	for _, c := range conditionsGetter.GetConditions() {
		if c.Type == condition.ReadyCondition {
			continue
		}
		if c.Status != corev1.ConditionTrue {
			return false
		}
	}
	return true
}

type conditionUpdater interface {
	Set(c *condition.Condition)
	MarkTrue(t condition.Type, messageFormat string, messageArgs ...interface{})
}

// ensureSecret - ensures that the Secret object exists and the expected fields
// are in the Secret. It returns a hash of the values of the expected fields.
func ensureSecret(
	ctx context.Context,
	secretName types.NamespacedName,
	expectedFields []string,
	reader client.Reader,
	conditionUpdater conditionUpdater,
	requeueTimeout time.Duration,
) (string, ctrl.Result, corev1.Secret, error) {
	secret := &corev1.Secret{}
	err := reader.Get(ctx, secretName, secret)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			conditionUpdater.Set(condition.FalseCondition(
				condition.InputReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				fmt.Sprintf(novav1.InputReadyWaitingMessage, "secret/"+secretName.Name)))
			return "",
				ctrl.Result{RequeueAfter: requeueTimeout},
				*secret,
				fmt.Errorf("Secret %s not found", secretName)
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
			err := fmt.Errorf("field '%s' not found in secret/%s", field, secretName.Name)
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

// ensureNetworkAttachments - checks the requested network attachments exists and
// returns the annotation to be set on the deployment objects.
func ensureNetworkAttachments(
	ctx context.Context,
	h *helper.Helper,
	networkAttachments []string,
	conditionUpdater conditionUpdater,
	requeueTimeout time.Duration,
) (map[string]string, ctrl.Result, error) {
	var nadAnnotations map[string]string
	var err error

	// networks to attach to
	for _, netAtt := range networkAttachments {
		_, err := nad.GetNADWithName(ctx, h, netAtt, h.GetBeforeObject().GetNamespace())
		if err != nil {
			if k8s_errors.IsNotFound(err) {
				conditionUpdater.Set(condition.FalseCondition(
					condition.NetworkAttachmentsReadyCondition,
					condition.RequestedReason,
					condition.SeverityInfo,
					condition.NetworkAttachmentsReadyWaitingMessage,
					netAtt))
				return nadAnnotations, ctrl.Result{RequeueAfter: requeueTimeout}, fmt.Errorf("network-attachment-definition %s not found", netAtt)
			}
			conditionUpdater.Set(condition.FalseCondition(
				condition.NetworkAttachmentsReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.NetworkAttachmentsReadyErrorMessage,
				err.Error()))
			return nadAnnotations, ctrl.Result{}, err
		}
	}

	nadAnnotations, err = nad.CreateNetworksAnnotation(h.GetBeforeObject().GetNamespace(), networkAttachments)
	if err != nil {
		return nadAnnotations, ctrl.Result{}, fmt.Errorf("failed create network annotation from %s: %w",
			networkAttachments, err)
	}

	return nadAnnotations, ctrl.Result{}, nil
}

// ReconcilerBase provides a common set of clients scheme and loggers for all reconcilers.
type ReconcilerBase struct {
	Client         client.Client
	Kclient        kubernetes.Interface
	Log            logr.Logger
	Scheme         *runtime.Scheme
	RequeueTimeout time.Duration
}

// Manageable all types that conform to this interface can be setup with a controller-runtime manager.
type Manageable interface {
	SetupWithManager(mgr ctrl.Manager) error
}

// Reconciler represents a generic interface for all Reconciler objects in nova
type Reconciler interface {
	Manageable
	SetRequeueTimeout(timeout time.Duration)
}

// NewReconcilerBase constructs a ReconcilerBase given a name manager and Kclient.
func NewReconcilerBase(
	name string, mgr ctrl.Manager, kclient kubernetes.Interface,
) ReconcilerBase {
	log := ctrl.Log.WithName("controllers").WithName(name)
	return ReconcilerBase{
		Client:         mgr.GetClient(),
		Scheme:         mgr.GetScheme(),
		Kclient:        kclient,
		Log:            log,
		RequeueTimeout: time.Duration(5) * time.Second,
	}
}

// SetRequeueTimeout overrides the default RequeueTimeout of the Reconciler
func (r *ReconcilerBase) SetRequeueTimeout(timeout time.Duration) {
	r.RequeueTimeout = timeout
}

// Reconcilers holds all the Reconciler objects of the nova-operator to
// allow generic management of them.
type Reconcilers struct {
	reconcilers map[string]Reconciler
}

// NewReconcilers constructs all nova Reconciler objects
func NewReconcilers(mgr ctrl.Manager, kclient *kubernetes.Clientset) *Reconcilers {
	return &Reconcilers{
		reconcilers: map[string]Reconciler{
			"Nova": &NovaReconciler{
				ReconcilerBase: NewReconcilerBase("Nova", mgr, kclient),
			},
			"NovaCell": &NovaCellReconciler{
				ReconcilerBase: NewReconcilerBase("NovaCell", mgr, kclient),
			},
			"NovaAPI": &NovaAPIReconciler{
				ReconcilerBase: NewReconcilerBase("NovaAPI", mgr, kclient),
			},
			"NovaScheduler": &NovaSchedulerReconciler{
				ReconcilerBase: NewReconcilerBase("NovaScheduler", mgr, kclient),
			},
			"NovaConductor": &NovaConductorReconciler{
				ReconcilerBase: NewReconcilerBase("NovaConductor", mgr, kclient),
			},
			"NovaMetadata": &NovaMetadataReconciler{
				ReconcilerBase: NewReconcilerBase("NovaMetadata", mgr, kclient),
			},
			"NovaNoVNCProxy": &NovaNoVNCProxyReconciler{
				ReconcilerBase: NewReconcilerBase("NovaNoVNCProxy", mgr, kclient),
			},
			"NovaComputeIronic": &NovaComputeIronicReconciler{
				ReconcilerBase: NewReconcilerBase("NovaComputeIronic", mgr, kclient),
			},
		}}
}

// Setup starts the reconcilers by connecting them to the Manager
func (r *Reconcilers) Setup(mgr ctrl.Manager, setupLog logr.Logger) error {
	var err error
	for name, controller := range r.reconcilers {
		if err = controller.SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", name)
			return err
		}
	}
	return nil
}

// OverrideRequeueTimeout overrides the default RequeueTimeout of our reconcilers
func (r *Reconcilers) OverrideRequeueTimeout(timeout time.Duration) {
	for _, reconciler := range r.reconcilers {
		reconciler.SetRequeueTimeout(timeout)
	}
}

// generateConfigsGeneric helper function to generate config maps
func (r *ReconcilerBase) generateConfigsGeneric(
	ctx context.Context, h *helper.Helper,
	instance client.Object, configName string, envVars *map[string]env.Setter,
	templateParameters map[string]interface{},
	extraData map[string]string, cmLabels map[string]string,
	additionalTemplates map[string]string,
	withScripts bool,
) error {

	extraTemplates := map[string]string{
		"01-nova.conf":    "/nova.conf",
		"nova-blank.conf": "/nova-blank.conf",
	}

	for k, v := range additionalTemplates {
		extraTemplates[k] = v
	}
	cms := []util.Template{
		{
			Name:               configName,
			Namespace:          instance.GetNamespace(),
			Type:               util.TemplateTypeConfig,
			InstanceType:       instance.GetObjectKind().GroupVersionKind().Kind,
			ConfigOptions:      templateParameters,
			Labels:             cmLabels,
			CustomData:         extraData,
			Annotations:        map[string]string{},
			AdditionalTemplate: extraTemplates,
		},
	}
	if withScripts {
		cms = append(cms, util.Template{
			Name:               nova.GetScriptSecretName(instance.GetName()),
			Namespace:          instance.GetNamespace(),
			Type:               util.TemplateTypeScripts,
			InstanceType:       instance.GetObjectKind().GroupVersionKind().Kind,
			AdditionalTemplate: map[string]string{},
			Annotations:        map[string]string{},
			Labels:             cmLabels,
		})
	}
	return secret.EnsureSecrets(ctx, h, instance, cms, envVars)
}

// GenerateConfigs helper function to generate config maps
func (r *ReconcilerBase) GenerateConfigs(
	ctx context.Context, h *helper.Helper,
	instance client.Object, configName string, envVars *map[string]env.Setter,
	templateParameters map[string]interface{},
	extraData map[string]string, cmLabels map[string]string,
	additionalTemplates map[string]string,
) error {
	return r.generateConfigsGeneric(
		ctx, h, instance, configName, envVars, templateParameters, extraData,
		cmLabels, additionalTemplates, false,
	)
}

// GenerateConfigsWithScripts helper function to generate config maps
// for service configs and scripts
func (r *ReconcilerBase) GenerateConfigsWithScripts(
	ctx context.Context, h *helper.Helper,
	instance client.Object, envVars *map[string]env.Setter,
	templateParameters map[string]interface{},
	extraData map[string]string, cmLabels map[string]string,
	additionalTemplates map[string]string,
) error {
	return r.generateConfigsGeneric(
		ctx, h, instance, nova.GetServiceConfigSecretName(instance.GetName()),
		envVars, templateParameters, extraData,
		cmLabels, additionalTemplates, true,
	)
}

func getNovaCellCRName(novaCRName string, cellName string) string {
	return novaCRName + "-" + cellName
}

func hashOfStringMap(input map[string]string) (string, error) {
	// we need to make sure that we have a stable map iteration order and
	// hash both the keys and the values
	keys := make([]string, 0, len(input))
	for k := range input {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var keyValues = []string{}
	for _, key := range keys {
		value := input[key]
		keyValues = append(keyValues, key+value)
	}
	return util.ObjectHash(keyValues)
}

type GetSecret interface {
	GetSecret() string
	client.Object
}

// GetSecretMapperFor returns a function that creates reconcile.Request for each
// NovaXXX CR where the value of Spec.Secret matches to the name of the given
// Secret. The specific CRD to match against is defined by the type of the crs
// parameter.
// This function gets called for each changes of each secrets in the deployment.
// If this becomes a performance bottle neck then we have two options
// a) we switch to immutable Secrets and required the end user to always create
//    a new secret with a new name when the content of the Secret needs to be
//    changed.
// b) we move the watch to a central place (nova controller, or even openstack
//    controller) and expose a "generation" field that the central component
//    can bump to trigger a reconcile if the secret content changed.

func (r *ReconcilerBase) GetSecretMapperFor(crs client.ObjectList) func(client.Object) []reconcile.Request {

	mapper := func(secret client.Object) []reconcile.Request {
		var namespace string = secret.GetNamespace()
		var secretName string = secret.GetName()
		result := []reconcile.Request{}

		listOpts := []client.ListOption{
			client.InNamespace(namespace),
		}
		if err := r.Client.List(context.TODO(), crs, listOpts...); err != nil {
			r.Log.Error(err, "Unable to retrieve the list of CRs")
			panic(err)
		}

		err := apimeta.EachListItem(crs, func(o runtime.Object) error {
			// NOTE(gibi): intentionally let the failed cast panic to catch
			// this implementation error as soon as possible.
			cr := o.(GetSecret)
			if cr.GetSecret() == secretName {
				name := client.ObjectKey{
					Namespace: namespace,
					Name:      cr.GetName(),
				}
				r.Log.Info(
					"Requesting reconcile due to secret change",
					"Secret", secretName, "CR", name.Name,
				)
				result = append(result, reconcile.Request{NamespacedName: name})
			}
			return nil
		})

		if err != nil {
			r.Log.Error(err, "Unable to iterate the list of CRs")
			panic(err)
		}

		if len(result) > 0 {
			return result
		}
		return nil
	}

	return mapper
}

// OwnedBy returns true if the owner has an OwnerReference on the owned object
func OwnedBy(owned client.Object, owner client.Object) bool {
	for _, ref := range owned.GetOwnerReferences() {
		if owner.GetUID() == ref.UID {
			return true
		}
	}
	return false
}

func (r *ReconcilerBase) ensureMetadataDeleted(
	ctx context.Context,
	h *helper.Helper,
	instance client.Object,
) error {
	metadataName := getNovaMetadataName(instance)
	metadata := &novav1.NovaMetadata{}
	err := r.Client.Get(ctx, metadataName, metadata)
	if k8s_errors.IsNotFound(err) {
		// Nothing to do as it does not exists
		return nil
	}
	if err != nil {
		return err
	}
	// If it is not created by us, we don't touch it
	if !OwnedBy(metadata, instance) {
		util.LogForObject(
			h, "NovaMetadata is disabled, but there is a "+
				"NovaMetadata CR not owned by us. Not deleting it.",
			instance, "NovaMetadata", metadata)
		return nil
	}

	// OK this was created by us so we go and delete it
	err = r.Client.Delete(ctx, metadata)
	if err != nil && k8s_errors.IsNotFound(err) {
		return nil
	}
	util.LogForObject(
		h, "NovaMetadata is disabled, so deleted NovaMetadata",
		instance, "NovaMetadata", metadata)

	return nil
}
