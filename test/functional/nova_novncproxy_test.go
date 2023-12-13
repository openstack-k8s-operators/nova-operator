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
	. "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
)

var _ = Describe("NovaNoVNCProxy controller", func() {
	When("with standard spec without network interface", func() {
		BeforeEach(func() {
			spec := GetDefaultNovaNoVNCProxySpec(cell1)
			spec["customServiceConfig"] = "foo=bar"
			DeferCleanup(th.DeleteInstance, CreateNovaNoVNCProxy(cell1.NoVNCProxyName, spec))
		})
		When("a NovaNoVNCProxy CR is created pointing to a non existent Secret", func() {
			It("is not Ready", func() {
				th.ExpectCondition(
					cell1.NoVNCProxyName,
					ConditionGetterFunc(NoVNCProxyConditionGetter),
					condition.ReadyCondition, corev1.ConditionFalse,
				)
			})

			It("has empty Status fields", func() {
				instance := GetNovaNoVNCProxy(cell1.NoVNCProxyName)
				// NOTE(gibi): Hash has `omitempty` tags so while
				// they are initialized to an empty map that value is omitted from
				// the output when sent to the client. So we see nils here.
				Expect(instance.Status.Hash).To(BeEmpty())
				Expect(instance.Status.ReadyCount).To(Equal(int32(0)))
			})
			It("is missing the secret", func() {
				th.ExpectConditionWithDetails(
					cell1.NoVNCProxyName,
					ConditionGetterFunc(NoVNCProxyConditionGetter),
					condition.InputReadyCondition,
					corev1.ConditionFalse,
					condition.RequestedReason,
					fmt.Sprintf("Input data resources missing: secret/%s", cell1.CellCRName.Name),
				)
			})
		})
		When("an unrelated Secret is created the CR state does not change", func() {
			BeforeEach(func() {
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "not-relevant-secret",
						Namespace: cell1.NoVNCProxyName.Namespace,
					},
				}
				Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
				DeferCleanup(k8sClient.Delete, ctx, secret)
			})

			It("is not Ready", func() {
				th.ExpectCondition(
					cell1.NoVNCProxyName,
					ConditionGetterFunc(NoVNCProxyConditionGetter),
					condition.ReadyCondition,
					corev1.ConditionFalse,
				)
			})

			It("is missing the secret", func() {
				th.ExpectCondition(
					cell1.NoVNCProxyName,
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
						Name:      cell1.InternalCellSecretName.Name,
						Namespace: cell1.InternalCellSecretName.Namespace,
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
					cell1.NoVNCProxyName,
					ConditionGetterFunc(NoVNCProxyConditionGetter),
					condition.ReadyCondition,
					corev1.ConditionFalse,
				)
			})

			It("reports that the inputs are not ready", func() {
				th.ExpectConditionWithDetails(
					cell1.NoVNCProxyName,
					ConditionGetterFunc(NoVNCProxyConditionGetter),
					condition.InputReadyCondition,
					corev1.ConditionFalse,
					condition.ErrorReason,
					fmt.Sprintf("Input data error occurred field 'ServicePassword' not found in secret/%s", cell1.InternalCellSecretName.Name),
				)
			})
		})
		When("the Secret is created with all the expected fields", func() {
			BeforeEach(func() {
				DeferCleanup(
					k8sClient.Delete, ctx, CreateCellInternalSecret(cell1))
			})

			It("reports that input is ready", func() {
				th.ExpectCondition(
					cell1.NoVNCProxyName,
					ConditionGetterFunc(NoVNCProxyConditionGetter),
					condition.InputReadyCondition,
					corev1.ConditionTrue,
				)
			})
			It("generated configs successfully", func() {
				th.ExpectCondition(
					cell1.NoVNCProxyName,
					ConditionGetterFunc(NoVNCProxyConditionGetter),
					condition.ServiceConfigReadyCondition,
					corev1.ConditionTrue,
				)

				configDataMap := th.GetSecret(
					types.NamespacedName{
						Namespace: cell1.NoVNCProxyName.Namespace,
						Name:      fmt.Sprintf("%s-config-data", cell1.NoVNCProxyName.Name),
					},
				)
				Expect(configDataMap).ShouldNot(BeNil())
				Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
				configData := string(configDataMap.Data["01-nova.conf"])
				Expect(configData).Should(ContainSubstring("novncproxy_host = \"::0\""))
				Expect(configData).Should(ContainSubstring("novncproxy_port = 6080"))
				Expect(configData).Should(ContainSubstring("password = service-password"))
				Expect(configData).Should(ContainSubstring("transport_url=rabbit://cell1/fake"))
				Expect(configData).Should(
					ContainSubstring("[upgrade_levels]\ncompute = auto"))
				Expect(configDataMap.Data).Should(HaveKey("02-nova-override.conf"))
				extraData := string(configDataMap.Data["02-nova-override.conf"])
				Expect(extraData).To(Equal("foo=bar"))
			})

			It("stored the input hash in the Status", func() {
				Eventually(func(g Gomega) {
					noVNCProxy := GetNovaNoVNCProxy(cell1.NoVNCProxyName)
					g.Expect(noVNCProxy.Status.Hash).Should(HaveKeyWithValue("input", Not(BeEmpty())))
				}, timeout, interval).Should(Succeed())

			})
		})
		When("NoVNCProxy is created with a proper Secret", func() {
			BeforeEach(func() {
				DeferCleanup(
					k8sClient.Delete, ctx, CreateCellInternalSecret(cell1))
			})

			It(" reports input ready", func() {
				th.ExpectCondition(
					cell1.NoVNCProxyName,
					ConditionGetterFunc(NoVNCProxyConditionGetter),
					condition.InputReadyCondition,
					corev1.ConditionTrue,
				)
			})

			It("creates a StatefulSet for the nova-novncproxy service", func() {
				th.ExpectConditionWithDetails(
					cell1.NoVNCProxyName,
					ConditionGetterFunc(NoVNCProxyConditionGetter),
					condition.DeploymentReadyCondition,
					corev1.ConditionFalse,
					condition.RequestedReason,
					condition.DeploymentReadyRunningMessage,
				)

				ss := th.GetStatefulSet(cell1.NoVNCProxyStatefulSetName)
				Expect(ss.Spec.Template.Spec.ServiceAccountName).To(Equal("nova-sa"))
				Expect(int(*ss.Spec.Replicas)).To(Equal(1))
				Expect(ss.Spec.Template.Spec.Volumes).To(HaveLen(1))
				Expect(ss.Spec.Template.Spec.Containers).To(HaveLen(1))
				Expect(ss.Spec.Selector.MatchLabels).To(Equal(map[string]string{"service": "nova-novncproxy", "cell": "cell1"}))

				container := ss.Spec.Template.Spec.Containers[0]
				Expect(container.VolumeMounts).To(HaveLen(2))
				Expect(container.Image).To(Equal(ContainerImage))

				Expect(container.LivenessProbe.HTTPGet.Port.IntVal).To(Equal(int32(6080)))
				Expect(container.ReadinessProbe.HTTPGet.Port.IntVal).To(Equal(int32(6080)))

			})

			When("the StatefulSet has at least one Replica ready", func() {
				BeforeEach(func() {
					th.ExpectConditionWithDetails(
						cell1.NoVNCProxyName,
						ConditionGetterFunc(NoVNCProxyConditionGetter),
						condition.DeploymentReadyCondition,
						corev1.ConditionFalse,
						condition.RequestedReason,
						condition.DeploymentReadyRunningMessage,
					)
					th.SimulateStatefulSetReplicaReady(cell1.NoVNCProxyStatefulSetName)
				})

				It("reports that the StatefulSet is ready", func() {
					th.GetStatefulSet(cell1.NoVNCProxyStatefulSetName)
					th.ExpectCondition(
						cell1.NoVNCProxyName,
						ConditionGetterFunc(NoVNCProxyConditionGetter),
						condition.DeploymentReadyCondition,
						corev1.ConditionTrue,
					)

					noVNCProxyName := GetNovaNoVNCProxy(cell1.NoVNCProxyName)
					Expect(noVNCProxyName.Status.ReadyCount).To(BeNumerically(">", 0))
				})
			})

			It("exposes the service", func() {
				th.SimulateStatefulSetReplicaReady(cell1.NoVNCProxyStatefulSetName)
				th.ExpectCondition(
					cell1.NoVNCProxyName,
					ConditionGetterFunc(NoVNCProxyConditionGetter),
					condition.ExposeServiceReadyCondition,
					corev1.ConditionTrue,
				)
				service := th.GetService(types.NamespacedName{Namespace: cell1.NoVNCProxyName.Namespace, Name: "nova-novncproxy-cell1-public"})
				Expect(service.Labels["service"]).To(Equal("nova-novncproxy"))
				Expect(service.Labels["cell"]).To(Equal("cell1"))
				// for novnc we only expose a public service intentionally
				th.AssertServiceDoesNotExist(types.NamespacedName{Namespace: cell1.NoVNCProxyName.Namespace, Name: "nova-novncproxy-cell1-internal"})
			})

			It("is Ready", func() {
				th.SimulateStatefulSetReplicaReady(cell1.NoVNCProxyStatefulSetName)

				th.ExpectCondition(
					cell1.NoVNCProxyName,
					ConditionGetterFunc(NoVNCProxyConditionGetter),
					condition.ReadyCondition,
					corev1.ConditionTrue,
				)
			})
		})
	})
	When("NovaNoVNCProxy CR is created without container image defined", func() {
		BeforeEach(func() {
			spec := GetDefaultNovaNoVNCProxySpec(cell1)
			spec["containerImage"] = ""
			novncproxy := CreateNovaNoVNCProxy(cell1.NoVNCProxyName, spec)
			DeferCleanup(th.DeleteInstance, novncproxy)
		})
		It("has the expected container image default", func() {
			novaNoVNCProxyDefault := GetNovaNoVNCProxy(cell1.NoVNCProxyName)
			Expect(novaNoVNCProxyDefault.Spec.ContainerImage).To(Equal(util.GetEnvVar("RELATED_IMAGE_NOVA_NOVNC_IMAGE_URL_DEFAULT", novav1.NovaNoVNCContainerImage)))
		})
	})
})

