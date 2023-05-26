/*
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
	networkv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
	. "github.com/openstack-k8s-operators/lib-common/modules/test/helpers"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("NovaNoVNCProxy controller", func() {
	When("with standard spec without network interface", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaMessageBusSecret(novaNames.NoVNCProxyName.Namespace, MessageBusSecretName))

			spec := GetDefaultNovaNoVNCProxySpec()
			spec["customServiceConfig"] = "foo=bar"
			DeferCleanup(th.DeleteInstance, CreateNovaNoVNCProxy(novaNames.NoVNCProxyName, spec))
		})
		When("a NovaNoVNCProxy CR is created pointing to a non existent Secret", func() {
			It("is not Ready", func() {
				th.ExpectCondition(
					novaNames.NoVNCProxyName,
					ConditionGetterFunc(NoVNCProxyConditionGetter),
					condition.ReadyCondition, corev1.ConditionFalse,
				)
			})

			It("has empty Status fields", func() {
				instance := GetNovaNoVNCProxy(novaNames.NoVNCProxyName)
				// NOTE(gibi): Hash has `omitempty` tags so while
				// they are initialized to an empty map that value is omitted from
				// the output when sent to the client. So we see nils here.
				Expect(instance.Status.Hash).To(BeEmpty())
				Expect(instance.Status.ReadyCount).To(Equal(int32(0)))
			})
			It("is missing the secret", func() {
				th.ExpectCondition(
					novaNames.NoVNCProxyName,
					ConditionGetterFunc(NoVNCProxyConditionGetter),
					condition.InputReadyCondition,
					corev1.ConditionFalse,
				)
			})
		})
		When("an unrelated Secret is created the CR state does not change", func() {
			BeforeEach(func() {
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "not-relevant-secret",
						Namespace: novaNames.NoVNCProxyName.Namespace,
					},
				}
				Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
				DeferCleanup(k8sClient.Delete, ctx, secret)
			})

			It("is not Ready", func() {
				th.ExpectCondition(
					novaNames.NoVNCProxyName,
					ConditionGetterFunc(NoVNCProxyConditionGetter),
					condition.ReadyCondition,
					corev1.ConditionFalse,
				)
			})

			It("is missing the secret", func() {
				th.ExpectCondition(
					novaNames.NoVNCProxyName,
					ConditionGetterFunc(NoVNCProxyConditionGetter),
					condition.InputReadyCondition,
					corev1.ConditionFalse,
				)
			})
		})
		When("the Secret is created but some fields are missing", func() {
			BeforeEach(func() {
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      SecretName,
						Namespace: novaNames.NoVNCProxyName.Namespace,
					},
					Data: map[string][]byte{
						"data": []byte("12345678"),
					},
				}
				Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
				DeferCleanup(k8sClient.Delete, ctx, secret)
			})

			It("is not Ready", func() {
				th.ExpectCondition(
					novaNames.NoVNCProxyName,
					ConditionGetterFunc(NoVNCProxyConditionGetter),
					condition.ReadyCondition,
					corev1.ConditionFalse,
				)
			})

			It("reports that the inputs are not ready", func() {
				th.ExpectCondition(
					novaNames.NoVNCProxyName,
					ConditionGetterFunc(NoVNCProxyConditionGetter),
					condition.InputReadyCondition,
					corev1.ConditionFalse,
				)
			})
		})
		When("the Secret is created with all the expected fields", func() {
			BeforeEach(func() {
				DeferCleanup(
					k8sClient.Delete,
					ctx,
					CreateNovaNoVNCProxySecret(novaNames.NoVNCProxyName.Namespace, SecretName),
				)
			})

			It("reports that input is ready", func() {
				th.ExpectCondition(
					novaNames.NoVNCProxyName,
					ConditionGetterFunc(NoVNCProxyConditionGetter),
					condition.InputReadyCondition,
					corev1.ConditionTrue,
				)
			})
			It("generated configs successfully", func() {
				th.ExpectCondition(
					novaNames.NoVNCProxyName,
					ConditionGetterFunc(NoVNCProxyConditionGetter),
					condition.ServiceConfigReadyCondition,
					corev1.ConditionTrue,
				)

				configDataMap := th.GetConfigMap(
					types.NamespacedName{
						Namespace: novaNames.NoVNCProxyName.Namespace,
						Name:      fmt.Sprintf("%s-config-data", novaNames.NoVNCProxyName.Name),
					},
				)
				Expect(configDataMap).ShouldNot(BeNil())
				Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
				Expect(configDataMap.Data).Should(
					HaveKeyWithValue("01-nova.conf",
						ContainSubstring("novncproxy_host = ")))
				Expect(configDataMap.Data).Should(
					HaveKeyWithValue("01-nova.conf",
						ContainSubstring("novncproxy_port = ")))
				Expect(configDataMap.Data).Should(
					HaveKeyWithValue("01-nova.conf",
						ContainSubstring("server_listen = ")))
				Expect(configDataMap.Data).Should(
					HaveKeyWithValue("02-nova-override.conf", "foo=bar"))
			})

			It("stored the input hash in the Status", func() {
				Eventually(func(g Gomega) {
					noVNCProxy := GetNovaNoVNCProxy(novaNames.NoVNCProxyName)
					g.Expect(noVNCProxy.Status.Hash).Should(HaveKeyWithValue("input", Not(BeEmpty())))
				}, timeout, interval).Should(Succeed())

			})

			When("the NoVNCProxy is deleted", func() {
				It("deletes the generated ConfigMaps", func() {
					th.ExpectCondition(
						novaNames.NoVNCProxyName,
						ConditionGetterFunc(NoVNCProxyConditionGetter),
						condition.ServiceConfigReadyCondition,
						corev1.ConditionTrue,
					)

					th.DeleteInstance(GetNovaNoVNCProxy(novaNames.NoVNCProxyName))

					Eventually(func() []corev1.ConfigMap {
						return th.ListConfigMaps(novaNames.NoVNCProxyName.Name).Items
					}, timeout, interval).Should(BeEmpty())
				})
			})
		})
		When("NoVNCProxy is created with a proper Secret", func() {
			BeforeEach(func() {
				DeferCleanup(
					k8sClient.Delete, ctx, CreateNovaNoVNCProxySecret(novaNames.NoVNCProxyName.Namespace, SecretName))
			})

			It(" reports input ready", func() {
				th.ExpectCondition(
					novaNames.NoVNCProxyName,
					ConditionGetterFunc(NoVNCProxyConditionGetter),
					condition.InputReadyCondition,
					corev1.ConditionTrue,
				)
			})

			It("creates a StatefulSet for the nova-novncproxy service", func() {
				th.ExpectConditionWithDetails(
					novaNames.NoVNCProxyName,
					ConditionGetterFunc(NoVNCProxyConditionGetter),
					condition.DeploymentReadyCondition,
					corev1.ConditionFalse,
					condition.RequestedReason,
					condition.DeploymentReadyRunningMessage,
				)

				ss := th.GetStatefulSet(novaNames.NoVNCProxyNameStatefulSetName)
				Expect(int(*ss.Spec.Replicas)).To(Equal(1))
				Expect(ss.Spec.Template.Spec.Volumes).To(HaveLen(2))
				Expect(ss.Spec.Template.Spec.Containers).To(HaveLen(2))
				Expect(ss.Spec.Selector.MatchLabels).To(Equal(map[string]string{"service": "nova-novncproxy"}))

				container := ss.Spec.Template.Spec.Containers[0]
				Expect(container.VolumeMounts).To(HaveLen(1))
				Expect(container.Image).To(Equal(ContainerImage))

				container = ss.Spec.Template.Spec.Containers[1]
				Expect(container.VolumeMounts).To(HaveLen(2))
				Expect(container.Image).To(Equal(ContainerImage))

				Expect(container.LivenessProbe.HTTPGet.Port.IntVal).To(Equal(int32(6082)))
				Expect(container.ReadinessProbe.HTTPGet.Port.IntVal).To(Equal(int32(6082)))

			})

			When("the StatefulSet has at least one Replica ready", func() {
				BeforeEach(func() {
					th.ExpectConditionWithDetails(
						novaNames.NoVNCProxyName,
						ConditionGetterFunc(NoVNCProxyConditionGetter),
						condition.DeploymentReadyCondition,
						corev1.ConditionFalse,
						condition.RequestedReason,
						condition.DeploymentReadyRunningMessage,
					)
					th.SimulateStatefulSetReplicaReady(novaNames.NoVNCProxyNameStatefulSetName)
				})

				It("reports that the StatefulSet is ready", func() {
					th.GetStatefulSet(novaNames.NoVNCProxyNameStatefulSetName)
					th.ExpectCondition(
						novaNames.NoVNCProxyName,
						ConditionGetterFunc(NoVNCProxyConditionGetter),
						condition.DeploymentReadyCondition,
						corev1.ConditionTrue,
					)

					noVNCProxyName := GetNovaNoVNCProxy(novaNames.NoVNCProxyName)
					Expect(noVNCProxyName.Status.ReadyCount).To(BeNumerically(">", 0))
				})
			})

			It("exposes the service", func() {
				th.SimulateStatefulSetReplicaReady(novaNames.NoVNCProxyNameStatefulSetName)
				th.ExpectCondition(
					novaNames.NoVNCProxyName,
					ConditionGetterFunc(NoVNCProxyConditionGetter),
					condition.ExposeServiceReadyCondition,
					corev1.ConditionTrue,
				)
				service := th.GetService(types.NamespacedName{Namespace: novaNames.NoVNCProxyName.Namespace, Name: "nova-novncproxy-cell1-internal"})
				Expect(service.Labels["service"]).To(Equal("nova-novncproxy"))
			})

			It("is Ready", func() {
				th.SimulateStatefulSetReplicaReady(novaNames.NoVNCProxyNameStatefulSetName)

				th.ExpectCondition(
					novaNames.NoVNCProxyName,
					ConditionGetterFunc(NoVNCProxyConditionGetter),
					condition.ReadyCondition,
					corev1.ConditionTrue,
				)
			})
		})
	})
	When("NovaNoVNCProxy CR is created without container image defined", func() {
		BeforeEach(func() {
			spec := GetDefaultNovaNoVNCProxySpec()
			spec["containerImage"] = ""
			novncproxy := CreateNovaNoVNCProxy(novaNames.NoVNCProxyName, spec)
			DeferCleanup(th.DeleteInstance, novncproxy)
		})
		It("has the expected container image default", func() {
			novaNoVNCProxyDefault := GetNovaNoVNCProxy(novaNames.NoVNCProxyName)
			Expect(novaNoVNCProxyDefault.Spec.ContainerImage).To(Equal(util.GetEnvVar("NOVA_NOVNC_IMAGE_URL_DEFAULT", novav1.NovaNoVNCContainerImage)))
		})
	})
})

var _ = Describe("NovaNoVNCProxy controller", func() {
	BeforeEach(func() {
		DeferCleanup(
			k8sClient.Delete, ctx, CreateNovaMessageBusSecret(novaNames.NoVNCProxyName.Namespace, MessageBusSecretName))
	})

	When(" is created with networkAttachments", func() {
		BeforeEach(func() {
			spec := GetDefaultNovaNoVNCProxySpec()
			spec["networkAttachments"] = []string{"internalapi"}
			DeferCleanup(th.DeleteInstance, CreateNovaNoVNCProxy(novaNames.NoVNCProxyName, spec))
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateNovaNoVNCProxySecret(novaNames.NoVNCProxyName.Namespace, SecretName),
			)
		})

		It("reports that the definition is missing", func() {
			th.ExpectConditionWithDetails(
				novaNames.NoVNCProxyName,
				ConditionGetterFunc(NoVNCProxyConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NetworkAttachment resources missing: internalapi",
			)
			th.ExpectCondition(
				novaNames.NoVNCProxyName,
				ConditionGetterFunc(NoVNCProxyConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})
		It("reports that network attachment is missing", func() {
			internalNoVNCName := types.NamespacedName{Namespace: novaNames.NoVNCProxyName.Namespace, Name: "internalapi"}
			nad := th.CreateNetworkAttachmentDefinition(internalNoVNCName)
			DeferCleanup(th.DeleteInstance, nad)

			ss := th.GetStatefulSet(novaNames.NoVNCProxyNameStatefulSetName)

			expectedAnnotation, err := json.Marshal(
				[]networkv1.NetworkSelectionElement{
					{
						Name:             "internalapi",
						Namespace:        novaNames.NoVNCProxyName.Namespace,
						InterfaceRequest: "internalapi",
					}})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(ss.Spec.Template.ObjectMeta.Annotations).To(
				HaveKeyWithValue("k8s.v1.cni.cncf.io/networks", string(expectedAnnotation)),
			)

			// We don't add network attachment status annotations to the Pods
			// to simulate that the network attachments are missing.
			SimulateStatefulSetReplicaReadyWithPods(novaNames.NoVNCProxyNameStatefulSetName, map[string][]string{})

			th.ExpectConditionWithDetails(
				novaNames.NoVNCProxyName,
				ConditionGetterFunc(NoVNCProxyConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occurred "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
			)
		})
		It("reports that an IP is missing", func() {
			internalNoVNCName := types.NamespacedName{Namespace: novaNames.NoVNCProxyName.Namespace, Name: "internalapi"}
			nad := th.CreateNetworkAttachmentDefinition(internalNoVNCName)
			DeferCleanup(th.DeleteInstance, nad)

			ss := th.GetStatefulSet(novaNames.NoVNCProxyNameStatefulSetName)

			expectedAnnotation, err := json.Marshal(
				[]networkv1.NetworkSelectionElement{
					{
						Name:             "internalapi",
						Namespace:        novaNames.NoVNCProxyName.Namespace,
						InterfaceRequest: "internalapi",
					}})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(ss.Spec.Template.ObjectMeta.Annotations).To(
				HaveKeyWithValue("k8s.v1.cni.cncf.io/networks", string(expectedAnnotation)),
			)

			// We simulate that there is no IP associated with the internalapi
			// network attachment
			SimulateStatefulSetReplicaReadyWithPods(
				novaNames.NoVNCProxyNameStatefulSetName,
				map[string][]string{novaNames.NoVNCProxyName.Namespace + "/internalapi": {}},
			)

			th.ExpectConditionWithDetails(
				novaNames.NoVNCProxyName,
				ConditionGetterFunc(NoVNCProxyConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occurred "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
			)
		})
		It("reports NetworkAttachmentsReady if the Pods got the proper annotations", func() {
			internalNoVNCName := types.NamespacedName{Namespace: novaNames.NoVNCProxyName.Namespace, Name: "internalapi"}
			nad := th.CreateNetworkAttachmentDefinition(internalNoVNCName)
			DeferCleanup(th.DeleteInstance, nad)

			SimulateStatefulSetReplicaReadyWithPods(
				novaNames.NoVNCProxyNameStatefulSetName,
				map[string][]string{novaNames.NoVNCProxyName.Namespace + "/internalapi": {"10.0.0.1"}},
			)

			th.ExpectCondition(
				novaNames.NoVNCProxyName,
				ConditionGetterFunc(NoVNCProxyConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionTrue,
			)

			Eventually(func(g Gomega) {
				instance := GetNovaNoVNCProxy(novaNames.NoVNCProxyName)
				g.Expect(instance.Status.NetworkAttachments).To(
					Equal(map[string][]string{novaNames.NoVNCProxyName.Namespace + "/internalapi": {"10.0.0.1"}}))

			}, timeout, interval).Should(Succeed())

			th.ExpectCondition(
				novaNames.NoVNCProxyName,
				ConditionGetterFunc(NoVNCProxyConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})

	When("NovaNoVNCProxy is created with externalEndpoints", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaNoVNCProxySecret(novaNames.NoVNCProxyName.Namespace, SecretName))

			spec := GetDefaultNovaNoVNCProxySpec()
			var externalEndpoints []interface{}
			externalEndpoints = append(
				externalEndpoints, map[string]interface{}{
					"endpoint":        "internal",
					"ipAddressPool":   "osp-internalapi",
					"loadBalancerIPs": []string{"internal-lb-ip-1", "internal-lb-ip-2"},
				},
			)
			spec["externalEndpoints"] = externalEndpoints

			noVNCP := CreateNovaNoVNCProxy(novaNames.NoVNCProxyName, spec)
			DeferCleanup(th.DeleteInstance, noVNCP)
		})

		It("creates MetalLB service", func() {
			th.SimulateStatefulSetReplicaReady(novaNames.NoVNCProxyNameStatefulSetName)

			// As the internal endpoint is configured in ExternalEndpoints it does not
			// get a Route but a Service with MetalLB annotations instead
			service := th.GetService(types.NamespacedName{Namespace: novaNames.NoVNCProxyName.Namespace, Name: "nova-novncproxy-cell1-internal"})
			Expect(service.Annotations).To(
				HaveKeyWithValue("metallb.universe.tf/address-pool", "osp-internalapi"))
			Expect(service.Annotations).To(
				HaveKeyWithValue("metallb.universe.tf/allow-shared-ip", "osp-internalapi"))
			Expect(service.Annotations).To(
				HaveKeyWithValue("metallb.universe.tf/loadBalancerIPs", "internal-lb-ip-1,internal-lb-ip-2"))
			th.AssertRouteNotExists(types.NamespacedName{Namespace: novaNames.NoVNCProxyName.Namespace, Name: "nova-novncproxy-cell1-internal"})

			th.ExpectCondition(
				novaNames.NoVNCProxyName,
				ConditionGetterFunc(NoVNCProxyConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})
	When("NovaNoVNCProxy is reconfigured", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaNoVNCProxySecret(novaNames.NoVNCProxyName.Namespace, SecretName))

			noVNCProxy := CreateNovaNoVNCProxy(novaNames.NoVNCProxyName, GetDefaultNovaNoVNCProxySpec())
			DeferCleanup(th.DeleteInstance, noVNCProxy)

			th.ExpectCondition(
				novaNames.NoVNCProxyName,
				ConditionGetterFunc(NoVNCProxyConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)

			th.SimulateStatefulSetReplicaReady(novaNames.NoVNCProxyNameStatefulSetName)
			th.ExpectCondition(
				novaNames.NoVNCProxyName,
				ConditionGetterFunc(NoVNCProxyConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("applies new NetworkAttachments configuration", func() {
			Eventually(func(g Gomega) {
				noVNCProxy := GetNovaNoVNCProxy(novaNames.NoVNCProxyName)
				noVNCProxy.Spec.NetworkAttachments = append(noVNCProxy.Spec.NetworkAttachments, "internalapi")

				err := k8sClient.Update(ctx, noVNCProxy)
				g.Expect(err == nil || k8s_errors.IsConflict(err)).To(BeTrue())
			}, timeout, interval).Should(Succeed())

			th.ExpectConditionWithDetails(
				novaNames.NoVNCProxyName,
				ConditionGetterFunc(NoVNCProxyConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NetworkAttachment resources missing: internalapi",
			)
			th.ExpectConditionWithDetails(
				novaNames.NoVNCProxyName,
				ConditionGetterFunc(NoVNCProxyConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NetworkAttachment resources missing: internalapi",
			)

			internalAPINADName := types.NamespacedName{Namespace: novaNames.NoVNCProxyName.Namespace, Name: "internalapi"}
			DeferCleanup(th.DeleteInstance, th.CreateNetworkAttachmentDefinition(internalAPINADName))

			th.ExpectConditionWithDetails(
				novaNames.NoVNCProxyName,
				ConditionGetterFunc(NoVNCProxyConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occurred "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
			)
			th.ExpectConditionWithDetails(
				novaNames.NoVNCProxyName,
				ConditionGetterFunc(NoVNCProxyConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occurred "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
			)

			SimulateStatefulSetReplicaReadyWithPods(
				novaNames.NoVNCProxyNameStatefulSetName,
				map[string][]string{novaNames.NoVNCProxyName.Namespace + "/internalapi": {"10.0.0.1"}},
			)

			th.ExpectCondition(
				novaNames.NoVNCProxyName,
				ConditionGetterFunc(NoVNCProxyConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionTrue,
			)

			Eventually(func(g Gomega) {
				noVNCProxy := GetNovaNoVNCProxy(novaNames.NoVNCProxyName)
				g.Expect(noVNCProxy.Status.NetworkAttachments).To(
					Equal(map[string][]string{novaNames.NoVNCProxyName.Namespace + "/internalapi": {"10.0.0.1"}}))

			}, timeout, interval).Should(Succeed())

			th.ExpectCondition(
				novaNames.NoVNCProxyName,
				ConditionGetterFunc(NoVNCProxyConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})
	When("starts zero replicas", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaNoVNCProxySecret(novaNames.NoVNCProxyName.Namespace, SecretName))

			spec := GetDefaultNovaNoVNCProxySpec()
			spec["replicas"] = 0
			noVNCProxy := CreateNovaNoVNCProxy(novaNames.NoVNCProxyName, spec)
			DeferCleanup(th.DeleteInstance, noVNCProxy)
		})
		It("and deployment is Ready", func() {
			ss := th.GetStatefulSet(novaNames.NoVNCProxyNameStatefulSetName)
			Expect(int(*ss.Spec.Replicas)).To(Equal(0))
			th.ExpectCondition(
				novaNames.NoVNCProxyName,
				ConditionGetterFunc(NoVNCProxyConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionTrue,
			)

		})
	})
})
