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
	service "github.com/openstack-k8s-operators/lib-common/modules/common/service"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// NovaMetadataTemplate defines the input parameters specified by the user to
// create a NovaMetadata via higher level CRDs.
type NovaMetadataTemplate struct {
	// +kubebuilder:validation:Optional
	// Enabled - Whether NovaMetadata services should be deployed and managed.
	// If it is set to false then the related NovaMetadata CR will be deleted
	// if exists and owned by a higher level nova CR (Nova or NovaCell). If it
	// exist but not owned by a higher level nova CR then the NovaMetadata CR
	// will not be touched.
	// If it is set to true the a NovaMetadata CR will be created.
	// If there is already a manually created NovaMetadata CR with the relevant
	// name then this operator will not try to update that CR, instead
	// the higher level nova CR will be in error state until the manually
	// create NovaMetadata CR is deleted manually.
	Enabled *bool `json:"enabled"`
	// NOTE(gibi): the default value of enabled depends on the context so
	// it is set by webhooks

	// +kubebuilder:validation:Optional
	// The service specific Container Image URL (will be set to environmental default if empty)
	ContainerImage string `json:"containerImage"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Maximum=32
	// +kubebuilder:validation:Minimum=0
	// Replicas of the service to run
	Replicas *int32 `json:"replicas"`

	// +kubebuilder:validation:Optional
	// NodeSelector to target subset of worker nodes running this service. Setting here overrides
	// any global NodeSelector settings within the Nova CR.
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// +kubebuilder:validation:Optional
	// CustomServiceConfig - customize the service config using this parameter to change service defaults,
	// or overwrite rendered information using raw OpenStack config format. The content gets added to
	// to /etc/<service>/<service>.conf.d directory as custom.conf file.
	CustomServiceConfig string `json:"customServiceConfig"`

	// +kubebuilder:validation:Optional
	// ConfigOverwrite - interface to overwrite default config files like e.g. logging.conf
	// But can also be used to add additional files. Those get added to the service config dir in /etc/<service> .
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
	Override MetadataOverrideSpec `json:"override,omitempty"`

	// +kubebuilder:validation:Optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// TLS - Parameters related to the TLS
	TLS tls.SimpleService `json:"tls,omitempty"`
}

// MetadataOverrideSpec to override the generated manifest of several child resources.
type MetadataOverrideSpec struct {
	// Override configuration for the Service created to serve traffic to the cluster for internal
	// communication.
	Service *service.OverrideSpec `json:"service,omitempty"`
}

// NovaMetadataSpec defines the desired state of NovaMetadata
type NovaMetadataSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// +kubebuilder:validation:Optional
	// CellName is the name of the Nova Cell this metadata service belongs to.
	// If not provided then the metadata serving every cells in the deployment
	CellName string `json:"cellName,omitempty"`

	// +kubebuilder:validation:Required
	// Secret is the name of the Secret instance containing password
	// information for the nova-conductor service. This secret is expected to
	// be generated by the nova-operator based on the information passed to the
	// Nova CR.
	Secret string `json:"secret"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=nova
	// ServiceUser - optional username used for this service to register in
	// keystone
	ServiceUser string `json:"serviceUser"`

	// +kubebuilder:validation:Required
	// KeystoneAuthURL - the URL that the nova-metadata service can use to talk
	// to keystone
	// TODO(ksambor) Add checking if dynamic vendor data is configured
	KeystoneAuthURL string `json:"keystoneAuthURL"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default="nova_api"
	// APIDatabaseUser - username to use when accessing the API DB
	APIDatabaseUser string `json:"apiDatabaseUser"`

	// +kubebuilder:validation:Optional
	// APIDatabaseHostname - hostname to use when accessing the API DB.
	// This filed is Required if the CellName is not provided
	// TODO(gibi): Add a webhook to validate the CellName constraint
	APIDatabaseHostname string `json:"apiDatabaseHostname"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=nova
	// CellDatabaseUser - username to use when accessing the cell DB
	CellDatabaseUser string `json:"cellDatabaseUser"`

	// +kubebuilder:validation:Optional
	// CellDatabaseHostname - hostname to use when accessing the cell DB
	// This is unused if CellName is not provided. But if it is provided then
	// CellDatabaseHostName is also Required.
	// TODO(gibi): add webhook to validate this CellName constraint
	CellDatabaseHostname string `json:"cellDatabaseHostname"`

	// +kubebuilder:validation:Optional
	// Debug - enable debug for different deploy stages. If an init container
	// is used, it runs and the actual action pod gets started with sleep
	// infinity
	Debug Debug `json:"debug,omitempty"`

	// +kubebuilder:validation:Required
	// NovaServiceBase specifies the generic fields of the service
	NovaServiceBase `json:",inline"`

	// +kubebuilder:validation:Optional
	// Override, provides the ability to override the generated manifest of several child resources.
	Override MetadataOverrideSpec `json:"override,omitempty"`

	// +kubebuilder:validation:Required
	// ServiceAccount - service account name used internally to provide Nova services the default SA name
	ServiceAccount string `json:"serviceAccount"`

	// +kubebuilder:validation:Optional
	// RegisteredCells is a map keyed by cell names that are registered in the
	// nova_api database with a value that is the hash of the given cell
	// configuration.
	// This is used to detect when a new cell is added or an existing cell is
	// reconfigured to trigger refresh of the in memory cell caches of the
	// service.
	// This is empty for the case when nova-metadata runs within the cell.
	RegisteredCells map[string]string `json:"registeredCells,omitempty"`

	// +kubebuilder:validation:Optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// TLS - Parameters related to the TLS
	TLS tls.SimpleService `json:"tls,omitempty"`
}

