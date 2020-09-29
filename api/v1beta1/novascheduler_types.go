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

// NovaSchedulerSpec defines the desired state of NovaScheduler
type NovaSchedulerSpec struct {
	// CR name of managing controller object to identify the config maps
	ManagingCrName string `json:"managingCrName,omitempty"`
	// Nova Database Hostname String
	DatabaseHostname string `json:"databaseHostname,omitempty"`
	// Nova Scheduler Container Image URL
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

// NovaSchedulerStatus defines the observed state of NovaScheduler
type NovaSchedulerStatus struct {
	// NovaScheduler statefulset hash
	NovaSchedulerHash string `json:"novaSchedulerHash"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// NovaScheduler is the Schema for the novaschedulers API
type NovaScheduler struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NovaSchedulerSpec   `json:"spec,omitempty"`
	Status NovaSchedulerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NovaSchedulerList contains a list of NovaScheduler
type NovaSchedulerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NovaScheduler `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NovaScheduler{}, &NovaSchedulerList{})
}
