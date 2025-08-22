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

	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
)

// Container image fall-back defaults
const (
	NovaAPIContainerImage       = "quay.io/podified-antelope-centos9/openstack-nova-api:current-podified"
	NovaConductorContainerImage = "quay.io/podified-antelope-centos9/openstack-nova-conductor:current-podified"
	NovaMetadataContainerImage  = "quay.io/podified-antelope-centos9/openstack-nova-api:current-podified"
	NovaNoVNCContainerImage     = "quay.io/podified-antelope-centos9/openstack-nova-novncproxy:current-podified"
	NovaSchedulerContainerImage = "quay.io/podified-antelope-centos9/openstack-nova-scheduler:current-podified"
	NovaComputeContainerImage   = "quay.io/podified-antelope-centos9/openstack-nova-compute:current-podified"
)

// Compute drivers names
const (
	IronicDriver = "ironic.IronicDriver"
)

const (
	// ComputeDiscoverHashKey is the key to hash of compute discovery job based on compute templates for cell
	ComputeDiscoverHashKey = "nova-compute-discovery"
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
	NodeSelector *map[string]string `json:"nodeSelector,omitempty"`

	// +kubebuilder:validation:Optional
	// CustomServiceConfig - customize the service config using this parameter to change service defaults,
	// or overwrite rendered information using raw OpenStack config format. The content gets added to
	// to /etc/<service>/<service>.conf.d directory as custom.conf file.
	CustomServiceConfig string `json:"customServiceConfig"`

	// +kubebuilder:validation:Optional
	// Resources - Compute Resources required by this service (Limits/Requests).
	// https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// +kubebuilder:validation:Optional
	// NetworkAttachments is a list of NetworkAttachment resource names to expose the services to the given network
	NetworkAttachments []string `json:"networkAttachments,omitempty"`

	// +kubebuilder:validation:Optional
	// TopologyRef to apply the Topology defined by the associated CR referenced
	// by name
	TopologyRef *topologyv1.TopoRef `json:"topologyRef,omitempty"`
}

// PasswordSelector to identify the DB and AdminUser password from the Secret
type PasswordSelector struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="NovaPassword"
	// Service - Selector to get the keystone service user password from the
	// Secret
	Service string `json:"service"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="MetadataSecret"
	// MetadataSecret - the name of the field to get the metadata secret from the
	// Secret
	MetadataSecret string `json:"metadataSecret"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="MetadataCellsSecret"
	// prefixMetadataCellsSecret - the prefix name of the field to get the metadata secret from the
	// Secret for cells. Vale of metadata_proxy_shared_secret
	// information for the nova-metadata service. This secret is shared
	// between nova and neutron ovn-metadata inside selected cell
	// and if this is not defined the global metadata_proxy_shared_secret
	// secret will be used
	PrefixMetadataCellsSecret string `json:"prefixMetadataCellsSecret"`
}

type NovaImages struct {
	// +kubebuilder:validation:Required
	// APIContainerImageURL
	APIContainerImageURL string `json:"apiContainerImageURL"`

	// +kubebuilder:validation:Required
	// SchedulerContainerImageURL
	SchedulerContainerImageURL string `json:"schedulerContainerImageURL"`

	NovaCellImages `json:",inline"`
}

func (r *NovaImages) Default(defaults NovaDefaults) {
	r.NovaCellImages.Default(defaults.NovaCellDefaults)
	if r.APIContainerImageURL == "" {
		r.APIContainerImageURL = defaults.APIContainerImageURL
	}
	if r.SchedulerContainerImageURL == "" {
		r.SchedulerContainerImageURL = defaults.SchedulerContainerImageURL
	}
}

type NovaCellImages struct {

	// +kubebuilder:validation:Required
	// ConductorContainerImageURL
	ConductorContainerImageURL string `json:"conductorContainerImageURL"`

	// +kubebuilder:validation:Required
	// MetadataContainerImageURL
	MetadataContainerImageURL string `json:"metadataContainerImageURL"`

	// +kubebuilder:validation:Required
	// NoVNCContainerImageURL
	NoVNCContainerImageURL string `json:"novncproxyContainerImageURL"`

	// +kubebuilder:validation:Required
	// NovaComputeContainerImageURL
	NovaComputeContainerImageURL string `json:"computeContainerImageURL"`
}

