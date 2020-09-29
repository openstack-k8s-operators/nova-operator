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

// NovaSpec defines the desired state of Nova
type NovaSpec struct {
	// Nova Cell name, e.g. cell0
	Cell string `json:"cell,omitempty"`
	// Nova Database Hostname String
	DatabaseHostname string `json:"databaseHostname,omitempty"`
	// Nova API Container Image URL
	NovaAPIContainerImage string `json:"novaAPIContainerImage,omitempty"`
	// Nova Scheduler Container Image URL
	NovaSchedulerContainerImage string `json:"novaSchedulerContainerImage,omitempty"`
	// Nova Conductor Container Image URL
	NovaConductorContainerImage string `json:"novaConductorContainerImage,omitempty"`
	// Nova API Replicas
	NovaAPIReplicas int32 `json:"novaAPIReplicas"`
	// Nova Scheduler Replicas
	NovaSchedulerReplicas int32 `json:"novaSchedulerReplicas"`
	// Nova Conductor Replicas
	NovaConductorReplicas int32 `json:"novaConductorReplicas"`
	// Secret containing: NovaPassword, TransportURL
	NovaSecret string `json:"novaSecret,omitempty"`
	// Secret containing: PlacementPassword
	PlacementSecret string `json:"placementSecret,omitempty"`
	// Secret containing: NeutronPassword
	NeutronSecret string `json:"neutronSecret,omitempty"`
	// Secret containing: cell transport_url
	TransportURLSecret string `json:"transportURLSecret,omitempty"`
	// Cells to create
	Cells []Cell `json:"cells,omitempty"`
}

// Cell defines nova cell configuration parameters
type Cell struct {
	// Name of cell
	Name string `json:"name,omitempty"`
	// Hostname of Cell DB server, if not provided same as NovaSpec DatabaseHostname
	DatabaseHostname string `json:"databaseHostname,omitempty"`
	// Name of secret which provides the cell transport url
	TransportURLSecret string `json:"transportURLSecret,omitempty"`
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
}

// NovaStatus defines the observed state of Nova
type NovaStatus struct {
	// DbSyncHash db sync hash
	DbSyncHash string `json:"dbSyncHash"`
	// NovaAPIHash deployment hash
	NovaAPIHash string `json:"novaAPIHash"`
	// NovaScheduler statefulset hash
	NovaSchedulerHash string `json:"novaSchedulerHash"`
	// NovaConductor statefulset hash
	NovaConductorHash string `json:"novaConductorHash"`
	// Cells status hash
	CellsHash string `json:"cellsHash"`
	// API endpoint
	APIEndpoint string `json:"apiEndpoint"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Nova is the Schema for the nova API
type Nova struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NovaSpec   `json:"spec,omitempty"`
	Status NovaStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NovaList contains a list of Nova
type NovaList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Nova `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Nova{}, &NovaList{})
}
