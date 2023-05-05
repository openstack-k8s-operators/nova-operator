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
	"os"

	corev1 "k8s.io/api/core/v1"

	"github.com/openstack-k8s-operators/lib-common/modules/common/endpoint"
)

// Container image fall-back defaults
const (
	NovaAPIContainerImage       = "quay.io/podified-antelope-centos9/openstack-nova-api:current-podified"
	NovaConductorContainerImage = "quay.io/podified-antelope-centos9/openstack-nova-conductor:current-podified"
	NovaMetadataContainerImage  = "quay.io/podified-antelope-centos9/openstack-nova-api:current-podified"
	NovaNoVNCContainerImage     = "quay.io/podified-antelope-centos9/openstack-nova-novncproxy:current-podified"
	NovaSchedulerContainerImage = "quay.io/podified-antelope-centos9/openstack-nova-scheduler:current-podified"
	NovaComputeContainerImage   = "quay.io/podified-antelope-centos9/openstack-nova-compute:current-podified"
	NovaLibvirtContainerImage   = "quay.io/podified-antelope-centos9/openstack-nova-libvirt:current-podified"
	AnsibleEEContainerImage     = "quay.io/openstack-k8s-operators/openstack-ansibleee-runner:latest"
)

// NovaServiceBase contains the fields that are needed for each nova service CRD
type NovaServiceBase struct {
	// +kubebuilder:validation:Optional
	// The service specific Container Image URL (will be set to environmental default if empty)
	ContainerImage string `json:"containerImage"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Maximum=32
	// +kubebuilder:validation:Minimum=0
	// Replicas of the service to run
	Replicas *int32 `json:"replicas"`

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
	NetworkAttachments []string `json:"networkAttachments,omitempty"`
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
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="MetadataSecret"
	// MetadataSecret - the name of the field to get the metadata secret from the
	// Secret
	MetadataSecret string `json:"metadataSecret"`
}

// MetalLBConfig to configure the MetalLB loadbalancer service
type MetalLBConfig struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=internal;public
	// Endpoint, OpenStack endpoint this service maps to
	Endpoint endpoint.Endpoint `json:"endpoint"`

	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// IPAddressPool expose VIP via MetalLB on the IPAddressPool
	IPAddressPool string `json:"ipAddressPool"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	// SharedIP if true, VIP/VIPs get shared with multiple services
	SharedIP bool `json:"sharedIP"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=""
	// SharedIPKey specifies the sharing key which gets set as the annotation on the LoadBalancer service.
	// Services which share the same VIP must have the same SharedIPKey. Defaults to the IPAddressPool if
	// SharedIP is true, but no SharedIPKey specified.
	SharedIPKey string `json:"sharedIPKey"`

	// +kubebuilder:validation:Optional
	// LoadBalancerIPs, request given IPs from the pool if available. Using a list to allow dual stack (IPv4/IPv6) support
	LoadBalancerIPs []string `json:"loadBalancerIPs"`
}

// TODO: This will be moved to lib-common so that all operators can use the pattern
// GetEnvDefault - Get the value associated with key from environment variables, but use baseDefault as a value in the case of an empty string
func GetEnvDefault(key string, baseDefault string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return baseDefault
}

// SetupDefaults - initializes any CRD field defaults based on environment variables (the defaulting mechanism itself is implemented via webhooks)
func SetupDefaults() {
	// Acquire environmental defaults and initialize Nova defaults with them
	novaDefaults := NovaDefaults{
		APIContainerImageURL:       GetEnvDefault("NOVA_API_IMAGE_URL_DEFAULT", NovaAPIContainerImage),
		ConductorContainerImageURL: GetEnvDefault("NOVA_CONDUCTOR_IMAGE_URL_DEFAULT", NovaConductorContainerImage),
		MetadataContainerImageURL:  GetEnvDefault("NOVA_METADATA_IMAGE_URL_DEFAULT", NovaMetadataContainerImage),
		NoVNCContainerImageURL:     GetEnvDefault("NOVA_NOVNC_IMAGE_URL_DEFAULT", NovaNoVNCContainerImage),
		SchedulerContainerImageURL: GetEnvDefault("NOVA_SCHEDULER_IMAGE_URL_DEFAULT", NovaSchedulerContainerImage),
	}

	SetupNovaDefaults(novaDefaults)

	// Acquire environmental defaults and initialize NovaCell defaults with them
	novaCellDefaults := NovaCellDefaults{
		ConductorContainerImageURL: GetEnvDefault("NOVA_CONDUCTOR_IMAGE_URL_DEFAULT", NovaConductorContainerImage),
		MetadataContainerImageURL:  GetEnvDefault("NOVA_METADATA_IMAGE_URL_DEFAULT", NovaMetadataContainerImage),
		NoVNCContainerImageURL:     GetEnvDefault("NOVA_NOVNC_IMAGE_URL_DEFAULT", NovaNoVNCContainerImage),
	}

	SetupNovaCellDefaults(novaCellDefaults)

	// Acquire environmental defaults and initialize NovaExternalCompute defaults with them
	novaExternalComputeDefaults := NovaExternalComputeDefaults{
		ComputeContainerImageURL:   GetEnvDefault("NOVA_COMPUTE_IMAGE_URL_DEFAULT", NovaComputeContainerImage),
		LibvirtContainerImageURL:   GetEnvDefault("NOVA_LIBVIRT_IMAGE_URL_DEFAULT", NovaLibvirtContainerImage),
		AnsibleEEContainerImageURL: GetEnvDefault("NOVA_ANSIBLE_EE_IMAGE_URL_DEFAULT", AnsibleEEContainerImage),
	}

	SetupNovaExternalComputeDefaults(novaExternalComputeDefaults)
}
