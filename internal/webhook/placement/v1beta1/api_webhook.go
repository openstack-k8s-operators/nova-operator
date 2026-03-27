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

// Package v1beta1 implements webhook handlers for Placement v1beta1 API resources.
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

	placementv1beta1 "github.com/openstack-k8s-operators/placement-operator/api/v1beta1"
)

var (
	// ErrInvalidObjectType is returned when an unexpected object type is provided
	ErrInvalidObjectType = errors.New("invalid object type")
)

// nolint:unused
// log is for logging in this package.
var placementapilog = logf.Log.WithName("placementapi-resource")

// SetupPlacementAPIWebhookWithManager registers the webhook for PlacementAPI in the manager.
func SetupPlacementAPIWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&placementv1beta1.PlacementAPI{}).
		WithValidator(&PlacementAPICustomValidator{}).
		WithDefaulter(&PlacementAPICustomDefaulter{}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-placement-openstack-org-v1beta1-placementapi,mutating=true,failurePolicy=fail,sideEffects=None,groups=placement.openstack.org,resources=placementapis,verbs=create;update,versions=v1beta1,name=mplacementapi-v1beta1.kb.io,admissionReviewVersions=v1

// PlacementAPICustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind PlacementAPI when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type PlacementAPICustomDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

var _ webhook.CustomDefaulter = &PlacementAPICustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind PlacementAPI.
func (d *PlacementAPICustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	placementapi, ok := obj.(*placementv1beta1.PlacementAPI)

	if !ok {
		return fmt.Errorf("expected an PlacementAPI object but got %T: %w", obj, ErrInvalidObjectType)
	}
	placementapilog.Info("Defaulting for PlacementAPI", "name", placementapi.GetName())

	// Call the Default method on the PlacementAPI type
	placementapi.Default()

	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-placement-openstack-org-v1beta1-placementapi,mutating=false,failurePolicy=fail,sideEffects=None,groups=placement.openstack.org,resources=placementapis,verbs=create;update,versions=v1beta1,name=vplacementapi-v1beta1.kb.io,admissionReviewVersions=v1

// PlacementAPICustomValidator struct is responsible for validating the PlacementAPI resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type PlacementAPICustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &PlacementAPICustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type PlacementAPI.
func (v *PlacementAPICustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	placementapi, ok := obj.(*placementv1beta1.PlacementAPI)
	if !ok {
		return nil, fmt.Errorf("expected a PlacementAPI object but got %T: %w", obj, ErrInvalidObjectType)
	}
	placementapilog.Info("Validation for PlacementAPI upon creation", "name", placementapi.GetName())

	// Call the ValidateCreate method on the PlacementAPI type
	return placementapi.ValidateCreate()
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type PlacementAPI.
func (v *PlacementAPICustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	placementapi, ok := newObj.(*placementv1beta1.PlacementAPI)
	if !ok {
		return nil, fmt.Errorf("expected a PlacementAPI object for the newObj but got %T: %w", newObj, ErrInvalidObjectType)
	}
	placementapilog.Info("Validation for PlacementAPI upon update", "name", placementapi.GetName())

	// Call the ValidateUpdate method on the PlacementAPI type
	return placementapi.ValidateUpdate(oldObj)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type PlacementAPI.
func (v *PlacementAPICustomValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	placementapi, ok := obj.(*placementv1beta1.PlacementAPI)
	if !ok {
		return nil, fmt.Errorf("expected a PlacementAPI object but got %T: %w", obj, ErrInvalidObjectType)
	}
	placementapilog.Info("Validation for PlacementAPI upon deletion", "name", placementapi.GetName())

	// Call the ValidateDelete method on the PlacementAPI type
	return placementapi.ValidateDelete()
}
