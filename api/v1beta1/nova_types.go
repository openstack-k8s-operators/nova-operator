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

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=keystone
	// KeystoneInstance to name of the KeystoneAPI CR to select the Service
	// instance used by the Nova services to authenticate.
	KeystoneInstance string `json:"keystoneInstance"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=openstack
	// APIDatabaseInstance is the name of the MariaDB CR to select the DB
	// Service instance used for the Nova API DB.
	APIDatabaseInstance string `json:"apiDatabaseInstance"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=rabbitmq
	// APIMessageBusInstance is the name of the RabbitMqCluster CR to select
	// the Message Bus Service instance used by the Nova top level services to
	// communicate.
	APIMessageBusInstance string `json:"apiMessageBusInstance"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default={cell0: {cellDatabaseUser: nova_cell0, hasAPIAccess: true}, cell1: {cellDatabaseUser: nova_cell1, cellDatabaseInstance: openstack-cell1, cellMessageBusInstance: rabbitmq-cell1, hasAPIAccess: true}}
	// Cells is a mapping of cell names to NovaCellTemplate objects defining
	// the cells in the deployment. The "cell0" cell is a mandatory cell in
	// every deployment. Moreover any real deployment needs at least one
	// additional normal cell as "cell0" cannot have any computes.
	CellTemplates map[string]NovaCellTemplate `json:"cellTemplates"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default="nova"
	// ServiceUser - optional username used for this service to register in keystone
	ServiceUser string `json:"serviceUser"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default="nova_api"
	// APIDatabaseUser - username to use when accessing the API DB
	APIDatabaseUser string `json:"apiDatabaseUser"`

	// +kubebuilder:validation:Required
	// Secret is the name of the Secret instance containing password
	// information for nova like the keystone service password and DB passwords
	Secret string `json:"secret"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default={service: NovaPassword}
	// PasswordSelectors - Selectors to identify the DB and ServiceUser
	// passwords from the Secret
	PasswordSelectors PasswordSelector `json:"passwordSelectors"`

	// +kubebuilder:validation:Optional
	// NodeSelector to target subset of worker nodes running this service. Setting
	// NodeSelector here acts as a default value and can be overridden by service
	// specific NodeSelector Settings.
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	// PreserveJobs - do not delete jobs after they finished e.g. to check logs
	PreserveJobs bool `json:"preserveJobs"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default={replicas:1}
	// APIServiceTemplate - define the nova-api service
	APIServiceTemplate NovaAPITemplate `json:"apiServiceTemplate"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default={replicas:1}
	// SchedulerServiceTemplate- define the nova-scheduler service
	SchedulerServiceTemplate NovaSchedulerTemplate `json:"schedulerServiceTemplate"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default={enabled: true}
	// MetadataServiceTemplate - defines the metadata service that is global
	// for the deployment serving all the cells. Note that if you want to
	// deploy metadata per cell then the metadata service should be disabled
	// here and enabled in the cellTemplates instead.
	MetadataServiceTemplate NovaMetadataTemplate `json:"metadataServiceTemplate"`

	// +kubebuilder:validation:Required
	// +kubebuilder:default=memcached
	// Global memcached instance name.
	MemcachedInstance string `json:"memcachedInstance"`
}

// NovaStatus defines the observed state of Nova
type NovaStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Conditions
	Conditions condition.Conditions `json:"conditions,omitempty" optional:"true"`

	// APIServiceReadyCount defines the number or replicas ready from nova-api
	APIServiceReadyCount int32 `json:"apiServiceReadyCount,omitempty"`

	// SchedulerServiceReadyCount defines the number or replicas ready from nova-scheduler
	SchedulerServiceReadyCount int32 `json:"schedulerServiceReadyCount,omitempty"`

	// MetadataReadyCount defines the number of replicas ready from
	// nova-metadata service
	MetadataServiceReadyCount int32 `json:"metadataServiceReadyCount,omitempty"`

	// RegisteredCells is a map keyed by cell names that are registered in the
	// nova_api database with a value that is the hash of the given cell
	// configuration.
	RegisteredCells map[string]string `json:"registeredCells,omitempty"`

	// DiscoveredCells is a map keyed by cell names that have discovered all kubernetes managed
	// computes in cell value is a hash of config from all kubernetes managed computes in cell
	DiscoveredCells map[string]string `json:"discoveredCells,omitempty"`
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

// GetConditions returns the list of conditions from the status
func (s NovaStatus) GetConditions() condition.Conditions {
	return s.Conditions
}

// IsReady returns true if Nova reconciled successfully
func (instance Nova) IsReady() bool {
	return instance.Status.Conditions.IsTrue(condition.ReadyCondition)
}

// RbacConditionsSet - set the conditions for the rbac object
func (instance Nova) RbacConditionsSet(c *condition.Condition) {
	instance.Status.Conditions.Set(c)
}

// RbacNamespace - return the namespace
func (instance Nova) RbacNamespace() string {
	return instance.Namespace
}

// RbacResourceName - return the name to be used for rbac objects (serviceaccount, role, rolebinding)
func (instance Nova) RbacResourceName() string {
	return "nova-" + instance.Name
}

// GetSecret returns the value of the Nova.Spec.Secret
func (instance Nova) GetSecret() string {
	return instance.Spec.Secret
}
