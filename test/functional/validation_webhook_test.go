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
	"fmt"
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
	DescribeTable("rejects Nova with wrong cell name format", func(cellName string) {
		spec := GetDefaultNovaSpec()
		cell0Template := GetDefaultNovaCellTemplate()
		spec["cellTemplates"] = map[string]interface{}{
			"cell0": cell0Template,
			// the limit is 35 chars, this is 5 + 31
			cellName: cell0Template,
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
				fmt.Sprintf(
					"invalid: spec.cellTemplates[%s]: Invalid value: "+
						"\"%s\": should match with the regex", cellName, cellName),
			),
		)
	},
		Entry("cell name starts with a capital letter", "Cell1xx"),
		Entry("cell name contain wrong signs", "cell1$xx__"),
		Entry("cell name contain upper case", "cellMy"),
	)
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
			ironicComputeName: GetDefaultNovaComputeTemplate(),
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
			ironicComputeName + strings.Repeat("x", 31): GetDefaultNovaComputeTemplate(),
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
				"invalid: spec.cellTemplates[cell1]." +
					"novaComputeTemplates[ironic-computexxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx]: " +
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
			ironicComputeName: novaCompute,
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
	It("rejects ironic NovaCompute with replicas > 1", func() {
		spec := GetDefaultNovaSpec()
		cell0 := GetDefaultNovaCellTemplate()
		cell1 := GetDefaultNovaCellTemplate()
		novaCompute := GetDefaultNovaComputeTemplate()
		novaCompute["replicas"] = 2
		cell1["novaComputeTemplates"] = map[string]interface{}{
			ironicComputeName: novaCompute,
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
				"invalid: spec.cellTemplates[cell1]." +
					"novaComputeTemplates[ironic-compute].replicas: " +
					"Invalid value: 2: should be max 1 for ironic.IronicDriver",
			),
		)
	})
	It("rejects NovaCell - cell0 contains novacomputetemplates", func() {
		spec := GetDefaultNovaCellSpec(cell0)
		spec["novaComputeTemplates"] = map[string]interface{}{
			ironicComputeName: GetDefaultNovaComputeTemplate(),
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
			ironicComputeName + strings.Repeat("x", 31): GetDefaultNovaComputeTemplate(),
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
				"invalid: spec.novaComputeTemplates[ironic-computexxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx]: " +
					"Invalid value: \"ironic-computexxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx\": " +
					"should be shorter than 20 characters",
			),
		)
	})
	DescribeTable("rejects NovaCell with wrong compute name", func(computeName string) {
		spec := GetDefaultNovaCellSpec(cell1)
		spec["novaComputeTemplates"] = map[string]interface{}{
			computeName: GetDefaultNovaComputeTemplate(),
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
				fmt.Sprintf(
					"invalid: spec.novaComputeTemplates[%s]: Invalid value: "+
						"\"%s\": should match with the regex", computeName, computeName),
			),
		)
	},
		Entry("compute name starts with a capital letter", "Compute1xx"),
		Entry("compute name contain wrong signs", "compute1-xx__"),
		Entry("compute name contain upper case", "computeFake1"),
	)
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
	It("check Cell validation with duplicate cellMessageBusInstance", func() {
		spec := GetDefaultNovaSpec()
		cell0 := GetDefaultNovaCellTemplate()
		cell1 := GetDefaultNovaCellTemplate()
		cell2 := GetDefaultNovaCellTemplate()
		cell1["cellMessageBusInstance"] = "rabbitmq-of-caerbannog"
		cell2["cellMessageBusInstance"] = "rabbitmq-of-caerbannog"
		spec["cellTemplates"] = map[string]interface{}{"cell0": cell0, "cell1": cell1, "cell2": cell2}
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
		Expect(statusError.ErrStatus.Message).To(
			ContainSubstring(
				"Invalid value: \"rabbitmq-of-caerbannog\": RabbitMqCluster " +
					"CR need to be uniq per cell. It's duplicated with cell:",
			),
		)
	})
	It("rejects NovaAPI with wrong defaultConfigOverwrite", func() {
		spec := GetDefaultNovaAPISpec(novaNames)
		spec["defaultConfigOverwrite"] = map[string]interface{}{
			"policy.yaml":   "custom policy",
			"api-paste.ini": "custom paste config",
			"foo.conf":      "wrong custom config",
		}
		raw := map[string]interface{}{
			"apiVersion": "nova.openstack.org/v1beta1",
			"kind":       "NovaAPI",
			"metadata": map[string]interface{}{
				"name":      novaNames.APIName.Name,
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
		Expect(statusError.ErrStatus.Details.Kind).To(Equal("NovaAPI"))
		Expect(statusError.ErrStatus.Message).To(
			ContainSubstring(
				"invalid: spec.defaultConfigOverwrite: " +
					"Invalid value: \"foo.conf\": Only the following keys " +
					"are valid: policy.yaml, api-paste.ini",
			),
		)
	})
	It("rejects Nova with wrong defaultConfigOverwrite in NovaAPI", func() {
		spec := GetDefaultNovaSpec()
		spec["cellTemplates"] = map[string]interface{}{
			"cell0": GetDefaultNovaCellTemplate(),
		}
		spec["apiServiceTemplate"] = map[string]interface{}{
			"defaultConfigOverwrite": map[string]interface{}{
				"policy.yaml":   "custom policy",
				"api-paste.ini": "custom paste config",
				"provider.yaml": "provider.yaml not supported here",
			},
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
				"invalid: spec.apiServiceTemplate.defaultConfigOverwrite: " +
					"Invalid value: \"provider.yaml\": Only the following " +
					"keys are valid: policy.yaml, api-paste.ini"),
		)
	})

	It("rejects NovaMetadata with wrong defaultConfigOverwrite", func() {
		spec := GetDefaultNovaMetadataSpec(novaNames.InternalTopLevelSecretName)
		spec["defaultConfigOverwrite"] = map[string]interface{}{
			"policy.yaml":   "custom policy not supported",
			"api-paste.ini": "custom paste config",
		}
		raw := map[string]interface{}{
			"apiVersion": "nova.openstack.org/v1beta1",
			"kind":       "NovaMetadata",
			"metadata": map[string]interface{}{
				"name":      novaNames.MetadataName.Name,
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
		Expect(statusError.ErrStatus.Details.Kind).To(Equal("NovaMetadata"))
		Expect(statusError.ErrStatus.Message).To(
			ContainSubstring(
				"invalid: spec.defaultConfigOverwrite: " +
					"Invalid value: \"policy.yaml\": Only the following keys " +
					"are valid: api-paste.ini",
			),
		)
	})
	It("rejects Nova with wrong defaultConfigOverwrite in top level NovaMetadata", func() {
		spec := GetDefaultNovaSpec()
		spec["cellTemplates"] = map[string]interface{}{
			"cell0": GetDefaultNovaCellTemplate(),
		}
		spec["metadataServiceTemplate"] = map[string]interface{}{
			"defaultConfigOverwrite": map[string]interface{}{
				"policy.yaml":   "custom policy not supported",
				"api-paste.ini": "custom paste config",
			},
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
				"invalid: spec.metadataServiceTemplate.defaultConfigOverwrite: " +
					"Invalid value: \"policy.yaml\": Only the following " +
					"keys are valid: api-paste.ini"),
		)
	})
	It("rejects Nova with wrong defaultConfigOverwrite in cell level NovaMetadata", func() {
		spec := GetDefaultNovaSpec()
		cell1 := GetDefaultNovaCellTemplate()
		cell1["metadataServiceTemplate"] = map[string]interface{}{
			"defaultConfigOverwrite": map[string]interface{}{
				"policy.yaml":   "custom policy not supported",
				"api-paste.ini": "custom paste config",
			},
		}
		spec["cellTemplates"] = map[string]interface{}{
			"cell0": GetDefaultNovaCellTemplate(),
			"cell1": cell1,
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
				"invalid: spec.cellTemplates[cell1].metadataServiceTemplate.defaultConfigOverwrite: " +
					"Invalid value: \"policy.yaml\": Only the following " +
					"keys are valid: api-paste.ini"),
		)
	})
	It("rejects NovaCompute with wrong defaultConfigOverwrite", func() {
		spec := GetDefaultNovaComputeSpec(cell1)
		spec["defaultConfigOverwrite"] = map[string]interface{}{
			"policy.yaml":      "custom policy not supported",
			"provider123.yaml": "provider*.yaml is supported",
		}
		raw := map[string]interface{}{
			"apiVersion": "nova.openstack.org/v1beta1",
			"kind":       "NovaCompute",
			"metadata": map[string]interface{}{
				"name":      cell1.NovaComputeName.Name,
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
		Expect(statusError.ErrStatus.Details.Kind).To(Equal("NovaCompute"))
		Expect(statusError.ErrStatus.Message).To(
			ContainSubstring(
				"invalid: spec.defaultConfigOverwrite: " +
					"Invalid value: \"policy.yaml\": " +
					"Only the following keys are valid: provider*.yaml",
			),
		)
	})
	It("rejects NovaCell with wrong defaultConfigOverwrite in computeTemplates", func() {
		spec := GetDefaultNovaCellSpec(cell1)
		novaCompute := GetDefaultNovaComputeTemplate()
		novaCompute["defaultConfigOverwrite"] = map[string]interface{}{
			"policy.yaml":      "custom policy not supported",
			"provider123.yaml": "provider*.yaml is supported",
		}
		spec["novaComputeTemplates"] = map[string]interface{}{
			ironicComputeName: novaCompute,
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
				"invalid: spec.novaComputeTemplates[ironic-compute].defaultConfigOverwrite: " +
					"Invalid value: \"policy.yaml\": " +
					"Only the following keys are valid: provider*.yaml",
			),
		)
	})
	It("rejects Nova with wrong defaultConfigOverwrite in computeTemplates", func() {
		spec := GetDefaultNovaSpec()
		cell0 := GetDefaultNovaCellTemplate()
		cell1 := GetDefaultNovaCellTemplate()
		novaCompute := GetDefaultNovaComputeTemplate()
		novaCompute["defaultConfigOverwrite"] = map[string]interface{}{
			"policy.yaml":      "custom policy not supported",
			"provider123.yaml": "provider*.yaml is supported",
		}
		cell1["novaComputeTemplates"] = map[string]interface{}{
			ironicComputeName: novaCompute,
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
				"invalid: spec.cellTemplates[cell1]." +
					"novaComputeTemplates[ironic-compute].defaultConfigOverwrite: " +
					"Invalid value: \"policy.yaml\": " +
					"Only the following keys are valid: provider*.yaml",
			),
		)
	})
})
