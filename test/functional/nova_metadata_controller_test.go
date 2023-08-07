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
	"encoding/json"

	networkv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
)

var _ = Describe("NovaMetadata controller", func() {
	When("with standard spec without network interface", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaMessageBusSecret(novaNames.MetadataName.Namespace, MessageBusSecretName))

			spec := GetDefaultNovaMetadataSpec()
			spec["customServiceConfig"] = "foo=bar"
			DeferCleanup(th.DeleteInstance, CreateNovaMetadata(novaNames.MetadataName, spec))
		})
		When("a NovaMetadata CR is created pointing to a non existent Secret", func() {

			It("is not Ready", func() {
				th.ExpectCondition(
					novaNames.MetadataName,
					ConditionGetterFunc(NovaMetadataConditionGetter),
					condition.ReadyCondition, corev1.ConditionFalse,
				)
			})

			It("has empty Status fields", func() {
				instance := GetNovaMetadata(novaNames.MetadataName)
				// NOTE(gibi): Hash has `omitempty` tags so while
				// they are initialized to an empty map that value is omitted from
				// the output when sent to the client. So we see nils here.
				Expect(instance.Status.Hash).To(BeEmpty())
				Expect(instance.Status.ReadyCount).To(Equal(int32(0)))
			})
			It("is missing the secret", func() {
				th.ExpectCondition(
					novaNames.MetadataName,
					ConditionGetterFunc(NovaMetadataConditionGetter),
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
						Namespace: novaNames.MetadataName.Namespace,
					},
				}
				Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
				DeferCleanup(k8sClient.Delete, ctx, secret)
			})

			It("is not Ready", func() {
				th.ExpectCondition(
					novaNames.MetadataName,
					ConditionGetterFunc(NovaMetadataConditionGetter),
					condition.ReadyCondition,
					corev1.ConditionFalse,
				)
			})

			It("is missing the secret", func() {
				th.ExpectCondition(
					novaNames.MetadataName,
					ConditionGetterFunc(NovaMetadataConditionGetter),
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
						Namespace: novaNames.MetadataName.Namespace,
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
					novaNames.MetadataName,
					ConditionGetterFunc(NovaMetadataConditionGetter),
					condition.ReadyCondition,
					corev1.ConditionFalse,
				)
			})

			It("reports that the inputs are not ready", func() {
				th.ExpectCondition(
					novaNames.MetadataName,
					ConditionGetterFunc(NovaMetadataConditionGetter),
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
					CreateNovaMetadataSecret(novaNames.MetadataName.Namespace, SecretName),
				)
			})

			It("reports that input is ready", func() {
				th.ExpectCondition(
					novaNames.MetadataName,
					ConditionGetterFunc(NovaMetadataConditionGetter),
					condition.InputReadyCondition,
					corev1.ConditionTrue,
				)
			})
			It("generated configs successfully", func() {
				th.ExpectCondition(
					novaNames.MetadataName,
					ConditionGetterFunc(NovaMetadataConditionGetter),
					condition.ServiceConfigReadyCondition,
					corev1.ConditionTrue,
				)

				configDataMap := th.GetSecret(novaNames.MetadataConfigDataName)
				Expect(configDataMap).ShouldNot(BeNil())
				Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
				configData := string(configDataMap.Data["01-nova.conf"])
				Expect(configData).Should(ContainSubstring("transport_url=rabbit://rabbitmq-secret/fake"))
				Expect(configData).Should(ContainSubstring("password = service-password"))
				Expect(configData).Should(ContainSubstring("metadata_proxy_shared_secret = metadata-secret"))
				Expect(configData).Should(ContainSubstring("local_metadata_per_cell = false"))
				Expect(configData).Should(ContainSubstring("enabled_apis=metadata"))
				Expect(configData).Should(ContainSubstring("metadata_workers=1"))
				Expect(configDataMap.Data).Should(HaveKey("02-nova-override.conf"))
				extraData := string(configDataMap.Data["02-nova-override.conf"])
				Expect(extraData).To(Equal("foo=bar"))
			})

			It("stored the input hash in the Status", func() {
				Eventually(func(g Gomega) {
					novaMetadata := GetNovaMetadata(novaNames.MetadataName)
					g.Expect(novaMetadata.Status.Hash).Should(HaveKeyWithValue("input", Not(BeEmpty())))
				}, timeout, interval).Should(Succeed())

			})
		})

		When("NovaMetadata is created with a proper Secret", func() {
			BeforeEach(func() {
				DeferCleanup(
					k8sClient.Delete, ctx, CreateNovaMetadataSecret(novaNames.MetadataName.Namespace, SecretName))
			})

			It(" reports input ready", func() {
				th.ExpectCondition(
					novaNames.MetadataName,
					ConditionGetterFunc(NovaMetadataConditionGetter),
					condition.InputReadyCondition,
					corev1.ConditionTrue,
				)
			})

			It("creates a StatefulSet for the nova-metadata service", func() {
				th.ExpectConditionWithDetails(
					novaNames.MetadataName,
					ConditionGetterFunc(NovaMetadataConditionGetter),
					condition.DeploymentReadyCondition,
					corev1.ConditionFalse,
					condition.RequestedReason,
					condition.DeploymentReadyRunningMessage,
				)

				ss := th.GetStatefulSet(novaNames.MetadataStatefulSetName)
				Expect(int(*ss.Spec.Replicas)).To(Equal(1))
				Expect(ss.Spec.Template.Spec.Volumes).To(HaveLen(2))
				Expect(ss.Spec.Template.Spec.Containers).To(HaveLen(2))
				Expect(ss.Spec.Selector.MatchLabels).To(Equal(map[string]string{"service": "nova-metadata"}))

				container := ss.Spec.Template.Spec.Containers[0]
				Expect(container.VolumeMounts).To(HaveLen(1))
				Expect(container.Image).To(Equal(ContainerImage))

				container = ss.Spec.Template.Spec.Containers[1]
				Expect(container.VolumeMounts).To(HaveLen(2))
				Expect(container.Image).To(Equal(ContainerImage))

				Expect(container.LivenessProbe.HTTPGet.Port.IntVal).To(Equal(int32(8775)))
				Expect(container.ReadinessProbe.HTTPGet.Port.IntVal).To(Equal(int32(8775)))

			})

			When("the StatefulSet has at least one Replica ready", func() {
				BeforeEach(func() {
					th.ExpectConditionWithDetails(
						novaNames.MetadataName,
						ConditionGetterFunc(NovaMetadataConditionGetter),
						condition.DeploymentReadyCondition,
						corev1.ConditionFalse,
						condition.RequestedReason,
						condition.DeploymentReadyRunningMessage,
					)
					th.SimulateStatefulSetReplicaReady(novaNames.MetadataStatefulSetName)
				})

				It("reports that the StatefulSet is ready", func() {
					th.GetStatefulSet(novaNames.MetadataStatefulSetName)
					th.ExpectCondition(
						novaNames.MetadataName,
						ConditionGetterFunc(NovaMetadataConditionGetter),
						condition.DeploymentReadyCondition,
						corev1.ConditionTrue,
					)

					novaMetadata := GetNovaMetadata(novaNames.MetadataName)
					Expect(novaMetadata.Status.ReadyCount).To(BeNumerically(">", 0))
				})
			})

			It("exposes the service", func() {
				th.SimulateStatefulSetReplicaReady(novaNames.MetadataStatefulSetName)
				th.ExpectCondition(
					novaNames.MetadataName,
					ConditionGetterFunc(NovaMetadataConditionGetter),
					condition.ExposeServiceReadyCondition,
					corev1.ConditionTrue,
				)
				service := th.GetService(novaNames.InternalNovaMetadataServiceName)
				Expect(service.Labels["service"]).To(Equal("nova-metadata"))
				Expect(service.Labels).NotTo(HaveKey("cell"))
			})

			It("is Ready", func() {
				th.SimulateStatefulSetReplicaReady(novaNames.MetadataStatefulSetName)

				th.ExpectCondition(
					novaNames.MetadataName,
					ConditionGetterFunc(NovaMetadataConditionGetter),
					condition.ReadyCondition,
					corev1.ConditionTrue,
				)
			})
		})
	})
})

