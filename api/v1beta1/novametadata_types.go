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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

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
	// information for the nova-conductor service.
	Secret string `json:"secret,omitempty"`

	// +kubebuilder:validation:Optional
	// PasswordSelectors - Field names to identify the passwords from the
	// Secret
	PasswordSelectors PasswordSelector `json:"passwordSelectors,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=nova
	// ServiceUser - optional username used for this service to register in
	// keystone
	ServiceUser string `json:"serviceUser"`

	// +kubebuilder:validation:Optional
	// KeystoneAuthURL - the URL that the nova-metadata service can use to talk
	// to keystone
	// NOTE(gibi): This is made optional here to allow reusing the
	// NovaMetadataSpec struct in the Nova CR for the metadataServiceTemplate
	// field where this information is not yet known. We could make this
	// required via multiple options:
	// a) create a NovaMetadataTemplate that duplicates NovaMetadataSpec
	//    without this field. Use NovaMetadataTemplate as type for
	//    metadataServiceTemplate in NovaSpec.
	// b) do a) but pull out a the fields to a base struct that are used in
	//    both NovaMetadataSpec and NovaMetadataTemplate
	// c) add a validating webhook here that runs only when NovaMetadata CR is
	//    created and does not run when Nova CR is created and make this field
	//    required via that webhook.
	KeystoneAuthURL string `json:"keystoneAuthURL"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=nova
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
	// This is unused if CellName is not provided. But if it is provided then
	// CellDatabaseHostName is also Required.
	// TODO(gibi): add webhook to validate this CellName constraint
	// CellDatabaseHostname - hostname to use when accessing the cell DB
	CellDatabaseHostname string `json:"cellDatabaseHostname"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=nova
	// CellMessageBusUser - username to use when accessing the cell message bus
	CellMessageBusUser string `json:"cellMessageBusUser"`

	// +kubebuilder:validation:Optional
	// This is unused if CellName is not provided. But if it is provided then
	// CellMessageBusHostname is required.
	// TODO(gibi): add webhook to validate this CellName constraint
	// CellMessageBusHostname - hostname to use when accessing the cell message
	// bus
	CellMessageBusHostname string `json:"cellMessageBusHostname"`

	// +kubebuilder:validation:Optional
	// Debug - enable debug for different deploy stages. If an init container
	// is used, it runs and the actual action pod gets started with sleep
	// infinity
	Debug Debug `json:"debug,omitempty"`

	// +kubebuilder:validation:Required
	// NovaServiceBase specifies the generic fields of the service
	NovaServiceBase `json:",inline"`
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
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

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
