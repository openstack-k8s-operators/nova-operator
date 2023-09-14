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
	"fmt"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
)

var _ = Describe("PlacementAPI controller", func() {

	var placementApiName types.NamespacedName
	var placementApiConfigMapName types.NamespacedName
	var keystoneAPI *keystonev1.KeystoneAPI

	BeforeEach(func() {
		placementApiName = types.NamespacedName{
			Name:      "placement",
			Namespace: namespace,
		}
		placementApiConfigMapName = types.NamespacedName{
			Namespace: namespace,
			Name:      placementApiName.Name + "-config-data",
		}

		// lib-common uses OPERATOR_TEMPLATES env var to locate the "templates"
		// directory of the operator. We need to set them othervise lib-common
		// will fail to generate the ConfigMap as it does not find common.sh
		err := os.Setenv("OPERATOR_TEMPLATES", "../../templates")
		Expect(err).NotTo(HaveOccurred())
	})

	When("A PlacementAPI instance is created", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreatePlacementAPI(placementApiName, GetDefaultPlacementAPISpec()))
		})

		It("should have the Spec fields defaulted", func() {
			Placement := GetPlacementAPI(placementApiName)
			Expect(Placement.Spec.DatabaseInstance).Should(Equal("openstack"))
			Expect(Placement.Spec.DatabaseUser).Should(Equal("placement"))
			Expect(Placement.Spec.ServiceUser).Should(Equal("placement"))
			Expect(*(Placement.Spec.Replicas)).Should(Equal(int32(1)))
		})

		It("should have the Status fields initialized", func() {
			Placement := GetPlacementAPI(placementApiName)
			Expect(Placement.Status.Hash).To(BeEmpty())
			Expect(Placement.Status.DatabaseHostname).To(Equal(""))
			Expect(Placement.Status.ReadyCount).To(Equal(int32(0)))
		})

		It("should have a finalizer", func() {
			// the reconciler loop adds the finalizer so we have to wait for
			// it to run
			Eventually(func() []string {
				return GetPlacementAPI(placementApiName).Finalizers
			}, timeout, interval).Should(ContainElement("PlacementAPI"))
		})

		It("should not create a config map", func() {
			th.AssertConfigMapDoesNotExist(placementApiConfigMapName)
		})

		It("should have input not ready and unknown Conditions initialized", func() {
			th.ExpectCondition(
				placementApiName,
				ConditionGetterFunc(PlacementConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
			th.ExpectCondition(
				placementApiName,
				ConditionGetterFunc(PlacementConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionFalse,
			)
			unknownConditions := []condition.Type{
				condition.DBReadyCondition,
				condition.DBSyncReadyCondition,
				condition.ExposeServiceReadyCondition,
				condition.ServiceConfigReadyCondition,
				condition.DeploymentReadyCondition,
				condition.KeystoneServiceReadyCondition,
				condition.KeystoneEndpointReadyCondition,
				condition.NetworkAttachmentsReadyCondition,
				condition.ServiceAccountReadyCondition,
				condition.RoleReadyCondition,
				condition.RoleBindingReadyCondition,
			}

			placement := GetPlacementAPI(placementApiName)
			// +2 as InputReady and Ready is False asserted above
			Expect(placement.Status.Conditions).To(HaveLen(len(unknownConditions) + 2))

			for _, cond := range unknownConditions {
				th.ExpectCondition(
					placementApiName,
					ConditionGetterFunc(PlacementConditionGetter),
					cond,
					corev1.ConditionUnknown,
				)
			}
		})
	})

	When("the proper secret is provided", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreatePlacementAPI(placementApiName, GetDefaultPlacementAPISpec()))
			DeferCleanup(
				k8sClient.Delete, ctx, CreatePlacementAPISecret(namespace, SecretName))
		})

		It("should have input ready", func() {
			th.ExpectCondition(
				placementApiName,
				ConditionGetterFunc(PlacementConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				placementApiName,
				ConditionGetterFunc(PlacementConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("should not create a config map", func() {
			th.AssertConfigMapDoesNotExist(placementApiConfigMapName)
		})
	})

	When("keystoneAPI instance is available", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreatePlacementAPI(placementApiName, GetDefaultPlacementAPISpec()))
			DeferCleanup(
				k8sClient.Delete, ctx, CreatePlacementAPISecret(namespace, SecretName))
			keystoneAPIName := th.CreateKeystoneAPI(namespace)
			keystoneAPI = th.GetKeystoneAPI(keystoneAPIName)
			DeferCleanup(th.DeleteKeystoneAPI, keystoneAPIName)
		})

		It("should have input ready", func() {
			th.ExpectCondition(
				placementApiName,
				ConditionGetterFunc(PlacementConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("should create a ConfigMap for placement.conf", func() {
			cm := th.GetConfigMap(placementApiConfigMapName)

			Expect(cm.Data["placement.conf"]).Should(
				ContainSubstring("auth_url = %s", keystoneAPI.Status.APIEndpoints["internal"]))
			Expect(cm.Data["placement.conf"]).Should(
				ContainSubstring("www_authenticate_uri = %s", keystoneAPI.Status.APIEndpoints["public"]))
		})
	})

	When("A PlacementAPI is created with service override", func() {
		BeforeEach(func() {
			DeferCleanup(k8sClient.Delete, ctx, CreatePlacementAPISecret(namespace, SecretName))
			DeferCleanup(th.DeleteKeystoneAPI, th.CreateKeystoneAPI(placementApiName.Namespace))

			spec := GetDefaultPlacementAPISpec()
			serviceOverride := map[string]interface{}{}
			serviceOverride["internal"] = map[string]interface{}{
				"metadata": map[string]map[string]string{
					"annotations": {
						"dnsmasq.network.openstack.org/hostname": "placement-internal.openstack.svc",
						"metallb.universe.tf/address-pool":       "osp-internalapi",
						"metallb.universe.tf/allow-shared-ip":    "osp-internalapi",
						"metallb.universe.tf/loadBalancerIPs":    "internal-lb-ip-1,internal-lb-ip-2",
					},
					"labels": {
						"internal": "true",
						"service":  "placement",
					},
				},
				"spec": map[string]interface{}{
					"type": "LoadBalancer",
				},
			}

			spec["override"] = map[string]interface{}{
				"service": serviceOverride,
			}

			placementAPI := CreatePlacementAPI(placementApiName, spec)
			DeferCleanup(
				th.DeleteDBService,
				th.CreateDBService(
					placementApiName.Namespace,
					GetPlacementAPI(placementApiName).Spec.DatabaseInstance,
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)

			th.SimulateMariaDBDatabaseCompleted(placementApiName)
			th.SimulateJobSuccess(types.NamespacedName{
				Namespace: placementApiName.Namespace,
				Name:      fmt.Sprintf("%s-db-sync", placementApiName.Name),
			})
			th.SimulateDeploymentReplicaReady(placementApiName)
			th.SimulateKeystoneServiceReady(placementApiName)
			th.SimulateKeystoneEndpointReady(placementApiName)
			DeferCleanup(th.DeleteInstance, placementAPI)
		})

		It("creates KeystoneEndpoint", func() {
			keystoneEndpoint := th.GetKeystoneEndpoint(placementApiName)
			endpoints := keystoneEndpoint.Spec.Endpoints
			Expect(endpoints).To(HaveKeyWithValue("public", "http://placement-public."+placementApiName.Namespace+".svc:8778"))
			Expect(endpoints).To(HaveKeyWithValue("internal", "http://placement-internal."+placementApiName.Namespace+".svc:8778"))

			th.ExpectCondition(
				placementApiName,
				ConditionGetterFunc(PlacementConditionGetter),
				condition.KeystoneEndpointReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("creates LoadBalancer service", func() {
			// As the internal endpoint is configured in ExternalEndpoints it
			// gets a LoadBalancer Service with MetalLB annotations
			service := th.GetService(types.NamespacedName{Namespace: namespace, Name: "placement-internal"})
			Expect(service.Annotations).To(
				HaveKeyWithValue("dnsmasq.network.openstack.org/hostname", "placement-internal.openstack.svc"))
			Expect(service.Annotations).To(
				HaveKeyWithValue("metallb.universe.tf/address-pool", "osp-internalapi"))
			Expect(service.Annotations).To(
				HaveKeyWithValue("metallb.universe.tf/allow-shared-ip", "osp-internalapi"))
			Expect(service.Annotations).To(
				HaveKeyWithValue("metallb.universe.tf/loadBalancerIPs", "internal-lb-ip-1,internal-lb-ip-2"))

			th.ExpectCondition(
				placementApiName,
				ConditionGetterFunc(PlacementConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})

	When("A PlacementAPI is created with service override endpointURL set", func() {
		BeforeEach(func() {
			DeferCleanup(k8sClient.Delete, ctx, CreatePlacementAPISecret(namespace, SecretName))
			DeferCleanup(th.DeleteKeystoneAPI, th.CreateKeystoneAPI(placementApiName.Namespace))

			spec := GetDefaultPlacementAPISpec()
			serviceOverride := map[string]interface{}{}
			serviceOverride["public"] = map[string]interface{}{
				"endpointURL": "http://placement-openstack.apps-crc.testing",
			}

			spec["override"] = map[string]interface{}{
				"service": serviceOverride,
			}

			placementAPI := CreatePlacementAPI(placementApiName, spec)
			DeferCleanup(
				th.DeleteDBService,
				th.CreateDBService(
					placementApiName.Namespace,
					GetPlacementAPI(placementApiName).Spec.DatabaseInstance,
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)

			th.SimulateMariaDBDatabaseCompleted(placementApiName)
			th.SimulateJobSuccess(types.NamespacedName{
				Namespace: placementApiName.Namespace,
				Name:      fmt.Sprintf("%s-db-sync", placementApiName.Name),
			})
			th.SimulateDeploymentReplicaReady(placementApiName)
			th.SimulateKeystoneServiceReady(placementApiName)
			th.SimulateKeystoneEndpointReady(placementApiName)
			DeferCleanup(th.DeleteInstance, placementAPI)
		})

		It("creates KeystoneEndpoint", func() {
			keystoneEndpoint := th.GetKeystoneEndpoint(placementApiName)
			endpoints := keystoneEndpoint.Spec.Endpoints
			Expect(endpoints).To(HaveKeyWithValue("public", "http://placement-openstack.apps-crc.testing"))
			Expect(endpoints).To(HaveKeyWithValue("internal", "http://placement-internal."+placementApiName.Namespace+".svc:8778"))

			th.ExpectCondition(
				placementApiName,
				ConditionGetterFunc(PlacementConditionGetter),
				condition.KeystoneEndpointReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})
})