var _ = Describe("NovaMetadata controller", func() {
	BeforeEach(func() {
		DeferCleanup(
			k8sClient.Delete, ctx, CreateNovaMessageBusSecret(novaNames.MetadataName.Namespace, MessageBusSecretName))
	})

	When("with configure cellname", func() {
		BeforeEach(func() {
			spec := GetDefaultNovaMetadataSpec()
			spec["cellName"] = "some-cell-name"
			metadata := CreateNovaMetadata(novaNames.MetadataName, spec)
			DeferCleanup(th.DeleteInstance, metadata)
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateNovaMetadataSecret(novaNames.MetadataName.Namespace, SecretName),
			)
		})
		It("generated config with correct local_metadata_per_cell", func() {
			th.ExpectCondition(
				novaNames.MetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)

			configDataMap := th.GetSecret(novaNames.MetadataConfigDataName)
			Expect(configDataMap).ShouldNot(BeNil())
			Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
			configData := string(configDataMap.Data["01-nova.conf"])
			Expect(configData).Should(
				ContainSubstring("transport_url=rabbit://rabbitmq-secret/fake"))
			Expect(configData).Should(
				ContainSubstring("metadata_proxy_shared_secret = metadata-secret"))
			Expect(configData).Should(
				ContainSubstring("password = service-password"))
			Expect(configData).Should(
				ContainSubstring("local_metadata_per_cell = true"))
			th.ExpectCondition(
				novaNames.MetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.ExposeServiceReadyCondition,
				corev1.ConditionTrue,
			)
			service := th.GetService(types.NamespacedName{Namespace: novaNames.MetadataName.Namespace, Name: "nova-metadata-some-cell-name-internal"})
			Expect(service.Labels["service"]).To(Equal("nova-metadata"))
			Expect(service.Labels["cell"]).To(Equal("some-cell-name"))

			ss := th.GetStatefulSet(novaNames.MetadataStatefulSetName)
			Expect(ss.Spec.Selector.MatchLabels).To(
				Equal(map[string]string{
					"service": "nova-metadata",
					"cell":    "some-cell-name",
				}))
		})
	})

	When("NovaMetadata is created with networkAttachments", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaMetadataSecret(novaNames.MetadataName.Namespace, SecretName))

			spec := GetDefaultNovaMetadataSpec()
			spec["networkAttachments"] = []string{"internalapi"}
			DeferCleanup(th.DeleteInstance, CreateNovaMetadata(novaNames.MetadataName, spec))
		})

		It("reports that the definition is missing", func() {
			th.ExpectConditionWithDetails(
				novaNames.MetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NetworkAttachment resources missing: internalapi",
			)
			th.ExpectCondition(
				novaNames.MetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})
		It("reports that network attachment is missing", func() {
			internalMetadataName := types.NamespacedName{Namespace: novaNames.MetadataName.Namespace, Name: "internalapi"}
			nad := th.CreateNetworkAttachmentDefinition(internalMetadataName)
			DeferCleanup(th.DeleteInstance, nad)

			ss := th.GetStatefulSet(novaNames.MetadataStatefulSetName)

			expectedAnnotation, err := json.Marshal(
				[]networkv1.NetworkSelectionElement{
					{
						Name:             "internalapi",
						Namespace:        novaNames.MetadataName.Namespace,
						InterfaceRequest: "internalapi",
					}})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(ss.Spec.Template.ObjectMeta.Annotations).To(
				HaveKeyWithValue("k8s.v1.cni.cncf.io/networks", string(expectedAnnotation)),
			)

			// We don't add network attachment status annotations to the Pods
			// to simulate that the network attachments are missing.
			th.SimulateStatefulSetReplicaReadyWithPods(novaNames.MetadataStatefulSetName, map[string][]string{})

			th.ExpectConditionWithDetails(
				novaNames.MetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occurred "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
			)
		})
		It("reports that an IP is missing", func() {
			internalMetadataName := types.NamespacedName{Namespace: novaNames.MetadataName.Namespace, Name: "internalapi"}
			nad := th.CreateNetworkAttachmentDefinition(internalMetadataName)
			DeferCleanup(th.DeleteInstance, nad)

			ss := th.GetStatefulSet(novaNames.MetadataStatefulSetName)

			expectedAnnotation, err := json.Marshal(
				[]networkv1.NetworkSelectionElement{
					{
						Name:             "internalapi",
						Namespace:        novaNames.MetadataName.Namespace,
						InterfaceRequest: "internalapi",
					}})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(ss.Spec.Template.ObjectMeta.Annotations).To(
				HaveKeyWithValue("k8s.v1.cni.cncf.io/networks", string(expectedAnnotation)),
			)

			// We simulate that there is no IP associated with the internalapi
			// network attachment
			th.SimulateStatefulSetReplicaReadyWithPods(
				novaNames.MetadataStatefulSetName,
				map[string][]string{novaNames.MetadataName.Namespace + "/internalapi": {}},
			)

			th.ExpectConditionWithDetails(
				novaNames.MetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occurred "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
			)
		})
		It("reports NetworkAttachmentsReady if the Pods got the proper annotations", func() {
			internalMetadataName := types.NamespacedName{Namespace: novaNames.MetadataName.Namespace, Name: "internalapi"}
			nad := th.CreateNetworkAttachmentDefinition(internalMetadataName)
			DeferCleanup(th.DeleteInstance, nad)

			th.SimulateStatefulSetReplicaReadyWithPods(
				novaNames.MetadataStatefulSetName,
				map[string][]string{novaNames.MetadataName.Namespace + "/internalapi": {"10.0.0.1"}},
			)

			th.ExpectCondition(
				novaNames.MetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionTrue,
			)

			Eventually(func(g Gomega) {
				instance := GetNovaMetadata(novaNames.MetadataName)
				g.Expect(instance.Status.NetworkAttachments).To(
					Equal(map[string][]string{novaNames.MetadataName.Namespace + "/internalapi": {"10.0.0.1"}}))

			}, timeout, interval).Should(Succeed())

			th.ExpectCondition(
				novaNames.MetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})
	When("NovaMetadata is created with service override", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaMetadataSecret(novaNames.MetadataName.Namespace, SecretName))

			spec := GetDefaultNovaMetadataSpec()
			serviceOverride := map[string]interface{}{}
			serviceOverride["internal"] = map[string]interface{}{
				"metadata": map[string]map[string]string{
					"annotations": {
						"dnsmasq.network.openstack.org/hostname": "nova-metadata-internal.openstack.svc",
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

			spec["override"] = map[string]interface{}{
				"service": serviceOverride,
			}

			metadata := CreateNovaMetadata(novaNames.MetadataName, spec)
			DeferCleanup(th.DeleteInstance, metadata)
		})

		It("creates LoadBalancer service", func() {
			th.SimulateStatefulSetReplicaReady(novaNames.MetadataStatefulSetName)

			// As the internal endpoint is configured in service override it
			// gets a LoadBalancer Service with MetalLB annotations
			service := th.GetService(novaNames.InternalNovaMetadataServiceName)
			Expect(service.Annotations).To(
				HaveKeyWithValue("dnsmasq.network.openstack.org/hostname", "nova-metadata-internal.openstack.svc"))
			Expect(service.Annotations).To(
				HaveKeyWithValue("metallb.universe.tf/address-pool", "osp-internalapi"))
			Expect(service.Annotations).To(
				HaveKeyWithValue("metallb.universe.tf/allow-shared-ip", "osp-internalapi"))
			Expect(service.Annotations).To(
				HaveKeyWithValue("metallb.universe.tf/loadBalancerIPs", "internal-lb-ip-1,internal-lb-ip-2"))

			th.ExpectCondition(
				novaNames.MetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})
	When("NovaMetadata is reconfigured", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaMetadataSecret(novaNames.MetadataName.Namespace, SecretName))

			metadata := CreateNovaMetadata(novaNames.MetadataName, GetDefaultNovaMetadataSpec())
			DeferCleanup(th.DeleteInstance, metadata)

			th.ExpectCondition(
				novaNames.MetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)

			th.SimulateStatefulSetReplicaReady(novaNames.MetadataStatefulSetName)
			th.ExpectCondition(
				novaNames.MetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("applies new NetworkAttachments configuration", func() {
			Eventually(func(g Gomega) {
				novaMetadata := GetNovaMetadata(novaNames.MetadataName)
				novaMetadata.Spec.NetworkAttachments = append(novaMetadata.Spec.NetworkAttachments, "internalapi")

				g.Expect(k8sClient.Update(ctx, novaMetadata)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			th.ExpectConditionWithDetails(
				novaNames.MetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NetworkAttachment resources missing: internalapi",
			)
			th.ExpectConditionWithDetails(
				novaNames.MetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NetworkAttachment resources missing: internalapi",
			)

			DeferCleanup(th.DeleteInstance, th.CreateNetworkAttachmentDefinition(novaNames.InternalAPINetworkNADName))

			th.ExpectConditionWithDetails(
				novaNames.MetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occurred "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
			)
			th.ExpectConditionWithDetails(
				novaNames.MetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occurred "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
			)

			th.SimulateStatefulSetReplicaReadyWithPods(
				novaNames.MetadataStatefulSetName,
				map[string][]string{novaNames.MetadataName.Namespace + "/internalapi": {"10.0.0.1"}},
			)

			th.ExpectCondition(
				novaNames.MetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionTrue,
			)

			Eventually(func(g Gomega) {
				novaMetadata := GetNovaMetadata(novaNames.MetadataName)
				g.Expect(novaMetadata.Status.NetworkAttachments).To(
					Equal(map[string][]string{novaNames.MetadataName.Namespace + "/internalapi": {"10.0.0.1"}}))

			}, timeout, interval).Should(Succeed())

			th.ExpectCondition(
				novaNames.MetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("applies new RegisteredCells input to its StatefulSet to trigger Pod restart", func() {
			originalConfigHash := GetEnvVarValue(
				th.GetStatefulSet(novaNames.MetadataStatefulSetName).Spec.Template.Spec.Containers[0].Env, "CONFIG_HASH", "")

			// Simulate that a new cell is added and Nova controller registered it and
			// therefore a new cell is added to RegisteredCells
			Eventually(func(g Gomega) {
				novaMetadata := GetNovaMetadata(novaNames.MetadataName)
				novaMetadata.Spec.RegisteredCells = map[string]string{"cell0": "cell0-config-hash"}
				g.Expect(k8sClient.Update(ctx, novaMetadata)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			// Assert that the CONFIG_HASH of the StateFulSet is changed due to this reconfiguration
			Eventually(func(g Gomega) {
				currentConfigHash := GetEnvVarValue(
					th.GetStatefulSet(novaNames.MetadataStatefulSetName).Spec.Template.Spec.Containers[0].Env, "CONFIG_HASH", "")
				g.Expect(originalConfigHash).NotTo(Equal(currentConfigHash))

			}, timeout, interval).Should(Succeed())
		})
	})

	When("starts zero replicas", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaMetadataSecret(novaNames.MetadataName.Namespace, SecretName))

			spec := GetDefaultNovaMetadataSpec()
			spec["replicas"] = 0
			metadata := CreateNovaMetadata(novaNames.MetadataName, spec)
			DeferCleanup(th.DeleteInstance, metadata)
		})
		It("and deployment is Ready", func() {
			ss := th.GetStatefulSet(novaNames.MetadataStatefulSetName)
			Expect(int(*ss.Spec.Replicas)).To(Equal(0))
			th.ExpectCondition(
				novaNames.MetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionTrue,
			)

		})
	})

	When("NovaMetadata CR is created without container image defined", func() {
		BeforeEach(func() {
			spec := GetDefaultNovaMetadataSpec()
			spec["containerImage"] = ""
			metadata := CreateNovaMetadata(novaNames.MetadataName, spec)
			DeferCleanup(th.DeleteInstance, metadata)
		})
		It("has the expected container image default", func() {
			novaMetadataDefault := GetNovaMetadata(novaNames.MetadataName)
			Expect(novaMetadataDefault.Spec.ContainerImage).To(Equal(util.GetEnvVar("RELATED_IMAGE_NOVA_METADATA_IMAGE_URL_DEFAULT", novav1.NovaMetadataContainerImage)))
		})
	})
})
