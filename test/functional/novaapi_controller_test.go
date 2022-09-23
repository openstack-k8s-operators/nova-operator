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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
)

const (
	Namespace  = "test-namespace"
	SecretName = "test-secret"

	timeout  = time.Second * 2
	interval = time.Millisecond * 200
)

var _ = Describe("NovaAPI controller", func() {
	var novaAPILookupKey types.NamespacedName
	var namespace *corev1.Namespace
	var api *novav1.NovaAPI
	var err error

	BeforeEach(func() {
		namespace, err = CreateNamespace(Namespace)
		Expect(err).Should(BeNil())
		novaAPILookupKey, err = CreateNovaAPI(novav1.NovaAPISpec{})
		Expect(err).Should(BeNil())
		// this asserts that we can read back the CR
		api, err = GetNovaAPI(novaAPILookupKey)
		Expect(err).Should(BeNil())
		Expect(api).ShouldNot(BeNil())
	})

	AfterEach(func() {
		defer func() {
			err := k8sClient.Delete(ctx, namespace)
			Expect(err).Should(BeNil())
		}()
		api, err = GetNovaAPI(novaAPILookupKey)
		Expect(err).Should(BeNil())
		if api != nil {
			Expect(k8sClient.Delete(ctx, api)).Should(Succeed())
		}
		// We have to wait for the controller to fully delete the instance
		Eventually(func() bool {
			err = k8sClient.Get(ctx, novaAPILookupKey, api)
			return k8s_errors.IsNotFound(err)
		}, timeout, interval).Should(BeTrue())

	})

	When("A NovaAPI CR instance is created without any input", func() {
		It("should not be Ready", func() {
			Eventually(func() *condition.Condition {
				con, err := GetConditionByType(
					novaAPILookupKey,
					condition.ReadyCondition,
				)
				Expect(err).Should(BeNil())
				return con
			}, timeout, interval).Should(
				HaveField("Status", corev1.ConditionUnknown))
		})
	})
})
