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

// NovaSchedulerSpec defines the desired state of NovaScheduler
type NovaSchedulerSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// +kubebuilder:validation:Required
	// Secret is the name of the Secret instance containing password
	// information for the nova-scheduler sevice.
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
	// KeystoneAuthURL - the URL that the nova-scheduler service can use to
	// talk to keystone
	// NOTE(gibi): This is made optional here to allow reusing the
	// NovaSchedulerSpec struct in the Nova CR for the schedulerServiceTemplate
	// field where this information is not yet known. We could make this \
	// required via multiple options:
	// a) create a NovaSchedulerTemplate that duplicates NovaSchedulerSpec
	//    without this field. Use NovaSchedulerTemplate as type for
	//    schedulerServiceTemplate in NovaSpec.
	// b) do a) but pull out a the fields to a base struct that are used in
	//    both NovaSchedulerSpec and NovaSchedulerTemplate
	// c) add a validating webhook here that runs only when NovaScheduler CR is
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

// NovaSchedulerStatus defines the observed state of NovaScheduler
type NovaSchedulerStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// Map of hashes to track e.g. job status
	Hash map[string]string `json:"hash,omitempty"`

	// Conditions
	Conditions condition.Conditions `json:"conditions,omitempty" optional:"true"`

	// ReadyCount defines the number of replicas ready from nova-scheduler
	ReadyCount int32 `json:"readyCount,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// NovaScheduler is the Schema for the novaschedulers API
type NovaScheduler struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NovaSchedulerSpec   `json:"spec,omitempty"`
	Status NovaSchedulerStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// NovaSchedulerList contains a list of NovaScheduler
type NovaSchedulerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NovaScheduler `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NovaScheduler{}, &NovaSchedulerList{})
}
