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
	"fmt"

	"github.com/openstack-k8s-operators/lib-common/modules/common/service"
        apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type NovaPlacementAPIDefaults struct {
	ContainerImageURL string
	APITimeout        int
}

var novaPlacementAPIDefaults NovaPlacementAPIDefaults

// log is for logging in this package.
var novaplacementapilog = logf.Log.WithName("novaplacementapi-resource")

// SetupNovaPlacementAPIDefaults - initialize NovaPlacementAPI spec defaults for use with either internal or external webhooks
func SetupNovaPlacementAPIDefaults(defaults NovaPlacementAPIDefaults) {
	novaPlacementAPIDefaults = defaults
	novaplacementapilog.Info("NovaPlacementAPI defaults initialized", "defaults", defaults)
}

// SetupWebhookWithManager sets up the webhook with the Manager
func (r *NovaPlacementAPI) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate-nova-openstack-org-v1beta1-novaplacementapi,mutating=true,failurePolicy=fail,sideEffects=None,groups=nova.openstack.org,resources=novaplacementapis,verbs=create;update,versions=v1beta1,name=mnovaplacementapi.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &NovaPlacementAPI{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *NovaPlacementAPI) Default() {
	novaplacementapilog.Info("default", "name", r.Name)

	r.Spec.Default()

}

// Default - set defaults for this NovaPlacementAPI spec
func (spec *NovaPlacementAPISpec) Default() {
        if spec.ContainerImage == "" {
                spec.ContainerImage = novaplacementAPIDefaults.ContainerImageURL
        }
        if spec.APITimeout == 0 {
                spec.APITimeout = novaPlacementAPIDefaults.APITimeout
        }

}

// Default - set defaults for this NovaPlacementAPI core spec (this version is used by the OpenStackControlplane webhook)
func (spec *NovaPlacementAPISpecCore) Default() {
        // nothing here yet
}


// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-nova-openstack-org-v1beta1-novaplacementapi,mutating=false,failurePolicy=fail,sideEffects=None,groups=nova.openstack.org,resources=novaplacementapis,verbs=create;update,versions=v1beta1,name=vnovaplacementapi.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &NovaPlacementAPI{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *NovaPlacementAPI) ValidateCreate() (admission.Warnings, error) {
	novaplacementapilog.Info("validate create", "name", r.Name)

        errors := r.Spec.ValidateCreate(field.NewPath("spec"), r.Namespace)
        if len(errors) != 0 {
                placementapilog.Info("validation failed", "name", r.Name)
                return nil, apierrors.NewInvalid(
                        schema.GroupKind{Group: "nova.openstack.org", Kind: "NovaPlacementAPI"},
                        r.Name, errors)
        }
        return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *NovaPlacementAPI) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	novaplacementapilog.Info("validate update", "name", r.Name)
        oldPlacement, ok := old.(*NovaPlacementAPI)
        if !ok || oldPlacement == nil {
                return nil, apierrors.NewInternalError(fmt.Errorf("unable to convert existing object"))
        }

        errors := r.Spec.ValidateUpdate(oldPlacement.Spec, field.NewPath("spec"), r.Namespace)
        if len(errors) != 0 {
                placementapilog.Info("validation failed", "name", r.Name)
                return nil, apierrors.NewInvalid(
                        schema.GroupKind{Group: "nova.openstack.org", Kind: "NovaPlacementAPI"},
                        r.Name, errors)
        }
        return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *NovaPlacementAPI) ValidateDelete() (admission.Warnings, error) {
	novaplacementapilog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil
}

func (r NovaPlacementAPISpec) ValidateCreate(basePath *field.Path, namespace string) field.ErrorList {
        return r.NovaPlacementAPISpecCore.ValidateCreate(basePath, namespace)
}

func (r NovaPlacementAPISpec) ValidateUpdate(old NovaPlacementAPISpec, basePath *field.Path, namespace string) field.ErrorList {
        return r.NovaPlacementAPISpecCore.ValidateCreate(basePath, namespace)
}

func (r NovaPlacementAPISpecCore) ValidateCreate(basePath *field.Path, namespace string) field.ErrorList {
        var allErrs field.ErrorList

        // validate the service override key is valid
        allErrs = append(allErrs, service.ValidateRoutedOverrides(basePath.Child("override").Child("service"), r.Override.Service)...)

        allErrs = append(allErrs, ValidateDefaultConfigOverwrite(basePath, r.DefaultConfigOverwrite)...)

        // When a TopologyRef CR is referenced, fail if a different Namespace is
        // referenced because is not supported
        allErrs = append(allErrs, r.ValidateTopology(basePath, namespace)...)

        return allErrs
}

func (r NovaPlacementAPISpecCore) ValidateUpdate(old NovaPlacementAPISpecCore, basePath *field.Path, namespace string) field.ErrorList {
        var allErrs field.ErrorList

        // validate the service override key is valid
        allErrs = append(allErrs, service.ValidateRoutedOverrides(basePath.Child("override").Child("service"), r.Override.Service)...)

        allErrs = append(allErrs, ValidateDefaultConfigOverwrite(basePath, r.DefaultConfigOverwrite)...)

        // When a TopologyRef CR is referenced, fail if a different Namespace is
        // referenced because is not supported
        allErrs = append(allErrs, r.ValidateTopology(basePath, namespace)...)

        return allErrs
}

func ValidateDefaultConfigOverwrite(
        basePath *field.Path,
        validateConfigOverwrite map[string]string,
) field.ErrorList {
        var errors field.ErrorList
        for requested := range validateConfigOverwrite {
                if requested != "policy.yaml" {
                        errors = append(
                                errors,
                                field.Invalid(
                                        basePath.Child("defaultConfigOverwrite"),
                                        requested,
                                        "Only the following keys are valid: policy.yaml",
                                ),
                        )
                }
        }
        return errors
}

// SetDefaultRouteAnnotations sets HAProxy timeout values of the route
func (spec *NovaPlacementAPISpecCore) SetDefaultRouteAnnotations(annotations map[string]string) {
        const haProxyAnno = "haproxy.router.openshift.io/timeout"
        // Use a custom annotation to flag when the operator has set the default HAProxy timeout
        // With the annotation func determines when to overwrite existing HAProxy timeout with the APITimeout
        const placementAnno = "api.nova.openstack.org/timeout"
        valPlacementAPI, okPlacementAPI := annotations[placementAnno]
        valHAProxy, okHAProxy := annotations[haProxyAnno]
        // Human operator set the HAProxy timeout manually
        if !okPlacementAPI && okHAProxy {
                return
        }
        // Human operator modified the HAProxy timeout manually without removing the Placemen flag
        if okPlacementAPI && okHAProxy && valPlacementAPI != valHAProxy {
                delete(annotations, placementAnno)
                placementapilog.Info("Human operator modified the HAProxy timeout manually without removing the Placement flag. Deleting the Placement flag to ensure proper configuration.")
                return
        }
        timeout := fmt.Sprintf("%ds", spec.APITimeout)
        annotations[placementAnno] = timeout
        annotations[haProxyAnno] = timeout
}

