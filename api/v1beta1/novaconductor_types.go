/*


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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NovaConductorSpec defines the desired state of NovaConductor
type NovaConductorSpec struct {
	// CR name of managing controller object to identify the config maps
	ManagingCrName string `json:"managingCrName,omitempty"`
	// Name of the cell, e.g. cell1
	Cell string `json:"cell,omitempty"`
	// Nova Database Hostname String
	DatabaseHostname string `json:"databaseHostname,omitempty"`
	// Nova Conductor Container Image URL
	ContainerImage string `json:"containerImage,omitempty"`
	// Nova API Replicas
	Replicas int32 `json:"replicas"`
	// Secret containing: NovaPassword, TransportURL
	NovaSecret string `json:"novaSecret,omitempty"`
	// Secret containing: PlacementPassword
	PlacementSecret string `json:"placementSecret,omitempty"`
	// Secret containing: NeutronPassword
	NeutronSecret string `json:"neutronSecret,omitempty"`
	// Secret containing: cell transport_url
	TransportURLSecret string `json:"transportURLSecret,omitempty"`
}

// NovaConductorStatus defines the observed state of NovaConductor
type NovaConductorStatus struct {
	// NovaConductor statefulset hash
	NovaConductorHash string `json:"novaConductorHash"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// NovaConductor is the Schema for the novaconductors API
type NovaConductor struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NovaConductorSpec   `json:"spec,omitempty"`
	Status NovaConductorStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NovaConductorList contains a list of NovaConductor
type NovaConductorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NovaConductor `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NovaConductor{}, &NovaConductorList{})
}
