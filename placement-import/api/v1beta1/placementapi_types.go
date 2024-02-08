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

package v1beta1

import (
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/service"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// DbSyncHash hash
	DbSyncHash = "dbsync"

	// DeploymentHash hash used to detect changes
	DeploymentHash = "deployment"

	// Container image fall-back defaults

	// PlacementAPIContainerImage is the fall-back container image for PlacementAPI
	PlacementAPIContainerImage = "quay.io/podified-antelope-centos9/openstack-placement-api:current-podified"
)

// PlacementAPISpec defines the desired state of PlacementAPI
type PlacementAPISpec struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=placement
	// ServiceUser - optional username used for this service to register in keystone
	ServiceUser string `json:"serviceUser"`

	// +kubebuilder:validation:Required
	// MariaDB instance name
	// Right now required by the maridb-operator to get the credentials from the instance to create the DB
	// Might not be required in future
	DatabaseInstance string `json:"databaseInstance"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=placement
	// DatabaseUser - optional username used for placement DB, defaults to placement
	// TODO: -> implement needs work in mariadb-operator, right now only placement
	DatabaseUser string `json:"databaseUser"`

	// +kubebuilder:validation:Required
	// PlacementAPI Container Image URL (will be set to environmental default if empty)
	ContainerImage string `json:"containerImage"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Maximum=32
	// +kubebuilder:validation:Minimum=0
	// Replicas of placement API to run
	Replicas *int32 `json:"replicas"`

	// +kubebuilder:validation:Required
	// Secret containing OpenStack password information for placement PlacementDatabasePassword, PlacementPassword
	Secret string `json:"secret"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default={database: PlacementDatabasePassword, service: PlacementPassword}
	// PasswordSelectors - Selectors to identify the DB and ServiceUser password from the Secret
	PasswordSelectors PasswordSelector `json:"passwordSelectors"`

	// +kubebuilder:validation:Optional
	// NodeSelector to target subset of worker nodes running this service
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	// PreserveJobs - do not delete jobs after they finished e.g. to check logs
	PreserveJobs bool `json:"preserveJobs"`

	// +kubebuilder:validation:Optional
	// CustomServiceConfig - customize the service config using this parameter to change service defaults,
	// or overwrite rendered information using raw OpenStack config format. The content gets added to
	// to /etc/<service>/<service>.conf.d directory as custom.conf file.
	CustomServiceConfig string `json:"customServiceConfig"`

	// +kubebuilder:validation:Optional
	// DefaultConfigOverwrite - interface to overwrite default config files like policy.yaml.
	DefaultConfigOverwrite map[string]string `json:"defaultConfigOverwrite,omitempty"`

	// +kubebuilder:validation:Optional
	// Resources - Compute Resources required by this service (Limits/Requests).
	// https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// +kubebuilder:validation:Optional
	// NetworkAttachments is a list of NetworkAttachment resource names to expose the services to the given network
	NetworkAttachments []string `json:"networkAttachments,omitempty"`

	// +kubebuilder:validation:Optional
	// Override, provides the ability to override the generated manifest of several child resources.
	Override APIOverrideSpec `json:"override,omitempty"`

	// +kubebuilder:validation:Optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// TLS - Parameters related to the TLS
	TLS tls.API `json:"tls,omitempty"`
}

// APIOverrideSpec to override the generated manifest of several child resources.
type APIOverrideSpec struct {
	// Override configuration for the Service created to serve traffic to the cluster.
	// The key must be the endpoint type (public, internal)
	Service map[service.Endpoint]service.RoutedOverrideSpec `json:"service,omitempty"`
}

// PasswordSelector to identify the DB and AdminUser password from the Secret
type PasswordSelector struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="PlacementDatabasePassword"
	// Database - Selector to get the Database user password from the Secret
	// TODO: not used, need change in mariadb-operator
	Database string `json:"database"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="PlacementPassword"
	// Service - Selector to get the service user password from the Secret
	Service string `json:"service"`
}

// PlacementAPIStatus defines the observed state of PlacementAPI
type PlacementAPIStatus struct {
	// ReadyCount of placement API instances
	ReadyCount int32 `json:"readyCount,omitempty"`

	// Map of hashes to track e.g. job status
	Hash map[string]string `json:"hash,omitempty"`

	// Conditions
	Conditions condition.Conditions `json:"conditions,omitempty" optional:"true"`

	// Placement Database Hostname
	DatabaseHostname string `json:"databaseHostname,omitempty"`

	// NetworkAttachments status of the deployment pods
	NetworkAttachments map[string][]string `json:"networkAttachments,omitempty"`
}

// PlacementAPI is the Schema for the placementapis API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="NetworkAttachments",type="string",JSONPath=".spec.networkAttachments",description="NetworkAttachments"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[0].status",description="Status"
// +kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.conditions[0].message",description="Message"
type PlacementAPI struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PlacementAPISpec   `json:"spec,omitempty"`
	Status PlacementAPIStatus `json:"status,omitempty"`
}

// PlacementAPIList contains a list of PlacementAPI
// +kubebuilder:object:root=true
type PlacementAPIList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PlacementAPI `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PlacementAPI{}, &PlacementAPIList{})
}

// IsReady - returns true if PlacementAPI is reconciled successfully
func (instance PlacementAPI) IsReady() bool {
	return instance.Status.Conditions.IsTrue(condition.ReadyCondition)
}

// RbacConditionsSet - set the conditions for the rbac object
func (instance PlacementAPI) RbacConditionsSet(c *condition.Condition) {
	instance.Status.Conditions.Set(c)
}

// RbacNamespace - return the namespace
func (instance PlacementAPI) RbacNamespace() string {
	return instance.Namespace
}

// RbacResourceName - return the name to be used for rbac objects (serviceaccount, role, rolebinding)
func (instance PlacementAPI) RbacResourceName() string {
	return "placement-" + instance.Name
}

// SetupDefaults - initializes any CRD field defaults based on environment variables (the defaulting mechanism itself is implemented via webhooks)
func SetupDefaults() {
	// Acquire environmental defaults and initialize Placement defaults with them
	placementDefaults := PlacementAPIDefaults{
		ContainerImageURL: util.GetEnvVar("RELATED_IMAGE_PLACEMENT_API_IMAGE_URL_DEFAULT", PlacementAPIContainerImage),
	}

	SetupPlacementAPIDefaults(placementDefaults)
}

// GetSecret returns the value of the Nova.Spec.Secret
func (instance PlacementAPI) GetSecret() string {
	return instance.Spec.Secret
}
