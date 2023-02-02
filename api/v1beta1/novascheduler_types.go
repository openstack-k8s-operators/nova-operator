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

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// NovaSchedulerTemplate defines the input parameters specified by the user to
// create a NovaScheduler via higher level CRDs.
type NovaSchedulerTemplate struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="quay.io/tripleozedcentos9/openstack-nova-scheduler:current-tripleo"
	// The service specific Container Image URL
	ContainerImage string `json:"containerImage"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Maximum=32
	// +kubebuilder:validation:Minimum=0
	// Replicas of the service to run
	Replicas int32 `json:"replicas"`

	// +kubebuilder:validation:Optional
	// NodeSelector to target subset of worker nodes running this service
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// +kubebuilder:validation:Optional
	// CustomServiceConfig - customize the service config using this parameter to change service defaults,
	// or overwrite rendered information using raw OpenStack config format. The content gets added to
	// to /etc/<service>/<service>.conf.d directory as custom.conf file.
	CustomServiceConfig string `json:"customServiceConfig"`

	// +kubebuilder:validation:Optional
	// ConfigOverwrite - interface to overwrite default config files like e.g. logging.conf
	// But can also be used to add additional files. Those get added to the service config dir in /etc/<service> .
	DefaultConfigOverwrite map[string]string `json:"defaultConfigOverwrite,omitempty"`

	// +kubebuilder:validation:Optional
	// Resources - Compute Resources required by this service (Limits/Requests).
	// https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// +kubebuilder:validation:Optional
	// NetworkAttachments is a list of NetworkAttachment resource names to expose the services to the given network
	NetworkAttachments []string `json:"networkAttachments"`
}

// NovaSchedulerSpec defines the desired state of NovaScheduler
type NovaSchedulerSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// +kubebuilder:validation:Required
	// Secret is the name of the Secret instance containing password
	// information for the nova-scheduler sevice.
	Secret string `json:"secret"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default={service: NovaPassword}
	// PasswordSelectors - Field names to identify the passwords from the
	// Secret
	PasswordSelectors PasswordSelector `json:"passwordSelectors"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=nova
	// ServiceUser - optional username used for this service to register in
	// keystone
	ServiceUser string `json:"serviceUser"`

	// +kubebuilder:validation:Required
	// KeystoneAuthURL - the URL that the nova-scheduler service can use to
	// talk to keystone
	KeystoneAuthURL string `json:"keystoneAuthURL"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=nova_api
	// APIDatabaseUser - username to use when accessing the API DB
	APIDatabaseUser string `json:"apiDatabaseUser"`

	// +kubebuilder:validation:Required
	// APIDatabaseHostname - hostname to use when accessing the API DB
	APIDatabaseHostname string `json:"apiDatabaseHostname"`

	// +kubebuilder:validation:Required
	// APIMessageBusSecretName - the name of the Secret conntaining the
	// transport URL information to use when accessing the API message
	// bus.
	APIMessageBusSecretName string `json:"apiMessageBusSecretName"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default="nova_cell0"
	// Cell0DatabaseUser - username to use when accessing the cell0 DB
	Cell0DatabaseUser string `json:"cell0DatabaseUser"`

	// +kubebuilder:validation:Required
	// Cell0DatabaseHostname - hostname to use when accessing the cell0 DB
	Cell0DatabaseHostname string `json:"cell0DatabaseHostname"`

	// +kubebuilder:validation:Optional
	// Debug - enable debug for different deploy stages. If an init container
	// is used, it runs and the actual action pod gets started with sleep
	// infinity
	Debug Debug `json:"debug,omitempty"`

	// +kubebuilder:validation:Required
	// NovaServiceBase specifies the generic fields of the service
	NovaServiceBase `json:",inline"`
}

// NovaSchedulerStatus defines the observed state of NovaScheduler
type NovaSchedulerStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// Map of hashes to track e.g. job status
	Hash map[string]string `json:"hash,omitempty"`

	// Conditions
	Conditions condition.Conditions `json:"conditions,omitempty" optional:"true"`

	// ReadyCount defines the number of replicas ready from nova-scheduler
	ReadyCount int32 `json:"readyCount,omitempty"`

	// NetworkAttachments status of the deployment pods
	NetworkAttachments map[string][]string `json:"networkAttachments,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="NetworkAttachments",type="string",JSONPath=".spec.networkAttachments",description="NetworkAttachments"
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[0].status",description="Status"
//+kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.conditions[0].message",description="Message"

// NovaScheduler is the Schema for the novaschedulers API
type NovaScheduler struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NovaSchedulerSpec   `json:"spec,omitempty"`
	Status NovaSchedulerStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// NovaSchedulerList contains a list of NovaScheduler
type NovaSchedulerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NovaScheduler `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NovaScheduler{}, &NovaSchedulerList{})
}

// GetConditions returns the list of conditions from the status
func (s NovaSchedulerStatus) GetConditions() condition.Conditions {
	return s.Conditions
}
