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
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// NovaExternalComputeSpec defines the desired state of NovaExternalCompute
type NovaExternalComputeSpec struct {
	// Important: Run "make" to regenerate code after modifying this file

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=nova
	// NovaInstance is the name of the Nova CR that represents the deployment
	// this compute belongs to
	NovaInstance string `json:"novaInstance"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=cell1
	// CellName defines the name of the cell this compute should be connected to
	CellName string `json:"cellName"`

	// +kubebuilder:validation:Optional
	// CustomServiceConfig - customize the service config using this parameter to change service defaults,
	// or overwrite rendered information using raw OpenStack config format. The content gets added to
	// to /etc/<service>/<service>.conf.d directory as custom.conf file.
	CustomServiceConfig string `json:"customServiceConfig"`

	// +kubebuilder:validation:Optional
	// ConfigOverwrite - interface to overwrite default config files like e.g. logging.conf
	// But can also be used to add additional files. Those get added to the service config dir in /etc/<service> .
	DefaultConfigOverwrite map[string]string `json:"defaultConfigOverwrite"`

	// +kubebuilder:validation:Required
	// InventoryConfigMapName is the name of the k8s config map that contains the ansible inventory information
	// for this node
	InventoryConfigMapName string `json:"inventoryConfigMapName"`

	// +kubebuilder:validation:Required
	// SSHKeySecretName is the name of the k8s Secret that contains the ssh keys to access the node
	SSHKeySecretName string `json:"sshKeySecretName"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	// Deploy true means the nova-operator is allowed to do changes on the external compute node
	// if necessary. If set to false then the operator will only generate the pre-requisite data (e.g. config maps)
	// but does not do any modification on the compute node itself.
	Deploy bool `json:"deploy"`
}

// NovaExternalComputeStatus defines the observed state of NovaExternalCompute
type NovaExternalComputeStatus struct {
	// Important: Run "make" to regenerate code after modifying this file

	// Map of hashes to track e.g. job status
	Hash map[string]string `json:"hash,omitempty"`

	// Conditions
	Conditions condition.Conditions `json:"conditions,omitempty" optional:"true"`

	// NOTE(gibi): If nova-operator ever needs RPM packages to be installed to
	// the host then we need to communicate in the Status to the
	// dataplane-operator probably as a list of pacakge names and a list of
	// package repositories.
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// NovaExternalCompute is the Schema for the novaexternalcomputes API
type NovaExternalCompute struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NovaExternalComputeSpec   `json:"spec,omitempty"`
	Status NovaExternalComputeStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// NovaExternalComputeList contains a list of NovaExternalCompute
type NovaExternalComputeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NovaExternalCompute `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NovaExternalCompute{}, &NovaExternalComputeList{})
}

// GetConditions returns the list of conditions from the status
func (s NovaExternalComputeStatus) GetConditions() condition.Conditions {
	return s.Conditions
}

// IsReady returns true if Nova reconciled successfully
func (c NovaExternalCompute) IsReady() bool {
	readyCond := c.Status.Conditions.Get(condition.ReadyCondition)
	return readyCond != nil && readyCond.Status == corev1.ConditionTrue
}
