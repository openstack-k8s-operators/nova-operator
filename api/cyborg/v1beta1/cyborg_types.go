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

// CyborgSpec defines the desired state of Cyborg.
type CyborgSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of Cyborg. Edit cyborg_types.go to remove/update
	Foo string `json:"foo,omitempty"`
}

// CyborgStatus defines the observed state of Cyborg.
type CyborgStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Cyborg is the Schema for the cyborgs API.
type Cyborg struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CyborgSpec   `json:"spec,omitempty"`
	Status CyborgStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CyborgList contains a list of Cyborg.
type CyborgList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Cyborg `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Cyborg{}, &CyborgList{})
}
