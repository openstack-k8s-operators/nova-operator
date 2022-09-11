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

// NovaAPISpec defines the desired state of NovaAPI
type NovaAPISpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// +kubebuilder:validation:Required
	// Secret is the name of the Secret instance containing password
	// information for the nova-api sevice.
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
	// KeystoneAuthURL - the URL that the nova-api service can use to talk to
	// keystone
	// NOTE(gibi): This is made optional here to allow reusing the NovaAPISpec
	// struct in the Nova CR for the apiServiceTemplate field where this
	// information is not yet known. We could make this required via multiple
	// options:
	// a) create a NovaAPITemplate that duplicates NovaAPISpec without this
	//    field. Use NovaAPITemplate as type for apiServiceTemplate in
	//    NovaSpec.
	// b) do a) but pull out a the fields to a base struct that are used in
	//    both NovaAPISpec and NovaAPITemplate
	// c) add a validating webhook here that runs only when NovaAPI CR is
	//    created and does not run when Nova CR is created and make this field
	//    required via that webhook.
	KeystoneAuthURL string `json:"keystoneAuthURL"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=nova
	// APIDatabaseUser - username to use when accessing the API DB
	APIDatabaseUser string `json:"apiDatabaseUser"`

	// +kubebuilder:validation:Optional
	// NOTE(gibi): This should be Required, see notes in KeystoneAuthURL
	// APIDatabaseHostname - hostname to use when accessing the API DB
	APIDatabaseHostname string `json:"apiDatabaseHostname"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=nova
	// APIMessageBusUser - username to use when accessing the API message bus
	APIMessageBusUser string `json:"apiMessageBusUser"`

	// +kubebuilder:validation:Optional
	// NOTE(gibi): This should be Required, see notes in KeystoneAuthURL
	// APIMessageBusHostname - hostname to use when accessing the API message
	// bus
	APIMessageBusHostname string `json:"apiMessageBusHostname"`

	// +kubebuilder:validation:Optional
	// Debug - enable debug for different deploy stages. If an init container
	// is used, it runs and the actual action pod gets started with sleep
	// infinity
	Debug Debug `json:"debug,omitempty"`

	// +kubebuilder:validation:Required
	// NovaServiceBase specifies the generic fields of the service
	NovaServiceBase `json:",inline"`
}

// NovaAPIStatus defines the observed state of NovaAPI
type NovaAPIStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Map of hashes to track e.g. job status
	Hash map[string]string `json:"hash,omitempty"`

	// API endpoint
	APIEndpoints map[string]string `json:"apiEndpoint,omitempty"`

	// Conditions
	Conditions condition.Conditions `json:"conditions,omitempty" optional:"true"`

	// ServiceID - the ID of the registered service in keystone
	ServiceID string `json:"serviceID,omitempty"`

	// ReadyCount defines the number of replicas ready from nova-api
	ReadyCount int32 `json:"readyCount,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// NovaAPI is the Schema for the novaapis API
type NovaAPI struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NovaAPISpec   `json:"spec,omitempty"`
	Status NovaAPIStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// NovaAPIList contains a list of NovaAPI
type NovaAPIList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NovaAPI `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NovaAPI{}, &NovaAPIList{})
}
