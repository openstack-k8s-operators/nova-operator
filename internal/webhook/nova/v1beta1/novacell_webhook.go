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
var novacelllog = logf.Log.WithName("novacell-resource")

var errExpectedNovaCellObject = errors.New("expected a NovaCell object")

// SetupNovaCellWebhookWithManager registers the webhook for NovaCell in the manager.
func SetupNovaCellWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&novav1beta1.NovaCell{}).
		WithValidator(&NovaCellCustomValidator{}).
		WithDefaulter(&NovaCellCustomDefaulter{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-nova-openstack-org-v1beta1-novacell,mutating=true,failurePolicy=fail,sideEffects=None,groups=nova.openstack.org,resources=novacells,verbs=create;update,versions=v1beta1,name=mnovacell-v1beta1.kb.io,admissionReviewVersions=v1

// NovaCellCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind NovaCell when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type NovaCellCustomDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

var _ webhook.CustomDefaulter = &NovaCellCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind NovaCell.
func (d *NovaCellCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	novacell, ok := obj.(*novav1beta1.NovaCell)

	if !ok {
		return fmt.Errorf("%w but got %T", errExpectedNovaCellObject, obj)
	}
	novacelllog.Info("Defaulting for NovaCell", "name", novacell.GetName())

	novacell.Default()

	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-nova-openstack-org-v1beta1-novacell,mutating=false,failurePolicy=fail,sideEffects=None,groups=nova.openstack.org,resources=novacells,verbs=create;update,versions=v1beta1,name=vnovacell-v1beta1.kb.io,admissionReviewVersions=v1

// NovaCellCustomValidator struct is responsible for validating the NovaCell resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type NovaCellCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &NovaCellCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type NovaCell.
func (v *NovaCellCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	novacell, ok := obj.(*novav1beta1.NovaCell)
	if !ok {
		return nil, fmt.Errorf("%w but got %T", errExpectedNovaCellObject, obj)
	}
	novacelllog.Info("Validation for NovaCell upon creation", "name", novacell.GetName())

	return novacell.ValidateCreate()
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type NovaCell.
func (v *NovaCellCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	novacell, ok := newObj.(*novav1beta1.NovaCell)
	if !ok {
		return nil, fmt.Errorf("%w for the newObj but got %T", errExpectedNovaCellObject, newObj)
	}
	novacelllog.Info("Validation for NovaCell upon update", "name", novacell.GetName())

	return novacell.ValidateUpdate(oldObj)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type NovaCell.
func (v *NovaCellCustomValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	novacell, ok := obj.(*novav1beta1.NovaCell)
	if !ok {
		return nil, fmt.Errorf("%w but got %T", errExpectedNovaCellObject, obj)
	}
	novacelllog.Info("Validation for NovaCell upon deletion", "name", novacell.GetName())

	return novacell.ValidateDelete()
}
