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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CyborgAPISpec defines the desired state of CyborgAPI.
type CyborgAPISpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of CyborgAPI. Edit cyborgapi_types.go to remove/update
	Foo string `json:"foo,omitempty"`
}

// CyborgAPIStatus defines the observed state of CyborgAPI.
type CyborgAPIStatus struct {
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// CyborgAPI is the Schema for the cyborgapis API.
type CyborgAPI struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CyborgAPISpec   `json:"spec,omitempty"`
	Status CyborgAPIStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CyborgAPIList contains a list of CyborgAPI.
type CyborgAPIList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CyborgAPI `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CyborgAPI{}, &CyborgAPIList{})
}
