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

var _ = Describe("NovaCompute controller", func() {
	When("with standard spec without network interface", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaMessageBusSecret(novaNames.NovaComputeName.Namespace, MessageBusSecretName))

			spec := GetDefaultNovaComputeSpec()
			spec["customServiceConfig"] = "foo=bar"
			DeferCleanup(th.DeleteInstance, CreateNovaCompute(novaNames.NovaComputeName, spec))
		})
		When("a NovaCompute CR is created pointing to a non existent Secret", func() {

			It("is not Ready", func() {
				th.ExpectCondition(
					novaNames.NovaComputeName,
					ConditionGetterFunc(NovaComputeConditionGetter),
					condition.ReadyCondition, corev1.ConditionFalse,
				)
			})

			It("has empty Status fields", func() {
				instance := GetNovaCompute(novaNames.NovaComputeName)
				// NOTE(gibi): Hash has `omitempty` tags so while
				// they are initialized to an empty map that value is omitted from
				// the output when sent to the client. So we see nils here.
				Expect(instance.Status.Hash).To(BeEmpty())
				Expect(instance.Status.ReadyCount).To(Equal(int32(0)))
			})
			It("is missing the secret", func() {
				th.ExpectCondition(
					novaNames.NovaComputeName,
					ConditionGetterFunc(NovaComputeConditionGetter),
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
						Namespace: novaNames.NovaComputeName.Namespace,
					},
				}
				Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
				DeferCleanup(k8sClient.Delete, ctx, secret)
			})

			It("is not Ready", func() {
				th.ExpectCondition(
					novaNames.NovaComputeName,
					ConditionGetterFunc(NovaComputeConditionGetter),
					condition.ReadyCondition,
					corev1.ConditionFalse,
				)
			})

			It("is missing the secret", func() {
				th.ExpectCondition(
					novaNames.NovaComputeName,
					ConditionGetterFunc(NovaComputeConditionGetter),
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
						Namespace: novaNames.NovaComputeName.Namespace,
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
					novaNames.NovaComputeName,
					ConditionGetterFunc(NovaComputeConditionGetter),
					condition.ReadyCondition,
					corev1.ConditionFalse,
				)
			})

		})

		When("the Secret is created with all the expected fields", func() {
			BeforeEach(func() {
				DeferCleanup(
					k8sClient.Delete,
					ctx,
					CreateNovaComputeSecret(novaNames.NovaComputeName.Namespace, SecretName),
				)
			})

			It("reports that input is ready", func() {
				th.ExpectCondition(
					novaNames.NovaComputeName,
					ConditionGetterFunc(NovaComputeConditionGetter),
					condition.InputReadyCondition,
					corev1.ConditionTrue,
				)
			})
			It("generated configs successfully", func() {
				th.ExpectCondition(
					novaNames.NovaComputeName,
					ConditionGetterFunc(NovaComputeConditionGetter),
					condition.ServiceConfigReadyCondition,
					corev1.ConditionTrue,
				)

				configDataMap := th.GetSecret(novaNames.NovaComputeConfigDataName)
				Expect(configDataMap).ShouldNot(BeNil())
				Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
				configData := string(configDataMap.Data["01-nova.conf"])
				Expect(configData).Should(ContainSubstring("transport_url=rabbit://rabbitmq-secret/fake"))
				Expect(configData).Should(ContainSubstring("password = service-password"))
				Expect(configData).Should(ContainSubstring("compute_driver = ironic.IronicDriver"))
				Expect(configDataMap.Data).Should(HaveKey("02-nova-override.conf"))
				extraData := string(configDataMap.Data["02-nova-override.conf"])
				Expect(extraData).To(Equal("foo=bar"))
			})

			It("stored the input hash in the Status", func() {
				Eventually(func(g Gomega) {
					novaCompute := GetNovaCompute(novaNames.NovaComputeName)
					g.Expect(novaCompute.Status.Hash).Should(HaveKeyWithValue("input", Not(BeEmpty())))
				}, timeout, interval).Should(Succeed())

			})
		})

		When("NovaCompute is created with a proper Secret", func() {
			BeforeEach(func() {
				DeferCleanup(
					k8sClient.Delete, ctx, CreateNovaComputeSecret(novaNames.NovaComputeName.Namespace, SecretName))
			})

			It(" reports input ready", func() {
				th.ExpectCondition(
					novaNames.NovaComputeName,
					ConditionGetterFunc(NovaComputeConditionGetter),
					condition.InputReadyCondition,
					corev1.ConditionTrue,
				)
			})

			It("creates a StatefulSet for the nova-compute service", func() {
				th.ExpectConditionWithDetails(
					novaNames.NovaComputeName,
					ConditionGetterFunc(NovaComputeConditionGetter),
					condition.DeploymentReadyCondition,
					corev1.ConditionFalse,
					condition.RequestedReason,
					condition.DeploymentReadyRunningMessage,
				)

				ss := th.GetStatefulSet(novaNames.NovaComputeStatefulSetName)
				Expect(int(*ss.Spec.Replicas)).To(Equal(1))
				Expect(ss.Spec.Template.Spec.Volumes).To(HaveLen(2))
				Expect(ss.Spec.Template.Spec.Containers).To(HaveLen(2))
				Expect(ss.Spec.Selector.MatchLabels).To(Equal(map[string]string{"service": "nova-compute", "cell": "cell1"}))

				container := ss.Spec.Template.Spec.Containers[0]
				Expect(container.VolumeMounts).To(HaveLen(1))
				Expect(container.Image).To(Equal(ContainerImage))

				container = ss.Spec.Template.Spec.Containers[1]
				Expect(container.VolumeMounts).To(HaveLen(2))
				Expect(container.Image).To(Equal(ContainerImage))

			})

			When("the StatefulSet has at least one Replica ready", func() {
				BeforeEach(func() {
					th.ExpectConditionWithDetails(
						novaNames.NovaComputeName,
						ConditionGetterFunc(NovaComputeConditionGetter),
						condition.DeploymentReadyCondition,
						corev1.ConditionFalse,
						condition.RequestedReason,
						condition.DeploymentReadyRunningMessage,
					)
					th.SimulateStatefulSetReplicaReady(novaNames.NovaComputeStatefulSetName)
				})

				It("reports that the StatefulSet is ready", func() {
					th.GetStatefulSet(novaNames.NovaComputeStatefulSetName)
					th.ExpectCondition(
						novaNames.NovaComputeName,
						ConditionGetterFunc(NovaComputeConditionGetter),
						condition.DeploymentReadyCondition,
						corev1.ConditionTrue,
					)

					novaCompute := GetNovaCompute(novaNames.NovaComputeName)
					Expect(novaCompute.Status.ReadyCount).To(BeNumerically(">", 0))
				})
			})

			It("is Ready", func() {
				th.SimulateStatefulSetReplicaReady(novaNames.NovaComputeStatefulSetName)

				th.ExpectCondition(
					novaNames.NovaComputeName,
					ConditionGetterFunc(NovaComputeConditionGetter),
					condition.ReadyCondition,
					corev1.ConditionTrue,
				)
			})
		})
	})
})

