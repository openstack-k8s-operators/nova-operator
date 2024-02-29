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
// operator-sdk create webhook --group nova --version v1beta1 --kind Nova --programmatic-validation --defaulting
//

package v1beta1

import (
	"fmt"

	"github.com/robfig/cron/v3"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// NovaDefaults -
type NovaDefaults struct {
	APIContainerImageURL         string
	ConductorContainerImageURL   string
	MetadataContainerImageURL    string
	NoVNCContainerImageURL       string
	SchedulerContainerImageURL   string
	NovaComputeContainerImageURL string
}

var novaDefaults NovaDefaults

// log is for logging in this package.
var novalog = logf.Log.WithName("nova-resource")

// SetupNovaDefaults - initialize Nova spec defaults for use with either internal or external webhooks
func SetupNovaDefaults(defaults NovaDefaults) {
	novaDefaults = defaults
	novalog.Info("Nova defaults initialized", "defaults", defaults)
}

// SetupWebhookWithManager sets up the webhook with the Manager
func (r *Nova) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-nova-openstack-org-v1beta1-nova,mutating=true,failurePolicy=fail,sideEffects=None,groups=nova.openstack.org,resources=nova,verbs=create;update,versions=v1beta1,name=mnova.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &Nova{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *Nova) Default() {
	novalog.Info("default", "name", r.Name)

	r.Spec.Default()
}

// Default - set defaults for this Nova spec
func (spec *NovaSpec) Default() {
	if spec.APIServiceTemplate.ContainerImage == "" {
		spec.APIServiceTemplate.ContainerImage = novaDefaults.APIContainerImageURL
	}

	if spec.MetadataServiceTemplate.ContainerImage == "" {
		spec.MetadataServiceTemplate.ContainerImage = novaDefaults.MetadataContainerImageURL
	}

	// NOTE(gibi): this cannot be expressed as kubebuilder defaults as the
	// MetadataServiceTemplate is used both in the cellTemplate and in the
	// NovaSpec but we need different defaults in the two places
	if spec.MetadataServiceTemplate.Enabled == nil {
		spec.MetadataServiceTemplate.Enabled = ptr.To(true)
	}

	if spec.SchedulerServiceTemplate.ContainerImage == "" {
		spec.SchedulerServiceTemplate.ContainerImage = novaDefaults.SchedulerContainerImageURL
	}

	for cellName, cellTemplate := range spec.CellTemplates {
		if cellTemplate.ConductorServiceTemplate.ContainerImage == "" {
			cellTemplate.ConductorServiceTemplate.ContainerImage = novaDefaults.ConductorContainerImageURL
		}

		if cellTemplate.MetadataServiceTemplate.ContainerImage == "" {
			cellTemplate.MetadataServiceTemplate.ContainerImage = novaDefaults.MetadataContainerImageURL
		}

		if cellTemplate.MetadataServiceTemplate.Enabled == nil {
			cellTemplate.MetadataServiceTemplate.Enabled = ptr.To(false)
		}

		if cellTemplate.NoVNCProxyServiceTemplate.ContainerImage == "" {
			cellTemplate.NoVNCProxyServiceTemplate.ContainerImage = novaDefaults.NoVNCContainerImageURL
		}

		if cellName == Cell0Name {
			// in cell0 disable VNC by default
			if cellTemplate.NoVNCProxyServiceTemplate.Enabled == nil {
				cellTemplate.NoVNCProxyServiceTemplate.Enabled = ptr.To(false)
			}
		} else {
			// in other cells enable VNC by default
			if cellTemplate.NoVNCProxyServiceTemplate.Enabled == nil {
				cellTemplate.NoVNCProxyServiceTemplate.Enabled = ptr.To(true)
			}
		}

		for computeName, computeTemplate := range cellTemplate.NovaComputeTemplates {
			if computeTemplate.ContainerImage == "" {
				computeTemplate.ContainerImage = novaCellDefaults.NovaComputeContainerImageURL
			}
			cellTemplate.NovaComputeTemplates[computeName] = computeTemplate
		}
		// "cellTemplate" is a by-value copy, so we need to re-inject the updated version of it into the map
		spec.CellTemplates[cellName] = cellTemplate
	}
}

// NOTE: change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-nova-openstack-org-v1beta1-nova,mutating=false,failurePolicy=fail,sideEffects=None,groups=nova.openstack.org,resources=nova,verbs=create;update,versions=v1beta1,name=vnova.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &Nova{}

func (r *NovaSpec) ValidateCellTemplates(basePath *field.Path) field.ErrorList {
	var errors field.ErrorList

	if _, ok := r.CellTemplates[Cell0Name]; !ok {
		errors = append(
			errors,
			field.Required(basePath.Child("cellTemplates"),
				"cell0 specification is missing, cell0 key is required in cellTemplates"),
		)
	}

	cellMessageBusNames := make(map[string]string)

	for name, cell := range r.CellTemplates {
		cellPath := basePath.Child("cellTemplates").Key(name)
		errors = append(
			errors,
			ValidateCellName(cellPath, name)...,
		)
		if name != Cell0Name {
			if dupName, ok := cellMessageBusNames[cell.CellMessageBusInstance]; ok {
				errors = append(errors, field.Invalid(
					cellPath.Child("cellMessageBusInstance"),
					cell.CellMessageBusInstance,
					fmt.Sprintf(
						"RabbitMqCluster CR need to be uniq per cell. It's duplicated with cell: %s",
						dupName),
				),
				)
			}

			cellMessageBusNames[cell.CellMessageBusInstance] = name
		}
		if *cell.MetadataServiceTemplate.Enabled && *r.MetadataServiceTemplate.Enabled {
			errors = append(
				errors,
				field.Invalid(
					cellPath.Child("metadataServiceTemplate").Child("enabled"),
					*cell.MetadataServiceTemplate.Enabled,
					"should be false as metadata is enabled on the top level too. "+
						"The metadata service can be either enabled on top "+
						"or in the cells but not in both places at the same time."),
			)
		}

		errors = append(
			errors,
			cell.MetadataServiceTemplate.ValidateDefaultConfigOverwrite(
				cellPath.Child("metadataServiceTemplate"))...)

		errors = append(
			errors,
			cell.DBPurge.Validate(cellPath.Child("dbPurge"))...)

		if name == Cell0Name {
			errors = append(
				errors,
				cell.MetadataServiceTemplate.ValidateCell0(
					cellPath.Child("metadataServiceTemplate"))...)
			errors = append(
				errors,
				cell.NoVNCProxyServiceTemplate.ValidateCell0(
					cellPath.Child("noVNCProxyServiceTemplate"))...)
			errors = append(
				errors,
				ValidateNovaComputeCell0(
					cellPath.Child("novaComputeTemplates"), len(cell.NovaComputeTemplates))...)
		}

		for computeName, computeTemplate := range cell.NovaComputeTemplates {
			if computeTemplate.ComputeDriver == IronicDriver {
				errors = append(
					errors, computeTemplate.ValidateIronicDriverReplicas(
						cellPath.Child("novaComputeTemplates").Key(computeName))...,
				)
			}
			errors = append(
				errors, ValidateNovaComputeName(
					cellPath.Child("novaComputeTemplates").Key(computeName), computeName)...,
			)
			errors = append(
				errors, computeTemplate.ValidateDefaultConfigOverwrite(
					cellPath.Child("novaComputeTemplates").Key(computeName))...,
			)
		}
	}

	return errors
}

func (r *NovaSpec) ValidateAPIServiceTemplate(basePath *field.Path) field.ErrorList {
	errors := ValidateAPIDefaultConfigOverwrite(
		basePath.Child("apiServiceTemplate").Child("defaultConfigOverwrite"),
		r.APIServiceTemplate.DefaultConfigOverwrite)
	return errors
}

// ValidateCreate validates the NovaSpec during the webhook invocation. It is
// expected to be called by the validation webhook in the higher level meta
// operator
func (r *NovaSpec) ValidateCreate(basePath *field.Path) field.ErrorList {
	errors := r.ValidateCellTemplates(basePath)
	errors = append(errors, r.ValidateAPIServiceTemplate(basePath)...)
	errors = append(
		errors,
		r.MetadataServiceTemplate.ValidateDefaultConfigOverwrite(
			basePath.Child("metadataServiceTemplate"))...)

	return errors
}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Nova) ValidateCreate() (admission.Warnings, error) {
	novalog.Info("validate create", "name", r.Name)

	errors := r.Spec.ValidateCreate(field.NewPath("spec"))
	if len(errors) != 0 {
		novalog.Info("validation failed", "name", r.Name)
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: "nova.openstack.org", Kind: "Nova"},
			r.Name, errors)
	}
	return nil, nil
}

