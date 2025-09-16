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

	. "github.com/onsi/ginkgo/v2" //revive:disable:dot-imports
	. "github.com/onsi/gomega"    //revive:disable:dot-imports

	//revive:disable-next-line:dot-imports
	. "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"

	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/service"
	placementv1 "github.com/openstack-k8s-operators/nova-operator/apis/placement/v1beta1"
)

var _ = Describe("PlacementAPI Webhook", func() {

	var placementAPIName types.NamespacedName

	BeforeEach(func() {

		placementAPIName = types.NamespacedName{
			Name:      "placement",
			Namespace: namespace,
		}

		err := os.Setenv("OPERATOR_TEMPLATES", "../../../templates")
		Expect(err).NotTo(HaveOccurred())
	})

	When("A PlacementAPI instance is created without container images", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreatePlacementAPI(placementAPIName, GetDefaultPlacementAPISpec()))
		})

		It("should have the defaults initialized by webhook", func() {
			PlacementAPI := GetPlacementAPI(placementAPIName)
			Expect(PlacementAPI.Spec.ContainerImage).Should(Equal(
				placementv1.PlacementAPIContainerImage,
			))
		})
	})

	When("A PlacementAPI instance is created with container images", func() {
		BeforeEach(func() {
			placementAPISpec := GetDefaultPlacementAPISpec()
			placementAPISpec["containerImage"] = "api-container-image"
			DeferCleanup(th.DeleteInstance, CreatePlacementAPI(placementAPIName, placementAPISpec))
		})

		It("should use the given values", func() {
			PlacementAPI := GetPlacementAPI(placementAPIName)
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
				"name":      placementAPIName.Name,
				"namespace": placementAPIName.Namespace,
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

	It("rejects with wrong service override endpoint type", func() {
		spec := GetDefaultPlacementAPISpec()
		spec["override"] = map[string]interface{}{
			"service": map[string]interface{}{
				"internal": map[string]interface{}{},
				"wrooong":  map[string]interface{}{},
			},
		}

		raw := map[string]interface{}{
			"apiVersion": "placement.openstack.org/v1beta1",
			"kind":       "PlacementAPI",
			"metadata": map[string]interface{}{
				"name":      placementAPIName.Name,
				"namespace": placementAPIName.Namespace,
			},
			"spec": spec,
		}

		unstructuredObj := &unstructured.Unstructured{Object: raw}
		_, err := controllerutil.CreateOrPatch(
			th.Ctx, th.K8sClient, unstructuredObj, func() error { return nil })
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(
			ContainSubstring(
				"invalid: spec.override.service[wrooong]: " +
					"Invalid value: \"wrooong\": invalid endpoint type: wrooong"),
		)
	})

	When("A PlacementAPI instance is updated with wrong service override endpoint", func() {
		BeforeEach(func() {
			DeferCleanup(k8sClient.Delete, ctx, CreatePlacementAPISecret(namespace, SecretName))
			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(namespace))

			placementAPI := CreatePlacementAPI(names.PlacementAPIName, GetDefaultPlacementAPISpec())
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					namespace,
					GetPlacementAPI(names.PlacementAPIName).Spec.DatabaseInstance,
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)

			mariadb.SimulateMariaDBDatabaseCompleted(names.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(names.MariaDBAccount)
			th.SimulateJobSuccess(names.DBSyncJobName)
			th.SimulateDeploymentReplicaReady(names.DeploymentName)
			keystone.SimulateKeystoneServiceReady(names.KeystoneServiceName)
			keystone.SimulateKeystoneEndpointReady(names.KeystoneEndpointName)
			DeferCleanup(th.DeleteInstance, placementAPI)

			th.ExpectCondition(
				names.PlacementAPIName,
				ConditionGetterFunc(PlacementConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("rejects update with wrong service override endpoint type", func() {
			PlacementAPI := GetPlacementAPI(names.PlacementAPIName)
			Expect(PlacementAPI).NotTo(BeNil())
			if PlacementAPI.Spec.Override.Service == nil {
				PlacementAPI.Spec.Override.Service = map[service.Endpoint]service.RoutedOverrideSpec{}
			}
			PlacementAPI.Spec.Override.Service["wrooong"] = service.RoutedOverrideSpec{}
			err := k8sClient.Update(ctx, PlacementAPI)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(
				ContainSubstring(
					"invalid: spec.override.service[wrooong]: " +
						"Invalid value: \"wrooong\": invalid endpoint type: wrooong"),
			)
		})
	})

	It("rejects a wrong TopologyRef on a different namespace", func() {
		spec := GetDefaultPlacementAPISpec()
		// Inject a topologyRef that points to a different namespace
		spec["topologyRef"] = map[string]interface{}{
			"name":      "foo",
			"namespace": "bar",
		}
		raw := map[string]interface{}{
			"apiVersion": "placement.openstack.org/v1beta1",
			"kind":       "PlacementAPI",
			"metadata": map[string]interface{}{
				"name":      placementAPIName.Name,
				"namespace": placementAPIName.Namespace,
			},
			"spec": spec,
		}
		unstructuredObj := &unstructured.Unstructured{Object: raw}
		_, err := controllerutil.CreateOrPatch(
			th.Ctx, th.K8sClient, unstructuredObj, func() error { return nil })
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(
			ContainSubstring(
				"spec.topologyRef.namespace: Invalid value: \"namespace\": Customizing namespace field is not supported"),
		)
	})
})