var _ = Describe("NovaCompute with ironic diver controller", func() {
	BeforeEach(func() {
		DeferCleanup(
			k8sClient.Delete, ctx, CreateNovaMessageBusSecret(novaNames.NovaComputeName.Namespace, MessageBusSecretName))
	})

	When("with configure cellname", func() {
		BeforeEach(func() {
			spec := GetDefaultNovaComputeSpec()
			spec["cellName"] = "some-cell-name"
			novaCompute := CreateNovaCompute(novaNames.NovaComputeName, spec)
			DeferCleanup(th.DeleteInstance, novaCompute)
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateNovaComputeSecret(novaNames.NovaComputeName.Namespace, SecretName),
			)
		})
	})

	When("NovaCompute is created with networkAttachments", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaComputeSecret(novaNames.NovaComputeName.Namespace, SecretName))

			spec := GetDefaultNovaComputeSpec()
			spec["networkAttachments"] = []string{"internalapi"}
			DeferCleanup(th.DeleteInstance, CreateNovaCompute(novaNames.NovaComputeName, spec))
		})

		It("reports that the definition is missing", func() {
			th.ExpectConditionWithDetails(
				novaNames.NovaComputeName,
				ConditionGetterFunc(NovaComputeConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NetworkAttachment resources missing: internalapi",
			)
			th.ExpectCondition(
				novaNames.NovaComputeName,
				ConditionGetterFunc(NovaComputeConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})
		It("reports that network attachment is missing", func() {
			internalNovaComputeName := types.NamespacedName{Namespace: novaNames.NovaComputeName.Namespace, Name: "internalapi"}
			nad := th.CreateNetworkAttachmentDefinition(internalNovaComputeName)
			DeferCleanup(th.DeleteInstance, nad)

			ss := th.GetStatefulSet(novaNames.NovaComputeStatefulSetName)

			expectedAnnotation, err := json.Marshal(
				[]networkv1.NetworkSelectionElement{
					{
						Name:             "internalapi",
						Namespace:        novaNames.NovaComputeName.Namespace,
						InterfaceRequest: "internalapi",
					}})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(ss.Spec.Template.ObjectMeta.Annotations).To(
				HaveKeyWithValue("k8s.v1.cni.cncf.io/networks", string(expectedAnnotation)),
			)

			// We don't add network attachment status annotations to the Pods
			// to simulate that the network attachments are missing.
			th.SimulateStatefulSetReplicaReadyWithPods(novaNames.NovaComputeStatefulSetName, map[string][]string{})

			th.ExpectConditionWithDetails(
				novaNames.NovaComputeName,
				ConditionGetterFunc(NovaComputeConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occurred "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
			)
		})
		It("reports that an IP is missing", func() {
			internalNovaComputeName := types.NamespacedName{Namespace: novaNames.NovaComputeName.Namespace, Name: "internalapi"}
			nad := th.CreateNetworkAttachmentDefinition(internalNovaComputeName)
			DeferCleanup(th.DeleteInstance, nad)

			ss := th.GetStatefulSet(novaNames.NovaComputeStatefulSetName)

			expectedAnnotation, err := json.Marshal(
				[]networkv1.NetworkSelectionElement{
					{
						Name:             "internalapi",
						Namespace:        novaNames.NovaComputeName.Namespace,
						InterfaceRequest: "internalapi",
					}})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(ss.Spec.Template.ObjectMeta.Annotations).To(
				HaveKeyWithValue("k8s.v1.cni.cncf.io/networks", string(expectedAnnotation)),
			)

			// We simulate that there is no IP associated with the internalapi
			// network attachment
			th.SimulateStatefulSetReplicaReadyWithPods(
				novaNames.NovaComputeStatefulSetName,
				map[string][]string{novaNames.NovaComputeName.Namespace + "/internalapi": {}},
			)

			th.ExpectConditionWithDetails(
				novaNames.NovaComputeName,
				ConditionGetterFunc(NovaComputeConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occurred "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
			)
		})
		It("reports NetworkAttachmentsReady if the Pods got the proper annotations", func() {
			internalNovaComputeName := types.NamespacedName{Namespace: novaNames.NovaComputeName.Namespace, Name: "internalapi"}
			nad := th.CreateNetworkAttachmentDefinition(internalNovaComputeName)
			DeferCleanup(th.DeleteInstance, nad)

			th.SimulateStatefulSetReplicaReadyWithPods(
				novaNames.NovaComputeStatefulSetName,
				map[string][]string{novaNames.NovaComputeName.Namespace + "/internalapi": {"10.0.0.1"}},
			)

			th.ExpectCondition(
				novaNames.NovaComputeName,
				ConditionGetterFunc(NovaComputeConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionTrue,
			)

			Eventually(func(g Gomega) {
				instance := GetNovaCompute(novaNames.NovaComputeName)
				g.Expect(instance.Status.NetworkAttachments).To(
					Equal(map[string][]string{novaNames.NovaComputeName.Namespace + "/internalapi": {"10.0.0.1"}}))

			}, timeout, interval).Should(Succeed())

			th.ExpectCondition(
				novaNames.NovaComputeName,
				ConditionGetterFunc(NovaComputeConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})

	When("NovaCompute with ironic diver is reconfigured", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaComputeSecret(novaNames.NovaComputeName.Namespace, SecretName))

			novaCompute := CreateNovaCompute(novaNames.NovaComputeName, GetDefaultNovaComputeSpec())
			DeferCleanup(th.DeleteInstance, novaCompute)

			th.ExpectCondition(
				novaNames.NovaComputeName,
				ConditionGetterFunc(NovaComputeConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)

			th.SimulateStatefulSetReplicaReady(novaNames.NovaComputeStatefulSetName)
			th.ExpectCondition(
				novaNames.NovaComputeName,
				ConditionGetterFunc(NovaComputeConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("applies new NetworkAttachments configuration", func() {
			Eventually(func(g Gomega) {
				novaCompute := GetNovaCompute(novaNames.NovaComputeName)
				novaCompute.Spec.NetworkAttachments = append(novaCompute.Spec.NetworkAttachments, "internalapi")

				g.Expect(k8sClient.Update(ctx, novaCompute)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			th.ExpectConditionWithDetails(
				novaNames.NovaComputeName,
				ConditionGetterFunc(NovaComputeConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NetworkAttachment resources missing: internalapi",
			)
			th.ExpectConditionWithDetails(
				novaNames.NovaComputeName,
				ConditionGetterFunc(NovaComputeConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NetworkAttachment resources missing: internalapi",
			)

			DeferCleanup(th.DeleteInstance, th.CreateNetworkAttachmentDefinition(novaNames.InternalAPINetworkNADName))

			th.ExpectConditionWithDetails(
				novaNames.NovaComputeName,
				ConditionGetterFunc(NovaComputeConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occurred "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
			)
			th.ExpectConditionWithDetails(
				novaNames.NovaComputeName,
				ConditionGetterFunc(NovaComputeConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occurred "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
			)

			th.SimulateStatefulSetReplicaReadyWithPods(
				novaNames.NovaComputeStatefulSetName,
				map[string][]string{novaNames.NovaComputeName.Namespace + "/internalapi": {"10.0.0.1"}},
			)

			th.ExpectCondition(
				novaNames.NovaComputeName,
				ConditionGetterFunc(NovaComputeConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionTrue,
			)

			Eventually(func(g Gomega) {
				novaCompute := GetNovaCompute(novaNames.NovaComputeName)
				g.Expect(novaCompute.Status.NetworkAttachments).To(
					Equal(map[string][]string{novaNames.NovaComputeName.Namespace + "/internalapi": {"10.0.0.1"}}))

			}, timeout, interval).Should(Succeed())

			th.ExpectCondition(
				novaNames.NovaComputeName,
				ConditionGetterFunc(NovaComputeConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})

	When("starts zero replicas", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaComputeSecret(novaNames.NovaComputeName.Namespace, SecretName))

			spec := GetDefaultNovaComputeSpec()
			spec["replicas"] = 0
			novaCompute := CreateNovaCompute(novaNames.NovaComputeName, spec)
			DeferCleanup(th.DeleteInstance, novaCompute)
		})
		It("and deployment is Ready", func() {
			ss := th.GetStatefulSet(novaNames.NovaComputeStatefulSetName)
			Expect(int(*ss.Spec.Replicas)).To(Equal(0))
			th.ExpectCondition(
				novaNames.NovaComputeName,
				ConditionGetterFunc(NovaComputeConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionTrue,
			)

		})
	})

	When("NovaCompute CR with ironic diver is created without container image defined", func() {
		BeforeEach(func() {
			spec := GetDefaultNovaComputeSpec()
			spec["containerImage"] = ""
			novaCompute := CreateNovaCompute(novaNames.NovaComputeName, spec)
			DeferCleanup(th.DeleteInstance, novaCompute)
		})
		It("has the expected container image default", func() {
			novaComputeDefault := GetNovaCompute(novaNames.NovaComputeName)
			Expect(novaComputeDefault.Spec.ContainerImage).To(Equal(util.GetEnvVar("NOVA_COMPUTE_IRONIC_IMAGE_URL_DEFAULT", novav1.NovaIronicComputeContainerImage)))
		})
	})
})
