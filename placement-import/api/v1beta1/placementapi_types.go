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

// PlacementAPISpec defines the desired state of PlacementAPI
type PlacementAPISpec struct {
	// Placement Database Hostname String
	DatabaseHostname string `json:"databaseHostname,omitempty"`
	// Placement Container Image URL
	ContainerImage string `json:"containerImage,omitempty"`
	// Replicas
	Replicas int32 `json:"replicas"`
	// Secret containing: PlacementPassword, TransportURL
	Secret string `json:"secret,omitempty"`
}

// PlacementAPIStatus defines the observed state of PlacementAPI
type PlacementAPIStatus struct {
	// DbSyncHash db sync hash
	DbSyncHash string `json:"dbSyncHash"`
	// DeploymentHash deployment hash
	DeploymentHash string `json:"deploymentHash"`
	// API endpoint
	APIEndpoint string `json:"apiEndpoint"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// PlacementAPI is the Schema for the placementapis API
type PlacementAPI struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PlacementAPISpec   `json:"spec,omitempty"`
	Status PlacementAPIStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PlacementAPIList contains a list of PlacementAPI
type PlacementAPIList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PlacementAPI `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PlacementAPI{}, &PlacementAPIList{})
}
