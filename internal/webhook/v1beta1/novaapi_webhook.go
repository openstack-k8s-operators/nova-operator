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
var novaapilog = logf.Log.WithName("novaapi-resource")

var errExpectedNovaAPIObject = errors.New("expected a NovaAPI object")

// SetupNovaAPIWebhookWithManager registers the webhook for NovaAPI in the manager.
func SetupNovaAPIWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&novav1beta1.NovaAPI{}).
		WithValidator(&NovaAPICustomValidator{}).
		WithDefaulter(&NovaAPICustomDefaulter{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-nova-openstack-org-v1beta1-novaapi,mutating=true,failurePolicy=fail,sideEffects=None,groups=nova.openstack.org,resources=novaapis,verbs=create;update,versions=v1beta1,name=mnovaapi-v1beta1.kb.io,admissionReviewVersions=v1

// NovaAPICustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind NovaAPI when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type NovaAPICustomDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

var _ webhook.CustomDefaulter = &NovaAPICustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind NovaAPI.
func (d *NovaAPICustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	novaapi, ok := obj.(*novav1beta1.NovaAPI)

	if !ok {
		return fmt.Errorf("%w but got %T", errExpectedNovaAPIObject, obj)
	}
	novaapilog.Info("Defaulting for NovaAPI", "name", novaapi.GetName())

	novaapi.Default()

	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-nova-openstack-org-v1beta1-novaapi,mutating=false,failurePolicy=fail,sideEffects=None,groups=nova.openstack.org,resources=novaapis,verbs=create;update,versions=v1beta1,name=vnovaapi-v1beta1.kb.io,admissionReviewVersions=v1

// NovaAPICustomValidator struct is responsible for validating the NovaAPI resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type NovaAPICustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &NovaAPICustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type NovaAPI.
func (v *NovaAPICustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	novaapi, ok := obj.(*novav1beta1.NovaAPI)
	if !ok {
		return nil, fmt.Errorf("%w but got %T", errExpectedNovaAPIObject, obj)
	}
	novaapilog.Info("Validation for NovaAPI upon creation", "name", novaapi.GetName())

	return novaapi.ValidateCreate()
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type NovaAPI.
func (v *NovaAPICustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	novaapi, ok := newObj.(*novav1beta1.NovaAPI)
	if !ok {
		return nil, fmt.Errorf("%w for the newObj but got %T", errExpectedNovaAPIObject, newObj)
	}
	novaapilog.Info("Validation for NovaAPI upon update", "name", novaapi.GetName())

	return novaapi.ValidateUpdate(oldObj)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type NovaAPI.
func (v *NovaAPICustomValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	novaapi, ok := obj.(*novav1beta1.NovaAPI)
	if !ok {
		return nil, fmt.Errorf("%w but got %T", errExpectedNovaAPIObject, obj)
	}
	novaapilog.Info("Validation for NovaAPI upon deletion", "name", novaapi.GetName())

	return novaapi.ValidateDelete()
}
