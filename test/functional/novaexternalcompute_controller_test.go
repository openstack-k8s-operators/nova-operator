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
	"os"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	. "github.com/openstack-k8s-operators/lib-common/modules/test/helpers"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("NovaExternalCompute", func() {
	var namespace string
	var computeName types.NamespacedName

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

		computeName = types.NamespacedName{
			Namespace: namespace,
			Name:      uuid.New().String(),
		}
	})

	When("created", func() {
		BeforeEach(func() {
			CreateNovaExternalCompute(computeName, GetDefaultNovaExternalComputeSpec(computeName.Name))
			DeferCleanup(DeleteNovaExternalCompute, computeName)
		})

		It("adds Finalizer to itself", func() {
			Eventually(func(g Gomega) {
				compute := GetNovaExternalCompute(computeName)
				g.Expect(compute.Finalizers).To(ContainElement("NovaExternalCompute"))

			}, timeout, interval).Should(Succeed())
		})

		It("initializes Status", func() {
			Eventually(func(g Gomega) {
				compute := GetNovaExternalCompute(computeName)
				g.Expect(compute.Status.Conditions).NotTo(BeEmpty())

			}, timeout, interval).Should(Succeed())
		})

		It("is Ready", func() {
			th.ExpectCondition(
				computeName,
				ConditionGetterFunc(NovaExternalComputeConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("can be deleted as the finalizer is automatically removed", func() {
			th.ExpectCondition(
				computeName,
				ConditionGetterFunc(NovaExternalComputeConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)

			// This would fail if the NovaExternalCompute CR is still exists
			// after the timeout. So if this passes then we know the the CR is
			// removed and that can only happen if the finalizer is removed from
			// it first
			DeleteNovaExternalCompute(computeName)
		})
	})
})