func (r *NovaCellImages) Default(defaults NovaCellDefaults) {
	if r.ConductorContainerImageURL == "" {
		r.ConductorContainerImageURL = defaults.ConductorContainerImageURL
	}
	if r.MetadataContainerImageURL == "" {
		r.MetadataContainerImageURL = defaults.MetadataContainerImageURL
	}
	if r.NoVNCContainerImageURL == "" {
		r.NoVNCContainerImageURL = defaults.NoVNCContainerImageURL
	}
	if r.NovaComputeContainerImageURL == "" {
		r.NovaComputeContainerImageURL = defaults.NovaComputeContainerImageURL
	}
}

// SetupDefaults - initializes any CRD field defaults based on environment variables (the defaulting mechanism itself is implemented via webhooks)
func SetupDefaults() {
	// Acquire environmental defaults and initialize NovaCell defaults with them
	novaCellDefaults := NovaCellDefaults{
		ConductorContainerImageURL:   util.GetEnvVar("RELATED_IMAGE_NOVA_CONDUCTOR_IMAGE_URL_DEFAULT", NovaConductorContainerImage),
		MetadataContainerImageURL:    util.GetEnvVar("RELATED_IMAGE_NOVA_API_IMAGE_URL_DEFAULT", NovaMetadataContainerImage),
		NoVNCContainerImageURL:       util.GetEnvVar("RELATED_IMAGE_NOVA_NOVNC_IMAGE_URL_DEFAULT", NovaNoVNCContainerImage),
		NovaComputeContainerImageURL: util.GetEnvVar("RELATED_IMAGE_NOVA_COMPUTE_IMAGE_URL_DEFAULT", NovaComputeContainerImage),
	}

	SetupNovaCellDefaults(novaCellDefaults)

	// Acquire environmental defaults and initialize Nova defaults with them
	novaDefaults := NovaDefaults{
		APIContainerImageURL:       util.GetEnvVar("RELATED_IMAGE_NOVA_API_IMAGE_URL_DEFAULT", NovaAPIContainerImage),
		SchedulerContainerImageURL: util.GetEnvVar("RELATED_IMAGE_NOVA_SCHEDULER_IMAGE_URL_DEFAULT", NovaSchedulerContainerImage),
		NovaCellDefaults:           novaCellDefaults,
		APITimeout:                 60,
	}

	SetupNovaDefaults(novaDefaults)

	// Acquire environmental defaults and initialize NovaAPI defaults with them
	novaAPIDefaults := NovaAPIDefaults{
		ContainerImageURL: util.GetEnvVar("RELATED_IMAGE_NOVA_API_IMAGE_URL_DEFAULT", NovaAPIContainerImage),
	}

	SetupNovaAPIDefaults(novaAPIDefaults)

	// Acquire environmental defaults and initialize NovaConductor defaults with them
	novaConductorDefaults := NovaConductorDefaults{
		ContainerImageURL: util.GetEnvVar("RELATED_IMAGE_NOVA_CONDUCTOR_IMAGE_URL_DEFAULT", NovaConductorContainerImage),
	}

	SetupNovaConductorDefaults(novaConductorDefaults)

	// Acquire environmental defaults and initialize NovaMetadata defaults with them
	novaMetadataDefaults := NovaMetadataDefaults{
		ContainerImageURL: util.GetEnvVar("RELATED_IMAGE_NOVA_API_IMAGE_URL_DEFAULT", NovaMetadataContainerImage),
	}

	SetupNovaMetadataDefaults(novaMetadataDefaults)

	// Acquire environmental defaults and initialize NovaNoVNCProxy defaults with them
	novaNoVNCProxyDefaults := NovaNoVNCProxyDefaults{
		ContainerImageURL: util.GetEnvVar("RELATED_IMAGE_NOVA_NOVNC_IMAGE_URL_DEFAULT", NovaNoVNCContainerImage),
	}

	SetupNovaNoVNCProxyDefaults(novaNoVNCProxyDefaults)

	// Acquire environmental defaults and initialize NovaScheduler defaults with them
	novaSchedulerDefaults := NovaSchedulerDefaults{
		ContainerImageURL: util.GetEnvVar("RELATED_IMAGE_NOVA_SCHEDULER_IMAGE_URL_DEFAULT", NovaSchedulerContainerImage),
	}
	SetupNovaSchedulerDefaults(novaSchedulerDefaults)

	// Acquire environmental defaults and initialize NovaCompute defaults with them
	novaComputeDefaults := NovaComputeDefaults{
		ContainerImageURL: util.GetEnvVar("RELATED_IMAGE_NOVA_COMPUTE_IMAGE_URL_DEFAULT", NovaComputeContainerImage),
	}

	SetupNovaComputeDefaults(novaComputeDefaults)
}
