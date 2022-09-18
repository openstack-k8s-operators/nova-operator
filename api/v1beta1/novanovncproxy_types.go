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

// NovaNoVNCProxySpec defines the desired state of NovaNoVNCProxy
type NovaNoVNCProxySpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// +kubebuilder:validation:Required
	// CellName is the name of the Nova Cell this novncproxy belongs to.
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
	// KeystoneAuthURL - the URL that the nova-novncproxy service can use to
	// talk to keystone
	// NOTE(gibi): This is made optional here to allow reusing the
	// NovaNoVNCProxySpec struct in the Nova CR via the NovaCell CR where this
	// information is not yet known. We could make this required via multiple
	// options:
	// a) create a NovaNoVNCProxyTemplate that duplicates NovaNoVNCProxySpec
	//    without this field. Use NovaNoVNCProxyTemplate in NovaCellSpec.
	// b) do a) but pull out a the fields to a base struct that are used in
	//    both NovaNoVNCPRoxySpec and NovaNoVNCProxyTemplate
	// c) add a validating webhook here that runs only when NovaNoVNCProxy CR
	//    is created and does not run when Nova CR is created and make this
	//    field required via that webhook.
	KeystoneAuthURL string `json:"keystoneAuthURL"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=nova
	// CellDatabaseUser - username to use when accessing the cell DB
	CellDatabaseUser string `json:"cellDatabaseUser"`

	// +kubebuilder:validation:Optional
	// NOTE(gibi): This should be Required, see notes in KeystoneAuthURL
	// CellDatabaseHostname - hostname to use when accessing the cell DB
	CellDatabaseHostname string `json:"cellDatabaseHostname"`

	// +kubebuilder:validation:Optional
	// Debug - enable debug for different deploy stages. If an init container
	// is used, it runs and the actual action pod gets started with sleep
	// infinity
	Debug Debug `json:"debug,omitempty"`

	// +kubebuilder:validation:Required
	// NovaServiceBase specifies the generic fields of the service
	NovaServiceBase `json:",inline"`
}

// NovaNoVNCProxyStatus defines the observed state of NovaNoVNCProxy
type NovaNoVNCProxyStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Map of hashes to track e.g. job status
	Hash map[string]string `json:"hash,omitempty"`

	// Conditions
	Conditions condition.Conditions `json:"conditions,omitempty" optional:"true"`

	// ReadyCount defines the number of replicas ready from nova-novncproxy
	ReadyCount int32 `json:"readyCount,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// NovaNoVNCProxy is the Schema for the novanovncproxies API
type NovaNoVNCProxy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NovaNoVNCProxySpec   `json:"spec,omitempty"`
	Status NovaNoVNCProxyStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// NovaNoVNCProxyList contains a list of NovaNoVNCProxy
type NovaNoVNCProxyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NovaNoVNCProxy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NovaNoVNCProxy{}, &NovaNoVNCProxyList{})
}
