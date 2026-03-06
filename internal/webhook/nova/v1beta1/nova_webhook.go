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

// Package v1beta1 implements webhooks for Nova v1beta1 API
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
var novalog = logf.Log.WithName("nova-resource")

var (
	errUnexpectedObjectType = errors.New("unexpected object type")
)

// SetupNovaWebhookWithManager registers the webhook for Nova in the manager.
func SetupNovaWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&novav1beta1.Nova{}).
		WithValidator(&NovaCustomValidator{}).
		WithDefaulter(&NovaCustomDefaulter{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-nova-openstack-org-v1beta1-nova,mutating=true,failurePolicy=fail,sideEffects=None,groups=nova.openstack.org,resources=nova,verbs=create;update,versions=v1beta1,name=mnova-v1beta1.kb.io,admissionReviewVersions=v1

// NovaCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind Nova when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type NovaCustomDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

var _ webhook.CustomDefaulter = &NovaCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind Nova.
func (d *NovaCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	nova, ok := obj.(*novav1beta1.Nova)

	if !ok {
		return fmt.Errorf("%w: expected an Nova object but got %T", errUnexpectedObjectType, obj)
	}
	novalog.Info("Defaulting for Nova", "name", nova.GetName())

	nova.Default()

	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-nova-openstack-org-v1beta1-nova,mutating=false,failurePolicy=fail,sideEffects=None,groups=nova.openstack.org,resources=nova,verbs=create;update,versions=v1beta1,name=vnova-v1beta1.kb.io,admissionReviewVersions=v1

// NovaCustomValidator struct is responsible for validating the Nova resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type NovaCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &NovaCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type Nova.
func (v *NovaCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	nova, ok := obj.(*novav1beta1.Nova)
	if !ok {
		return nil, fmt.Errorf("%w: expected a Nova object but got %T", errUnexpectedObjectType, obj)
	}
	novalog.Info("Validation for Nova upon creation", "name", nova.GetName())

	return nova.ValidateCreate()
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type Nova.
func (v *NovaCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	nova, ok := newObj.(*novav1beta1.Nova)
	if !ok {
		return nil, fmt.Errorf("%w: expected a Nova object for the newObj but got %T", errUnexpectedObjectType, newObj)
	}
	novalog.Info("Validation for Nova upon update", "name", nova.GetName())

	return nova.ValidateUpdate(oldObj)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type Nova.
func (v *NovaCustomValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	nova, ok := obj.(*novav1beta1.Nova)
	if !ok {
		return nil, fmt.Errorf("%w: expected a Nova object but got %T", errUnexpectedObjectType, obj)
	}
	novalog.Info("Validation for Nova upon deletion", "name", nova.GetName())

	return nova.ValidateDelete()
}
