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
var novametadatalog = logf.Log.WithName("novametadata-resource")

var errExpectedNovaMetadataObject = errors.New("expected a NovaMetadata object")

// SetupNovaMetadataWebhookWithManager registers the webhook for NovaMetadata in the manager.
func SetupNovaMetadataWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&novav1beta1.NovaMetadata{}).
		WithValidator(&NovaMetadataCustomValidator{}).
		WithDefaulter(&NovaMetadataCustomDefaulter{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-nova-openstack-org-v1beta1-novametadata,mutating=true,failurePolicy=fail,sideEffects=None,groups=nova.openstack.org,resources=novametadata,verbs=create;update,versions=v1beta1,name=mnovametadata-v1beta1.kb.io,admissionReviewVersions=v1

// NovaMetadataCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind NovaMetadata when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type NovaMetadataCustomDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

var _ webhook.CustomDefaulter = &NovaMetadataCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind NovaMetadata.
func (d *NovaMetadataCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	novametadata, ok := obj.(*novav1beta1.NovaMetadata)

	if !ok {
		return fmt.Errorf("%w but got %T", errExpectedNovaMetadataObject, obj)
	}
	novametadatalog.Info("Defaulting for NovaMetadata", "name", novametadata.GetName())

	novametadata.Default()

	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-nova-openstack-org-v1beta1-novametadata,mutating=false,failurePolicy=fail,sideEffects=None,groups=nova.openstack.org,resources=novametadata,verbs=create;update,versions=v1beta1,name=vnovametadata-v1beta1.kb.io,admissionReviewVersions=v1

// NovaMetadataCustomValidator struct is responsible for validating the NovaMetadata resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type NovaMetadataCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &NovaMetadataCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type NovaMetadata.
func (v *NovaMetadataCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	novametadata, ok := obj.(*novav1beta1.NovaMetadata)
	if !ok {
		return nil, fmt.Errorf("%w but got %T", errExpectedNovaMetadataObject, obj)
	}
	novametadatalog.Info("Validation for NovaMetadata upon creation", "name", novametadata.GetName())

	return novametadata.ValidateCreate()
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type NovaMetadata.
func (v *NovaMetadataCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	novametadata, ok := newObj.(*novav1beta1.NovaMetadata)
	if !ok {
		return nil, fmt.Errorf("%w for the newObj but got %T", errExpectedNovaMetadataObject, newObj)
	}
	novametadatalog.Info("Validation for NovaMetadata upon update", "name", novametadata.GetName())

	return novametadata.ValidateUpdate(oldObj)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type NovaMetadata.
func (v *NovaMetadataCustomValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	novametadata, ok := obj.(*novav1beta1.NovaMetadata)
	if !ok {
		return nil, fmt.Errorf("%w but got %T", errExpectedNovaMetadataObject, obj)
	}
	novametadatalog.Info("Validation for NovaMetadata upon deletion", "name", novametadata.GetName())

	return novametadata.ValidateDelete()
}
