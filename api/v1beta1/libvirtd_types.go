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

// LibvirtdSpec defines the desired state of Libvirtd
type LibvirtdSpec struct {
	// Image is the Docker image to run for the daemon
	NovaLibvirtImage string `json:"novaLibvirtImage"`
	// Name of the worker role created for OSP computes
	RoleName string `json:"roleName"`
}

// LibvirtdStatus defines the observed state of Libvirtd
type LibvirtdStatus struct {
	// Count is the number of nodes the daemon is deployed to
	Count int32 `json:"count"`
	// hashes of Secrets, CMs
	Hashes []Hash `json:"hashes,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Libvirtd is the Schema for the libvirtds API
type Libvirtd struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LibvirtdSpec   `json:"spec,omitempty"`
	Status LibvirtdStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// LibvirtdList contains a list of Libvirtd
type LibvirtdList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Libvirtd `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Libvirtd{}, &LibvirtdList{})
}
