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

// NovaSpec defines the desired state of Nova
type NovaSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// +kubebuilder:validation:Required
	// KeystoneServiceSelector to select the Keystone Service instance used
	// by the Nova services to authenticate.
	KeystoneServiceSelector map[string]string `json:"keystoneServiceSelector,omitempty"`

	// +kubebuilder:validation:Optional
	// APIDBSelector to select the DB Service instance used for the Nova API
	// DB. If not provided the the default DB Service of the deployment will
	// be used for the Nova API DB.
	APIDBSelector map[string]string `json:"apiDBSelector,omitempty"`

	// +kubebuilder:validation:Optional
	// APIMessageBusSelector to select the Message Bus Service instance used
	// by the Nova top level services to communicate. If not provided then
	// the deployment's default Message Bus instance will be used.
	APIMessageBusSelector map[string]string `json:"apiMessageBusSelector,omitempty"`

	// +kubebuilder:validation:Required
	// Cells is a mapping of cell names to NovaCell objects defining the cells
	// in the deployment. The "cell0" cell is a mandatory cell in every
	// deployment. Moreover any real deployment needs at least one additional
	// normal cell as "cell0" cannot have any computes.
	CellTemplates map[string]NovaCellSpec `json:"cellTemplates"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=nova
	// ServiceUser - optional username used for this service to register in keystone
	ServiceUser string `json:"serviceUser"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=nova
	// APIDatabaseUser - username to use when accessing the API DB
	APIDatabaseUser string `json:"apiDatabaseUser"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=nova
	// APIMessageBusUser - username to use when accessing the API message bus
	APIMessageBusUser string `json:"apiMessageBusUser"`

	// +kubebuilder:validation:Required
	// Secret is the name of the Secret instance containing password
	// information for nova like the keystone service password and DB passwords
	Secret string `json:"secret,omitempty"`

	// +kubebuilder:validation:Optional
	// PasswordSelectors - Selectors to identify the DB and ServiceUser
	// passwords from the Secret
	PasswordSelectors PasswordSelector `json:"passwordSelectors,omitempty"`

	// +kubebuilder:validation:Optional
	// Debug - enable debug for different deploy stages. If an init container
	// is used, it runs and the actual action pod gets started with sleep
	// infinity
	Debug Debug `json:"debug,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default={}
	// APIServiceTemplate - define the nova-api service
	APIServiceTemplate NovaAPITemplate `json:"apiServiceTemplate"`

	// +kubebuilder:validation:Required
	// SchedulerServiceTemplate- define the nova-scheduler service
	SchedulerServiceTemplate NovaSchedulerSpec `json:"schedulerServiceTemplate"`

	// +kubebuilder:validation:Optional
	// MetadataServiceTemplate - defines the metadata service that is global for the
	// deployment serving all the cells.
	MetadataServiceTemplate NovaMetadataSpec `json:"metadataServiceTemplate"`
}

// NovaStatus defines the observed state of Nova
type NovaStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Map of hashes to track e.g. job status
	Hash map[string]string `json:"hash,omitempty"`

	// Conditions
	Conditions condition.Conditions `json:"conditions,omitempty" optional:"true"`

	// APIServiceReadyCount defines the number or replicas ready from nova-api
	APIServiceReadyCount int32 `json:"apiServiceReadyCount,omitempty"`

	// SchedulerServiceReadyCount defines the number or replicas ready from nova-scheduler
	SchedulerServiceReadyCount int32 `json:"schedulerServiceReadyCount,omitempty"`

	// MetadataReadyCount defines the number of replicas ready from
	// nova-metadata service
	MetadataServiceReadyCount int32 `json:"metadataServiceReadyCount,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Nova is the Schema for the nova API
type Nova struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NovaSpec   `json:"spec,omitempty"`
	Status NovaStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// NovaList contains a list of Nova
type NovaList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Nova `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Nova{}, &NovaList{})
}
