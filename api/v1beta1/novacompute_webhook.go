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
// operator-sdk create webhook --group nova --version v1beta1 --kind NovaCompute --programmatic-validation --defaulting
//

package v1beta1

import (
	"fmt"
	"regexp"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// NovaComputeDefaults -
type NovaComputeDefaults struct {
	ContainerImageURL string
}

var novaComputeDefaults NovaComputeDefaults

// log is for logging in this package.
var novacomputelog = logf.Log.WithName("novacompute-resource")

// SetupNovaComputeDefaults - initialize NovaCompute spec defaults for use with either internal or external webhooks
func SetupNovaComputeDefaults(defaults NovaComputeDefaults) {
	novaComputeDefaults = defaults
	novacomputelog.Info("NovaCompute defaults initialized", "defaults", defaults)
}

// SetupWebhookWithManager sets up the webhook with the Manager
func (r *NovaCompute) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-nova-openstack-org-v1beta1-novacompute,mutating=true,failurePolicy=fail,sideEffects=None,groups=nova.openstack.org,resources=novacomputes,verbs=create;update,versions=v1beta1,name=mnovacompute.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &NovaCompute{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *NovaCompute) Default() {
	novacomputelog.Info("default", "name", r.Name)

	r.Spec.Default()
}

// Default - set defaults for this novacompute spec
func (spec *NovaComputeSpec) Default() {
	if spec.ContainerImage == "" {
		spec.ContainerImage = novaComputeDefaults.ContainerImageURL
	}
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-nova-openstack-org-v1beta1-novacompute,mutating=false,failurePolicy=fail,sideEffects=None,groups=nova.openstack.org,resources=novacomputes,verbs=create;update,versions=v1beta1,name=vnovacompute.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &NovaCompute{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *NovaCompute) ValidateCreate() (admission.Warnings, error) {
	novacomputelog.Info("validate create", "name", r.Name)

	errors := r.Spec.ValidateCreate(field.NewPath("spec"))

	if len(errors) != 0 {
		novacomputelog.Info("validation failed", "name", r.Name)
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: "nova.openstack.org", Kind: "NovaCompute"},
			r.Name, errors)
	}
	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *NovaCompute) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	novacomputelog.Info("validate update", "name", r.Name)
	oldNovaCompute, ok := old.(*NovaCompute)
	if !ok || oldNovaCompute == nil {
		return nil, apierrors.NewInternalError(fmt.Errorf("unable to convert existing object"))
	}

	errors := r.Spec.ValidateUpdate(oldNovaCompute.Spec, field.NewPath("spec"))

	if len(errors) != 0 {
		novacomputelog.Info("validation failed", "name", r.Name)
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: "nova.openstack.org", Kind: "NovaCompute"},
			r.Name, errors)
	}

	return nil, nil
}

func (r *NovaComputeSpec) ValidateCreate(basePath *field.Path) field.ErrorList {
	return r.validate(basePath)
}

func (r *NovaComputeSpec) ValidateUpdate(old NovaComputeSpec, basePath *field.Path) field.ErrorList {
	return r.validate(basePath)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *NovaCompute) ValidateDelete() (admission.Warnings, error) {
	novacomputelog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil, nil
}

func (r *NovaComputeSpec) validate(basePath *field.Path) field.ErrorList {
	var errors field.ErrorList

	if r.ComputeDriver == IronicDriver && *r.NovaServiceBase.Replicas > 1 {
		errors = append(
			errors,
			field.Invalid(
				basePath.Child("replicas"), *r.NovaServiceBase.Replicas, "should be max 1 for ironic.IronicDriver"),
		)
	}
	errors = append(
		errors,
		ValidateComputeDefaultConfigOverwrite(
			basePath.Child("defaultConfigOverwrite"), r.DefaultConfigOverwrite)...)

	return errors
}

// ValidateReplicas validates replicas depend on compute driver
func ValidateIronicDriverReplicas(basePath *field.Path, replicaCount int) field.ErrorList {
	var errors field.ErrorList
	if replicaCount > 1 {
		errors = append(
			errors,
			field.Invalid(
				basePath.Child("replicas"), replicaCount, "should be max 1 for ironic.IronicDriver"),
		)
	}
	return errors
}

func ComputeValidateDefaultConfigOverwrite(basePath *field.Path, defaultConfigOverwrite map[string]string) field.ErrorList {
	return ValidateComputeDefaultConfigOverwrite(
		basePath.Child("defaultConfigOverwrite"), defaultConfigOverwrite)
}

func ValidateComputeDefaultConfigOverwrite(
	basePath *field.Path,
	defaultConfigOverwrite map[string]string,
) field.ErrorList {
	return ValidateDefaultConfigOverwrite(
		basePath, defaultConfigOverwrite, []string{"provider*.yaml"})
}

// ValidateNovaComputeName validates the compute name. It is expected to be called
// from various webhooks.
func ValidateNovaComputeName(path *field.Path, computeName string) field.ErrorList {
	var errors field.ErrorList
	if len(computeName) > 20 {
		errors = append(
			errors,
			field.Invalid(
				path, computeName, "should be shorter than 20 characters"),
		)
	}
	match, _ := regexp.MatchString(CRDNameRegex, computeName)
	if !match {
		errors = append(
			errors,
			field.Invalid(
				path, computeName,
				fmt.Sprintf("should match with the regex '%s'", CRDNameRegex)),
		)
	}
	return errors
}

// ValidateNovaComputeCell0 validates cell0 NoVNCProxy template. This is expected to be
// called by higher level validation webhooks
func ValidateNovaComputeCell0(basePath *field.Path, mapLength int) field.ErrorList {
	var errors field.ErrorList
	if mapLength > 0 {
		errors = append(
			errors,
			field.Invalid(
				basePath, "novaComputeTemplates", "should have zero elements for cell0"),
		)
	}
	return errors
}
