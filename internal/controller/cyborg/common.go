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

package controller

import (
	"errors"
	"fmt"

	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"
	internalcommon "github.com/openstack-k8s-operators/nova-operator/internal/common"
)

const (
	passwordSecretField     = ".spec.secret"
	authAppCredSecretField  = ".spec.auth.applicationCredentialSecret" //nolint:gosec
	caBundleSecretNameField = ".spec.tls.caBundleSecretName"           //nolint:gosec
	tlsAPIInternalField     = ".spec.tls.api.internal.secretName"
	tlsAPIPublicField       = ".spec.tls.api.public.secretName"

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

const (
	// ACConsumerFinalizer is added to AC secrets that cyborg is actively consuming
	ACConsumerFinalizer = "openstack.org/cyborg-ac-consumer"
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
)

// ReconcilerBase provides a common set of clients scheme and loggers for all reconcilers.
type ReconcilerBase struct {
	internalcommon.ReconcilerBase
}

// NewReconcilerBase constructs a ReconcilerBase given a manager and Kclient.
func NewReconcilerBase(
	mgr ctrl.Manager, kclient kubernetes.Interface,
) ReconcilerBase {
	return ReconcilerBase{
		ReconcilerBase: internalcommon.NewReconcilerBase(mgr, kclient),
	}
}

func generateMyCnf(tlsCfg *tls.Service) string {
	if tlsCfg != nil {
		return fmt.Sprintf("[client]\nssl-ca=%s\nssl=1\n", tls.DownstreamTLSCABundlePath)
	}
	return "[client]\n"
}

// NewReconcilers constructs all cyborg Reconciler objects
func NewReconcilers(mgr ctrl.Manager, kclient *kubernetes.Clientset) *internalcommon.Reconcilers {
	return internalcommon.NewReconcilers(map[string]internalcommon.Reconciler{
		"Cyborg": &CyborgReconciler{
			ReconcilerBase: NewReconcilerBase(mgr, kclient),
		},
		"CyborgConductor": &CyborgConductorReconciler{
			ReconcilerBase: NewReconcilerBase(mgr, kclient),
		},
		"CyborgAPI": &CyborgAPIReconciler{
			ReconcilerBase: NewReconcilerBase(mgr, kclient),
		},
	})
}