// ValidateUpdate validates the NovaSpec during the webhook invocation. It is
// expected to be called by the validation webhook in the higher level meta
// operator
func (r *NovaSpec) ValidateUpdate(old NovaSpec, basePath *field.Path) field.ErrorList {
	errors := r.ValidateCellTemplates(basePath)
	errors = append(errors, r.ValidateAPIServiceTemplate(basePath)...)
	errors = append(
		errors,
		r.MetadataServiceTemplate.ValidateDefaultConfigOverwrite(
			basePath.Child("metadataServiceTemplate"))...)

	return errors
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Nova) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	novalog.Info("validate update", "name", r.Name)
	oldNova, ok := old.(*Nova)
	if !ok || oldNova == nil {
		return nil, apierrors.NewInternalError(fmt.Errorf("unable to convert existing object"))
	}

	errors := r.Spec.ValidateUpdate(oldNova.Spec, field.NewPath("spec"))
	if len(errors) != 0 {
		novalog.Info("validation failed", "name", r.Name)
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: "nova.openstack.org", Kind: "Nova"},
			r.Name, errors)
	}
	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *Nova) ValidateDelete() (admission.Warnings, error) {
	novalog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil, nil
}

// Validate the field values
func (r *NovaCellDBPurge) Validate(basePath *field.Path) field.ErrorList {
	var errors field.ErrorList
	// k8s uses the same cron lib to validate the schedule of the CronJob
	// https://github.com/kubernetes/kubernetes/blob/master/pkg/apis/batch/validation/validation.go
	if _, err := cron.ParseStandard(*r.Schedule); err != nil {
		errors = append(
			errors,
			field.Invalid(
				basePath.Child("schedule"), r.Schedule, err.Error()),
		)
	}
	return errors
}
