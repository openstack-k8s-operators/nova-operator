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
var cyborgconductorlog = logf.Log.WithName("cyborgconductor-resource")

// SetupCyborgConductorWebhookWithManager registers the webhook for CyborgConductor in the manager.
func SetupCyborgConductorWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&cyborgv1beta1.CyborgConductor{}).
		WithValidator(&CyborgConductorCustomValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-cyborg-openstack-org-v1beta1-cyborgconductor,mutating=false,failurePolicy=fail,sideEffects=None,groups=cyborg.openstack.org,resources=cyborgconductors,verbs=create;update,versions=v1beta1,name=vcyborgconductor-v1beta1.kb.io,admissionReviewVersions=v1

// CyborgConductorCustomValidator implements webhook.CustomValidator for the CyborgConductor type.
type CyborgConductorCustomValidator struct{}

var _ webhook.CustomValidator = &CyborgConductorCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type CyborgConductor.
func (v *CyborgConductorCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	cyborgconductor, ok := obj.(*cyborgv1beta1.CyborgConductor)
	if !ok {
		return nil, fmt.Errorf("%w: expected a CyborgConductor object but got %T", errUnexpectedCyborgObjectType, obj)
	}
	cyborgconductorlog.Info("Validation for CyborgConductor upon creation", "name", cyborgconductor.GetName())

	return cyborgconductor.ValidateCreate()
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type CyborgConductor.
func (v *CyborgConductorCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	cyborgconductor, ok := newObj.(*cyborgv1beta1.CyborgConductor)
	if !ok {
		return nil, fmt.Errorf("%w: expected a CyborgConductor object for the newObj but got %T", errUnexpectedCyborgObjectType, newObj)
	}
	cyborgconductorlog.Info("Validation for CyborgConductor upon update", "name", cyborgconductor.GetName())

	return cyborgconductor.ValidateUpdate(oldObj)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type CyborgConductor.
func (v *CyborgConductorCustomValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	cyborgconductor, ok := obj.(*cyborgv1beta1.CyborgConductor)
	if !ok {
		return nil, fmt.Errorf("%w: expected a CyborgConductor object but got %T", errUnexpectedCyborgObjectType, obj)
	}
	cyborgconductorlog.Info("Validation for CyborgConductor upon deletion", "name", cyborgconductor.GetName())

	return cyborgconductor.ValidateDelete()
}
