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

// NovaCellSpec defines the desired state of NovaCell
type NovaCellSpec struct {
	// Nova Cell name, e.g. cell0
	Cell string `json:"cell,omitempty"`
	// Nova Database Hostname String
	DatabaseHostname string `json:"databaseHostname,omitempty"`
	// Nova Conductor Container Image URL
	NovaConductorContainerImage string `json:"novaConductorContainerImage,omitempty"`
	// Nova Metadata Container Image URL
	NovaMetadataContainerImage string `json:"novaMetadataContainerImage,omitempty"`
	// Nova noVnc Container Image URL
	NovaNoVNCProxyContainerImage string `json:"novaNoVNCProxyContainerImage,omitempty"`
	// Nova Conductor Replicas
	NovaConductorReplicas int32 `json:"novaConductorReplicas"`
	// Nova Metadata Replicas
	NovaMetadataReplicas int32 `json:"novaMetadataReplicas"`
	// Nova NoVNC Replicas
	NovaNoVNCProxyReplicas int32 `json:"novaNoVNCProxyReplicas"`
	// Secret containing: NovaPassword, TransportURL
	NovaSecret string `json:"novaSecret,omitempty"`
	// Secret containing: PlacementPassword
	PlacementSecret string `json:"placementSecret,omitempty"`
	// Secret containing: NeutronPassword
	NeutronSecret string `json:"neutronSecret,omitempty"`
	// Secret containing: cell transport_url
	TransportURLSecret string `json:"transportURLSecret,omitempty"`
}

// NovaCellStatus defines the observed state of NovaCell
type NovaCellStatus struct {
	// DbSyncHash db sync hash
	DbSyncHash string `json:"dbSyncHash"`
	// CreateCellHash sync hash
	CreateCellHash string `json:"createCellHash"`
	// NovaConductor statefulset hash
	NovaConductorHash string `json:"novaConductorHash"`
	// NovaMetadata deployment hash
	NovaMetadataHash string `json:"novaMetadataHash"`
	// NovaNoVNCProxy deployment hash
	NovaNoVNCProxyHash string `json:"novaNoVNCProxyHash"`
	// noVNC endpoint
	NoVNCProxyEndpoint string `json:"noVNCProxyEndpoint"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// NovaCell is the Schema for the novacells API
type NovaCell struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NovaCellSpec   `json:"spec,omitempty"`
	Status NovaCellStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NovaCellList contains a list of NovaCell
type NovaCellList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NovaCell `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NovaCell{}, &NovaCellList{})
}
