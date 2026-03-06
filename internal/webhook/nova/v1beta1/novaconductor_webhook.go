/*
Copyright 2025.

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
	"context"
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	novav1beta1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
)

// nolint:unused
// log is for logging in this package.
var novaconductorlog = logf.Log.WithName("novaconductor-resource")

var errExpectedNovaConductorObject = errors.New("expected a NovaConductor object")

// SetupNovaConductorWebhookWithManager registers the webhook for NovaConductor in the manager.
func SetupNovaConductorWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&novav1beta1.NovaConductor{}).
		WithValidator(&NovaConductorCustomValidator{}).
		WithDefaulter(&NovaConductorCustomDefaulter{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-nova-openstack-org-v1beta1-novaconductor,mutating=true,failurePolicy=fail,sideEffects=None,groups=nova.openstack.org,resources=novaconductors,verbs=create;update,versions=v1beta1,name=mnovaconductor-v1beta1.kb.io,admissionReviewVersions=v1

// NovaConductorCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind NovaConductor when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type NovaConductorCustomDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

var _ webhook.CustomDefaulter = &NovaConductorCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind NovaConductor.
func (d *NovaConductorCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	novaconductor, ok := obj.(*novav1beta1.NovaConductor)

	if !ok {
		return fmt.Errorf("%w but got %T", errExpectedNovaConductorObject, obj)
	}
	novaconductorlog.Info("Defaulting for NovaConductor", "name", novaconductor.GetName())

	novaconductor.Default()

	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-nova-openstack-org-v1beta1-novaconductor,mutating=false,failurePolicy=fail,sideEffects=None,groups=nova.openstack.org,resources=novaconductors,verbs=create;update,versions=v1beta1,name=vnovaconductor-v1beta1.kb.io,admissionReviewVersions=v1

// NovaConductorCustomValidator struct is responsible for validating the NovaConductor resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type NovaConductorCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &NovaConductorCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type NovaConductor.
func (v *NovaConductorCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	novaconductor, ok := obj.(*novav1beta1.NovaConductor)
	if !ok {
		return nil, fmt.Errorf("%w but got %T", errExpectedNovaConductorObject, obj)
	}
	novaconductorlog.Info("Validation for NovaConductor upon creation", "name", novaconductor.GetName())

	return novaconductor.ValidateCreate()
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type NovaConductor.
func (v *NovaConductorCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	novaconductor, ok := newObj.(*novav1beta1.NovaConductor)
	if !ok {
		return nil, fmt.Errorf("%w for the newObj but got %T", errExpectedNovaConductorObject, newObj)
	}
	novaconductorlog.Info("Validation for NovaConductor upon update", "name", novaconductor.GetName())

	return novaconductor.ValidateUpdate(oldObj)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type NovaConductor.
func (v *NovaConductorCustomValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	novaconductor, ok := obj.(*novav1beta1.NovaConductor)
	if !ok {
		return nil, fmt.Errorf("%w but got %T", errExpectedNovaConductorObject, obj)
	}
	novaconductorlog.Info("Validation for NovaConductor upon deletion", "name", novaconductor.GetName())

	return novaconductor.ValidateDelete()
}
