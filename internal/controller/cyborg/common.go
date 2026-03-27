/*
Copyright 2026.

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

// Package cyborg implements the controllers for the Cyborg accelerator lifecycle management service.
package cyborg

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	passwordSecretField    = ".spec.secret"
	authAppCredSecretField = ".spec.auth.applicationCredentialSecret" //nolint:gosec

	// TransportURLSelector is the key for the transport URL in secrets
	TransportURLSelector = "transport_url"
	// QuorumQueuesSelector is the key for quorum queues in TransportURL secrets
	QuorumQueuesSelector = "quorumqueues"
	// DatabaseAccount is the key for the database account name
	DatabaseAccount = "database_account"
	// DatabaseUsername is the key for the database username
	DatabaseUsername = "database_username"
	// DatabasePassword is the key for the database password
	DatabasePassword = "database_password"
	// DatabaseHostname is the key for the database hostname
	DatabaseHostname = "database_hostname"
)

var (
	cyborgWatchFields = []string{
		passwordSecretField,
		authAppCredSecretField,
	}

	// ErrRetrievingSecretData indicates an error retrieving required data from a secret
	ErrRetrievingSecretData = errors.New("error retrieving required data from secret")
	// ErrRetrievingTransportURLSecretData indicates an error retrieving transport URL secret data
	ErrRetrievingTransportURLSecretData = errors.New("error retrieving required data from transporturl secret")
	// ErrTransportURLFieldMissing indicates the TransportURL secret is missing the transport_url field
	ErrTransportURLFieldMissing = errors.New("the TransportURL secret does not have 'transport_url' field")
	// ErrSecretFieldNotFound indicates a required field was not found in a secret
	ErrSecretFieldNotFound = errors.New("field not found in secret")
	// ErrACSecretNotFound indicates the ApplicationCredential secret was not found
	ErrACSecretNotFound = errors.New("ApplicationCredential secret not found")
	// ErrACSecretMissingKeys indicates the ApplicationCredential secret is missing required keys
	ErrACSecretMissingKeys = errors.New("ApplicationCredential secret missing required keys")
)

// ReconcilerBase provides a common set of clients scheme and loggers for all reconcilers.
type ReconcilerBase struct {
	Client         client.Client
	Kclient        kubernetes.Interface
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

// NewReconcilerBase constructs a ReconcilerBase given a manager and Kclient.
func NewReconcilerBase(
	mgr ctrl.Manager, kclient kubernetes.Interface,
) ReconcilerBase {
	return ReconcilerBase{
		Client:         mgr.GetClient(),
		Scheme:         mgr.GetScheme(),
		Kclient:        kclient,
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
			"Cyborg": &CyborgReconciler{
				ReconcilerBase: NewReconcilerBase(mgr, kclient),
			},
			"CyborgConductor": &CyborgConductorReconciler{
				ReconcilerBase: NewReconcilerBase(mgr, kclient),
			},
		}}
}

// OverrideRequeueTimeout overrides the default RequeueTimeout of our reconcilers
func (r *Reconcilers) OverrideRequeueTimeout(timeout time.Duration) {
	for _, reconciler := range r.reconcilers {
		reconciler.SetRequeueTimeout(timeout)
	}
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

type conditionUpdater interface {
	Set(c *condition.Condition)
	MarkTrue(t condition.Type, messageFormat string, messageArgs ...any)
}

type topologyHandler interface {
	GetSpecTopologyRef() *topologyv1.TopoRef
	GetLastAppliedTopology() *topologyv1.TopoRef
	SetLastAppliedTopology(t *topologyv1.TopoRef)
}

// ensureTopology - when a Topology CR is referenced, remove the
// finalizer from a previous referenced Topology (if any), and retrieve the
// newly referenced topology object
func ensureTopology(
	ctx context.Context,
	h *helper.Helper,
	instance topologyHandler,
	finalizer string,
	condUpdater conditionUpdater,
	defaultLabelSelector metav1.LabelSelector,
) (*topologyv1.Topology, error) {
	topology, err := topologyv1.EnsureServiceTopology(
		ctx,
		h,
		instance.GetSpecTopologyRef(),
		instance.GetLastAppliedTopology(),
		finalizer,
		defaultLabelSelector,
	)
	if err != nil {
		condUpdater.Set(condition.FalseCondition(
			condition.TopologyReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.TopologyReadyErrorMessage,
			err.Error()))
		return nil, fmt.Errorf("waiting for Topology requirements: %w", err)
	}

	tr := instance.GetSpecTopologyRef()
	instance.SetLastAppliedTopology(tr)

	if tr != nil {
		condUpdater.MarkTrue(
			condition.TopologyReadyCondition,
			condition.TopologyReadyMessage,
		)
	}
	return topology, nil
}
