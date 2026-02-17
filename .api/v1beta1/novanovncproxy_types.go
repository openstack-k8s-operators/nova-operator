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
	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	service "github.com/openstack-k8s-operators/lib-common/modules/common/service"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// NovaNoVNCProxyTemplate defines the input parameters specified by the user to
// create a NovaNoVNCProxy via higher level CRDs.
type NovaNoVNCProxyTemplate struct {
	// +kubebuilder:validation:Optional
	// Enabled - Whether NovaNoVNCProxy services should be deployed and managed.
	// If it is set to false then the related NovaNoVNCProxy CR will be deleted
	// if exists and owned by the NovaCell. If it exist but not owned by the
	// NovaCell then the NovaNoVNCProxy will not be touched.
	// If it is set to true the a NovaNoVNCProxy CR will be created.
	// If there is already a manually created NovaNoVNCProxy CR with the
	// relevant name then the cell will not try to update that CR, instead the
	// NovaCell be in error state until the manually create NovaNoVNCProxy CR
	// is deleted by the operator.
	Enabled *bool `json:"enabled"`

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
	// Override, provides the ability to override the generated manifest of several child resources.
	Override VNCProxyOverrideSpec `json:"override,omitempty"`

	// +kubebuilder:validation:Optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// TLS - Parameters related to the TLS
	TLS TLSSection `json:"tls"`

	// +kubebuilder:validation:Optional
	// TopologyRef to apply the Topology defined by the associated CR referenced
	// by name
	TopologyRef *topologyv1.TopoRef `json:"topologyRef,omitempty"`
}

// TLSSection defines the desired state of TLS configuration
type TLSSection struct {
	// +kubebuilder:validation:optional
	//+operator-sdk:csv:customresourcedefinitions:type=spec
	// Service - Cert secret used for the nova novnc service endpoint
	Service tls.GenericService `json:"service,omitempty"`

	// +kubebuilder:validation:optional
	//+operator-sdk:csv:customresourcedefinitions:type=spec
	// Vencrypt - cert secret containing the x509 certificate to be presented to the VNC server.
	// The CommonName field should match the primary hostname of the controller node. If using a HA deployment,
	// the Organization field can also be configured to a value that is common across all console proxy instances in the deployment.
	// https://docs.openstack.org/nova/latest/admin/remote-console-access.html#novnc-proxy-server-configuration
	Vencrypt tls.GenericService `json:"vencrypt,omitempty"`

	// +kubebuilder:validation:optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// Secret containing CA bundle
	tls.Ca `json:",inline"`
}

// VNCProxyOverrideSpec to override the generated manifest of several child resources.
type VNCProxyOverrideSpec struct {
	// Override configuration for the Service created to serve traffic to the cluster.
	Service *service.RoutedOverrideSpec `json:"service,omitempty"`
}

// NovaNoVNCProxySpec defines the desired state of NovaNoVNCProxy
type NovaNoVNCProxySpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// +kubebuilder:validation:Required
	// CellName is the name of the Nova Cell this novncproxy belongs to.
	CellName string `json:"cellName"`

	// +kubebuilder:validation:Required
	// Secret is the name of the Secret instance containing password
	// information for the nova-novncproxy service. This secret is expected to
	// be generated by the nova-operator based on the information passed to the
	// Nova CR.
	Secret string `json:"secret"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=nova
	// ServiceUser - optional username used for this service to register in
	// keystone
	ServiceUser string `json:"serviceUser"`

	// +kubebuilder:validation:Required
	// KeystoneAuthURL - the URL that the nova-novncproxy service can use to
	// talk to keystone
	KeystoneAuthURL string `json:"keystoneAuthURL"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=regionOne
	// Region - the region name to use for service endpoint discovery
	Region string `json:"region"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=nova
	// CellDatabaseAccount - MariaDBAccount to use when accessing the cell DB
	CellDatabaseAccount string `json:"cellDatabaseAccount"`

	// +kubebuilder:validation:Required
	// CellDatabaseHostname - hostname to use when accessing the cell DB
	CellDatabaseHostname string `json:"cellDatabaseHostname"`

	// NovaServiceBase specifies the generic fields of the service
	NovaServiceBase `json:",inline"`

	// +kubebuilder:validation:Optional
	// Override, provides the ability to override the generated manifest of several child resources.
	Override VNCProxyOverrideSpec `json:"override,omitempty"`

	// +kubebuilder:validation:Required
	// ServiceAccount - service account name used internally to provide Nova services the default SA name
	ServiceAccount string `json:"serviceAccount"`

	// +kubebuilder:validation:Optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// TLS - Parameters related to the TLS
	TLS TLSSection `json:"tls"`

	// +kubebuilder:validation:Required
	// MemcachedInstance is the name of the Memcached CR that all nova service will use.
	MemcachedInstance string `json:"memcachedInstance"`
}

// NovaNoVNCProxyStatus defines the observed state of NovaNoVNCProxy
type NovaNoVNCProxyStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Map of hashes to track e.g. job status
	Hash map[string]string `json:"hash,omitempty"`

	// Conditions
	Conditions condition.Conditions `json:"conditions,omitempty" optional:"true"`

	// ReadyCount defines the number of replicas ready from nova-novncproxy
	ReadyCount int32 `json:"readyCount,omitempty"`

	// NetworkAttachments status of the deployment pods
	NetworkAttachments map[string][]string `json:"networkAttachments,omitempty"`

	// ObservedGeneration - the most recent generation observed for this
	// service. If the observed generation is less than the spec generation,
	// then the controller has not processed the latest changes injected by
	// the openstack-operator in the top-level CR (e.g. the ContainerImage)
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// LastAppliedTopology - the last applied Topology
	LastAppliedTopology *topologyv1.TopoRef `json:"lastAppliedTopology,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="NetworkAttachments",type="string",JSONPath=".spec.networkAttachments",description="NetworkAttachments"
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[0].status",description="Status"
//+kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.conditions[0].message",description="Message"

