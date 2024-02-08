/*
Copyright 2023.

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
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	placementv1 "github.com/openstack-k8s-operators/placement-operator/api/v1beta1"
)

var _ = Describe("PlacementAPI Webhook", func() {

	var placementApiName types.NamespacedName

	BeforeEach(func() {

		placementApiName = types.NamespacedName{
			Name:      "placement",
			Namespace: namespace,
		}

		err := os.Setenv("OPERATOR_TEMPLATES", "../../templates")
		Expect(err).NotTo(HaveOccurred())
	})

	When("A PlacementAPI instance is created without container images", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreatePlacementAPI(placementApiName, GetDefaultPlacementAPISpec()))
		})

		It("should have the defaults initialized by webhook", func() {
			PlacementAPI := GetPlacementAPI(placementApiName)
			Expect(PlacementAPI.Spec.ContainerImage).Should(Equal(
				placementv1.PlacementAPIContainerImage,
			))
		})
	})

	When("A PlacementAPI instance is created with container images", func() {
		BeforeEach(func() {
			placementApiSpec := GetDefaultPlacementAPISpec()
			placementApiSpec["containerImage"] = "api-container-image"
			DeferCleanup(th.DeleteInstance, CreatePlacementAPI(placementApiName, placementApiSpec))
		})

		It("should use the given values", func() {
			PlacementAPI := GetPlacementAPI(placementApiName)
			Expect(PlacementAPI.Spec.ContainerImage).Should(Equal(
				"api-container-image",
			))
		})
	})

	It("rejects PlacementAPI with wrong defaultConfigOverwrite", func() {
		spec := GetDefaultPlacementAPISpec()
		spec["defaultConfigOverwrite"] = map[string]interface{}{
			"policy.yaml":   "support",
			"api-paste.ini": "not supported",
		}
		raw := map[string]interface{}{
			"apiVersion": "placement.openstack.org/v1beta1",
			"kind":       "PlacementAPI",
			"metadata": map[string]interface{}{
				"name":      placementApiName.Name,
				"namespace": placementApiName.Namespace,
			},
			"spec": spec,
		}
		unstructuredObj := &unstructured.Unstructured{Object: raw}
		_, err := controllerutil.CreateOrPatch(
			ctx, k8sClient, unstructuredObj, func() error { return nil })

		Expect(err).Should(HaveOccurred())
		var statusError *k8s_errors.StatusError
		Expect(errors.As(err, &statusError)).To(BeTrue())
		Expect(statusError.ErrStatus.Details.Kind).To(Equal("PlacementAPI"))
		Expect(statusError.ErrStatus.Message).To(
			ContainSubstring(
				"invalid: spec.defaultConfigOverwrite: " +
					"Invalid value: \"api-paste.ini\": " +
					"Only the following keys are valid: policy.yaml",
			),
		)
	})

})
