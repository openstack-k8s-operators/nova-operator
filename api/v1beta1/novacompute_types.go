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

// NovaComputeSpec defines the desired state of NovaCompute
type NovaComputeSpec struct {
	// name of configmap which holds general information on the OSP env
	CommonConfigMap string `json:"commonConfigMap"`
	// name of secret which holds sensitive information on the OSP env
	OspSecrets string `json:"ospSecrets"`
	// container image to run for the daemon
	NovaComputeImage string `json:"novaComputeImage"`
	// Mask of host CPUs that can be used for ``VCPU`` resources and offloaded
	// emulator threads. For more information, refer to the documentation.
	NovaComputeCPUSharedSet string `json:"novaComputeCPUSharedSet,omitempty"`
	// A list or range of host CPU cores to which processes for pinned instance
	// CPUs (PCPUs) can be scheduled.
	// Ex. NovaComputeCPUDedicatedSet: [4-12,^8,15] will reserve cores from 4-12
	// and 15, excluding 8.
	NovaComputeCPUDedicatedSet string `json:"novaComputeCPUDedicatedSet,omitempty"`
	// service account used to create pods
	ServiceAccount string `json:"serviceAccount"`
	// Name of the worker role created for OSP computes
	RoleName string `json:"roleName"`
}

// NovaComputeStatus defines the observed state of NovaCompute
type NovaComputeStatus struct {
	// Count is the number of nodes the daemon is deployed to
	Count int32 `json:"count"`
	// Daemonset hash used to detect changes
	DaemonsetHash string `json:"daemonsetHash"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// NovaCompute is the Schema for the novacomputes API
type NovaCompute struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NovaComputeSpec   `json:"spec,omitempty"`
	Status NovaComputeStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NovaComputeList contains a list of NovaCompute
type NovaComputeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NovaCompute `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NovaCompute{}, &NovaComputeList{})
}
