package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// VirtlogdSpec defines the desired state of Virtlogd
// +k8s:openapi-gen=true
type VirtlogdSpec struct {
	// Image is the Docker image to run for the daemon
	NovaLibvirtImage string `json:"novaLibvirtImage"`
	// service account used to create pods
	ServiceAccount string `json:"serviceAccount"`
        // Name of the worker role created for OSP computes
        RoleName string `json:"roleName"`
}

// VirtlogdStatus defines the observed state of Virtlogd
// +k8s:openapi-gen=true
type VirtlogdStatus struct {
	// Count is the number of nodes the daemon is deployed to
	Count int32 `json:"count"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Virtlogd is the Schema for the virtlogds API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
type Virtlogd struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtlogdSpec   `json:"spec,omitempty"`
	Status VirtlogdStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VirtlogdList contains a list of Virtlogd
type VirtlogdList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Virtlogd `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Virtlogd{}, &VirtlogdList{})
}
