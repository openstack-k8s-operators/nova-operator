package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// IscsidSpec defines the desired state of Iscsid
// +k8s:openapi-gen=true
type IscsidSpec struct {
	// Image is the Docker image to run for the daemon
	IscsidImage string `json:"iscsidImage"`
	// service account used to create pods
	ServiceAccount string `json:"serviceAccount"`
        // Name of the worker role created for OSP computes
        RoleName string `json:"roleName"`
}

// IscsidStatus defines the observed state of Iscsid
// +k8s:openapi-gen=true
type IscsidStatus struct {
	// Count is the number of nodes the daemon is deployed to
	Count int32 `json:"count"`
	// Daemonset hash used to detect changes
	DaemonsetHash string `json:"daemonsetHash"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Iscsid is the Schema for the iscsids API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
type Iscsid struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IscsidSpec   `json:"spec,omitempty"`
	Status IscsidStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// IscsidList contains a list of Iscsid
type IscsidList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Iscsid `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Iscsid{}, &IscsidList{})
}
