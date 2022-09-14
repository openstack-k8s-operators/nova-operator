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

// NovaCellSpec defines the desired state of NovaCell
type NovaCellSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// +kubebuilder:validation:Required
	// CellName is the name of the Nova Cell. The value "cell0" has a special
	// meaning. The "cell0" Cell cannot have compute nodes associated and the
	// conductor in this cell acts as the super conductor for all the cells in
	// the deployment.
	CellName string `json:"cellName,omitempty"`

	// +kubebuilder:validation:Required
	// Secret is the name of the Secret instance containing password
	// information for the nova cell.
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
	// KeystoneAuthURL - the URL that the service in the cell can use to talk
	// to keystone
	// NOTE(gibi): This is made optional here to allow reusing the NovaCellSpec
	// struct in the Nova CR for the cellTemplates field where this
	// information is not yet known. We could make this required via multiple
	// options:
	// a) create a NovaCellTemplate that duplicates NovaCellSpec without this
	//    field. Use NovaCellTemplate as type for cellTemplates in
	//    NovaSpec.
	// b) do a) but pull out a the fields to a base struct that are used in
	//    both NovaCellSpec and NovaCellTemplate
	// c) add a validating webhook here that runs only when NovaCell CR is
	//    created and does not run when Nova CR is created and make this field
	//    required via that webhook.
	KeystoneAuthURL string `json:"keystoneAuthURL"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=nova
	// APIDatabaseUser - username to use when accessing the API DB
	APIDatabaseUser string `json:"apiDatabaseUser"`

	// +kubebuilder:validation:Optional
	// APIDatabaseHostname - hostname to use when accessing the API DB. If not
	// provided then upcalls will be disabled. This filed is Required for
	// cell0.
	// TODO(gibi): Add a webhook to validate cell0 constraint
	APIDatabaseHostname string `json:"apiDatabaseHostname"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=nova
	// APIMessageBusUser - username to use when accessing the API message bus
	APIMessageBusUser string `json:"apiMessageBusUser"`

	// +kubebuilder:validation:Optional
	// APIMessageBusHostname - hostname to use when accessing the API message
	// bus. If not provided then upcalls will be disabled. This field is
	// Required for cell0.
	// TODO(gibi): Add a webhook to validate cell0 constraint.
	APIMessageBusHostname string `json:"apiMessageBusHostname"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=nova
	// CellDatabaseUser - username to use when accessing the cell DB
	CellDatabaseUser string `json:"cellDatabaseUser"`

	// +kubebuilder:validation:Optional
	// NOTE(gibi): This should be Required, see notes in KeystoneAuthURL
	// CellDatabaseHostname - hostname to use when accessing the cell DB
	CellDatabaseHostname string `json:"cellDatabaseHostname"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=nova
	// CellMessageBusUser - username to use when accessing the cell message bus
	CellMessageBusUser string `json:"cellMessageBusUser"`

	// +kubebuilder:validation:Optional
	// NOTE(gibi): This should be Required, see notes in KeystoneAuthURL
	// CellMessageBusHostname - hostname to use when accessing the cell message
	// bus
	CellMessageBusHostname string `json:"cellMessageBusHostname"`

	// +kubebuilder:validation:Optional
	// Debug - enable debug for different deploy stages. If an init container
	// is used, it runs and the actual action pod gets started with sleep
	// infinity
	Debug Debug `json:"debug,omitempty"`

	// +kubebuilder:validation:Required
	// ConductorServiceTemplate - defines the cell conductor deployment for the cell
	ConductorServiceTemplate NovaConductorSpec `json:"conductorServiceTemplate"`

	// +kubebuilder:validation:Optional
	// MetadataServiceTemplate - defines the metadata serive dedicated for the cell.
	MetadataServiceTemplate NovaMetadataSpec `json:"metadataServiceTemplate"`

	// +kubebuilder:validation:Optional
	// NoVNCProxyServiceTemplate - defines the novvncproxy serive dedicated for
	// the cell.
	NoVNCProxyServiceTemplate NovaNoVNCProxySpec `json:"noVNCProxyServiceTemplate"`
}

// NovaCellStatus defines the observed state of NovaCell
type NovaCellStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// Map of hashes to track e.g. job status
	Hash map[string]string `json:"hash,omitempty"`

	// Conditions
	Conditions condition.Conditions `json:"conditions,omitempty" optional:"true"`

	// ConductorServiceReadyCount defines the number of replicas ready from
	// nova-conductor service in the cell
	ConductorServiceReadyCount int32 `json:"conductorServiceReadyCount,omitempty"`

	// MetadataServiceReadyCount defines the number of replicas ready from
	// nova-metadata service in the cell
	MetadataServiceReadyCount int32 `json:"metadataServiceReadyCount,omitempty"`

	// NoVNCPRoxyServiceReadyCount defines the number of replicas ready from
	// nova-novncproxy service in the cell
	NoVNCPRoxyServiceReadyCount int32 `json:"noVNCProxyServiceReadyCount,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// NovaCell is the Schema for the novacells API
type NovaCell struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NovaCellSpec   `json:"spec,omitempty"`
	Status NovaCellStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// NovaCellList contains a list of NovaCell
type NovaCellList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NovaCell `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NovaCell{}, &NovaCellList{})
}
