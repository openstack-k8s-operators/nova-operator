/*
Copyright 2026.

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

// Package v1beta1 implements webhooks for Cyborg v1beta1 API
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

	cyborgv1beta1 "github.com/openstack-k8s-operators/nova-operator/api/cyborg/v1beta1"
)

// nolint:unused
var cyborglog = logf.Log.WithName("cyborg-resource")

var (
	errUnexpectedCyborgObjectType = errors.New("unexpected object type")
)

// SetupCyborgWebhookWithManager registers the webhook for Cyborg in the manager.
func SetupCyborgWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&cyborgv1beta1.Cyborg{}).
		WithValidator(&CyborgCustomValidator{}).
		WithDefaulter(&CyborgCustomDefaulter{}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-cyborg-openstack-org-v1beta1-cyborg,mutating=true,failurePolicy=fail,sideEffects=None,groups=cyborg.openstack.org,resources=cyborgs,verbs=create;update,versions=v1beta1,name=mcyborg-v1beta1.kb.io,admissionReviewVersions=v1

// CyborgCustomDefaulter implements webhook.CustomDefaulter for the Cyborg type.
type CyborgCustomDefaulter struct{}

var _ webhook.CustomDefaulter = &CyborgCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind Cyborg.
func (d *CyborgCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	cyborg, ok := obj.(*cyborgv1beta1.Cyborg)
	if !ok {
		return fmt.Errorf("%w: expected a Cyborg object but got %T", errUnexpectedCyborgObjectType, obj)
	}
	cyborglog.Info("Defaulting for Cyborg", "name", cyborg.GetName())

	cyborg.Default()

	return nil
}

// +kubebuilder:webhook:path=/validate-cyborg-openstack-org-v1beta1-cyborg,mutating=false,failurePolicy=fail,sideEffects=None,groups=cyborg.openstack.org,resources=cyborgs,verbs=create;update,versions=v1beta1,name=vcyborg-v1beta1.kb.io,admissionReviewVersions=v1

// CyborgCustomValidator implements webhook.CustomValidator for the Cyborg type.
type CyborgCustomValidator struct{}

var _ webhook.CustomValidator = &CyborgCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type Cyborg.
func (v *CyborgCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	cyborg, ok := obj.(*cyborgv1beta1.Cyborg)
	if !ok {
		return nil, fmt.Errorf("%w: expected a Cyborg object but got %T", errUnexpectedCyborgObjectType, obj)
	}
	cyborglog.Info("Validation for Cyborg upon creation", "name", cyborg.GetName())

	return cyborg.ValidateCreate()
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type Cyborg.
func (v *CyborgCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	cyborg, ok := newObj.(*cyborgv1beta1.Cyborg)
	if !ok {
		return nil, fmt.Errorf("%w: expected a Cyborg object for the newObj but got %T", errUnexpectedCyborgObjectType, newObj)
	}
	cyborglog.Info("Validation for Cyborg upon update", "name", cyborg.GetName())

	return cyborg.ValidateUpdate(oldObj)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type Cyborg.
func (v *CyborgCustomValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	cyborg, ok := obj.(*cyborgv1beta1.Cyborg)
	if !ok {
		return nil, fmt.Errorf("%w: expected a Cyborg object but got %T", errUnexpectedCyborgObjectType, obj)
	}
	cyborglog.Info("Validation for Cyborg upon deletion", "name", cyborg.GetName())

	return cyborg.ValidateDelete()
}