// NovaNoVNCProxy is the Schema for the novanovncproxies API
type NovaNoVNCProxy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NovaNoVNCProxySpec   `json:"spec,omitempty"`
	Status NovaNoVNCProxyStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// NovaNoVNCProxyList contains a list of NovaNoVNCProxy
type NovaNoVNCProxyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NovaNoVNCProxy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NovaNoVNCProxy{}, &NovaNoVNCProxyList{})
}

// GetConditions returns the list of conditions from the status
func (s NovaNoVNCProxyStatus) GetConditions() condition.Conditions {
	return s.Conditions
}

// NewNovaNoVNCProxySpec constructs a NewNovaNoVNCProxySpec
func NewNovaNoVNCProxySpec(
	novaCell NovaCellSpec,
) NovaNoVNCProxySpec {
	noVNCProxSpec := NovaNoVNCProxySpec{
		CellName:             novaCell.CellName,
		Secret:               novaCell.Secret,
		CellDatabaseHostname: novaCell.CellDatabaseHostname,
		CellDatabaseAccount:  novaCell.CellDatabaseAccount,
		NovaServiceBase: NovaServiceBase{
			ContainerImage:      novaCell.NoVNCContainerImageURL,
			Replicas:            novaCell.NoVNCProxyServiceTemplate.Replicas,
			NodeSelector:        novaCell.NoVNCProxyServiceTemplate.NodeSelector,
			CustomServiceConfig: novaCell.NoVNCProxyServiceTemplate.CustomServiceConfig,
			Resources:           novaCell.NoVNCProxyServiceTemplate.Resources,
			NetworkAttachments:  novaCell.NoVNCProxyServiceTemplate.NetworkAttachments,
			TopologyRef:         novaCell.NoVNCProxyServiceTemplate.TopologyRef,
		},
		KeystoneAuthURL:   novaCell.KeystoneAuthURL,
		ServiceUser:       novaCell.ServiceUser,
		Region:            novaCell.Region,
		ServiceAccount:    novaCell.ServiceAccount,
		Override:          novaCell.NoVNCProxyServiceTemplate.Override,
		TLS:               novaCell.NoVNCProxyServiceTemplate.TLS,
		MemcachedInstance: novaCell.MemcachedInstance,
	}

	if noVNCProxSpec.NodeSelector == nil {
		noVNCProxSpec.NodeSelector = novaCell.NodeSelector
	}

	if noVNCProxSpec.TopologyRef == nil {
		noVNCProxSpec.TopologyRef = novaCell.TopologyRef
	}

	return noVNCProxSpec
}

// GetSecret returns the value of the NovaMetadata.Spec.Secret
func (n NovaNoVNCProxy) GetSecret() string {
	return n.Spec.Secret
}

// GetSpecTopologyRef - Returns the LastAppliedTopology Set in the Status
func (n *NovaNoVNCProxy) GetSpecTopologyRef() *topologyv1.TopoRef {
	return n.Spec.TopologyRef
}

// GetLastAppliedTopology - Returns the LastAppliedTopology Set in the Status
func (n *NovaNoVNCProxy) GetLastAppliedTopology() *topologyv1.TopoRef {
	return n.Status.LastAppliedTopology
}

// SetLastAppliedTopology - Sets the LastAppliedTopology value in the Status
func (n *NovaNoVNCProxy) SetLastAppliedTopology(topologyRef *topologyv1.TopoRef) {
	n.Status.LastAppliedTopology = topologyRef
}
