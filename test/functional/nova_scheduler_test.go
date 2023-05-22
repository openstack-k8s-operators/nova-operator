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
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/openstack-k8s-operators/lib-common/modules/test/helpers"
)

var _ = Describe("NovaScheduler controller", func() {
	BeforeEach(func() {
		// Uncomment this if you need the full output in the logs from gomega
		// matchers
		// format.MaxLength = 0
		DeferCleanup(
			k8sClient.Delete, ctx, CreateNovaMessageBusSecret(novaNames.SchedulerName.Namespace, MessageBusSecretName))

		spec := GetDefaultNovaSchedulerSpec()
		spec["customServiceConfig"] = "foo=bar"
		DeferCleanup(th.DeleteInstance, CreateNovaScheduler(novaNames.SchedulerName, spec))
	})

	When("a NovaScheduler CR is created pointing to a non existent Secret", func() {
		It("is not Ready", func() {
			th.ExpectCondition(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("has empty Status fields", func() {
			instance := GetNovaScheduler(novaNames.SchedulerName)
			// NOTE(gibi): Hash and Endpoints have `omitempty` tags so while
			// they are initialized to {} that value is omitted from the output
			// when sent to the client. So we see nils here.
			Expect(instance.Status.Hash).To(BeEmpty())
			Expect(instance.Status.ReadyCount).To(Equal(int32(0)))
		})

		It("is missing the secret", func() {
			th.ExpectCondition(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
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
					Namespace: novaNames.SchedulerName.Namespace,
				},
			}
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
			DeferCleanup(k8sClient.Delete, ctx, secret)
		})

		It("is not Ready", func() {
			th.ExpectCondition(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("is missing the secret", func() {
			th.ExpectCondition(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
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
					Namespace: novaNames.SchedulerName.Namespace,
				},
				Data: map[string][]byte{
					"NovaPassword": []byte("12345678"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
			DeferCleanup(k8sClient.Delete, ctx, secret)
		})

		It("is not Ready", func() {
			th.ExpectCondition(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("reports that the inputs are not ready", func() {
			th.ExpectCondition(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionFalse,
			)
		})
	})

	When("the Secret is created with all the expected fields", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaAPISecret(novaNames.SchedulerName.Namespace, SecretName))
		})

		It("reports that input is ready", func() {
			th.ExpectCondition(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("generated configs successfully", func() {
			// NOTE(gibi): NovaScheduler has no external dependency right now to
			// generate the configs.
			th.ExpectCondition(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)

			configDataMap := th.GetConfigMap(
				types.NamespacedName{
					Namespace: novaNames.SchedulerName.Namespace,
					Name:      fmt.Sprintf("%s-config-data", novaNames.SchedulerName.Name),
				},
			)
			Expect(configDataMap).ShouldNot(BeNil())
			Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
			Expect(configDataMap.Data).Should(
				HaveKeyWithValue("01-nova.conf",
					ContainSubstring("transport_url=rabbit://rabbitmq-secret/fake")))
			Expect(configDataMap.Data).Should(
				HaveKeyWithValue("02-nova-override.conf", "foo=bar"))
		})

		It("stored the input hash in the Status", func() {
			Eventually(func(g Gomega) {
				novaScheduler := GetNovaScheduler(novaNames.SchedulerName)
				g.Expect(novaScheduler.Status.Hash).Should(HaveKeyWithValue("input", Not(BeEmpty())))
			}, timeout, interval).Should(Succeed())

		})
	})

	When("the NovaScheduler is deleted", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaAPISecret(novaNames.SchedulerName.Namespace, SecretName))
		})
		It("deletes the generated ConfigMaps", func() {
			th.ExpectCondition(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)

			th.DeleteInstance(GetNovaScheduler(novaNames.SchedulerName))

			Eventually(func() []corev1.ConfigMap {
				return th.ListConfigMaps(novaNames.SchedulerName.Name).Items
			}, timeout, interval).Should(BeEmpty())
		})
	})

	When("the NovaScheduler is created with a proper Secret", func() {

		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaAPISecret(novaNames.SchedulerName.Namespace, SecretName))
		})

		It(" reports input ready", func() {
			th.ExpectCondition(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("creates a StatefulSet for the nova-scheduler service", func() {
			th.ExpectConditionWithDetails(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				condition.DeploymentReadyRunningMessage,
			)

			ss := th.GetStatefulSet(novaNames.SchedulerStatefulSetName)
			Expect(int(*ss.Spec.Replicas)).To(Equal(1))
			Expect(ss.Spec.Template.Spec.Volumes).To(HaveLen(1))
			Expect(ss.Spec.Template.Spec.Containers).To(HaveLen(1))

			container := ss.Spec.Template.Spec.Containers[0]
			Expect(container.VolumeMounts).To(HaveLen(1))
			Expect(container.Image).To(Equal(ContainerImage))
		})

		When("the StatefulSet has at least one Replica ready", func() {
			BeforeEach(func() {
				th.ExpectConditionWithDetails(
					novaNames.SchedulerName,
					ConditionGetterFunc(NovaSchedulerConditionGetter),
					condition.DeploymentReadyCondition,
					corev1.ConditionFalse,
					condition.RequestedReason,
					condition.DeploymentReadyRunningMessage,
				)
				th.SimulateStatefulSetReplicaReady(novaNames.SchedulerStatefulSetName)
			})

			It("reports that the StatefulSet is ready", func() {
				th.ExpectCondition(
					novaNames.SchedulerName,
					ConditionGetterFunc(NovaSchedulerConditionGetter),
					condition.DeploymentReadyCondition,
					corev1.ConditionTrue,
				)

				novaScheduler := GetNovaScheduler(novaNames.SchedulerName)
				Expect(novaScheduler.Status.ReadyCount).To(BeNumerically(">", 0))
			})
		})

		It("is Ready", func() {
			th.SimulateStatefulSetReplicaReady(novaNames.SchedulerStatefulSetName)
			th.ExpectCondition(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})
})

var _ = Describe("NovaScheduler controller", func() {
	BeforeEach(func() {
		DeferCleanup(
			k8sClient.Delete, ctx, CreateNovaMessageBusSecret(novaNames.SchedulerName.Namespace, MessageBusSecretName))
	})

	When("NovaScheduler is created with networkAttachments", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaAPISecret(novaNames.SchedulerName.Namespace, SecretName))

			spec := GetDefaultNovaSchedulerSpec()
			spec["networkAttachments"] = []string{"internalapi"}
			DeferCleanup(th.DeleteInstance, CreateNovaScheduler(novaNames.SchedulerName, spec))
		})

		It("reports that the definition is missing", func() {
			th.ExpectConditionWithDetails(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NetworkAttachment resources missing: internalapi",
			)
			th.ExpectCondition(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})
		It("reports that network attachment is missing", func() {
			internalAPINADName := types.NamespacedName{Namespace: novaNames.SchedulerName.Namespace, Name: "internalapi"}
			nad := th.CreateNetworkAttachmentDefinition(internalAPINADName)
			DeferCleanup(th.DeleteInstance, nad)

			ss := th.GetStatefulSet(novaNames.SchedulerStatefulSetName)

			expectedAnnotation, err := json.Marshal(
				[]networkv1.NetworkSelectionElement{
					{
						Name:             "internalapi",
						Namespace:        novaNames.SchedulerName.Namespace,
						InterfaceRequest: "internalapi",
					}})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(ss.Spec.Template.ObjectMeta.Annotations).To(
				HaveKeyWithValue("k8s.v1.cni.cncf.io/networks", string(expectedAnnotation)),
			)

			// We don't add network attachment status annotations to the Pods
			// to simulate that the network attachments are missing.
			SimulateStatefulSetReplicaReadyWithPods(novaNames.SchedulerStatefulSetName, map[string][]string{})

			th.ExpectConditionWithDetails(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occurred "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
			)
		})
		It("reports that an IP is missing", func() {
			internalAPINADName := types.NamespacedName{Namespace: novaNames.SchedulerName.Namespace, Name: "internalapi"}
			nad := th.CreateNetworkAttachmentDefinition(internalAPINADName)
			DeferCleanup(th.DeleteInstance, nad)

			ss := th.GetStatefulSet(novaNames.SchedulerStatefulSetName)

			expectedAnnotation, err := json.Marshal(
				[]networkv1.NetworkSelectionElement{
					{
						Name:             "internalapi",
						Namespace:        novaNames.SchedulerName.Namespace,
						InterfaceRequest: "internalapi",
					}})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(ss.Spec.Template.ObjectMeta.Annotations).To(
				HaveKeyWithValue("k8s.v1.cni.cncf.io/networks", string(expectedAnnotation)),
			)

			// We simulate that there is no IP associated with the internalapi
			// network attachment
			SimulateStatefulSetReplicaReadyWithPods(
				novaNames.SchedulerStatefulSetName,
				map[string][]string{novaNames.SchedulerName.Namespace + "/internalapi": {}},
			)

			th.ExpectConditionWithDetails(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occurred "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
			)
		})
		It("reports NetworkAttachmentsReady if the Pods got the proper annotations", func() {
			internalAPINADName := types.NamespacedName{Namespace: novaNames.SchedulerName.Namespace, Name: "internalapi"}
			nad := th.CreateNetworkAttachmentDefinition(internalAPINADName)
			DeferCleanup(th.DeleteInstance, nad)

			SimulateStatefulSetReplicaReadyWithPods(
				novaNames.SchedulerStatefulSetName,
				map[string][]string{novaNames.SchedulerName.Namespace + "/internalapi": {"10.0.0.1"}},
			)

			th.ExpectCondition(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionTrue,
			)

			Eventually(func(g Gomega) {
				novaScheduler := GetNovaScheduler(novaNames.SchedulerName)
				g.Expect(novaScheduler.Status.NetworkAttachments).To(
					Equal(map[string][]string{novaNames.SchedulerName.Namespace + "/internalapi": {"10.0.0.1"}}))
			}, timeout, interval).Should(Succeed())

			th.ExpectCondition(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})
	When("NovaScheduler is reconfigured", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaAPISecret(novaNames.SchedulerName.Namespace, SecretName))
			DeferCleanup(th.DeleteInstance, CreateNovaScheduler(novaNames.SchedulerName, GetDefaultNovaSchedulerSpec()))

			th.ExpectCondition(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)

			th.SimulateStatefulSetReplicaReady(novaNames.SchedulerStatefulSetName)
			th.ExpectCondition(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("applies new NetworkAttachments configuration", func() {
			Eventually(func(g Gomega) {
				novaScheduler := GetNovaScheduler(novaNames.SchedulerName)
				novaScheduler.Spec.NetworkAttachments = append(novaScheduler.Spec.NetworkAttachments, "internalapi")

				err := k8sClient.Update(ctx, novaScheduler)
				g.Expect(err == nil || k8s_errors.IsConflict(err)).To(BeTrue())
			}, timeout, interval).Should(Succeed())

			th.ExpectConditionWithDetails(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NetworkAttachment resources missing: internalapi",
			)

			th.ExpectConditionWithDetails(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NetworkAttachment resources missing: internalapi",
			)

			internalAPINADName := types.NamespacedName{Namespace: novaNames.SchedulerName.Namespace, Name: "internalapi"}
			DeferCleanup(th.DeleteInstance, th.CreateNetworkAttachmentDefinition(internalAPINADName))

			th.ExpectConditionWithDetails(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occurred "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
			)

			th.ExpectConditionWithDetails(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occurred "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
			)

			SimulateStatefulSetReplicaReadyWithPods(
				novaNames.SchedulerStatefulSetName,
				map[string][]string{novaNames.SchedulerName.Namespace + "/internalapi": {"10.0.0.1"}},
			)

			th.ExpectCondition(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionTrue,
			)

			Eventually(func(g Gomega) {
				novaScheduler := GetNovaScheduler(novaNames.SchedulerName)
				g.Expect(novaScheduler.Status.NetworkAttachments).To(
					Equal(map[string][]string{novaNames.SchedulerName.Namespace + "/internalapi": {"10.0.0.1"}}))

			}, timeout, interval).Should(Succeed())

			th.ExpectCondition(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("applies new RegisteredCells input to its StatefulSet to trigger Pod restart", func() {
			originalConfigHash := GetEnvVarValue(
				th.GetStatefulSet(novaNames.SchedulerStatefulSetName).Spec.Template.Spec.Containers[0].Env, "CONFIG_HASH", "")

			// Simulate that a new cell is added and Nova controller registered it and
			// therefore a new cell is added to RegisteredCells
			Eventually(func(g Gomega) {
				novaAPI := GetNovaScheduler(novaNames.SchedulerName)
				novaAPI.Spec.RegisteredCells = map[string]string{"cell0": "cell0-config-hash"}
				err := k8sClient.Update(ctx, novaAPI)
				g.Expect(err == nil || k8s_errors.IsConflict(err)).To(BeTrue())
			}, timeout, interval).Should(Succeed())

			// Assert that the CONFIG_HASH of the StateFulSet is changed due to this reconfiguration
			Eventually(func(g Gomega) {
				currentConfigHash := GetEnvVarValue(
					th.GetStatefulSet(novaNames.SchedulerStatefulSetName).Spec.Template.Spec.Containers[0].Env, "CONFIG_HASH", "")
				g.Expect(originalConfigHash).NotTo(Equal(currentConfigHash))

			}, timeout, interval).Should(Succeed())
		})
	})

	When("NovaScheduler CR is created without container image defined", func() {
		BeforeEach(func() {
			spec := GetDefaultNovaSchedulerSpec()
			spec["containerImage"] = ""
			scheduler := CreateNovaScheduler(novaNames.SchedulerName, spec)
			DeferCleanup(th.DeleteInstance, scheduler)
		})
		It("has the expected container image default", func() {
			novaSchedulerDefault := GetNovaScheduler(novaNames.SchedulerName)
			Expect(novaSchedulerDefault.Spec.ContainerImage).To(Equal(util.GetEnvVar("NOVA_SCHEDULER_IMAGE_URL_DEFAULT", novav1.NovaSchedulerContainerImage)))
		})
	})
})
