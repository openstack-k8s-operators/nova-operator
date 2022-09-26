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
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
)

const (
	SecretName = "test-secret"

	timeout  = time.Second * 2
	interval = time.Millisecond * 200
)

var _ = Describe("NovaAPI controller", func() {
	var namespace string
	var novaAPIName types.NamespacedName

	BeforeEach(func() {
		// NOTE(gibi): We need to create a unique namespace for each test run
		// as namespaces cannot be deleted in a locally running envtest. See
		// https://book.kubebuilder.io/reference/envtest.html#namespace-usage-limitation
		namespace = uuid.New().String()
		CreateNamespace(namespace)
		// We still request the delete of the Namespace in AfterEach to
		// properly cleanup if we run the test in an existing cluster.
		DeferCleanup(DeleteNamespace, namespace)
	})

	When("A NovaAPI CR instance is created without any input", func() {
		BeforeEach(func() {
			novaAPIName = CreateNovaAPI(namespace, novav1.NovaAPISpec{})
			DeferCleanup(DeleteNovaAPI, novaAPIName)
		})

		It("is not Ready", func() {
			ExpectNovaAPICondition(
				novaAPIName, condition.ReadyCondition, corev1.ConditionUnknown)
		})

		It("has empty Status fields", func() {
			instance := GetNovaAPI(novaAPIName)
			// NOTE(gibi): Hash and Endpoints have `omitempty` tags so while
			// they are initialized to {} that value is omited from the output
			// when sent to the client. So we see nils here.
			Expect(instance.Status.Hash).To(BeEmpty())
			Expect(instance.Status.APIEndpoints).To(BeEmpty())
			Expect(instance.Status.ReadyCount).To(Equal(int32(0)))
			Expect(instance.Status.ServiceID).To(Equal(""))
		})

	})

	When("a NovaAPI CR is created pointing to a non existent Secret", func() {
		BeforeEach(func() {
			novaAPIName = CreateNovaAPI(
				namespace, novav1.NovaAPISpec{Secret: SecretName})
			DeferCleanup(DeleteNovaAPI, novaAPIName)
		})

		It("is not Ready", func() {
			ExpectNovaAPICondition(
				novaAPIName, condition.ReadyCondition, corev1.ConditionUnknown)
		})

		It("is missing the secret", func() {
			ExpectNovaAPICondition(
				novaAPIName, condition.InputReadyCondition, corev1.ConditionFalse)
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
				ExpectNovaAPICondition(
					novaAPIName, condition.ReadyCondition, corev1.ConditionUnknown)
			})

			It("is missing the secret", func() {
				ExpectNovaAPICondition(
					novaAPIName, condition.InputReadyCondition, corev1.ConditionFalse)
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
				ExpectNovaAPICondition(
					novaAPIName, condition.ReadyCondition, corev1.ConditionUnknown)
			})

			It("reports that the inputes are not ready", func() {
				ExpectNovaAPICondition(
					novaAPIName, condition.InputReadyCondition, corev1.ConditionFalse)
			})
		})

		When("the Secret is created with all the expected fields", func() {
			BeforeEach(func() {
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      SecretName,
						Namespace: namespace,
					},
					Data: map[string][]byte{
						"NovaPassword":              []byte("12345678"),
						"NovaAPIDatabasePassword":   []byte("12345678"),
						"NovaAPIMessageBusPassword": []byte("12345678"),
					},
				}
				Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
				DeferCleanup(k8sClient.Delete, ctx, secret)
			})

			It("is Ready", func() {
				ExpectNovaAPICondition(
					novaAPIName, condition.ReadyCondition, corev1.ConditionTrue)
			})

			It("reports that input is ready", func() {
				ExpectNovaAPICondition(
					novaAPIName, condition.InputReadyCondition, corev1.ConditionTrue)
			})
		})
	})
})
