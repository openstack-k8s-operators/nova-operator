package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ComputeSpec defines the desired state of Compute
// +k8s:openapi-gen=true
type ComputeSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book.kubebuilder.io/beyond_basics/generating_crd.html

        // Label is the value of the 'daemon=' label to set on a node that should run the daemon
        Label string `json:"label"`

        // Image is the Docker image to run for the daemon
        NovaLibvirtImage string `json:"novaLibvirtImage"`
        NovaComputeImage string `json:"novaComputeImage"`
}

// ComputeStatus defines the observed state of Compute
// +k8s:openapi-gen=true
type ComputeStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book.kubebuilder.io/beyond_basics/generating_crd.html

        // Count is the number of nodes the daemon is deployed to
        Count int32 `json:"count"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Compute is the Schema for the computes API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
type Compute struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ComputeSpec   `json:"spec,omitempty"`
	Status ComputeStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ComputeList contains a list of Compute
type ComputeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Compute `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Compute{}, &ComputeList{})
}
