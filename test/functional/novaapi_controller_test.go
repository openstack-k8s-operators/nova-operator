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
	var novaAPILookupKey types.NamespacedName

	BeforeEach(func() {
		// NOTE(gibi): We need to create a unique namespace for each test run
		// as namespaces cannot be deleted in a locally running envtest. See
		// https://book.kubebuilder.io/reference/envtest.html#namespace-usage-limitation
		// We still request the delete of the Namespace in AfterEach to
		// properly cleanup if we run the test in an existing cluster.
		namespace = uuid.New().String()
		CreateNamespace(namespace)

		novaAPILookupKey = CreateNovaAPI(namespace, novav1.NovaAPISpec{})
		// this asserts that we can read back the CR
		GetNovaAPI(novaAPILookupKey)
	})

	AfterEach(func() {
		DeleteNovaAPI(novaAPILookupKey)
		DeleteNamespace(namespace)
	})

	When("A NovaAPI CR instance is created without any input", func() {
		It("should not be Ready", func() {
			Eventually(func() *condition.Condition {
				return GetConditionByType(
					novaAPILookupKey,
					condition.ReadyCondition,
				)
			}, timeout, interval).Should(
				HaveField("Status", corev1.ConditionUnknown))
		})

		It("...", func() {
			Expect(true).To(BeTrue())
		})
	})
})
