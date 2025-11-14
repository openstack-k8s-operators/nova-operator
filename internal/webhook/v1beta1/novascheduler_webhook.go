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
var novaschedulerlog = logf.Log.WithName("novascheduler-resource")

var errExpectedNovaSchedulerObject = errors.New("expected a NovaScheduler object")

// SetupNovaSchedulerWebhookWithManager registers the webhook for NovaScheduler in the manager.
func SetupNovaSchedulerWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&novav1beta1.NovaScheduler{}).
		WithValidator(&NovaSchedulerCustomValidator{}).
		WithDefaulter(&NovaSchedulerCustomDefaulter{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-nova-openstack-org-v1beta1-novascheduler,mutating=true,failurePolicy=fail,sideEffects=None,groups=nova.openstack.org,resources=novaschedulers,verbs=create;update,versions=v1beta1,name=mnovascheduler-v1beta1.kb.io,admissionReviewVersions=v1

// NovaSchedulerCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind NovaScheduler when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type NovaSchedulerCustomDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

var _ webhook.CustomDefaulter = &NovaSchedulerCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind NovaScheduler.
func (d *NovaSchedulerCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	novascheduler, ok := obj.(*novav1beta1.NovaScheduler)

	if !ok {
		return fmt.Errorf("%w but got %T", errExpectedNovaSchedulerObject, obj)
	}
	novaschedulerlog.Info("Defaulting for NovaScheduler", "name", novascheduler.GetName())

	novascheduler.Default()

	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-nova-openstack-org-v1beta1-novascheduler,mutating=false,failurePolicy=fail,sideEffects=None,groups=nova.openstack.org,resources=novaschedulers,verbs=create;update,versions=v1beta1,name=vnovascheduler-v1beta1.kb.io,admissionReviewVersions=v1

// NovaSchedulerCustomValidator struct is responsible for validating the NovaScheduler resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type NovaSchedulerCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &NovaSchedulerCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type NovaScheduler.
func (v *NovaSchedulerCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	novascheduler, ok := obj.(*novav1beta1.NovaScheduler)
	if !ok {
		return nil, fmt.Errorf("%w but got %T", errExpectedNovaSchedulerObject, obj)
	}
	novaschedulerlog.Info("Validation for NovaScheduler upon creation", "name", novascheduler.GetName())

	return novascheduler.ValidateCreate()
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type NovaScheduler.
func (v *NovaSchedulerCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	novascheduler, ok := newObj.(*novav1beta1.NovaScheduler)
	if !ok {
		return nil, fmt.Errorf("%w for the newObj but got %T", errExpectedNovaSchedulerObject, newObj)
	}
	novaschedulerlog.Info("Validation for NovaScheduler upon update", "name", novascheduler.GetName())

	return novascheduler.ValidateUpdate(oldObj)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type NovaScheduler.
func (v *NovaSchedulerCustomValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	novascheduler, ok := obj.(*novav1beta1.NovaScheduler)
	if !ok {
		return nil, fmt.Errorf("%w but got %T", errExpectedNovaSchedulerObject, obj)
	}
	novaschedulerlog.Info("Validation for NovaScheduler upon deletion", "name", novascheduler.GetName())

	return novascheduler.ValidateDelete()
}
