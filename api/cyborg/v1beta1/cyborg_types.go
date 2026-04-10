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
	rabbitmqv1 "github.com/openstack-k8s-operators/infra-operator/apis/rabbitmq/v1beta1"
	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CyborgSpecCore defines the template for CyborgSpec used in OpenStackControlPlane
type CyborgSpecCore struct {

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=keystone
	// KeystoneInstance to name of the KeystoneAPI CR to select the Service
	// instance used by the Cyborg services to authenticate.
	KeystoneInstance *string `json:"keystoneInstance"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=openstack
	// DatabaseInstance is the name of the MariaDB CR to select the DB
	// Service instance used for the Cyborg API DB.
	DatabaseInstance *string `json:"databaseInstance"`

	// +kubebuilder:validation:Optional
	// MessagingBus configuration (username, vhost, and cluster)
	MessagingBus rabbitmqv1.RabbitMqConfig `json:"messagingBus,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default="cyborg"
	// ServiceUser - optional username used for this service to register in keystone
	ServiceUser *string `json:"serviceUser"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default={service: CyborgPassword}
	// PasswordSelectors - Selectors to identify the DB and ServiceUser
	// passwords from the Secret
	PasswordSelectors *PasswordSelector `json:"passwordSelectors"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default="cyborg"
	// DatabaseAccount - MariaDBAccount to use when accessing the API DB
	DatabaseAccount *string `json:"databaseAccount"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=60
	// +kubebuilder:validation:Minimum=10
	// APITimeout for Route and Apache
	APITimeout *int `json:"apiTimeout"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=osp-secret
	// Secret is the name of the Secret instance containing password
	// information for cyborg like the keystone service password and DB passwords
	Secret *string `json:"secret"`

	// +kubebuilder:validation:Optional
	// NodeSelector to target subset of worker nodes running this service. Setting
	// NodeSelector here acts as a default value and can be overridden by service
	// specific NodeSelector Settings.
	NodeSelector *map[string]string `json:"nodeSelector,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	// PreserveJobs - do not delete jobs after they finished e.g. to check logs
	PreserveJobs bool `json:"preserveJobs"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default={replicas:1}
	// APIServiceTemplate - define the cyborg-api service
	APIServiceTemplate CyborgAPITemplate `json:"apiServiceTemplate"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default={replicas:1}
	// ConductorServiceTemplate - define the cyborg-conductor service
	ConductorServiceTemplate CyborgConductorTemplate `json:"conductorServiceTemplate"`

	// +kubebuilder:validation:Optional
	// TopologyRef to apply the Topology defined by the associated CR referenced
	// by name
	TopologyRef *topologyv1.TopoRef `json:"topologyRef,omitempty"`

	// +kubebuilder:validation:Optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// Auth - Parameters related to authentication (shared by all Cyborg services)
	Auth AuthSpec `json:"auth,omitempty"`
}

// CyborgSpec defines the desired state of Cyborg.
type CyborgSpec struct {
	CyborgSpecCore `json:",inline"`

	CyborgImages `json:",inline"`
}

// CyborgStatus defines the observed state of Cyborg.
type CyborgStatus struct {

	// Conditions
	Conditions condition.Conditions `json:"conditions,omitempty" optional:"true"`

	// ServiceID - The ID of the cyborg service registered in keystone
	ServiceID string `json:"serviceID,omitempty"`

	// Hash - Map of hashes to track e.g. job status
	Hash map[string]string `json:"hash,omitempty"`

	// APIServiceReadyCount defines the number or replicas ready from cyborg-api
	APIServiceReadyCount int32 `json:"apiServiceReadyCount,omitempty"`

	// ConductorServiceReadyCount defines the number or replicas ready from cyborg-conductor
	ConductorServiceReadyCount int32 `json:"conductorServiceReadyCount,omitempty"`

	// ObservedGeneration - the most recent generation observed for this
	// service. If the observed generation is less than the spec generation,
	// then the controller has not processed the latest changes.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Cyborg is the Schema for the cyborgs API.
type Cyborg struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CyborgSpec   `json:"spec,omitempty"`
	Status CyborgStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CyborgList contains a list of Cyborg.
type CyborgList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Cyborg `json:"items"`
}

// RbacConditionsSet sets the conditions for the RBAC reconciliation
func (instance Cyborg) RbacConditionsSet(c *condition.Condition) {
	instance.Status.Conditions.Set(c)
}

// RbacNamespace returns the namespace
func (instance Cyborg) RbacNamespace() string {
	return instance.Namespace
}

// RbacResourceName returns the name to be used for RBAC objects (serviceaccount, role, rolebinding)
func (instance Cyborg) RbacResourceName() string {
	return "cyborg-" + instance.Name
}

// IsReady returns true if the ReadyCondition is true
func (instance *Cyborg) IsReady() bool {
	return instance.Status.Conditions.IsTrue(condition.ReadyCondition)
}

func init() {
	SchemeBuilder.Register(&Cyborg{}, &CyborgList{})
}
