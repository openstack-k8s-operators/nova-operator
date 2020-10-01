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

// IscsidSpec defines the desired state of Iscsid
type IscsidSpec struct {
	// Image is the Docker image to run for the daemon
	IscsidImage string `json:"iscsidImage"`
	// Name of the worker role created for OSP computes
	RoleName string `json:"roleName"`
}

// IscsidStatus defines the observed state of Iscsid
type IscsidStatus struct {
	// Count is the number of nodes the daemon is deployed to
	Count int32 `json:"count"`
	// hashes of Secrets, CMs
	Hashes []Hash `json:"hashes,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Iscsid is the Schema for the iscsids API
type Iscsid struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IscsidSpec   `json:"spec,omitempty"`
	Status IscsidStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// IscsidList contains a list of Iscsid
type IscsidList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Iscsid `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Iscsid{}, &IscsidList{})
}
