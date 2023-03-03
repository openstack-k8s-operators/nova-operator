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
	"fmt"
	"os"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/openstack-k8s-operators/lib-common/modules/test/helpers"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
)

var _ = Describe("NovaMetadata controller", func() {
	var namespace string
	var novaMetadataName types.NamespacedName

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
		spec := GetDefaultNovaMetadataSpec()
		spec["customServiceConfig"] = "foo=bar"
		novaMetadataName = CreateNovaMetadata(namespace, spec)
		DeferCleanup(DeleteNovaMetadata, novaMetadataName)

	})

	When("a NovaMetadata CR is created pointing to a non existent Secret", func() {

		It("is not Ready", func() {
			th.ExpectCondition(
				novaMetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.ReadyCondition, corev1.ConditionFalse,
			)
		})

		It("has empty Status fields", func() {
			instance := GetNovaMetadata(novaMetadataName)
			// NOTE(gibi): Hash has `omitempty` tags so while
			// they are initialized to an empty map that value is omited from
			// the output when sent to the client. So we see nils here.
			Expect(instance.Status.Hash).To(BeEmpty())
			Expect(instance.Status.ReadyCount).To(Equal(int32(0)))
		})
		It("is missing the secret", func() {
			th.ExpectCondition(
				novaMetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
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
				novaMetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("is missing the secret", func() {
			th.ExpectCondition(
				novaMetadataName,
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
				novaMetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("reports that the inputes are not ready", func() {
			th.ExpectCondition(
				novaMetadataName,
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
				CreateNovaMetadataSecret(namespace, SecretName),
			)
		})

		It("reports that input is ready", func() {
			th.ExpectCondition(
				novaMetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("generated configs successfully", func() {
			th.ExpectCondition(
				novaMetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)

			configDataMap := th.GetConfigMap(
				types.NamespacedName{
					Namespace: namespace,
					Name:      fmt.Sprintf("%s-config-data", novaMetadataName.Name),
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
				novaMetadata := GetNovaMetadata(novaMetadataName)
				g.Expect(novaMetadata.Status.Hash).Should(HaveKeyWithValue("input", Not(BeEmpty())))
			}, timeout, interval).Should(Succeed())

		})

		When("the NovaMetadata is deleted", func() {
			It("deletes the generated ConfigMaps", func() {
				th.ExpectCondition(
					novaMetadataName,
					ConditionGetterFunc(NovaMetadataConditionGetter),
					condition.ServiceConfigReadyCondition,
					corev1.ConditionTrue,
				)

				DeleteNovaConductor(novaMetadataName)

				Eventually(func() []corev1.ConfigMap {
					return th.ListConfigMaps(novaMetadataName.Name).Items
				}, timeout, interval).Should(BeEmpty())
			})
		})
	})

	When("the NovaMetadata is deleted", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaMetadataSecret(namespace, SecretName))
		})
		It("deletes the generated ConfigMaps", func() {
			th.ExpectCondition(
				novaMetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)

			DeleteNovaScheduler(novaMetadataName)

			Eventually(func() []corev1.ConfigMap {
				return th.ListConfigMaps(novaMetadataName.Name).Items
			}, timeout, interval).Should(BeEmpty())
		})
	})

	When("NovaMetadata is created with a proper Secret", func() {
		var statefulSetName types.NamespacedName

		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaMetadataSecret(namespace, SecretName))

			statefulSetName = types.NamespacedName{
				Namespace: namespace,
				Name:      novaMetadataName.Name,
			}
		})

		It(" reports input ready", func() {
			th.ExpectCondition(
				novaMetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("creates a StatefulSet for the nova-metadata service", func() {
			th.ExpectConditionWithDetails(
				novaMetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				condition.DeploymentReadyRunningMessage,
			)

			ss := th.GetStatefulSet(statefulSetName)
			Expect(int(*ss.Spec.Replicas)).To(Equal(1))
			Expect(ss.Spec.Template.Spec.Volumes).To(HaveLen(2))
			Expect(ss.Spec.Template.Spec.Containers).To(HaveLen(2))

			container := ss.Spec.Template.Spec.Containers[0]
			Expect(container.VolumeMounts).To(HaveLen(2))
			Expect(container.Image).To(Equal(ContainerImage))

			Expect(container.LivenessProbe.HTTPGet.Port.IntVal).To(Equal(int32(8775)))
			Expect(container.ReadinessProbe.HTTPGet.Port.IntVal).To(Equal(int32(8775)))

		})

		When("the StatefulSet has at least one Replica ready", func() {
			BeforeEach(func() {
				th.ExpectConditionWithDetails(
					novaMetadataName,
					ConditionGetterFunc(NovaMetadataConditionGetter),
					condition.DeploymentReadyCondition,
					corev1.ConditionFalse,
					condition.RequestedReason,
					condition.DeploymentReadyRunningMessage,
				)
				th.SimulateStatefulSetReplicaReady(statefulSetName)
			})

			It("reports that the StatefulSet is ready", func() {
				ss := th.GetStatefulSet(statefulSetName)
				_ = ss
				th.ExpectCondition(
					novaMetadataName,
					ConditionGetterFunc(NovaMetadataConditionGetter),
					condition.DeploymentReadyCondition,
					corev1.ConditionTrue,
				)

				novaMetadata := GetNovaMetadata(novaMetadataName)
				Expect(novaMetadata.Status.ReadyCount).To(BeNumerically(">", 0))
			})
		})

		It("exposes the service", func() {
			th.SimulateStatefulSetReplicaReady(statefulSetName)
			th.ExpectCondition(
				novaMetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.ExposeServiceReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("is Ready", func() {
			th.SimulateStatefulSetReplicaReady(statefulSetName)

			th.ExpectCondition(
				novaMetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})
})
