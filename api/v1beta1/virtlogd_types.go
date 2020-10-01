/*
Copyright 2020 Red Hat

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

// VirtlogdSpec defines the desired state of Virtlogd
type VirtlogdSpec struct {
	// Image is the Docker image to run for the daemon
	NovaLibvirtImage string `json:"novaLibvirtImage"`
	// Name of the worker role created for OSP computes
	RoleName string `json:"roleName"`
}

// VirtlogdStatus defines the observed state of Virtlogd
type VirtlogdStatus struct {
	// Count is the number of nodes the daemon is deployed to
	Count int32 `json:"count"`
	// hashes of Secrets, CMs
	Hashes []Hash `json:"hashes,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Virtlogd is the Schema for the virtlogds API
type Virtlogd struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtlogdSpec   `json:"spec,omitempty"`
	Status VirtlogdStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VirtlogdList contains a list of Virtlogd
type VirtlogdList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Virtlogd `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Virtlogd{}, &VirtlogdList{})
}
