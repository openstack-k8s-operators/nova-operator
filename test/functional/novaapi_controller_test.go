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
	"encoding/json"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"

	networkv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
)

var _ = Describe("NovaAPI controller", func() {
	When("a NovaAPI CR is created pointing to a non existent Secret", func() {
		BeforeEach(func() {
			spec := GetDefaultNovaAPISpec(novaNames)
			spec["customServiceConfig"] = "foo=bar"
			DeferCleanup(th.DeleteInstance, CreateNovaAPI(novaNames.APIName, spec))
		})

		It("is not Ready", func() {
			th.ExpectCondition(
				novaNames.APIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("has empty Status fields", func() {
			instance := GetNovaAPI(novaNames.APIName)
			// NOTE(gibi): Hash and Endpoints have `omitempty` tags so while
			// they are initialized to {} that value is omitted from the output
			// when sent to the client. So we see nils here.
			Expect(instance.Status.Hash).To(BeEmpty())
			Expect(instance.Status.ReadyCount).To(Equal(int32(0)))
		})

		It("is missing the secret", func() {
			th.ExpectConditionWithDetails(
				novaNames.APIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				fmt.Sprintf("Input data resources missing: secret/%s", novaNames.InternalTopLevelSecretName.Name),
			)
		})

		When("an unrelated Secret is created the CR state does not change", func() {
			BeforeEach(func() {
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "not-relevant-secret",
						Namespace: novaNames.APIName.Namespace,
					},
				}
				Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
				DeferCleanup(k8sClient.Delete, ctx, secret)
			})

			It("is not Ready", func() {
				th.ExpectCondition(
					novaNames.APIName,
					ConditionGetterFunc(NovaAPIConditionGetter),
					condition.ReadyCondition,
					corev1.ConditionFalse,
				)
			})

			It("is missing the secret", func() {
				th.ExpectConditionWithDetails(
					novaNames.APIName,
					ConditionGetterFunc(NovaAPIConditionGetter),
					condition.InputReadyCondition,
					corev1.ConditionFalse,
					condition.RequestedReason,
					fmt.Sprintf("Input data resources missing: secret/%s", novaNames.InternalTopLevelSecretName.Name),
				)
			})
		})

		When("the Secret is created but some fields are missing", func() {
			BeforeEach(func() {
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      novaNames.InternalTopLevelSecretName.Name,
						Namespace: novaNames.InternalTopLevelSecretName.Namespace,
					},
					Data: map[string][]byte{
						"ServicePassword": []byte("12345678"),
					},
				}
				Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
				DeferCleanup(k8sClient.Delete, ctx, secret)
			})

			It("is not Ready", func() {
				th.ExpectCondition(
					novaNames.APIName,
					ConditionGetterFunc(NovaAPIConditionGetter),
					condition.ReadyCondition,
					corev1.ConditionFalse,
				)
			})

			It("reports that the inputs are not ready", func() {
				th.ExpectCondition(
					novaNames.APIName,
					ConditionGetterFunc(NovaAPIConditionGetter),
					condition.InputReadyCondition,
					corev1.ConditionFalse,
				)
			})
		})

		When("the Secret is created with all the expected fields", func() {
			BeforeEach(func() {
				DeferCleanup(
					k8sClient.Delete, ctx, CreateInternalTopLevelSecret(novaNames))
			})

			It("reports that input is ready", func() {
				th.ExpectCondition(
					novaNames.APIName,
					ConditionGetterFunc(NovaAPIConditionGetter),
					condition.InputReadyCondition,
					corev1.ConditionTrue,
				)
			})

			It("generated configs successfully", func() {
				// NOTE(gibi): NovaAPI has no external dependency right now to
				// generate the configs.
				th.ExpectCondition(
					novaNames.APIName,
					ConditionGetterFunc(NovaAPIConditionGetter),
					condition.ServiceConfigReadyCondition,
					corev1.ConditionTrue,
				)

				configDataMap := th.GetSecret(novaNames.APIConfigDataName)
				Expect(configDataMap).ShouldNot(BeNil())
				Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
				configData := string(configDataMap.Data["01-nova.conf"])
				Expect(configData).Should(ContainSubstring("transport_url=rabbit://api/fake"))
				// as of I3629b84d3255a8fe9d8a7cea8c6131d7c40899e8 nova now requires
				// service_user configuration to work to address Bug: #2004555
				Expect(configData).Should(ContainSubstring("[service_user]"))
				Expect(configData).Should(ContainSubstring("password = service-password"))
				// as part of additional hardening we now require service_token_roles_required
				// to be set to true to ensure that the service token is not just a user token
				// nova does not currently rely on the service token for enforcement of elevated
				// privileges but this is a good practice to follow and might be required in the
				// future
				Expect(configData).Should(ContainSubstring("service_token_roles_required = true"))
				Expect(configData).Should(ContainSubstring("enabled_apis=osapi_compute"))
				Expect(configData).Should(ContainSubstring("osapi_compute_workers=1"))
				Expect(configData).Should(ContainSubstring("auth_url = keystone-internal-auth-url"))
				Expect(configData).Should(ContainSubstring("www_authenticate_uri = keystone-public-auth-url"))
				Expect(configData).Should(
					ContainSubstring("[upgrade_levels]\ncompute = auto"))
				Expect(configData).Should(ContainSubstring("enforce_new_defaults=true"))
				Expect(configData).Should(ContainSubstring("enforce_scope=true"))
				// test config override
				Expect(configDataMap.Data).Should(HaveKey("02-nova-override.conf"))
				extraData := string(configDataMap.Data["02-nova-override.conf"])
				Expect(extraData).To(Equal("foo=bar"))
			})

			It("stored the input hash in the Status", func() {
				Eventually(func(g Gomega) {
					novaAPI := GetNovaAPI(novaNames.APIName)
					g.Expect(novaAPI.Status.Hash).Should(HaveKeyWithValue("input", Not(BeEmpty())))
				}, timeout, interval).Should(Succeed())

			})
		})
	})

	When("NovAPI is created with a proper Secret", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateInternalTopLevelSecret(novaNames))
			DeferCleanup(th.DeleteInstance, CreateNovaAPI(novaNames.APIName, GetDefaultNovaAPISpec(novaNames)))
		})

		It("reports input ready", func() {
			th.ExpectCondition(
				novaNames.APIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})

	When("NovAPI is created", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateInternalTopLevelSecret(novaNames))
			DeferCleanup(th.DeleteInstance, CreateNovaAPI(novaNames.APIName, GetDefaultNovaAPISpec(novaNames)))

			th.ExpectCondition(
				novaNames.APIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("creates a StatefulSet for the nova-api service", func() {
			th.ExpectConditionWithDetails(
				novaNames.APIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				condition.DeploymentReadyRunningMessage,
			)

			ss := th.GetStatefulSet(novaNames.APIStatefulSetName)
			Expect(ss.Spec.Template.Spec.ServiceAccountName).To(Equal("nova-sa"))
			Expect(int(*ss.Spec.Replicas)).To(Equal(1))
			Expect(ss.Spec.Template.Spec.Volumes).To(HaveLen(2))
			Expect(ss.Spec.Template.Spec.Containers).To(HaveLen(2))
			Expect(ss.Spec.Selector.MatchLabels).To(Equal(map[string]string{"service": "nova-api"}))

			container := ss.Spec.Template.Spec.Containers[0]
			Expect(container.VolumeMounts).To(HaveLen(1))
			Expect(container.Image).To(Equal(ContainerImage))

			container = ss.Spec.Template.Spec.Containers[1]
			Expect(container.VolumeMounts).To(HaveLen(3))
			Expect(container.Image).To(Equal(ContainerImage))

			Expect(container.LivenessProbe.HTTPGet.Port.IntVal).To(Equal(int32(8774)))
			Expect(container.ReadinessProbe.HTTPGet.Port.IntVal).To(Equal(int32(8774)))

		})

		When("the StatefulSet has at least one Replica ready", func() {
			BeforeEach(func() {
				th.ExpectConditionWithDetails(
					novaNames.APIName,
					ConditionGetterFunc(NovaAPIConditionGetter),
					condition.DeploymentReadyCondition,
					corev1.ConditionFalse,
					condition.RequestedReason,
					condition.DeploymentReadyRunningMessage,
				)
				th.SimulateStatefulSetReplicaReady(novaNames.APIStatefulSetName)
			})

			It("reports that the StatefulSet is ready", func() {
				th.ExpectCondition(
					novaNames.APIName,
					ConditionGetterFunc(NovaAPIConditionGetter),
					condition.DeploymentReadyCondition,
					corev1.ConditionTrue,
				)

				novaAPI := GetNovaAPI(novaNames.APIName)
				Expect(novaAPI.Status.ReadyCount).To(BeNumerically(">", 0))
			})
		})

		It("exposes the service", func() {
			th.SimulateStatefulSetReplicaReady(novaNames.APIStatefulSetName)
			th.ExpectCondition(
				novaNames.APIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.ExposeServiceReadyCondition,
				corev1.ConditionTrue,
			)
			public := th.GetService(novaNames.PublicNovaServiceName)
			Expect(public.Labels["service"]).To(Equal("nova-api"))
			internal := th.GetService(novaNames.InternalNovaServiceName)
			Expect(internal.Labels["service"]).To(Equal("nova-api"))
		})

		It("creates KeystoneEndpoint", func() {
			th.SimulateStatefulSetReplicaReady(novaNames.APIStatefulSetName)
			keystone.SimulateKeystoneEndpointReady(types.NamespacedName{Namespace: novaNames.APIName.Namespace, Name: "nova"})

			keystoneEndpoint := keystone.GetKeystoneEndpoint(types.NamespacedName{Namespace: novaNames.APIName.Namespace, Name: "nova"})
			endpoints := keystoneEndpoint.Spec.Endpoints
			Expect(endpoints).To(HaveKeyWithValue("public", "http://nova-public."+novaNames.APIName.Namespace+".svc:8774/v2.1"))
			Expect(endpoints).To(HaveKeyWithValue("internal", "http://nova-internal."+novaNames.APIName.Namespace+".svc:8774/v2.1"))

			th.ExpectCondition(
				novaNames.APIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.KeystoneEndpointReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("is Ready", func() {
			th.SimulateStatefulSetReplicaReady(novaNames.APIStatefulSetName)
			keystone.SimulateKeystoneEndpointReady(types.NamespacedName{Namespace: novaNames.APIName.Namespace, Name: "nova"})

			th.ExpectCondition(
				novaNames.APIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})
	When("NovaAPI CR instance is deleted", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateInternalTopLevelSecret(novaNames))
			DeferCleanup(th.DeleteInstance, CreateNovaAPI(novaNames.APIName, GetDefaultNovaAPISpec(novaNames)))
		})

		It("removes the finalizer from KeystoneEndpoint", func() {
			th.SimulateStatefulSetReplicaReady(novaNames.APIStatefulSetName)
			keystone.SimulateKeystoneEndpointReady(novaNames.APIKeystoneEndpointName)
			th.ExpectCondition(
				novaNames.APIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)

			endpoint := keystone.GetKeystoneEndpoint(novaNames.APIKeystoneEndpointName)
			Expect(endpoint.Finalizers).To(ContainElement("NovaAPI"))

			th.DeleteInstance(GetNovaAPI(novaNames.APIName))
			endpoint = keystone.GetKeystoneEndpoint(novaNames.APIKeystoneEndpointName)
			Expect(endpoint.Finalizers).NotTo(ContainElement("NovaAPI"))
		})
	})
})

