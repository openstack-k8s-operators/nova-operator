package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NovaComputeSpec defines the desired state of NovaCompute
// +k8s:openapi-gen=true
type NovaComputeSpec struct {
        // Label is the value of the 'daemon=' label to set on a node that should run the daemon
        Label string `json:"label"`
        // container image to run for the daemon
        NovaComputeImage string `json:"novaComputeImage"`
        // Control Plane public VIP String
        PublicVip string `json:"publicVip"`
        // Control Plane internalAPI VIP String
        InternalApiVip string `json:"internalApiVip"`
        // Memcache Servers String
        MemcacheServers string `json:"memcacheServers,omitempty"`
        // RabbitMQ transport URL String
        RabbitTransportUrl string `json:"rabbitTransportUrl"`
        // Cinder API Admin Password
        CinderPassword string `json:"cinderPassword"`
        // Nova API Admin Password
        NovaPassword string `json:"novaPassword"`
        // Mask of host CPUs that can be used for ``VCPU`` resources and offloaded
        // emulator threads. For more information, refer to the documentation.
        NovaComputeCpuSharedSet string `json:"novaComputeCpuSharedSet,omitempty"`
        // A list or range of host CPU cores to which processes for pinned instance
        // CPUs (PCPUs) can be scheduled.
        // Ex. NovaComputeCpuDedicatedSet: [4-12,^8,15] will reserve cores from 4-12
        // and 15, excluding 8.
        NovaComputeCpuDedicatedSet string `json:"novaComputeCpuDedicatedSet,omitempty"`
        // Neutron API Admin Password
        NeutronPassword string `json:"neutronPassword"`
        // Placement API Admin Password
        PlacementPassword string `json:"placementPassword"`
        // service account used to create pods
        ServiceAccount string `json:"serviceAccount"`
}

// NovaComputeStatus defines the observed state of NovaCompute
// +k8s:openapi-gen=true
type NovaComputeStatus struct {
        // Count is the number of nodes the daemon is deployed to
        Count int32 `json:"count"`
        // Daemonset hash used to detect changes
        DaemonsetHash string `json:"daemonsetHash"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// NovaCompute is the Schema for the novacomputes API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
type NovaCompute struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NovaComputeSpec   `json:"spec,omitempty"`
	Status NovaComputeStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// NovaComputeList contains a list of NovaCompute
type NovaComputeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NovaCompute `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NovaCompute{}, &NovaComputeList{})
}
