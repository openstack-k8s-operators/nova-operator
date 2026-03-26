/*
Copyright 2026.

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
	"fmt"

	"github.com/google/go-cmp/cmp"
	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	service "github.com/openstack-k8s-operators/lib-common/modules/common/service"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// CyborgDefaults -
type CyborgDefaults struct {
	APIContainerImageURL       string
	ConductorContainerImageURL string
	AgentContainerImageURL     string
	APITimeout                 int
}

var cyborgDefaults CyborgDefaults

// log is for logging in this package.
var cyborglog = logf.Log.WithName("cyborg-resource")

// SetupCyborgDefaults - initialize Cyborg spec defaults for use with either internal or external webhooks
func SetupCyborgDefaults(defaults CyborgDefaults) {
	cyborgDefaults = defaults
	cyborglog.Info("Cyborg defaults initialized", "defaults", defaults)
}

var _ webhook.Defaulter = &Cyborg{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *Cyborg) Default() {
	cyborglog.Info("default", "name", r.Name)

	r.Spec.Default()
}

// Default - set defaults for this Cyborg spec.
func (spec *CyborgSpec) Default() {
	spec.CyborgImages.Default(cyborgDefaults)
	spec.CyborgSpecCore.Default()
}

func (images *CyborgImages) Default(defaults CyborgDefaults) {
	if images.APIContainerImageURL == "" {
		images.APIContainerImageURL = cyborgDefaults.APIContainerImageURL
	}
	if images.ConductorContainerImageURL == "" {
		images.ConductorContainerImageURL = cyborgDefaults.ConductorContainerImageURL
	}
	if images.AgentContainerImageURL == "" {
		images.AgentContainerImageURL = cyborgDefaults.AgentContainerImageURL
	}
}

// Default - set defaults for this CyborgSpecCore spec. Expected to be called from
// the higher level meta operator.
func (spec *CyborgSpecCore) Default() {
	if spec.APITimeout == nil {
		spec.APITimeout = ptr.To(cyborgDefaults.APITimeout)
	}

	// Default MessagingBus.Cluster if not set
	// Migration from deprecated fields is handled by openstack-operator
	if spec.MessagingBus.Cluster == "" {
		spec.MessagingBus.Cluster = "rabbitmq"
	}
}

var _ webhook.Validator = &Cyborg{}

// ValidateAPIServiceTemplate -
func (spec *CyborgSpecCore) ValidateAPIServiceTemplate(basePath *field.Path, namespace string) field.ErrorList {
	errors := field.ErrorList{}

	// validate the service override key is valid
	errors = append(errors,
		service.ValidateRoutedOverrides(
			basePath.Child("apiServiceTemplate").Child("override").Child("service"),
			spec.APIServiceTemplate.Override.Service)...)

	errors = append(errors,
		spec.APIServiceTemplate.ValidateTopology(
			basePath.Child("apiServiceTemplate"),
			namespace)...)

	return errors
}

// ValidateConductorServiceTemplate -
func (spec *CyborgSpecCore) ValidateConductorServiceTemplate(basePath *field.Path, namespace string) field.ErrorList {
	errors := field.ErrorList{}
	// validate the referenced TopologyRef
	errors = append(errors,
		spec.ConductorServiceTemplate.ValidateTopology(
			basePath.Child("conductorServiceTemplate"),
			namespace)...)
	return errors
}

// ValidateCreate validates the CyborgSpec during the webhook invocation.
func (spec *CyborgSpec) ValidateCreate(basePath *field.Path, namespace string) (admission.Warnings, field.ErrorList) {
	return spec.CyborgSpecCore.ValidateCreate(basePath, namespace)
}

// ValidateCreate validates the CyborgSpecCore during the webhook invocation. It is
// expected to be called by the validation webhook in the higher level meta
// operator
func (spec *CyborgSpecCore) ValidateCreate(basePath *field.Path, namespace string) (admission.Warnings, field.ErrorList) {

	errors := field.ErrorList{}

	errors = append(errors, spec.ValidateAPIServiceTemplate(basePath, namespace)...)
	errors = append(errors, spec.ValidateConductorServiceTemplate(basePath, namespace)...)

	// validate top-level topology
	errors = append(errors,
		topologyv1.ValidateTopologyRef(
			spec.TopologyRef, *basePath.Child("topologyRef"), namespace)...)

	return nil, errors
}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Cyborg) ValidateCreate() (admission.Warnings, error) {
	cyborglog.Info("validate create", "name", r.Name)

	warnings, errors := r.Spec.ValidateCreate(field.NewPath("spec"), r.Namespace)
	if len(errors) != 0 {
		cyborglog.Info("validation failed", "name", r.Name)
		return warnings, apierrors.NewInvalid(
			schema.GroupKind{Group: "cyborg.openstack.org", Kind: "Cyborg"},
			r.Name, errors)
	}
	return warnings, nil
}

// ValidateUpdate validates the CyborgSpec during the webhook invocation.
func (spec *CyborgSpec) ValidateUpdate(old CyborgSpec, basePath *field.Path, namespace string) (admission.Warnings, field.ErrorList) {
	return spec.CyborgSpecCore.ValidateUpdate(old.CyborgSpecCore, basePath, namespace)
}

// ValidateUpdate validates the CyborgSpecCore during the webhook invocation. It is
// expected to be called by the validation webhook in the higher level meta
// operator
func (spec *CyborgSpecCore) ValidateUpdate(old CyborgSpecCore, basePath *field.Path, namespace string) (admission.Warnings, field.ErrorList) {
	var errors field.ErrorList
	var warnings admission.Warnings

	// Validate top-level TopologyRef
	errors = append(errors, topologyv1.ValidateTopologyRef(
		spec.TopologyRef, *basePath.Child("topologyRef"), namespace)...)

	errors = append(errors, spec.ValidateAPIServiceTemplate(basePath, namespace)...)
	errors = append(errors, spec.ValidateConductorServiceTemplate(basePath, namespace)...)

	return warnings, errors
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Cyborg) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	cyborglog.Info("validate update", "name", r.Name)
	oldCyborg, ok := old.(*Cyborg)
	if !ok || oldCyborg == nil {
		return nil, apierrors.NewInternalError(fmt.Errorf("unable to convert existing object"))
	}

	cyborglog.Info("validate update", "diff", cmp.Diff(oldCyborg, r))

	warnings, errors := r.Spec.ValidateUpdate(oldCyborg.Spec, field.NewPath("spec"), r.Namespace)
	if len(errors) != 0 {
		cyborglog.Info("validation failed", "name", r.Name)
		return warnings, apierrors.NewInvalid(
			schema.GroupKind{Group: "cyborg.openstack.org", Kind: "Cyborg"},
			r.Name, errors)
	}
	return warnings, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *Cyborg) ValidateDelete() (admission.Warnings, error) {
	cyborglog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil, nil
}

// SetDefaultRouteAnnotations sets HAProxy timeout values of the route
// NOTE: it is used by ctlplane webhook on openstack-operator
func (spec *CyborgSpecCore) SetDefaultRouteAnnotations(annotations map[string]string) {
	const haProxyAnno = "haproxy.router.openshift.io/timeout"
	// Use a custom annotation to flag when the operator has set the default HAProxy timeout
	// With the annotation func determines when to overwrite existing HAProxy timeout with the APITimeout
	const cyborgAnno = "api.cyborg.openstack.org/timeout"

	valCyborg, okCyborg := annotations[cyborgAnno]
	valHAProxy, okHAProxy := annotations[haProxyAnno]

	// Human operator set the HAProxy timeout manually
	if !okCyborg && okHAProxy {
		return
	}

	// Human operator modified the HAProxy timeout manually without removing the Cyborg flag
	if okCyborg && okHAProxy && valCyborg != valHAProxy {
		delete(annotations, cyborgAnno)
		return
	}

	timeout := fmt.Sprintf("%ds", *spec.APITimeout)
	annotations[cyborgAnno] = timeout
	annotations[haProxyAnno] = timeout
}
