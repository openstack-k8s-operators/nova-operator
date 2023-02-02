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
	corev1 "k8s.io/api/core/v1"
)

// NovaServiceBase contains the fields that are needed for each nova service CRD
type NovaServiceBase struct {
	// +kubebuilder:validation:Required
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
	// ConfigOverwrite - interface to overwrite default config files like e.g. logging.conf or policy.json.
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

// Debug allows enabling different debug option for the operator
type Debug struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	// StopDBSync allows stopping the init container before running db sync
	// to apply the DB schema
	// QUESTION(gibi): Not all CR will run dbsync, should we have per CR
	// Debug struct or keep this generic one and ignore fields in the
	// controller that are not applicable
	StopDBSync bool `json:"stopDBSync"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	// StopService allows stopping the service container before staring
	// the openstack service binary
	// QUESTION(gibi): Not all CR will run a service, should we have per CR
	// Debug struct or keep this generic one and ignore fields in the
	// controller that are not applicable
	StopService bool `json:"stopService"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	// PreserveJobs - do not delete jobs after they finished e.g. to check logs
	PreserveJobs bool `json:"preserveJobs"`
}

// PasswordSelector to identify the DB and AdminUser password from the Secret
type PasswordSelector struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="NovaPassword"
	// Service - Selector to get the keystone service user password from the
	// Secret
	Service string `json:"service"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="NovaAPIDatabasePassword"
	// APIDatabase - the name of the field to get the API DB password from the
	// Secret
	APIDatabase string `json:"apiDatabase"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="NovaCell0DatabasePassword"
	// CellDatabase - the name of the field to get the Cell DB password from
	// the Secret
	CellDatabase string `json:"cellDatabase"`
}
