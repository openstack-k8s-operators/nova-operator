/*
Copyright 2022.

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
package functional_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var _ = Describe("Nova validation", func() {
	It("rejects Nova with metadata in cell0", func() {
		spec := GetDefaultNovaSpec()
		cell0Template := GetDefaultNovaCellTemplate()
		cell0Template["metadataServiceTemplate"] = map[string]interface{}{
			"replicas": 1,
		}

		spec["cellTemplates"] = map[string]interface{}{
			"cell0": cell0Template,
			// note that this is intentional to test that metadata 0 is allowed
			// in cell1 but not in cell0
			"cell1": cell0Template,
		}
		raw := map[string]interface{}{
			"apiVersion": "nova.openstack.org/v1beta1",
			"kind":       "Nova",
			"metadata": map[string]interface{}{
				"name":      novaNames.NovaName.Name,
				"namespace": novaNames.Namespace,
			},
			"spec": spec,
		}

		unstructuredObj := &unstructured.Unstructured{Object: raw}
		_, err := controllerutil.CreateOrPatch(
			ctx, k8sClient, unstructuredObj, func() error { return nil })

		Expect(err).Should(HaveOccurred())
		statusError, ok := err.(*errors.StatusError)
		Expect(ok).To(BeTrue())
		Expect(statusError.ErrStatus.Details.Kind).To(Equal("Nova"))
		Expect(statusError.ErrStatus.Message).To(
			ContainSubstring(
				"invalid: spec.cellTemplates[cell0].metadataServiceTemplate.replicas: " +
					"Invalid value: 1: should be 0 for cell0"),
		)
	})
	It("rejects NovaCell with metadata in cell0", func() {
		spec := GetDefaultNovaCellSpec("cell0")
		spec["metadataServiceTemplate"] = map[string]interface{}{
			"replicas": 3,
		}
		raw := map[string]interface{}{
			"apiVersion": "nova.openstack.org/v1beta1",
			"kind":       "NovaCell",
			"metadata": map[string]interface{}{
				"name":      cell0.CellName.Name,
				"namespace": novaNames.Namespace,
			},
			"spec": spec,
		}

		unstructuredObj := &unstructured.Unstructured{Object: raw}
		_, err := controllerutil.CreateOrPatch(
			ctx, k8sClient, unstructuredObj, func() error { return nil })

		Expect(err).Should(HaveOccurred())
		statusError, ok := err.(*errors.StatusError)
		Expect(ok).To(BeTrue())
		Expect(statusError.ErrStatus.Details.Kind).To(Equal("NovaCell"))
		Expect(statusError.ErrStatus.Message).To(
			ContainSubstring(
				"invalid: spec.metadataServiceTemplate.replicas: " +
					"Invalid value: 3: should be 0 for cell0"),
		)
	})
	It("rejects Nova with too long cell name", func() {
		spec := GetDefaultNovaSpec()
		cell0Template := GetDefaultNovaCellTemplate()
		spec["cellTemplates"] = map[string]interface{}{
			// the limit is 35 chars, this is 5 + 31
			"cell1" + strings.Repeat("x", 31): cell0Template,
		}
		raw := map[string]interface{}{
			"apiVersion": "nova.openstack.org/v1beta1",
			"kind":       "Nova",
			"metadata": map[string]interface{}{
				"name":      novaNames.NovaName.Name,
				"namespace": novaNames.Namespace,
			},
			"spec": spec,
		}

		unstructuredObj := &unstructured.Unstructured{Object: raw}
		_, err := controllerutil.CreateOrPatch(
			ctx, k8sClient, unstructuredObj, func() error { return nil })

		Expect(err).Should(HaveOccurred())
		statusError, ok := err.(*errors.StatusError)
		Expect(ok).To(BeTrue())
		Expect(statusError.ErrStatus.Details.Kind).To(Equal("Nova"))
		Expect(statusError.ErrStatus.Message).To(
			ContainSubstring(
				"invalid: spec.cellTemplates[cell1xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx]: " +
					"Invalid value: \"cell1xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx\": " +
					"should be shorter than 36 characters"),
		)
	})
	It("rejects NovaCell with too long cell name", func() {
		spec := GetDefaultNovaCellSpec("cell1" + strings.Repeat("x", 31))
		raw := map[string]interface{}{
			"apiVersion": "nova.openstack.org/v1beta1",
			"kind":       "NovaCell",
			"metadata": map[string]interface{}{
				"name":      cell0.CellName.Name,
				"namespace": novaNames.Namespace,
			},
			"spec": spec,
		}
		unstructuredObj := &unstructured.Unstructured{Object: raw}
		_, err := controllerutil.CreateOrPatch(
			ctx, k8sClient, unstructuredObj, func() error { return nil })

		Expect(err).Should(HaveOccurred())
		statusError, ok := err.(*errors.StatusError)
		Expect(ok).To(BeTrue())
		Expect(statusError.ErrStatus.Details.Kind).To(Equal("NovaCell"))
		Expect(statusError.ErrStatus.Message).To(
			ContainSubstring(
				"invalid: spec.cellName: " +
					"Invalid value: \"cell1xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx\": " +
					"should be shorter than 36 characters"),
		)
	})
	It("rejects Nova with multiple errors", func() {
		spec := GetDefaultNovaSpec()
		cell0Template := GetDefaultNovaCellTemplate()
		cell0Template["metadataServiceTemplate"] = map[string]interface{}{
			"replicas": 1,
		}

		spec["cellTemplates"] = map[string]interface{}{
			// error: this is cell0 with metadata
			"cell0": cell0Template,
			// error: this is a too long cell name
			"cell1" + strings.Repeat("x", 31): cell0Template,
		}
		raw := map[string]interface{}{
			"apiVersion": "nova.openstack.org/v1beta1",
			"kind":       "Nova",
			"metadata": map[string]interface{}{
				"name":      novaNames.NovaName.Name,
				"namespace": novaNames.Namespace,
			},
			"spec": spec,
		}

		unstructuredObj := &unstructured.Unstructured{Object: raw}
		_, err := controllerutil.CreateOrPatch(
			ctx, k8sClient, unstructuredObj, func() error { return nil })

		Expect(err).Should(HaveOccurred())
		statusError, ok := err.(*errors.StatusError)
		Expect(ok).To(BeTrue())
		Expect(statusError.ErrStatus.Details.Kind).To(Equal("Nova"))
		Expect(statusError.ErrStatus.Details.Causes).To(HaveLen(2))
		Expect(statusError.ErrStatus.Details.Causes).To(
			ContainElement(metav1.StatusCause{
				Type:    "FieldValueInvalid",
				Message: "Invalid value: 1: should be 0 for cell0",
				Field:   "spec.cellTemplates[cell0].metadataServiceTemplate.replicas",
			}),
		)
		Expect(statusError.ErrStatus.Details.Causes).To(
			ContainElement(metav1.StatusCause{
				Type: "FieldValueInvalid",
				Message: "Invalid value: \"cell1xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx\": " +
					"should be shorter than 36 characters",
				Field: "spec.cellTemplates[cell1xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx]",
			}),
		)
	})
})
