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
	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CyborgConductorTemplate defines the input parameters specified by the user to
// create a CyborgConductor via higher level CRDs.
type CyborgConductorTemplate struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Maximum=32
	// +kubebuilder:validation:Minimum=0
	// Replicas of the service to run
	Replicas *int32 `json:"replicas"`

	// +kubebuilder:validation:Optional
	// NodeSelector to target subset of worker nodes running this service. Setting here overrides
	// any global NodeSelector settings within the Cyborg CR.
	NodeSelector *map[string]string `json:"nodeSelector,omitempty"`

	// +kubebuilder:validation:Optional
	// CustomServiceConfig - customize the service config using this parameter to change service defaults,
	// or overwrite rendered information using raw OpenStack config format. The content gets added to
	// to /etc/<service>/<service>.conf.d directory as custom.conf file.
	CustomServiceConfig string `json:"customServiceConfig"`

	// +kubebuilder:validation:Optional
	// Resources - Compute Resources required by this service (Limits/Requests).
	// https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// +kubebuilder:validation:Optional
	// TopologyRef to apply the Topology defined by the associated CR referenced
	// by name
	TopologyRef *topologyv1.TopoRef `json:"topologyRef,omitempty"`
}

// CyborgConductorSpec defines the desired state of CyborgConductor.
type CyborgConductorSpec struct {
	CyborgConductorTemplate `json:",inline"`

	// +kubebuilder:validation:Required
	// Secret is the name of the sub-level secret containing all required data
	// (transport URL, DB creds, keystone auth, service password, etc.)
	ConfigSecret string `json:"configSecret"`

	// +kubebuilder:validation:Required
	// ContainerImage is the container image URL for cyborg-conductor
	ContainerImage string `json:"containerImage"`

	// +kubebuilder:validation:Required
	// ServiceAccount used by the conductor pods
	ServiceAccount string `json:"serviceAccount"`

	// +kubebuilder:validation:Optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// TLS - Parameters related to the TLS
	TLS tls.Ca `json:"tls,omitempty"`
}

// CyborgConductorStatus defines the observed state of CyborgConductor.
type CyborgConductorStatus struct {
	// ReadyCount defines the number of replicas ready
	ReadyCount int32 `json:"readyCount,omitempty"`

	// Conditions
	Conditions condition.Conditions `json:"conditions,omitempty" optional:"true"`

	// ObservedGeneration - the most recent generation observed
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Hash - Map of hashes to track config changes
	Hash map[string]string `json:"hash,omitempty"`

	// LastAppliedTopology - the last applied Topology
	LastAppliedTopology *topologyv1.TopoRef `json:"lastAppliedTopology,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[0].status",description="Status"
// +kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.conditions[0].message",description="Message"

// CyborgConductor is the Schema for the cyborgconductors API.
type CyborgConductor struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CyborgConductorSpec   `json:"spec,omitempty"`
	Status CyborgConductorStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CyborgConductorList contains a list of CyborgConductor.
type CyborgConductorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CyborgConductor `json:"items"`
}

// IsReady returns true if the ReadyCondition is true
func (instance *CyborgConductor) IsReady() bool {
	return instance.Status.Conditions.IsTrue(condition.ReadyCondition)
}

// GetSpecTopologyRef returns the TopologyRef defined in the Spec
func (instance *CyborgConductor) GetSpecTopologyRef() *topologyv1.TopoRef {
	return instance.Spec.TopologyRef
}

// GetLastAppliedTopology returns the LastAppliedTopology from the Status
func (instance *CyborgConductor) GetLastAppliedTopology() *topologyv1.TopoRef {
	return instance.Status.LastAppliedTopology
}

// SetLastAppliedTopology sets the LastAppliedTopology value in the Status
func (instance *CyborgConductor) SetLastAppliedTopology(topologyRef *topologyv1.TopoRef) {
	instance.Status.LastAppliedTopology = topologyRef
}

func init() {
	SchemeBuilder.Register(&CyborgConductor{}, &CyborgConductorList{})
}
