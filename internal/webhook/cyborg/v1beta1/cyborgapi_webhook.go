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

package v1beta1

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	cyborgv1beta1 "github.com/openstack-k8s-operators/nova-operator/api/cyborg/v1beta1"
)

// nolint:unused
var cyborgapilog = logf.Log.WithName("cyborgapi-resource")

// SetupCyborgAPIWebhookWithManager registers the webhook for CyborgAPI in the manager.
func SetupCyborgAPIWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&cyborgv1beta1.CyborgAPI{}).
		WithValidator(&CyborgAPICustomValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-cyborg-openstack-org-v1beta1-cyborgapi,mutating=false,failurePolicy=fail,sideEffects=None,groups=cyborg.openstack.org,resources=cyborgapis,verbs=create;update,versions=v1beta1,name=vcyborgapi-v1beta1.kb.io,admissionReviewVersions=v1

// CyborgAPICustomValidator implements webhook.CustomValidator for the CyborgAPI type.
type CyborgAPICustomValidator struct{}

var _ webhook.CustomValidator = &CyborgAPICustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type CyborgAPI.
func (v *CyborgAPICustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	cyborgapi, ok := obj.(*cyborgv1beta1.CyborgAPI)
	if !ok {
		return nil, fmt.Errorf("%w: expected a CyborgAPI object but got %T", errUnexpectedCyborgObjectType, obj)
	}
	cyborgapilog.Info("Validation for CyborgAPI upon creation", "name", cyborgapi.GetName())

	return cyborgapi.ValidateCreate()
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type CyborgAPI.
func (v *CyborgAPICustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	cyborgapi, ok := newObj.(*cyborgv1beta1.CyborgAPI)
	if !ok {
		return nil, fmt.Errorf("%w: expected a CyborgAPI object for the newObj but got %T", errUnexpectedCyborgObjectType, newObj)
	}
	cyborgapilog.Info("Validation for CyborgAPI upon update", "name", cyborgapi.GetName())

	return cyborgapi.ValidateUpdate(oldObj)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type CyborgAPI.
func (v *CyborgAPICustomValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	cyborgapi, ok := obj.(*cyborgv1beta1.CyborgAPI)
	if !ok {
		return nil, fmt.Errorf("%w: expected a CyborgAPI object but got %T", errUnexpectedCyborgObjectType, obj)
	}
	cyborgapilog.Info("Validation for CyborgAPI upon deletion", "name", cyborgapi.GetName())

	return cyborgapi.ValidateDelete()
}