// NovaMetadataStatus defines the observed state of NovaMetadata
type NovaMetadataStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Map of hashes to track e.g. job status
	Hash map[string]string `json:"hash,omitempty"`

	// Conditions
	Conditions condition.Conditions `json:"conditions,omitempty" optional:"true"`

	// ReadyCount defines the number of replicas ready from nova-metadata
	ReadyCount int32 `json:"readyCount,omitempty"`

	// NetworkAttachments status of the deployment pods
	NetworkAttachments map[string][]string `json:"networkAttachments,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="NetworkAttachments",type="string",JSONPath=".spec.networkAttachments",description="NetworkAttachments"
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[0].status",description="Status"
//+kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.conditions[0].message",description="Message"

// NovaMetadata is the Schema for the novametadata API
type NovaMetadata struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NovaMetadataSpec   `json:"spec,omitempty"`
	Status NovaMetadataStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// NovaMetadataList contains a list of NovaMetadata
type NovaMetadataList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NovaMetadata `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NovaMetadata{}, &NovaMetadataList{})
}

// GetConditions returns the list of conditions from the status
func (s NovaMetadataStatus) GetConditions() condition.Conditions {
	return s.Conditions
}

// NewNovaMetadataSpec constructs a NovaMetadataSpec
func NewNovaMetadataSpec(
	novaCell NovaCellSpec,
) NovaMetadataSpec {
	metadataSpec := NovaMetadataSpec{
		CellName:             novaCell.CellName,
		Secret:               novaCell.Secret,
		CellDatabaseHostname: novaCell.CellDatabaseHostname,
		CellDatabaseUser:     novaCell.CellDatabaseUser,
		APIDatabaseHostname:  novaCell.APIDatabaseHostname,
		APIDatabaseUser:      novaCell.APIDatabaseUser,
		Debug:                novaCell.Debug,
		NovaServiceBase: NovaServiceBase{
			ContainerImage:         novaCell.MetadataServiceTemplate.ContainerImage,
			Replicas:               novaCell.MetadataServiceTemplate.Replicas,
			NodeSelector:           novaCell.MetadataServiceTemplate.NodeSelector,
			CustomServiceConfig:    novaCell.MetadataServiceTemplate.CustomServiceConfig,
			DefaultConfigOverwrite: novaCell.MetadataServiceTemplate.DefaultConfigOverwrite,
			Resources:              novaCell.MetadataServiceTemplate.Resources,
			NetworkAttachments:     novaCell.MetadataServiceTemplate.NetworkAttachments,
		},
		KeystoneAuthURL: novaCell.KeystoneAuthURL,
		ServiceUser:     novaCell.ServiceUser,
		ServiceAccount:  novaCell.ServiceAccount,
		Override:        novaCell.MetadataServiceTemplate.Override,
		TLS:             novaCell.MetadataServiceTemplate.TLS,
	}
	return metadataSpec
}

// GetSecret returns the value of the NovaMetadata.Spec.Secret
func (n NovaMetadata) GetSecret() string {
	return n.Spec.Secret
}
