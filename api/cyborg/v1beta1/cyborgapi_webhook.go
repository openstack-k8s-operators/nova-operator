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
	"fmt"

	"github.com/google/go-cmp/cmp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common/service"
)

// log is for logging in this package.
var cyborgapilog = logf.Log.WithName("cyborgapi-resource")

var _ webhook.Validator = &CyborgAPI{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *CyborgAPI) ValidateCreate() (admission.Warnings, error) {
	cyborgapilog.Info("validate create", "name", r.Name)

	errors := field.ErrorList{}
	basePath := field.NewPath("spec")

	// validate the service override key is valid
	errors = append(errors, service.ValidateRoutedOverrides(basePath.Child("override").Child("service"), r.Spec.Override.Service)...)

	errors = append(errors, topologyv1.ValidateTopologyRef(
		r.Spec.TopologyRef, *basePath.Child("").Child("topologyRef"), r.Namespace)...)

	if len(errors) != 0 {
		cyborgapilog.Info("validation failed", "name", r.Name)
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: "cyborg.openstack.org", Kind: "CyborgAPI"},
			r.Name, errors)
	}
	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *CyborgAPI) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	cyborgapilog.Info("validate update", "name", r.Name)
	oldCyborgAPI, ok := old.(*CyborgAPI)
	if !ok || oldCyborgAPI == nil {
		return nil, apierrors.NewInternalError(fmt.Errorf("unable to convert existing object"))
	}

	cyborgapilog.Info("validate update", "diff", cmp.Diff(oldCyborgAPI, r))

	errors := field.ErrorList{}
	basePath := field.NewPath("spec")

	// validate the service override key is valid
	errors = append(errors, service.ValidateRoutedOverrides(basePath.Child("override").Child("service"), r.Spec.Override.Service)...)

	errors = append(errors, r.Spec.ValidateTopology(basePath.Child(""), r.Namespace)...)

	if len(errors) != 0 {
		cyborgapilog.Info("validation failed", "name", r.Name)
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: "cyborg.openstack.org", Kind: "CyborgAPI"},
			r.Name, errors)
	}
	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *CyborgAPI) ValidateDelete() (admission.Warnings, error) {
	cyborgapilog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil, nil
}

// ValidateTopology validates the referenced TopoRef.Namespace.
func (r *CyborgAPITemplate) ValidateTopology(
	basePath *field.Path,
	namespace string,
) field.ErrorList {
	return topologyv1.ValidateTopologyRef(
		r.TopologyRef,
		*basePath.Child("topologyRef"),
		namespace,
	)
}
