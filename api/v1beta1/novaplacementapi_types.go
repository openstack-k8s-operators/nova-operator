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
        service "github.com/openstack-k8s-operators/lib-common/modules/common/service"
        "github.com/openstack-k8s-operators/lib-common/modules/common/tls"
        corev1 "k8s.io/api/core/v1"
        metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
        topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// NovaPlacementAPITemplate defines the input parameters specified by the user to
// create a NovaPlacement via higher level CRDs.

// NovaPlacementAPISpec defines the desired state of NovaPlacementAPI
type NovaPlacementAPISpec struct {
	NovaPlacementAPISpecCore `json:",inline"`

	// +kubebuilder:validation:Required
	//NovaPlacementAPI Container Image URL (will be set to environmental default if empty)
	ContainerImage string `json:"containerImage"`
}

// NovaPlacementAPISpecCore -
type NovaPlacementAPISpecCore struct {
        // +kubebuilder:validation:Optional
        // +kubebuilder:default=60
        // +kubebuilder:validation:Minimum=10
        // APITimeout for HAProxy, Apache
        APITimeout int `json:"apiTimeout"`

        // +kubebuilder:validation:Optional
        // +kubebuilder:default=placement
        // ServiceUser - optional username used for this service to register in keystone
        ServiceUser string `json:"serviceUser"`

        // +kubebuilder:validation:Required
        // MariaDB instance name
        // Right now required by the maridb-operator to get the credentials from the instance to create the DB
        // Might not be required in future
        DatabaseInstance string `json:"databaseInstance"`

        // +kubebuilder:validation:Optional
        // +kubebuilder:default=placement
        // DatabaseAccount - name of MariaDBAccount which will be used to connect.
        DatabaseAccount string `json:"databaseAccount"`

        // +kubebuilder:validation:Optional
        // +kubebuilder:default=1
        // +kubebuilder:validation:Maximum=32
        // +kubebuilder:validation:Minimum=0
        // Replicas of placement API to run
        Replicas *int32 `json:"replicas"`

        // +kubebuilder:validation:Required
        // Secret containing OpenStack password information for placement PlacementPassword
        Secret string `json:"secret"`

        // +kubebuilder:validation:Optional
        // +kubebuilder:default={service: PlacementPassword}
        // PasswordSelectors - Selectors to identify the DB and ServiceUser password from the Secret
        PasswordSelectors PasswordSelector `json:"passwordSelectors"`

        // +kubebuilder:validation:Optional
        // NodeSelector to target subset of worker nodes running this service
        NodeSelector *map[string]string `json:"nodeSelector,omitempty"`

        // +kubebuilder:validation:Optional
        // +kubebuilder:default=false
        // PreserveJobs - do not delete jobs after they finished e.g. to check logs
        PreserveJobs bool `json:"preserveJobs"`

        // +kubebuilder:validation:Optional
        // CustomServiceConfig - customize the service config using this parameter to change service defaults,
        // or overwrite rendered information using raw OpenStack config format. The content gets added to
        // to /etc/<service>/<service>.conf.d directory as custom.conf file.
        CustomServiceConfig string `json:"customServiceConfig"`

        // +kubebuilder:validation:Optional
        // DefaultConfigOverwrite - interface to overwrite default config files like policy.yaml.
        DefaultConfigOverwrite map[string]string `json:"defaultConfigOverwrite,omitempty"`

        // +kubebuilder:validation:Optional
        // Resources - Compute Resources required by this service (Limits/Requests).
        // https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
        Resources corev1.ResourceRequirements `json:"resources,omitempty"`

        // +kubebuilder:validation:Optional
        // NetworkAttachments is a list of NetworkAttachment resource names to expose the services to the given network
        NetworkAttachments []string `json:"networkAttachments,omitempty"`

        // +kubebuilder:validation:Optional
        // Override, provides the ability to override the generated manifest of several child resources.
        Override APIOverrideSpec `json:"override,omitempty"`

        // +kubebuilder:validation:Optional
        // +operator-sdk:csv:customresourcedefinitions:type=spec
        // TLS - Parameters related to the TLS
        TLS tls.API `json:"tls,omitempty"`

        // +kubebuilder:validation:Optional
        // TopologyRef to apply the Topology defined by the associated CR referenced
        // by name
        TopologyRef *topologyv1.TopoRef `json:"topologyRef,omitempty"`
}