var _ = Describe("NovaNoVNCProxy controller", func() {
	When(" is created with networkAttachments", func() {
		BeforeEach(func() {
			spec := GetDefaultNovaNoVNCProxySpec(cell1)
			spec["networkAttachments"] = []string{"internalapi"}
			DeferCleanup(th.DeleteInstance, CreateNovaNoVNCProxy(cell1.NoVNCProxyName, spec))
			DeferCleanup(
				k8sClient.Delete, ctx, CreateCellInternalSecret(cell1))
		})

		It("reports that the definition is missing", func() {
			th.ExpectConditionWithDetails(
				cell1.NoVNCProxyName,
				ConditionGetterFunc(NoVNCProxyConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NetworkAttachment resources missing: internalapi",
			)
			th.ExpectCondition(
				cell1.NoVNCProxyName,
				ConditionGetterFunc(NoVNCProxyConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})
		It("reports that network attachment is missing", func() {
			internalNoVNCName := types.NamespacedName{Namespace: cell1.NoVNCProxyName.Namespace, Name: "internalapi"}
			nad := th.CreateNetworkAttachmentDefinition(internalNoVNCName)
			DeferCleanup(th.DeleteInstance, nad)

			ss := th.GetStatefulSet(cell1.NoVNCProxyStatefulSetName)

			expectedAnnotation, err := json.Marshal(
				[]networkv1.NetworkSelectionElement{
					{
						Name:             "internalapi",
						Namespace:        cell1.NoVNCProxyName.Namespace,
						InterfaceRequest: "internalapi",
					}})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(ss.Spec.Template.ObjectMeta.Annotations).To(
				HaveKeyWithValue("k8s.v1.cni.cncf.io/networks", string(expectedAnnotation)),
			)

			// We don't add network attachment status annotations to the Pods
			// to simulate that the network attachments are missing.
			th.SimulateStatefulSetReplicaReadyWithPods(cell1.NoVNCProxyStatefulSetName, map[string][]string{})

			th.ExpectConditionWithDetails(
				cell1.NoVNCProxyName,
				ConditionGetterFunc(NoVNCProxyConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occurred "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
			)
		})
		It("reports that an IP is missing", func() {
			internalNoVNCName := types.NamespacedName{Namespace: cell1.NoVNCProxyName.Namespace, Name: "internalapi"}
			nad := th.CreateNetworkAttachmentDefinition(internalNoVNCName)
			DeferCleanup(th.DeleteInstance, nad)

			ss := th.GetStatefulSet(cell1.NoVNCProxyStatefulSetName)

			expectedAnnotation, err := json.Marshal(
				[]networkv1.NetworkSelectionElement{
					{
						Name:             "internalapi",
						Namespace:        cell1.NoVNCProxyName.Namespace,
						InterfaceRequest: "internalapi",
					}})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(ss.Spec.Template.ObjectMeta.Annotations).To(
				HaveKeyWithValue("k8s.v1.cni.cncf.io/networks", string(expectedAnnotation)),
			)

			// We simulate that there is no IP associated with the internalapi
			// network attachment
			th.SimulateStatefulSetReplicaReadyWithPods(
				cell1.NoVNCProxyStatefulSetName,
				map[string][]string{cell1.NoVNCProxyName.Namespace + "/internalapi": {}},
			)

			th.ExpectConditionWithDetails(
				cell1.NoVNCProxyName,
				ConditionGetterFunc(NoVNCProxyConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occurred "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
			)
		})
		It("reports NetworkAttachmentsReady if the Pods got the proper annotations", func() {
			internalNoVNCName := types.NamespacedName{Namespace: cell1.NoVNCProxyName.Namespace, Name: "internalapi"}
			nad := th.CreateNetworkAttachmentDefinition(internalNoVNCName)
			DeferCleanup(th.DeleteInstance, nad)

			th.SimulateStatefulSetReplicaReadyWithPods(
				cell1.NoVNCProxyStatefulSetName,
				map[string][]string{cell1.NoVNCProxyName.Namespace + "/internalapi": {"10.0.0.1"}},
			)

			th.ExpectCondition(
				cell1.NoVNCProxyName,
				ConditionGetterFunc(NoVNCProxyConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionTrue,
			)

			Eventually(func(g Gomega) {
				instance := GetNovaNoVNCProxy(cell1.NoVNCProxyName)
				g.Expect(instance.Status.NetworkAttachments).To(
					Equal(map[string][]string{cell1.NoVNCProxyName.Namespace + "/internalapi": {"10.0.0.1"}}))

			}, timeout, interval).Should(Succeed())

			th.ExpectCondition(
				cell1.NoVNCProxyName,
				ConditionGetterFunc(NoVNCProxyConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})

	When("NovaNoVNCProxy is created with service override with endpointURL set", func() {

		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateCellInternalSecret(cell1))

			spec := GetDefaultNovaNoVNCProxySpec(cell1)
			serviceOverride := map[string]interface{}{
				"endpointURL": "http://nova-novncproxy-cell1-" + novaNames.Namespace + ".apps-crc.testing",
				"metadata": map[string]map[string]string{
					"labels": {
						"service": "nova-novncproxy",
					},
				},
			}

			spec["override"] = map[string]interface{}{
				"service": serviceOverride,
			}

			noVNCP := CreateNovaNoVNCProxy(cell1.NoVNCProxyName, spec)
			DeferCleanup(th.DeleteInstance, noVNCP)
		})

		It("creates ClusterIP service", func() {
			th.SimulateStatefulSetReplicaReady(cell1.NoVNCProxyStatefulSetName)

			// As the public endpoint is configured to be created via overrides
			// as a route, the annotation gets added and type is ClusterIP
			service := th.GetService(types.NamespacedName{Namespace: cell1.NoVNCProxyName.Namespace, Name: "nova-novncproxy-cell1-public"})
			Expect(service.Annotations).To(
				HaveKeyWithValue("core.openstack.org/ingress_create", "true"))
			Expect(service.Labels).To(
				HaveKeyWithValue("service", "nova-novncproxy"))
			Expect(service.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))

			th.ExpectCondition(
				cell1.NoVNCProxyName,
				ConditionGetterFunc(NoVNCProxyConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})

	When("NovaNoVNCProxy is created with service override with no endpointURL set", func() {

		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateCellInternalSecret(cell1))

			spec := GetDefaultNovaNoVNCProxySpec(cell1)
			serviceOverride := map[string]interface{}{
				"metadata": map[string]map[string]string{
					"annotations": {
						"dnsmasq.network.openstack.org/hostname": "nova-novncproxy-cell1-public.openstack.svc",
						"metallb.universe.tf/address-pool":       "osp-internalapi",
						"metallb.universe.tf/allow-shared-ip":    "osp-internalapi",
						"metallb.universe.tf/loadBalancerIPs":    "internal-lb-ip-1,internal-lb-ip-2",
					},
					"labels": {
						"service": "nova-novncproxy",
					},
				},
				"spec": map[string]interface{}{
					"type": "LoadBalancer",
				},
			}

			spec["override"] = map[string]interface{}{
				"service": serviceOverride,
			}

			noVNCP := CreateNovaNoVNCProxy(cell1.NoVNCProxyName, spec)
			DeferCleanup(th.DeleteInstance, noVNCP)
		})

		It("creates LoadBalancer service", func() {
			th.SimulateStatefulSetReplicaReady(cell1.NoVNCProxyStatefulSetName)

			// As the endpoint has service override configured it
			// gets a LoadBalancer Service with MetalLB annotations
			service := th.GetService(types.NamespacedName{Namespace: cell1.NoVNCProxyName.Namespace, Name: "nova-novncproxy-cell1-public"})
			Expect(service.Annotations).To(
				HaveKeyWithValue("core.openstack.org/ingress_create", "false"))
			Expect(service.Annotations).To(
				HaveKeyWithValue("dnsmasq.network.openstack.org/hostname", "nova-novncproxy-cell1-public.openstack.svc"))
			Expect(service.Annotations).To(
				HaveKeyWithValue("metallb.universe.tf/address-pool", "osp-internalapi"))
			Expect(service.Annotations).To(
				HaveKeyWithValue("metallb.universe.tf/allow-shared-ip", "osp-internalapi"))
			Expect(service.Annotations).To(
				HaveKeyWithValue("metallb.universe.tf/loadBalancerIPs", "internal-lb-ip-1,internal-lb-ip-2"))
			Expect(service.Labels).To(
				HaveKeyWithValue("service", "nova-novncproxy"))
			Expect(service.Spec.Type).To(Equal(corev1.ServiceTypeLoadBalancer))

			th.ExpectCondition(
				cell1.NoVNCProxyName,
				ConditionGetterFunc(NoVNCProxyConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})

	When("NovaNoVNCProxy is reconfigured", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateCellInternalSecret(cell1))

			noVNCProxy := CreateNovaNoVNCProxy(cell1.NoVNCProxyName, GetDefaultNovaNoVNCProxySpec(cell1))
			DeferCleanup(th.DeleteInstance, noVNCProxy)

			th.ExpectCondition(
				cell1.NoVNCProxyName,
				ConditionGetterFunc(NoVNCProxyConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)

			th.SimulateStatefulSetReplicaReady(cell1.NoVNCProxyStatefulSetName)
			th.ExpectCondition(
				cell1.NoVNCProxyName,
				ConditionGetterFunc(NoVNCProxyConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("applies new NetworkAttachments configuration", func() {
			Eventually(func(g Gomega) {
				noVNCProxy := GetNovaNoVNCProxy(cell1.NoVNCProxyName)
				noVNCProxy.Spec.NetworkAttachments = append(noVNCProxy.Spec.NetworkAttachments, "internalapi")

				g.Expect(k8sClient.Update(ctx, noVNCProxy)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			th.ExpectConditionWithDetails(
				cell1.NoVNCProxyName,
				ConditionGetterFunc(NoVNCProxyConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NetworkAttachment resources missing: internalapi",
			)
			th.ExpectConditionWithDetails(
				cell1.NoVNCProxyName,
				ConditionGetterFunc(NoVNCProxyConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NetworkAttachment resources missing: internalapi",
			)

			internalAPINADName := types.NamespacedName{Namespace: cell1.NoVNCProxyName.Namespace, Name: "internalapi"}
			DeferCleanup(th.DeleteInstance, th.CreateNetworkAttachmentDefinition(internalAPINADName))

			th.ExpectConditionWithDetails(
				cell1.NoVNCProxyName,
				ConditionGetterFunc(NoVNCProxyConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occurred "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
			)
			th.ExpectConditionWithDetails(
				cell1.NoVNCProxyName,
				ConditionGetterFunc(NoVNCProxyConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occurred "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
			)

			th.SimulateStatefulSetReplicaReadyWithPods(
				cell1.NoVNCProxyStatefulSetName,
				map[string][]string{cell1.NoVNCProxyName.Namespace + "/internalapi": {"10.0.0.1"}},
			)

			th.ExpectCondition(
				cell1.NoVNCProxyName,
				ConditionGetterFunc(NoVNCProxyConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionTrue,
			)

			Eventually(func(g Gomega) {
				noVNCProxy := GetNovaNoVNCProxy(cell1.NoVNCProxyName)
				g.Expect(noVNCProxy.Status.NetworkAttachments).To(
					Equal(map[string][]string{cell1.NoVNCProxyName.Namespace + "/internalapi": {"10.0.0.1"}}))

			}, timeout, interval).Should(Succeed())

			th.ExpectCondition(
				cell1.NoVNCProxyName,
				ConditionGetterFunc(NoVNCProxyConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})
	When("starts zero replicas", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateCellInternalSecret(cell1))

			spec := GetDefaultNovaNoVNCProxySpec(cell1)
			spec["replicas"] = 0
			noVNCProxy := CreateNovaNoVNCProxy(cell1.NoVNCProxyName, spec)
			DeferCleanup(th.DeleteInstance, noVNCProxy)
		})
		It("and deployment is Ready", func() {
			ss := th.GetStatefulSet(cell1.NoVNCProxyStatefulSetName)
			Expect(int(*ss.Spec.Replicas)).To(Equal(0))
			th.ExpectCondition(
				cell1.NoVNCProxyName,
				ConditionGetterFunc(NoVNCProxyConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionTrue,
			)

		})
	})
})

var _ = Describe("NovaNoVNCProxy controller", func() {
	When("NovaNoVNCProxy is created with TLS CA cert secret", func() {
		BeforeEach(func() {
			spec := GetDefaultNovaNoVNCProxySpec(cell1)
			spec["tls"] = map[string]interface{}{
				"secretName":         ptr.To(novaNames.InternalCertSecretName.Name),
				"caBundleSecretName": novaNames.CaBundleSecretName.Name,
			}
			DeferCleanup(th.DeleteInstance, CreateNovaNoVNCProxy(cell1.NoVNCProxyName, spec))
			DeferCleanup(
				k8sClient.Delete, ctx, CreateCellInternalSecret(cell1))
		})

		It("reports that the CA secret is missing", func() {
			th.ExpectConditionWithDetails(
				cell1.NoVNCProxyName,
				ConditionGetterFunc(NoVNCProxyConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				fmt.Sprintf("TLSInput error occured in TLS sources Secret %s/combined-ca-bundle not found", novaNames.Namespace),
			)
			th.ExpectCondition(
				cell1.NoVNCProxyName,
				ConditionGetterFunc(NoVNCProxyConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("reports that the service cert secret is missing", func() {
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCABundleSecret(novaNames.CaBundleSecretName))

			th.ExpectConditionWithDetails(
				cell1.NoVNCProxyName,
				ConditionGetterFunc(NoVNCProxyConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				fmt.Sprintf("TLSInput error occured in TLS sources Secret %s/internal-tls-certs not found", novaNames.Namespace),
			)
			th.ExpectCondition(
				cell1.NoVNCProxyName,
				ConditionGetterFunc(NoVNCProxyConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("creates a StatefulSet for nova-metadata service with TLS CA cert attached", func() {
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCABundleSecret(novaNames.CaBundleSecretName))
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(novaNames.InternalCertSecretName))
			th.SimulateStatefulSetReplicaReady(cell1.NoVNCProxyStatefulSetName)

			ss := th.GetStatefulSet(cell1.NoVNCProxyStatefulSetName)

			// Check the resulting deployment fields
			Expect(int(*ss.Spec.Replicas)).To(Equal(1))
			Expect(ss.Spec.Template.Spec.Volumes).To(HaveLen(3))
			Expect(ss.Spec.Template.Spec.Containers).To(HaveLen(1))

			// cert deployment volumes
			th.AssertVolumeExists(novaNames.CaBundleSecretName.Name, ss.Spec.Template.Spec.Volumes)
			th.AssertVolumeExists("nova-novncproxy-tls-certs", ss.Spec.Template.Spec.Volumes)

			// CA container certs
			apiContainer := ss.Spec.Template.Spec.Containers[0]
			th.AssertVolumeMountExists(novaNames.CaBundleSecretName.Name, "tls-ca-bundle.pem", apiContainer.VolumeMounts)
			th.AssertVolumeMountExists("nova-novncproxy-tls-certs", "tls.key", apiContainer.VolumeMounts)
			th.AssertVolumeMountExists("nova-novncproxy-tls-certs", "tls.crt", apiContainer.VolumeMounts)

			th.ExpectCondition(
				cell1.NoVNCProxyName,
				ConditionGetterFunc(NoVNCProxyConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)

			configDataMap := th.GetSecret(
				types.NamespacedName{
					Namespace: cell1.NoVNCProxyName.Namespace,
					Name:      fmt.Sprintf("%s-config-data", cell1.NoVNCProxyName.Name),
				},
			)
			Expect(configDataMap).ShouldNot(BeNil())
			Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
			configData := string(configDataMap.Data["01-nova.conf"])
			Expect(configData).Should(ContainSubstring("ssl_only=true"))
			Expect(configData).Should(ContainSubstring("cert=/etc/pki/tls/certs/nova-novncproxy.crt"))
			Expect(configData).Should(ContainSubstring("key=/etc/pki/tls/private/nova-novncproxy.key"))
		})
	})
})
