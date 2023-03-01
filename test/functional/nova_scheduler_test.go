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
	"os"

	"github.com/google/uuid"
	networkv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/openstack-k8s-operators/lib-common/modules/test/helpers"
)

var _ = Describe("NovaScheduler controller", func() {
	var namespace string
	var novaSchedulerName types.NamespacedName

	BeforeEach(func() {
		// NOTE(gibi): We need to create a unique namespace for each test run
		// as namespaces cannot be deleted in a locally running envtest. See
		// https://book.kubebuilder.io/reference/envtest.html#namespace-usage-limitation
		namespace = uuid.New().String()
		th.CreateNamespace(namespace)
		// We still request the delete of the Namespace to properly cleanup if
		// we run the test in an existing cluster.
		DeferCleanup(th.DeleteNamespace, namespace)
		// NOTE(gibi): ConfigMap generation looks up the local templates
		// directory via ENV, so provide it
		DeferCleanup(os.Setenv, "OPERATOR_TEMPLATES", os.Getenv("OPERATOR_TEMPLATES"))
		os.Setenv("OPERATOR_TEMPLATES", "../../templates")

		// Uncomment this if you need the full output in the logs from gomega
		// matchers
		// format.MaxLength = 0
		DeferCleanup(
			k8sClient.Delete, ctx, CreateNovaMessageBusSecret(namespace, MessageBusSecretName))
		spec := GetDefaultNovaSchedulerSpec()
		spec["customServiceConfig"] = "foo=bar"
		instance := CreateNovaScheduler(namespace, spec)
		novaSchedulerName = types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
		DeferCleanup(DeleteInstance, instance)

	})

	When("a NovaScheduler CR is created pointing to a non existent Secret", func() {
		It("is not Ready", func() {
			th.ExpectCondition(
				novaSchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("has empty Status fields", func() {
			instance := GetNovaScheduler(novaSchedulerName)
			// NOTE(gibi): Hash and Endpoints have `omitempty` tags so while
			// they are initialized to {} that value is omited from the output
			// when sent to the client. So we see nils here.
			Expect(instance.Status.Hash).To(BeEmpty())
			Expect(instance.Status.ReadyCount).To(Equal(int32(0)))
		})

		It("is missing the secret", func() {
			th.ExpectCondition(
				novaSchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionFalse,
			)
		})

	})

	When("an unrealated Secret is created the CR state does not change", func() {
		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "not-relevant-secret",
					Namespace: namespace,
				},
			}
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
			DeferCleanup(k8sClient.Delete, ctx, secret)
		})

		It("is not Ready", func() {
			th.ExpectCondition(
				novaSchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("is missing the secret", func() {
			th.ExpectCondition(
				novaSchedulerName,
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
					Namespace: namespace,
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
				novaSchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("reports that the inputes are not ready", func() {
			th.ExpectCondition(
				novaSchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionFalse,
			)
		})
	})

	When("the Secret is created with all the expected fields", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaAPISecret(namespace, SecretName))
		})

		It("reports that input is ready", func() {
			th.ExpectCondition(
				novaSchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("generated configs successfully", func() {
			// NOTE(gibi): NovaScheduler has no external dependency right now to
			// generate the configs.
			th.ExpectCondition(
				novaSchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)

			configDataMap := th.GetConfigMap(
				types.NamespacedName{
					Namespace: namespace,
					Name:      fmt.Sprintf("%s-config-data", novaSchedulerName.Name),
				},
			)
			Expect(configDataMap).ShouldNot(BeNil())
			Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
			Expect(configDataMap.Data).Should(
				HaveKeyWithValue("01-nova.conf",
					ContainSubstring("transport_url=rabbit://fake")))
			Expect(configDataMap.Data).Should(
				HaveKeyWithValue("02-nova-override.conf", "foo=bar"))
		})

		It("stored the input hash in the Status", func() {
			Eventually(func(g Gomega) {
				novaScheduler := GetNovaScheduler(novaSchedulerName)
				g.Expect(novaScheduler.Status.Hash).Should(HaveKeyWithValue("input", Not(BeEmpty())))
			}, timeout, interval).Should(Succeed())

		})
	})

	When("the NovaScheduler is deleted", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaAPISecret(namespace, SecretName))
		})
		It("deletes the generated ConfigMaps", func() {
			th.ExpectCondition(
				novaSchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)

			DeleteInstance(GetNovaScheduler(novaSchedulerName))

			Eventually(func() []corev1.ConfigMap {
				return th.ListConfigMaps(novaSchedulerName.Name).Items
			}, timeout, interval).Should(BeEmpty())
		})
	})

	When("the NovaScheduler is created with a proper Secret", func() {
		var statefulSetName types.NamespacedName

		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaAPISecret(namespace, SecretName))
			statefulSetName = types.NamespacedName{
				Namespace: namespace,
				Name:      novaSchedulerName.Name,
			}
		})

		It(" reports input ready", func() {
			th.ExpectCondition(
				novaSchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("creates a StatefulSet for the nova-scheduler service", func() {
			th.ExpectConditionWithDetails(
				novaSchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				condition.DeploymentReadyRunningMessage,
			)

			ss := th.GetStatefulSet(statefulSetName)
			Expect(int(*ss.Spec.Replicas)).To(Equal(1))
			Expect(ss.Spec.Template.Spec.Volumes).To(HaveLen(2))
			Expect(ss.Spec.Template.Spec.Containers).To(HaveLen(1))

			container := ss.Spec.Template.Spec.Containers[0]
			Expect(container.VolumeMounts).To(HaveLen(2))
			Expect(container.Image).To(Equal(ContainerImage))
		})

		When("the StatefulSet has at least one Replica ready", func() {
			BeforeEach(func() {
				th.ExpectConditionWithDetails(
					novaSchedulerName,
					ConditionGetterFunc(NovaSchedulerConditionGetter),
					condition.DeploymentReadyCondition,
					corev1.ConditionFalse,
					condition.RequestedReason,
					condition.DeploymentReadyRunningMessage,
				)
				th.SimulateStatefulSetReplicaReady(statefulSetName)
			})

			It("reports that the StatefulSet is ready", func() {
				th.ExpectCondition(
					novaSchedulerName,
					ConditionGetterFunc(NovaSchedulerConditionGetter),
					condition.DeploymentReadyCondition,
					corev1.ConditionTrue,
				)

				novaScheduler := GetNovaScheduler(novaSchedulerName)
				Expect(novaScheduler.Status.ReadyCount).To(BeNumerically(">", 0))
			})
		})

		It("is Ready", func() {
			th.SimulateStatefulSetReplicaReady(statefulSetName)
			th.ExpectCondition(
				novaSchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})
	When("NovaScheduler is created with networkAttachments", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaAPISecret(namespace, SecretName))

			spec := GetDefaultNovaSchedulerSpec()
			spec["networkAttachments"] = []string{"internalapi"}
			instance := CreateNovaScheduler(namespace, spec)
			novaSchedulerName = types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
			DeferCleanup(DeleteInstance, instance)
		})

		It("reports that the definition is missing", func() {
			th.ExpectConditionWithDetails(
				novaSchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NetworkAttachment resources missing: internalapi",
			)
			th.ExpectCondition(
				novaSchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})
		It("reports that network attachment is missing", func() {
			internalAPINADName := types.NamespacedName{Namespace: namespace, Name: "internalapi"}
			nad := CreateNetworkAttachmentDefinition(internalAPINADName)
			DeferCleanup(DeleteInstance, nad)

			statefulSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      novaSchedulerName.Name,
			}
			ss := th.GetStatefulSet(statefulSetName)

			expectedAnnotation, err := json.Marshal(
				[]networkv1.NetworkSelectionElement{
					{
						Name:      "internalapi",
						Namespace: namespace,
					}})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(ss.Spec.Template.ObjectMeta.Annotations).To(
				HaveKeyWithValue("k8s.v1.cni.cncf.io/networks", string(expectedAnnotation)),
			)

			// We don't add network attachment status annotations to the Pods
			// to simulate that the network attachments are missing.
			SimulateStatefulSetReplicaReadyWithPods(statefulSetName, map[string][]string{})

			th.ExpectConditionWithDetails(
				novaSchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occured "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
			)
		})
		It("reports that an IP is missing", func() {
			internalAPINADName := types.NamespacedName{Namespace: namespace, Name: "internalapi"}
			nad := CreateNetworkAttachmentDefinition(internalAPINADName)
			DeferCleanup(DeleteInstance, nad)

			statefulSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      novaSchedulerName.Name,
			}
			ss := th.GetStatefulSet(statefulSetName)

			expectedAnnotation, err := json.Marshal(
				[]networkv1.NetworkSelectionElement{
					{
						Name:      "internalapi",
						Namespace: namespace,
					}})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(ss.Spec.Template.ObjectMeta.Annotations).To(
				HaveKeyWithValue("k8s.v1.cni.cncf.io/networks", string(expectedAnnotation)),
			)

			// We simulat that there is no IP associated with the internalapi
			// network attachment
			SimulateStatefulSetReplicaReadyWithPods(
				statefulSetName,
				map[string][]string{namespace + "/internalapi": {}},
			)

			th.ExpectConditionWithDetails(
				novaSchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occured "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
			)
		})
		It("reports NetworkAttachmentsReady if the Pods got the proper annotiations", func() {
			internalAPINADName := types.NamespacedName{Namespace: namespace, Name: "internalapi"}
			nad := CreateNetworkAttachmentDefinition(internalAPINADName)
			DeferCleanup(DeleteInstance, nad)

			statefulSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      novaSchedulerName.Name,
			}
			SimulateStatefulSetReplicaReadyWithPods(
				statefulSetName,
				map[string][]string{namespace + "/internalapi": {"10.0.0.1"}},
			)

			th.ExpectCondition(
				novaSchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionTrue,
			)

			Eventually(func(g Gomega) {
				novaScheduler := GetNovaScheduler(novaSchedulerName)
				g.Expect(novaScheduler.Status.NetworkAttachments).To(
					Equal(map[string][]string{namespace + "/internalapi": {"10.0.0.1"}}))
			}, timeout, interval).Should(Succeed())

			th.ExpectCondition(
				novaSchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})
	When("NovaScheduler is reconfigured", func() {
		var statefulSetName types.NamespacedName

		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaAPISecret(namespace, SecretName))

			instance := CreateNovaScheduler(namespace, GetDefaultNovaSchedulerSpec())
			novaSchedulerName = types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
			DeferCleanup(DeleteInstance, instance)

			th.ExpectCondition(
				novaSchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)

			statefulSetName = types.NamespacedName{
				Namespace: namespace,
				Name:      novaSchedulerName.Name,
			}
			th.SimulateStatefulSetReplicaReady(statefulSetName)
			th.ExpectCondition(
				novaSchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("applys new NetworkAttachments configuration", func() {
			Eventually(func(g Gomega) {
				novaScheduler := GetNovaScheduler(novaSchedulerName)
				novaScheduler.Spec.NetworkAttachments = append(novaScheduler.Spec.NetworkAttachments, "internalapi")

				err := k8sClient.Update(ctx, novaScheduler)
				g.Expect(err == nil || k8s_errors.IsConflict(err)).To(BeTrue())
			}, timeout, interval).Should(Succeed())

			th.ExpectConditionWithDetails(
				novaSchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NetworkAttachment resources missing: internalapi",
			)

			th.ExpectConditionWithDetails(
				novaSchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NetworkAttachment resources missing: internalapi",
			)

			internalAPINADName := types.NamespacedName{Namespace: namespace, Name: "internalapi"}
			DeferCleanup(DeleteInstance, CreateNetworkAttachmentDefinition(internalAPINADName))

			th.ExpectConditionWithDetails(
				novaSchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occured "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
			)

			th.ExpectConditionWithDetails(
				novaSchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occured "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
			)

			SimulateStatefulSetReplicaReadyWithPods(
				statefulSetName,
				map[string][]string{namespace + "/internalapi": {"10.0.0.1"}},
			)

			th.ExpectCondition(
				novaSchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionTrue,
			)

			Eventually(func(g Gomega) {
				novaScheduler := GetNovaScheduler(novaSchedulerName)
				g.Expect(novaScheduler.Status.NetworkAttachments).To(
					Equal(map[string][]string{namespace + "/internalapi": {"10.0.0.1"}}))

			}, timeout, interval).Should(Succeed())

			th.ExpectCondition(
				novaSchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})
})