var _ = Describe("NovaAPI controller", func() {
	When("NovaAPI is created with networkAttachments", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateInternalTopLevelSecret(novaNames))

			spec := GetDefaultNovaAPISpec(novaNames)
			spec["networkAttachments"] = []string{"internalapi"}
			DeferCleanup(th.DeleteInstance, CreateNovaAPI(novaNames.APIName, spec))
		})

		It("reports that the definition is missing", func() {
			th.ExpectConditionWithDetails(
				novaNames.APIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NetworkAttachment resources missing: internalapi",
			)
			th.ExpectCondition(
				novaNames.APIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})
		It("reports that network attachment is missing", func() {
			nad := th.CreateNetworkAttachmentDefinition(novaNames.InternalAPINetworkNADName)
			DeferCleanup(th.DeleteInstance, nad)

			ss := th.GetStatefulSet(novaNames.APIStatefulSetName)

			expectedAnnotation, err := json.Marshal(
				[]networkv1.NetworkSelectionElement{
					{
						Name:             "internalapi",
						Namespace:        novaNames.APIName.Namespace,
						InterfaceRequest: "internalapi",
					}})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(ss.Spec.Template.ObjectMeta.Annotations).To(
				HaveKeyWithValue("k8s.v1.cni.cncf.io/networks", string(expectedAnnotation)),
			)

			// We don't add network attachment status annotations to the Pods
			// to simulate that the network attachments are missing.
			th.SimulateStatefulSetReplicaReadyWithPods(novaNames.APIStatefulSetName, map[string][]string{})

			th.ExpectConditionWithDetails(
				novaNames.APIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occurred "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
			)
		})
		It("reports that an IP is missing", func() {
			nad := th.CreateNetworkAttachmentDefinition(novaNames.InternalAPINetworkNADName)
			DeferCleanup(th.DeleteInstance, nad)

			ss := th.GetStatefulSet(novaNames.APIStatefulSetName)

			expectedAnnotation, err := json.Marshal(
				[]networkv1.NetworkSelectionElement{
					{
						Name:             "internalapi",
						Namespace:        novaNames.APIName.Namespace,
						InterfaceRequest: "internalapi",
					}})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(ss.Spec.Template.ObjectMeta.Annotations).To(
				HaveKeyWithValue("k8s.v1.cni.cncf.io/networks", string(expectedAnnotation)),
			)

			// We simulate that there is no IP associated with the internalapi
			// network attachment
			th.SimulateStatefulSetReplicaReadyWithPods(
				novaNames.APIStatefulSetName,
				map[string][]string{novaNames.APIName.Namespace + "/internalapi": {}},
			)

			th.ExpectConditionWithDetails(
				novaNames.APIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occurred "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
			)
		})
		It("reports NetworkAttachmentsReady if the Pods got the proper annotations", func() {
			nad := th.CreateNetworkAttachmentDefinition(novaNames.InternalAPINetworkNADName)
			DeferCleanup(th.DeleteInstance, nad)

			th.SimulateStatefulSetReplicaReadyWithPods(
				novaNames.APIStatefulSetName,
				map[string][]string{novaNames.APIName.Namespace + "/internalapi": {"10.0.0.1"}},
			)

			th.ExpectCondition(
				novaNames.APIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionTrue,
			)

			Eventually(func(g Gomega) {
				novaAPI := GetNovaAPI(novaNames.APIName)
				g.Expect(novaAPI.Status.NetworkAttachments).To(
					Equal(map[string][]string{novaNames.APIName.Namespace + "/internalapi": {"10.0.0.1"}}))

			}, timeout, interval).Should(Succeed())

			keystone.SimulateKeystoneEndpointReady(novaNames.APIKeystoneEndpointName)

			th.ExpectCondition(
				novaNames.APIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})

	When("NovaAPI is created with service override with endpointURL set", func() {
		BeforeEach(func() {
			spec := GetDefaultNovaAPISpec(novaNames)
			serviceOverride := map[string]interface{}{}
			serviceOverride["internal"] = map[string]interface{}{
				"metadata": map[string]map[string]string{
					"annotations": {
						"dnsmasq.network.openstack.org/hostname": "nova-internal.openstack.svc",
						"metallb.universe.tf/address-pool":       "osp-internalapi",
						"metallb.universe.tf/allow-shared-ip":    "osp-internalapi",
						"metallb.universe.tf/loadBalancerIPs":    "internal-lb-ip-1,internal-lb-ip-2",
					},
					"labels": {
						"internal": "true",
						"service":  "nova",
					},
				},
				"spec": map[string]interface{}{
					"type": "LoadBalancer",
				},
			}
			serviceOverride["public"] = map[string]interface{}{
				"endpointURL": "http://nova-api-" + novaNames.APIName.Namespace + ".apps-crc.testing",
				"metadata": map[string]map[string]string{
					"labels": {
						"service": "nova-api",
					},
				},
			}

			spec["override"] = map[string]interface{}{
				"service": serviceOverride,
			}

			DeferCleanup(
				k8sClient.Delete, ctx, CreateInternalTopLevelSecret(novaNames))
			DeferCleanup(th.DeleteInstance, CreateNovaAPI(novaNames.APIName, spec))
		})

		It("creates LoadBalancer services", func() {
			th.SimulateStatefulSetReplicaReady(novaNames.APIStatefulSetName)

			keystone.SimulateKeystoneEndpointReady(novaNames.APIKeystoneEndpointName)

			// As the internal endpoint has service override configured it
			// gets a LoadBalancer Service with MetalLB annotations
			service := th.GetService(novaNames.InternalNovaServiceName)
			Expect(service.Annotations).To(
				HaveKeyWithValue("dnsmasq.network.openstack.org/hostname", "nova-internal.openstack.svc"))
			Expect(service.Annotations).To(
				HaveKeyWithValue("metallb.universe.tf/address-pool", "osp-internalapi"))
			Expect(service.Annotations).To(
				HaveKeyWithValue("metallb.universe.tf/allow-shared-ip", "osp-internalapi"))
			Expect(service.Annotations).To(
				HaveKeyWithValue("metallb.universe.tf/loadBalancerIPs", "internal-lb-ip-1,internal-lb-ip-2"))

			// As the public endpoint does not have overrides, a generic Service
			// is created with annotations to create a route
			service = th.GetService(novaNames.PublicNovaServiceName)
			Expect(service.Annotations).NotTo(HaveKey("metallb.universe.tf/address-pool"))
			Expect(service.Annotations).NotTo(HaveKey("metallb.universe.tf/allow-shared-ip"))
			Expect(service.Annotations).NotTo(HaveKey("metallb.universe.tf/loadBalancerIPs"))
			Expect(service.Annotations).To(
				HaveKeyWithValue("core.openstack.org/ingress_create", "true"))
			Expect(service.Labels).To(
				HaveKeyWithValue("service", "nova-api"))
			Expect(service.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))

			th.SimulateStatefulSetReplicaReady(novaNames.APIStatefulSetName)
			keystone.SimulateKeystoneEndpointReady(types.NamespacedName{Namespace: novaNames.APIName.Namespace, Name: "nova"})

			// it registers the endpointURL as the public endpoint and svc
			// for the internal
			keystoneEndpoint := keystone.GetKeystoneEndpoint(types.NamespacedName{Namespace: novaNames.APIName.Namespace, Name: "nova"})
			endpoints := keystoneEndpoint.Spec.Endpoints
			Expect(endpoints).To(HaveKeyWithValue("public", "http://nova-api-"+novaNames.APIName.Namespace+".apps-crc.testing/v2.1"))
			Expect(endpoints).To(HaveKeyWithValue("internal", "http://nova-internal."+novaNames.APIName.Namespace+".svc:8774/v2.1"))

			th.ExpectCondition(
				novaNames.APIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})

	When("NovaAPI is created with service override with no endpointURL set", func() {
		BeforeEach(func() {
			spec := GetDefaultNovaAPISpec(novaNames)
			serviceOverride := map[string]interface{}{}
			serviceOverride["internal"] = map[string]interface{}{
				"metadata": map[string]map[string]string{
					"annotations": {
						"dnsmasq.network.openstack.org/hostname": "nova-internal.openstack.svc",
						"metallb.universe.tf/address-pool":       "osp-internalapi",
						"metallb.universe.tf/allow-shared-ip":    "osp-internalapi",
						"metallb.universe.tf/loadBalancerIPs":    "internal-lb-ip-1,internal-lb-ip-2",
					},
					"labels": {
						"internal": "true",
						"service":  "nova",
					},
				},
				"spec": map[string]interface{}{
					"type": "LoadBalancer",
				},
			}
			serviceOverride["public"] = map[string]interface{}{
				"metadata": map[string]map[string]string{
					"annotations": {
						"dnsmasq.network.openstack.org/hostname": "nova-public.openstack.svc",
						"metallb.universe.tf/address-pool":       "osp-internalapi",
						"metallb.universe.tf/allow-shared-ip":    "osp-internalapi",
						"metallb.universe.tf/loadBalancerIPs":    "internal-lb-ip-1,internal-lb-ip-2",
					},
					"labels": {
						"service": "nova-api",
					},
				},
				"spec": map[string]interface{}{
					"type": "LoadBalancer",
				},
			}

			spec["override"] = map[string]interface{}{
				"service": serviceOverride,
			}

			DeferCleanup(
				k8sClient.Delete, ctx, CreateInternalTopLevelSecret(novaNames))
			DeferCleanup(th.DeleteInstance, CreateNovaAPI(novaNames.APIName, spec))
		})

		It("creates LoadBalancer services", func() {
			th.SimulateStatefulSetReplicaReady(novaNames.APIStatefulSetName)

			keystone.SimulateKeystoneEndpointReady(novaNames.APIKeystoneEndpointName)

			// As the internal endpoint has service override configured it
			// gets a LoadBalancer Service with MetalLB annotations
			service := th.GetService(novaNames.InternalNovaServiceName)
			Expect(service.Annotations).To(
				HaveKeyWithValue("dnsmasq.network.openstack.org/hostname", "nova-internal.openstack.svc"))
			Expect(service.Annotations).To(
				HaveKeyWithValue("metallb.universe.tf/address-pool", "osp-internalapi"))
			Expect(service.Annotations).To(
				HaveKeyWithValue("metallb.universe.tf/allow-shared-ip", "osp-internalapi"))
			Expect(service.Annotations).To(
				HaveKeyWithValue("metallb.universe.tf/loadBalancerIPs", "internal-lb-ip-1,internal-lb-ip-2"))

			// As the public endpoint has service override configured it
			// gets a LoadBalancer Service with MetalLB annotations
			service = th.GetService(novaNames.PublicNovaServiceName)
			Expect(service.Annotations).To(
				HaveKeyWithValue("dnsmasq.network.openstack.org/hostname", "nova-public.openstack.svc"))
			Expect(service.Annotations).To(
				HaveKeyWithValue("metallb.universe.tf/address-pool", "osp-internalapi"))
			Expect(service.Annotations).To(
				HaveKeyWithValue("metallb.universe.tf/allow-shared-ip", "osp-internalapi"))
			Expect(service.Annotations).To(
				HaveKeyWithValue("metallb.universe.tf/loadBalancerIPs", "internal-lb-ip-1,internal-lb-ip-2"))
			Expect(service.Annotations).To(
				HaveKeyWithValue("core.openstack.org/ingress_create", "false"))
			Expect(service.Spec.Type).To(Equal(corev1.ServiceTypeLoadBalancer))

			th.SimulateStatefulSetReplicaReady(novaNames.APIStatefulSetName)
			keystone.SimulateKeystoneEndpointReady(types.NamespacedName{Namespace: novaNames.APIName.Namespace, Name: "nova"})

			// it registers the endpointURL as the public endpoint and svc
			// for the internal
			keystoneEndpoint := keystone.GetKeystoneEndpoint(types.NamespacedName{Namespace: novaNames.APIName.Namespace, Name: "nova"})
			endpoints := keystoneEndpoint.Spec.Endpoints
			Expect(endpoints).To(HaveKeyWithValue("public", "http://nova-public."+novaNames.APIName.Namespace+".svc:8774/v2.1"))
			Expect(endpoints).To(HaveKeyWithValue("internal", "http://nova-internal."+novaNames.APIName.Namespace+".svc:8774/v2.1"))

			th.ExpectCondition(
				novaNames.APIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})

	When("NovAPI is reconfigured", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateInternalTopLevelSecret(novaNames))
			DeferCleanup(th.DeleteInstance, CreateNovaAPI(novaNames.APIName, GetDefaultNovaAPISpec(novaNames)))

			th.ExpectCondition(
				novaNames.APIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)

			th.SimulateStatefulSetReplicaReady(novaNames.APIStatefulSetName)
			keystone.SimulateKeystoneEndpointReady(novaNames.APIKeystoneEndpointName)
			th.ExpectCondition(
				novaNames.APIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("applies new NetworkAttachments configuration", func() {
			Eventually(func(g Gomega) {
				novaAPI := GetNovaAPI(novaNames.APIName)
				novaAPI.Spec.NetworkAttachments = append(novaAPI.Spec.NetworkAttachments, "internalapi")

				g.Expect(k8sClient.Update(ctx, novaAPI)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			th.ExpectConditionWithDetails(
				novaNames.APIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NetworkAttachment resources missing: internalapi",
			)
			th.ExpectConditionWithDetails(
				novaNames.APIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NetworkAttachment resources missing: internalapi",
			)

			DeferCleanup(th.DeleteInstance, th.CreateNetworkAttachmentDefinition(novaNames.InternalAPINetworkNADName))

			th.ExpectConditionWithDetails(
				novaNames.APIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occurred "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
			)
			th.ExpectConditionWithDetails(
				novaNames.APIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occurred "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
			)

			th.SimulateStatefulSetReplicaReadyWithPods(
				novaNames.APIStatefulSetName,
				map[string][]string{novaNames.APIName.Namespace + "/internalapi": {"10.0.0.1"}},
			)

			th.ExpectCondition(
				novaNames.APIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionTrue,
			)

			Eventually(func(g Gomega) {
				novaAPI := GetNovaAPI(novaNames.APIName)
				g.Expect(novaAPI.Status.NetworkAttachments).To(
					Equal(map[string][]string{novaNames.APIName.Namespace + "/internalapi": {"10.0.0.1"}}))

			}, timeout, interval).Should(Succeed())

			th.ExpectCondition(
				novaNames.APIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("applies new RegisteredCells input to its StatefulSet to trigger Pod restart", func() {
			originalConfigHash := GetEnvVarValue(
				th.GetStatefulSet(novaNames.APIStatefulSetName).Spec.Template.Spec.Containers[0].Env, "CONFIG_HASH", "")

			// Simulate that a new cell is added and Nova controller registered it and
			// therefore a new cell is added to RegisteredCells
			Eventually(func(g Gomega) {
				novaAPI := GetNovaAPI(novaNames.APIName)
				novaAPI.Spec.RegisteredCells = map[string]string{"cell0": "cell0-config-hash"}
				g.Expect(k8sClient.Update(ctx, novaAPI)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			// Assert that the CONFIG_HASH of the StateFulSet is changed due to this reconfiguration
			Eventually(func(g Gomega) {
				currentConfigHash := GetEnvVarValue(
					th.GetStatefulSet(novaNames.APIStatefulSetName).Spec.Template.Spec.Containers[0].Env, "CONFIG_HASH", "")
				g.Expect(originalConfigHash).NotTo(Equal(currentConfigHash))

			}, timeout, interval).Should(Succeed())
		})
	})
})

var _ = Describe("NovaAPI controller", func() {
	When("NovaAPI CR is created without container image defined", func() {
		BeforeEach(func() {
			spec := GetDefaultNovaAPISpec(novaNames)
			spec["containerImage"] = ""
			api := CreateNovaAPI(novaNames.APIName, spec)
			DeferCleanup(th.DeleteInstance, api)
		})
		It("has the expected container image default", func() {
			novaApiDefault := GetNovaAPI(novaNames.APIName)
			Expect(novaApiDefault.Spec.ContainerImage).To(Equal(util.GetEnvVar("RELATED_IMAGE_NOVA_API_IMAGE_URL_DEFAULT", novav1.NovaAPIContainerImage)))
		})
	})
})

var _ = Describe("NovaAPI controller", func() {
	When("NovaAPI is created with TLS cert secrets", func() {
		BeforeEach(func() {
			spec := GetDefaultNovaAPISpec(novaNames)
			spec["tls"] = map[string]interface{}{
				"api": map[string]interface{}{
					"internal": map[string]interface{}{
						"secretName": novaNames.InternalCertSecretName.Name,
					},
					"public": map[string]interface{}{
						"secretName": novaNames.PublicCertSecretName.Name,
					},
				},
				"caBundleSecretName": novaNames.CaBundleSecretName.Name,
			}

			DeferCleanup(
				k8sClient.Delete, ctx, CreateInternalTopLevelSecret(novaNames))
			DeferCleanup(th.DeleteInstance, CreateNovaAPI(novaNames.APIName, spec))
		})

		It("reports that the CA secret is missing", func() {
			th.ExpectConditionWithDetails(
				novaNames.APIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				fmt.Sprintf("TLSInput error occured in TLS sources Secret %s/combined-ca-bundle not found", novaNames.Namespace),
			)
			th.ExpectCondition(
				novaNames.APIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("reports that the internal cert secret is missing", func() {
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCABundleSecret(novaNames.CaBundleSecretName))

			th.ExpectConditionWithDetails(
				novaNames.APIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				fmt.Sprintf("TLSInput error occured in TLS sources Secret %s/internal-tls-certs not found", novaNames.Namespace),
			)
			th.ExpectCondition(
				novaNames.APIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("reports that the public cert secret is missing", func() {
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCABundleSecret(novaNames.CaBundleSecretName))
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(novaNames.InternalCertSecretName))

			th.ExpectConditionWithDetails(
				novaNames.APIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				fmt.Sprintf("TLSInput error occured in TLS sources Secret %s/public-tls-certs not found", novaNames.Namespace),
			)
			th.ExpectCondition(
				novaNames.APIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("creates a StatefulSet for nova-api service with TLS certs attached", func() {
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCABundleSecret(novaNames.CaBundleSecretName))
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(novaNames.InternalCertSecretName))
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(novaNames.PublicCertSecretName))
			th.SimulateStatefulSetReplicaReady(novaNames.APIStatefulSetName)
			keystone.SimulateKeystoneEndpointReady(novaNames.APIKeystoneEndpointName)

			ss := th.GetStatefulSet(novaNames.APIStatefulSetName)
			// Check the resulting deployment fields
			Expect(int(*ss.Spec.Replicas)).To(Equal(1))
			Expect(ss.Spec.Template.Spec.Volumes).To(HaveLen(5))
			Expect(ss.Spec.Template.Spec.Containers).To(HaveLen(2))

			// cert deployment volumes
			th.AssertVolumeExists(novaNames.CaBundleSecretName.Name, ss.Spec.Template.Spec.Volumes)
			th.AssertVolumeExists(novaNames.InternalCertSecretName.Name, ss.Spec.Template.Spec.Volumes)
			th.AssertVolumeExists(novaNames.PublicCertSecretName.Name, ss.Spec.Template.Spec.Volumes)

			// httpd container certs
			apiContainer := ss.Spec.Template.Spec.Containers[1]
			th.AssertVolumeMountExists(novaNames.InternalCertSecretName.Name, "tls.key", apiContainer.VolumeMounts)
			th.AssertVolumeMountExists(novaNames.InternalCertSecretName.Name, "tls.crt", apiContainer.VolumeMounts)
			th.AssertVolumeMountExists(novaNames.PublicCertSecretName.Name, "tls.key", apiContainer.VolumeMounts)
			th.AssertVolumeMountExists(novaNames.PublicCertSecretName.Name, "tls.crt", apiContainer.VolumeMounts)
			th.AssertVolumeMountExists(novaNames.CaBundleSecretName.Name, "tls-ca-bundle.pem", apiContainer.VolumeMounts)

			Expect(apiContainer.ReadinessProbe.HTTPGet.Scheme).To(Equal(corev1.URISchemeHTTPS))
			Expect(apiContainer.LivenessProbe.HTTPGet.Scheme).To(Equal(corev1.URISchemeHTTPS))
			Expect(apiContainer.StartupProbe.HTTPGet.Scheme).To(Equal(corev1.URISchemeHTTPS))

			configDataMap := th.GetSecret(novaNames.APIConfigDataName)
			Expect(configDataMap).ShouldNot(BeNil())
			Expect(configDataMap.Data).Should(HaveKey("httpd.conf"))
			Expect(configDataMap.Data).Should(HaveKey("ssl.conf"))
			configData := string(configDataMap.Data["httpd.conf"])
			Expect(configData).Should(ContainSubstring("SSLEngine on"))
			Expect(configData).Should(ContainSubstring("SSLCertificateFile      \"/etc/pki/tls/certs/internal.crt\""))
			Expect(configData).Should(ContainSubstring("SSLCertificateKeyFile   \"/etc/pki/tls/private/internal.key\""))
			Expect(configData).Should(ContainSubstring("SSLCertificateFile      \"/etc/pki/tls/certs/public.crt\""))
			Expect(configData).Should(ContainSubstring("SSLCertificateKeyFile   \"/etc/pki/tls/private/public.key\""))
		})

		It("TLS Endpoints are created", func() {
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCABundleSecret(novaNames.CaBundleSecretName))
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(novaNames.InternalCertSecretName))
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(novaNames.PublicCertSecretName))
			th.SimulateStatefulSetReplicaReady(novaNames.APIStatefulSetName)
			keystone.SimulateKeystoneEndpointReady(novaNames.APIKeystoneEndpointName)

			keystoneEndpoint := keystone.GetKeystoneEndpoint(types.NamespacedName{Namespace: novaNames.APIName.Namespace, Name: "nova"})
			endpoints := keystoneEndpoint.Spec.Endpoints
			Expect(endpoints).To(HaveKeyWithValue("public", string("https://nova-public."+novaNames.APIName.Namespace+".svc:8774/v2.1")))
			Expect(endpoints).To(HaveKeyWithValue("internal", "https://nova-internal."+novaNames.APIName.Namespace+".svc:8774/v2.1"))

			th.ExpectCondition(
				novaNames.APIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})
})
