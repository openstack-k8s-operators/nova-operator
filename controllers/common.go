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

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/configmap"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	helper "github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	nad "github.com/openstack-k8s-operators/lib-common/modules/common/networkattachment"
	util "github.com/openstack-k8s-operators/lib-common/modules/common/util"
)

const (
	// NovaAPILabelPrefix - a unique, service binary specific prefix for the
	// labeles the NovaAPI controller uses on children objects
	NovaAPILabelPrefix = "nova-api"
	// NovaConductorLabelPrefix - a unique, service binary specific prefix for
	// the labeles the NovaConductor controller uses on children objects
	NovaConductorLabelPrefix = "nova-conductor"
	// NovaSchedulerLabelPrefix - a unique, service binary specific prefix for
	// the labeles the NovaScheduler controller uses on children objects
	NovaSchedulerLabelPrefix = "nova-scheduler"
	// DbSyncHash - the field name in Status.Hashes storing the has of the DB
	// sync job
	DbSyncHash = "dbsync"
	// Cell0Name is the name of Cell0 cell that is mandatory in every deployment
	Cell0Name = "cell0"
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
) (string, ctrl.Result, error) {
	secret := &corev1.Secret{}
	err := reader.Get(ctx, secretName, secret)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// TODO(gibi): Change the message to state which input
			// (i.e. Secret with a given name) is missing
			conditionUpdater.Set(condition.FalseCondition(
				condition.InputReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				condition.InputReadyWaitingMessage))
			return "",
				ctrl.Result{RequeueAfter: requeueTimeout},
				fmt.Errorf("OpenStack secret %s not found", secretName)
		}
		conditionUpdater.Set(condition.FalseCondition(
			condition.InputReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.InputReadyErrorMessage,
			err.Error()))
		return "", ctrl.Result{}, err
	}

	// collect the secret values the caller expects to exist
	values := [][]byte{}
	for _, field := range expectedFields {
		val, ok := secret.Data[field]
		if !ok {
			err := fmt.Errorf("field %s not found in Secret %s", field, secretName)
			conditionUpdater.Set(condition.FalseCondition(
				condition.InputReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.InputReadyErrorMessage,
				err.Error()))
			return "", ctrl.Result{}, err
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
		return "", ctrl.Result{}, err
	}

	return hash, ctrl.Result{}, nil
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

// hashOfInputHashes - calculates the overal hash of all our inputs
func hashOfInputHashes(
	ctx context.Context,
	hashes map[string]env.Setter,
) (string, error) {
	mergedMapVars := env.MergeEnvs([]corev1.EnvVar{}, hashes)
	hash, err := util.ObjectHash(mergedMapVars)
	if err != nil {
		return hash, err
	}
	return hash, nil
}

// ReconcilerBase provides a common set of clients scheme and loggers for all reconcilers.
type ReconcilerBase struct {
	Client         client.Client
	Kclient        kubernetes.Interface
	Log            logr.Logger
	Scheme         *runtime.Scheme
	RequeueTimeout time.Duration
}

// Managable all types that conform to this interface can be setup with a controller-runtime manager.
type Managable interface {
	SetupWithManager(mgr ctrl.Manager) error
}

// Reconciler represents a generic interface for all Reconciler objects in nova
type Reconciler interface {
	Managable
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
// allow generic managemenet of them.
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

// OverriedRequeueTimeout overrides the default RequeueTimeout of our reconcilers
func (r *Reconcilers) OverriedRequeueTimeout(timeout time.Duration) {
	for _, reconciler := range r.reconcilers {
		reconciler.SetRequeueTimeout(timeout)
	}
}

// GenerateConfigs helper function to generate config maps
func (r *ReconcilerBase) GenerateConfigs(
	ctx context.Context, h *helper.Helper,
	instance client.Object, envVars *map[string]env.Setter,
	templateParameters map[string]interface{},
	extraData map[string]string, cmLabels map[string]string,
) error {
	additionalTemplates := map[string]string{
		"01-nova.conf":    "/nova.conf",
		"nova-blank.conf": "/nova-blank.conf",
	}
	cms := []util.Template{
		// ConfigMap
		{
			Name:               fmt.Sprintf("%s-config-data", instance.GetName()),
			Namespace:          instance.GetNamespace(),
			Type:               util.TemplateTypeConfig,
			InstanceType:       instance.GetObjectKind().GroupVersionKind().Kind,
			ConfigOptions:      templateParameters,
			Labels:             cmLabels,
			CustomData:         extraData,
			Annotations:        map[string]string{},
			AdditionalTemplate: additionalTemplates,
		},
	}
	// TODO(sean): make this create a secret instead.
	// consider taking this as a function pointer or interface
	// to enable unit testing at some point.
	return configmap.EnsureConfigMaps(ctx, h, instance, cms, envVars)
}
