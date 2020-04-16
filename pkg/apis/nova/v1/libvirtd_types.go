package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LibvirtdSpec defines the desired state of Libvirtd
// +k8s:openapi-gen=true
type LibvirtdSpec struct {
        // Label is the value of the 'daemon=' label to set on a node that should run the daemon
        Label string `json:"label"` 

        // Image is the Docker image to run for the daemon
        NovaLibvirtImage string `json:"novaLibvirtImage"`

        // service account used to create pods
        ServiceAccount string `json:"serviceAccount"`
}

// LibvirtdStatus defines the observed state of Libvirtd
// +k8s:openapi-gen=true
type LibvirtdStatus struct {
        // Count is the number of nodes the daemon is deployed to
        Count int32 `json:"count"`
        // Daemonset hash used to detect changes
        DaemonsetHash string `json:"daemonsetHash"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Libvirtd is the Schema for the libvirtds API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
type Libvirtd struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LibvirtdSpec   `json:"spec,omitempty"`
	Status LibvirtdStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// LibvirtdList contains a list of Libvirtd
type LibvirtdList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Libvirtd `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Libvirtd{}, &LibvirtdList{})
}
