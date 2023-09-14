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
	"errors"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var _ = Describe("Nova validation", func() {
	It("rejects Nova with metadata in cell0", func() {
		spec := GetDefaultNovaSpec()
		spec["metadataServiceTemplate"] = map[string]interface{}{
			"enabled": false,
		}
		cell0Template := GetDefaultNovaCellTemplate()
		cell0Template["metadataServiceTemplate"] = map[string]interface{}{
			"enabled": true,
		}

		spec["cellTemplates"] = map[string]interface{}{
			"cell0": cell0Template,
			// note that this is intentional to test that metadata 1 is allowed
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
		var statusError *k8s_errors.StatusError
		Expect(errors.As(err, &statusError)).To(BeTrue())
		Expect(statusError.ErrStatus.Details.Kind).To(Equal("Nova"))
		Expect(statusError.ErrStatus.Message).To(
			ContainSubstring(
				"invalid: spec.cellTemplates[cell0].metadataServiceTemplate.enabled: " +
					"Invalid value: true: should be false for cell0"),
		)
	})
	It("rejects NovaCell with metadata in cell0", func() {
		spec := GetDefaultNovaCellSpec(cell0)
		spec["metadataServiceTemplate"] = map[string]interface{}{
			"enabled": true,
		}
		raw := map[string]interface{}{
			"apiVersion": "nova.openstack.org/v1beta1",
			"kind":       "NovaCell",
			"metadata": map[string]interface{}{
				"name":      cell0.CellCRName.Name,
				"namespace": novaNames.Namespace,
			},
			"spec": spec,
		}

		unstructuredObj := &unstructured.Unstructured{Object: raw}
		_, err := controllerutil.CreateOrPatch(
			ctx, k8sClient, unstructuredObj, func() error { return nil })

		Expect(err).Should(HaveOccurred())
		var statusError *k8s_errors.StatusError
		Expect(errors.As(err, &statusError)).To(BeTrue())
		Expect(statusError.ErrStatus.Details.Kind).To(Equal("NovaCell"))
		Expect(statusError.ErrStatus.Message).To(
			ContainSubstring(
				"invalid: spec.metadataServiceTemplate.enabled: " +
					"Invalid value: true: should be false for cell0"),
		)
	})
	It("rejects Nova with NoVNCProxy in cell0", func() {
		spec := GetDefaultNovaSpec()
		cell0Template := GetDefaultNovaCellTemplate()
		cell0Template["noVNCProxyServiceTemplate"] = map[string]interface{}{
			"enabled": true,
		}

		spec["cellTemplates"] = map[string]interface{}{
			"cell0": cell0Template,
			// note that this is intentional to test that novncproxy is allowed
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
		var statusError *k8s_errors.StatusError
		Expect(errors.As(err, &statusError)).To(BeTrue())
		Expect(statusError.ErrStatus.Details.Kind).To(Equal("Nova"))
		Expect(statusError.ErrStatus.Message).To(
			ContainSubstring(
				"invalid: spec.cellTemplates[cell0].noVNCProxyServiceTemplate.enabled: " +
					"Invalid value: true: should be false for cell0"),
		)
	})
	It("rejects NovaCell with NoVNCProxy in cell0", func() {
		spec := GetDefaultNovaCellSpec(cell0)
		spec["noVNCProxyServiceTemplate"] = map[string]interface{}{
			"enabled": true,
		}
		raw := map[string]interface{}{
			"apiVersion": "nova.openstack.org/v1beta1",
			"kind":       "NovaCell",
			"metadata": map[string]interface{}{
				"name":      cell0.CellCRName.Name,
				"namespace": novaNames.Namespace,
			},
			"spec": spec,
		}

		unstructuredObj := &unstructured.Unstructured{Object: raw}
		_, err := controllerutil.CreateOrPatch(
			ctx, k8sClient, unstructuredObj, func() error { return nil })

		Expect(err).Should(HaveOccurred())
		var statusError *k8s_errors.StatusError
		Expect(errors.As(err, &statusError)).To(BeTrue())
		Expect(statusError.ErrStatus.Details.Kind).To(Equal("NovaCell"))
		Expect(statusError.ErrStatus.Message).To(
			ContainSubstring(
				"invalid: spec.noVNCProxyServiceTemplate.enabled: " +
					"Invalid value: true: should be false for cell0"),
		)
	})
	It("rejects Nova with too long cell name", func() {
		spec := GetDefaultNovaSpec()
		cell0Template := GetDefaultNovaCellTemplate()
		spec["cellTemplates"] = map[string]interface{}{
			"cell0": cell0Template,
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
		var statusError *k8s_errors.StatusError
		Expect(errors.As(err, &statusError)).To(BeTrue())
		Expect(statusError.ErrStatus.Details.Kind).To(Equal("Nova"))
		Expect(statusError.ErrStatus.Message).To(
			ContainSubstring(
				"invalid: spec.cellTemplates[cell1xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx]: " +
					"Invalid value: \"cell1xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx\": " +
					"should be shorter than 36 characters"),
		)
	})
	It("rejects NovaCell with too long cell name", func() {
		cell := GetCellNames(novaNames.NovaName, "cell1"+strings.Repeat("x", 31))
		spec := GetDefaultNovaCellSpec(cell)
		raw := map[string]interface{}{
			"apiVersion": "nova.openstack.org/v1beta1",
			"kind":       "NovaCell",
			"metadata": map[string]interface{}{
				"name":      cell0.CellCRName.Name,
				"namespace": novaNames.Namespace,
			},
			"spec": spec,
		}
		unstructuredObj := &unstructured.Unstructured{Object: raw}
		_, err := controllerutil.CreateOrPatch(
			ctx, k8sClient, unstructuredObj, func() error { return nil })

		Expect(err).Should(HaveOccurred())
		var statusError *k8s_errors.StatusError
		Expect(errors.As(err, &statusError)).To(BeTrue())
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
		spec["metadataServiceTemplate"] = map[string]interface{}{
			"enabled": false,
		}
		cell0Template := GetDefaultNovaCellTemplate()
		cell0Template["metadataServiceTemplate"] = map[string]interface{}{
			"enabled": true,
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
		var statusError *k8s_errors.StatusError
		Expect(errors.As(err, &statusError)).To(BeTrue())
		Expect(statusError.ErrStatus.Details.Kind).To(Equal("Nova"))
		Expect(statusError.ErrStatus.Details.Causes).To(HaveLen(2))
		Expect(statusError.ErrStatus.Details.Causes).To(
			ContainElement(metav1.StatusCause{
				Type:    "FieldValueInvalid",
				Message: "Invalid value: true: should be false for cell0",
				Field:   "spec.cellTemplates[cell0].metadataServiceTemplate.enabled",
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
	It("rejects Nova if cell0 is missing", func() {
		spec := GetDefaultNovaSpec()
		cell1Template := GetDefaultNovaCellTemplate()

		spec["cellTemplates"] = map[string]interface{}{
			// We explicitly not define cell0 template to trigger the
			// validation
			"cell1": cell1Template,
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
		var statusError *k8s_errors.StatusError
		Expect(errors.As(err, &statusError)).To(BeTrue())
		Expect(statusError.ErrStatus.Details.Kind).To(Equal("Nova"))
		Expect(statusError.ErrStatus.Message).To(
			ContainSubstring(
				"invalid: spec.cellTemplates: Required value: " +
					"cell0 specification is missing, cell0 key is required in cellTemplates"),
		)
	})
	It("rejects Nova if cell0 contains novacomputetemplates", func() {
		spec := GetDefaultNovaSpec()
		cell0 := GetDefaultNovaCellTemplate()
		cell0["novaComputeTemplates"] = map[string]interface{}{
			"ironic-compute": GetDefaultNovaComputeTemplate(),
		}
		spec["cellTemplates"] = map[string]interface{}{"cell0": cell0}
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
		var statusError *k8s_errors.StatusError
		Expect(errors.As(err, &statusError)).To(BeTrue())
		Expect(statusError.ErrStatus.Details.Kind).To(Equal("Nova"))
		Expect(statusError.ErrStatus.Message).To(
			ContainSubstring(
				"invalid: spec.cellTemplates[cell0].novaComputeTemplates: " +
					"Invalid value: \"novaComputeTemplates\": should have zero elements for cell0",
			),
		)
	})
	It("rejects Nova with too long compute name", func() {
		spec := GetDefaultNovaSpec()
		cell0 := GetDefaultNovaCellTemplate()
		cell1 := GetDefaultNovaCellTemplate()
		cell1["novaComputeTemplates"] = map[string]interface{}{
			"ironic-compute" + strings.Repeat("x", 31): GetDefaultNovaComputeTemplate(),
		}
		spec["cellTemplates"] = map[string]interface{}{"cell0": cell0, "cell1": cell1}
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
		var statusError *k8s_errors.StatusError
		Expect(errors.As(err, &statusError)).To(BeTrue())
		Expect(statusError.ErrStatus.Details.Kind).To(Equal("Nova"))
		Expect(statusError.ErrStatus.Message).To(
			ContainSubstring(
				"invalid: spec.novaComputeTemplates: " +
					"Invalid value: \"ironic-computexxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx\": " +
					"should be shorter than 20 characters",
			),
		)
	})
	It("check NovaCompute validation with replicas = nil", func() {
		spec := GetDefaultNovaSpec()
		cell0 := GetDefaultNovaCellTemplate()
		cell1 := GetDefaultNovaCellTemplate()
		novaCompute := GetDefaultNovaComputeTemplate()
		novaCompute["replicas"] = nil
		cell1["novaComputeTemplates"] = map[string]interface{}{
			"ironic-compute": novaCompute,
		}
		spec["cellTemplates"] = map[string]interface{}{"cell0": cell0, "cell1": cell1}
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

		Expect(err).Should(Succeed())
	})
	It("rejects NovaCell - cell0 contains novacomputetemplates", func() {
		spec := GetDefaultNovaCellSpec(cell0)
		spec["novaComputeTemplates"] = map[string]interface{}{
			"ironic-compute": GetDefaultNovaComputeTemplate(),
		}
		raw := map[string]interface{}{
			"apiVersion": "nova.openstack.org/v1beta1",
			"kind":       "NovaCell",
			"metadata": map[string]interface{}{
				"name":      cell0.CellCRName.Name,
				"namespace": novaNames.Namespace,
			},
			"spec": spec,
		}
		unstructuredObj := &unstructured.Unstructured{Object: raw}
		_, err := controllerutil.CreateOrPatch(
			ctx, k8sClient, unstructuredObj, func() error { return nil })

		Expect(err).Should(HaveOccurred())
		var statusError *k8s_errors.StatusError
		Expect(errors.As(err, &statusError)).To(BeTrue())
		Expect(statusError.ErrStatus.Details.Kind).To(Equal("NovaCell"))
		Expect(statusError.ErrStatus.Message).To(
			ContainSubstring(
				"invalid: spec.novaComputeTemplates: " +
					"Invalid value: \"novaComputeTemplates\": should have zero elements for cell0",
			),
		)
	})
	It("rejects NovaCell with too long compute name", func() {
		spec := GetDefaultNovaCellSpec(cell1)
		spec["novaComputeTemplates"] = map[string]interface{}{
			"ironic-compute" + strings.Repeat("x", 31): GetDefaultNovaComputeTemplate(),
		}
		raw := map[string]interface{}{
			"apiVersion": "nova.openstack.org/v1beta1",
			"kind":       "NovaCell",
			"metadata": map[string]interface{}{
				"name":      cell1.CellCRName.Name,
				"namespace": novaNames.Namespace,
			},
			"spec": spec,
		}
		unstructuredObj := &unstructured.Unstructured{Object: raw}
		_, err := controllerutil.CreateOrPatch(
			ctx, k8sClient, unstructuredObj, func() error { return nil })

		Expect(err).Should(HaveOccurred())
		var statusError *k8s_errors.StatusError
		Expect(errors.As(err, &statusError)).To(BeTrue())
		Expect(statusError.ErrStatus.Details.Kind).To(Equal("NovaCell"))
		Expect(statusError.ErrStatus.Message).To(
			ContainSubstring(
				"invalid: spec.novaComputeTemplates: " +
					"Invalid value: \"ironic-computexxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx\": " +
					"should be shorter than 20 characters",
			),
		)
	})
	It("rejects Nova with metadata both on top and in cells", func() {
		spec := GetDefaultNovaSpec()
		spec["metadataServiceTemplate"] = map[string]interface{}{
			"enabled": true,
		}
		cell0Template := GetDefaultNovaCellTemplate()
		cell1Template := GetDefaultNovaCellTemplate()
		cell1Template["metadataServiceTemplate"] = map[string]interface{}{
			"enabled": true,
		}

		spec["cellTemplates"] = map[string]interface{}{
			"cell0": cell0Template,
			"cell1": cell1Template,
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
		var statusError *k8s_errors.StatusError
		Expect(errors.As(err, &statusError)).To(BeTrue())
		Expect(statusError.ErrStatus.Details.Kind).To(Equal("Nova"))
		Expect(statusError.ErrStatus.Message).To(
			ContainSubstring(
				"invalid: spec.cellTemplates[cell1].metadataServiceTemplate.enabled: " +
					"Invalid value: true: should be false " +
					"as metadata is enabled on the top level too. " +
					"The metadata service can be either enabled on top " +
					"or in the cells but not in both places at the same time.",
			),
		)
	})
})
