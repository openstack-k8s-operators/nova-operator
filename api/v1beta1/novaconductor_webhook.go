/*
Copyright 2023.

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

//
// Generated by:
//
// operator-sdk create webhook --group nova --version v1beta1 --kind NovaConductor --programmatic-validation --defaulting
//

package v1beta1

import (
	"fmt"

	"github.com/google/go-cmp/cmp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
)

// NovaConductorDefaults -
type NovaConductorDefaults struct {
	ContainerImageURL string
}

var novaConductorDefaults NovaConductorDefaults

// log is for logging in this package.
var novaconductorlog = logf.Log.WithName("novaconductor-resource")

// SetupNovaConductorDefaults - initialize NovaConductor spec defaults for use with either internal or external webhooks
func SetupNovaConductorDefaults(defaults NovaConductorDefaults) {
	novaConductorDefaults = defaults
	novaconductorlog.Info("NovaConductor defaults initialized", "defaults", defaults)
}

// SetupWebhookWithManager sets up the webhook with the Manager
func (r *NovaConductor) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-nova-openstack-org-v1beta1-novaconductor,mutating=true,failurePolicy=fail,sideEffects=None,groups=nova.openstack.org,resources=novaconductors,verbs=create;update,versions=v1beta1,name=mnovaconductor.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &NovaConductor{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *NovaConductor) Default() {
	novaconductorlog.Info("default", "name", r.Name)

	r.Spec.Default()
}

// Default - set defaults for this NovaConductor spec
func (spec *NovaConductorSpec) Default() {
	if spec.ContainerImage == "" {
		spec.ContainerImage = novaConductorDefaults.ContainerImageURL
	}
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-nova-openstack-org-v1beta1-novaconductor,mutating=false,failurePolicy=fail,sideEffects=None,groups=nova.openstack.org,resources=novaconductors,verbs=create;update,versions=v1beta1,name=vnovaconductor.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &NovaConductor{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *NovaConductor) ValidateCreate() (admission.Warnings, error) {
	novaconductorlog.Info("validate create", "name", r.Name)
	errors := field.ErrorList{}
	basePath := field.NewPath("spec")

	errors = append(errors,r.Spec.DBPurge.Validate(
		basePath.Child("dbPurge"))...)

	errors = append(errors, topologyv1.ValidateTopologyRef(
		r.Spec.TopologyRef, *basePath.Child("topologyRef"), r.Namespace)...)

	if len(errors) != 0 {
		novaconductorlog.Info("validation failed", "name", r.Name)
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: "nova.openstack.org", Kind: "NovaConductor"},
			r.Name, errors)
	}
	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *NovaConductor) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	novaconductorlog.Info("validate update", "name", r.Name)
	errors := field.ErrorList{}
	basePath := field.NewPath("spec")

	oldConductor, ok := old.(*NovaConductor)
	if !ok || oldConductor == nil {
		return nil, apierrors.NewInternalError(fmt.Errorf("unable to convert existing object"))
	}

	novaconductorlog.Info("validate update", "diff", cmp.Diff(oldConductor, r))

	errors = append(errors, r.Spec.DBPurge.Validate(
		basePath.Child("dbPurge"))...)

	errors = append(errors, topologyv1.ValidateTopologyRef(
		r.Spec.TopologyRef, *basePath.Child("topologyRef"), r.Namespace)...)

	if len(errors) != 0 {
		novaconductorlog.Info("validation failed", "name", r.Name)
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: "nova.openstack.org", Kind: "NovaConductor"},
			r.Name, errors)
	}
	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *NovaConductor) ValidateDelete() (admission.Warnings, error) {
	novaconductorlog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil, nil
}

// ValidateTopology validates the referenced TopoRef.Namespace.
func (r *NovaConductorTemplate) ValidateTopology(
	basePath *field.Path,
	namespace string,
) field.ErrorList {
	return topologyv1.ValidateTopologyRef(
		r.TopologyRef,
		*basePath.Child("topologyRef"),
		namespace,
	)
}