// APIOverrideSpec to override the generated manifest of several child resources.
type APIOverrideSpec struct {
        // Override configuration for the Service created to serve traffic to the cluster.
        // The key must be the endpoint type (public, internal)
        Service map[service.Endpoint]service.RoutedOverrideSpec `json:"service,omitempty"`
}

// PasswordSelector to identify the DB and AdminUser password from the Secret
type PasswordSelector struct {
        // +kubebuilder:validation:Optional
        // +kubebuilder:default="PlacementPassword"
        // Service - Selector to get the service user password from the Secret
        Service string `json:"service"`
}

// NovaPlacementAPIStatus defines the observed state of NovaPlacementAPI
type NovaPlacementAPIStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
        // ReadyCount of placement API instances
        ReadyCount int32 `json:"readyCount,omitempty"`

        // Map of hashes to track e.g. job status
        Hash map[string]string `json:"hash,omitempty"`

        // Conditions
        Conditions condition.Conditions `json:"conditions,omitempty" optional:"true"`

        // Placement Database Hostname
        DatabaseHostname string `json:"databaseHostname,omitempty"`

        // NetworkAttachments status of the deployment pods
        NetworkAttachments map[string][]string `json:"networkAttachments,omitempty"`

        //ObservedGeneration - the most recent generation observed for this service. If the observed generation is less than the spec generation, then the controller has not processed the latest changes.
        ObservedGeneration int64 `json:"observedGeneration,omitempty"`

        // LastAppliedTopology - the last applied Topology
        LastAppliedTopology *topologyv1.TopoRef `json:"lastAppliedTopology,omitempty"`
}

// NovaPlacementAPI is the Schema for the novaplacementapis API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="NetworkAttachments",type="string",JSONPath=".spec.networkAttachments",description="NetworkAttachments"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[0].status",description="Status"
// +kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.conditions[0].message",description="Message"
type NovaPlacementAPI struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NovaPlacementAPISpec   `json:"spec,omitempty"`
	Status NovaPlacementAPIStatus `json:"status,omitempty"`
}

// NovaPlacementAPIList contains a list of NovaPlacementAPI
//+kubebuilder:object:root=true
type NovaPlacementAPIList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NovaPlacementAPI `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NovaPlacementAPI{}, &NovaPlacementAPIList{})
}

// IsReady - returns true if NovaPlacementAPI is reconciled successfully
func (instance NovaPlacementAPI) IsReady() bool {
        return instance.Status.Conditions.IsTrue(condition.ReadyCondition)
}

// RbacConditionsSet - set the conditions for the rbac object
func (instance NovaPlacementAPI) RbacConditionsSet(c *condition.Condition) {
        instance.Status.Conditions.Set(c)
}

// RbacNamespace - return the namespace
func (instance NovaPlacementAPI) RbacNamespace() string {
        return instance.Namespace
}

// RbacResourceName - return the name to be used for rbac objects (serviceaccount, role, rolebinding)
func (instance NovaPlacementAPI) RbacResourceName() string {
        return "placement-" + instance.Name
}

// SetupDefaults - initializes any CRD field defaults based on environment variables (the defaulting mechanism itself is implemented via webhooks)
func SetupDefaults() {
        // Acquire environmental defaults and initialize NovaPlacement defaults with them
        placementDefaults := NovaPlacementAPIDefaults{
                ContainerImageURL: util.GetEnvVar("RELATED_IMAGE_PLACEMENT_API_IMAGE_URL_DEFAULT", NovaPlacementAPIContainerImage),
                APITimeout:        60,
        }

        SetupNovaPlacementAPIDefaults(placementDefaults)
}

// GetSecret returns the value of the Nova.Spec.Secret
func (instance NovaPlacementAPI) GetSecret() string {
        return instance.Spec.Secret
}

// ValidateTopology -
func (instance *NovaPlacementAPISpecCore) ValidateTopology(
        basePath *field.Path,
        namespace string,
) field.ErrorList {
        var allErrs field.ErrorList
        allErrs = append(allErrs, topologyv1.ValidateTopologyRef(
                instance.TopologyRef,
                *basePath.Child("topologyRef"), namespace)...)
        return allErrs
}
