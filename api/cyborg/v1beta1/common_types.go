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

package v1beta1

import (
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
)

// Container image fall-back defaults
const (
	CyborgAPIContainerImage       = "quay.io/openstack.kolla/cyborg-api:master-rocky-10"
	CyborgConductorContainerImage = "quay.io/openstack.kolla/cyborg-conductor:master-rocky-10"
	CyborgAgentContainerImage     = "quay.io/openstack.kolla/cyborg-agent:master-rocky-10"
)

// PasswordSelector to identify the DB and AdminUser password from the Secret
type PasswordSelector struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="CyborgPassword"
	// Service - Selector to get the keystone service user password from the
	// Secret
	Service string `json:"service"`
}

// AuthSpec defines authentication parameters for Cyborg services
type AuthSpec struct {
	// +kubebuilder:validation:Optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// ApplicationCredentialSecret - the name of the k8s Secret that contains the
	// application credential data used for authentication
	ApplicationCredentialSecret string `json:"applicationCredentialSecret,omitempty"`
}

// CyborgImages defines container images used by top level Cyborg CR
type CyborgImages struct {
	// +kubebuilder:validation:Required
	// APIContainerImageURL
	APIContainerImageURL string `json:"apiContainerImageURL"`

	// +kubebuilder:validation:Required
	// ConductorContainerImageURL
	ConductorContainerImageURL string `json:"conductorContainerImageURL"`

	// +kubebuilder:validation:Required
	// AgentContainerImageURL
	AgentContainerImageURL string `json:"agentContainerImageURL"`
}

// SetupDefaults - initializes any CRD field defaults based on environment variables (the defaulting mechanism itself is implemented via webhooks)
func SetupDefaults() {

	// Acquire environmental defaults and initialize Cyborg defaults with them
	cyborgDefaults := CyborgDefaults{
		APIContainerImageURL:       util.GetEnvVar("RELATED_IMAGE_CYBORG_API_IMAGE_URL_DEFAULT", CyborgAPIContainerImage),
		ConductorContainerImageURL: util.GetEnvVar("RELATED_IMAGE_CYBORG_CONDUCTOR_IMAGE_URL_DEFAULT", CyborgConductorContainerImage),
		AgentContainerImageURL:     util.GetEnvVar("RELATED_IMAGE_CYBORG_AGENT_IMAGE_URL_DEFAULT", CyborgAgentContainerImage),
		APITimeout:                 60,
	}

	SetupCyborgDefaults(cyborgDefaults)

}
