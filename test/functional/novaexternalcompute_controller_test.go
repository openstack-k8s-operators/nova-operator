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
	"fmt"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	. "github.com/openstack-k8s-operators/lib-common/modules/test/helpers"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
)

func CreateNovaCellAndEnsureReady(namespace string, novaCellName types.NamespacedName) {
	DeferCleanup(
		k8sClient.Delete, ctx, CreateNovaConductorSecret(namespace, SecretName))
	DeferCleanup(
		k8sClient.Delete, ctx, CreateNovaMessageBusSecret(namespace, MessageBusSecretName))

	spec := GetDefaultNovaCellSpec()
	spec["cellName"] = "cell1"
	instance := CreateNovaCell(novaCellName, spec)
	DeferCleanup(DeleteInstance, instance)
	cellName := types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}

	novaConductorName := types.NamespacedName{
		Namespace: namespace,
		Name:      cellName.Name + "-conductor",
	}
	novaConductorDBSyncJobName := types.NamespacedName{
		Namespace: namespace,
		Name:      novaConductorName.Name + "-db-sync",
	}
	th.SimulateJobSuccess(novaConductorDBSyncJobName)

	conductorStatefulSetName := types.NamespacedName{
		Namespace: namespace,
		Name:      novaConductorName.Name,
	}
	th.SimulateStatefulSetReplicaReady(conductorStatefulSetName)

	th.ExpectCondition(
		cellName,
		ConditionGetterFunc(NovaCellConditionGetter),
		condition.ReadyCondition,
		corev1.ConditionTrue,
	)

}

var _ = Describe("NovaExternalCompute", func() {
	var computeName types.NamespacedName
	var novaName types.NamespacedName
	var novaCellName types.NamespacedName

	BeforeEach(func() {
		// Uncomment this if you need the full output in the logs from gomega
		// matchers
		// format.MaxLength = 0

		computeName = types.NamespacedName{
			Namespace: namespace,
			Name:      uuid.New().String(),
		}
		novaName = types.NamespacedName{
			Namespace: namespace,
			Name:      uuid.New().String(),
		}
		novaCellName = types.NamespacedName{
			Namespace: namespace,
			Name:      novaName.Name + "-" + "cell1",
		}
	})

	When("created", func() {

		BeforeEach(func() {
			// Create the NovaCell the compute will belong to
			CreateNovaCellAndEnsureReady(namespace, novaCellName)
			// Create the compute
			instance := CreateNovaExternalCompute(computeName, GetDefaultNovaExternalComputeSpec(novaName.Name, computeName.Name))
			DeferCleanup(DeleteInstance, instance)
			compute := GetNovaExternalCompute(computeName)
			inventoryName := types.NamespacedName{
				Namespace: namespace,
				Name:      compute.Spec.InventoryConfigMapName,
			}
			CreateNovaExternalComputeInventoryConfigMap(inventoryName)
			DeferCleanup(DeleteConfigMap, inventoryName)

			sshSecretName := types.NamespacedName{
				Namespace: namespace,
				Name:      compute.Spec.SSHKeySecretName,
			}
			CreateNovaExternalComputeSSHSecret(sshSecretName)
			DeferCleanup(DeleteSecret, sshSecretName)
			libvirtAEEName := types.NamespacedName{
				Namespace: namespace,
				Name:      fmt.Sprintf("%s-%s-deploy-libvirt", compute.Spec.NovaInstance, compute.Name),
			}
			SimulateAEESucceded(libvirtAEEName)
			novaAEEName := types.NamespacedName{
				Namespace: namespace,
				Name:      fmt.Sprintf("%s-%s-deploy-nova", compute.Spec.NovaInstance, compute.Name),
			}
			SimulateAEESucceded(novaAEEName)

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
				g.Expect(compute.Status.Hash).NotTo(BeNil())

			}, timeout, interval).Should(Succeed())
		})

		It("reports InputReady and stores that input hash", func() {
			th.ExpectCondition(
				computeName,
				ConditionGetterFunc(NovaExternalComputeConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)
			compute := GetNovaExternalCompute(computeName)
			Expect(compute.Status.Hash["input"]).NotTo(BeEmpty())
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
			compute := GetNovaExternalCompute(computeName)
			DeleteInstance(compute)
		})
	})
	When("created but Secrets are missing or fields missing", func() {
		BeforeEach(func() {
			compute := CreateNovaExternalCompute(computeName, GetDefaultNovaExternalComputeSpec(novaName.Name, computeName.Name))
			DeferCleanup(DeleteInstance, compute)
		})

		It("reports missing Inventory configmap", func() {
			compute := GetNovaExternalCompute(computeName)
			th.ExpectConditionWithDetails(
				computeName,
				ConditionGetterFunc(NovaExternalComputeConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"Input data resources missing: configmap/"+compute.Spec.InventoryConfigMapName,
			)
			compute = GetNovaExternalCompute(computeName)
			Expect(compute.Status.Hash["input"]).To(BeEmpty())
		})

		It("reports missing field from Inventory configmap", func() {
			compute := GetNovaExternalCompute(computeName)
			CreateEmptyConfigMap(
				types.NamespacedName{Namespace: namespace, Name: compute.Spec.InventoryConfigMapName})
			th.ExpectConditionWithDetails(
				computeName,
				ConditionGetterFunc(NovaExternalComputeConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"Input data error occurred field 'inventory' not found in configmap/"+compute.Spec.InventoryConfigMapName,
			)
			compute = GetNovaExternalCompute(computeName)
			Expect(compute.Status.Hash["input"]).To(BeEmpty())
		})

		It("reports missing SSH key secret", func() {
			compute := GetNovaExternalCompute(computeName)
			inventoryName := types.NamespacedName{Namespace: namespace, Name: compute.Spec.InventoryConfigMapName}
			CreateNovaExternalComputeInventoryConfigMap(inventoryName)
			DeferCleanup(DeleteConfigMap, inventoryName)
			th.ExpectConditionWithDetails(
				computeName,
				ConditionGetterFunc(NovaExternalComputeConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"Input data resources missing: secret/"+compute.Spec.SSHKeySecretName,
			)
			compute = GetNovaExternalCompute(computeName)
			Expect(compute.Status.Hash["input"]).To(BeEmpty())
		})

		It("reports missing field from SSH key secret", func() {
			compute := GetNovaExternalCompute(computeName)
			CreateNovaExternalComputeInventoryConfigMap(
				types.NamespacedName{Namespace: namespace, Name: compute.Spec.InventoryConfigMapName})
			CreateEmptySecret(
				types.NamespacedName{Namespace: namespace, Name: compute.Spec.SSHKeySecretName})
			th.ExpectConditionWithDetails(
				computeName,
				ConditionGetterFunc(NovaExternalComputeConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"Input data error occurred field 'ssh-privatekey' not found in secret/"+compute.Spec.SSHKeySecretName,
			)
			compute = GetNovaExternalCompute(computeName)
			Expect(compute.Status.Hash["input"]).To(BeEmpty())
		})
	})

	When("created but NovaCell is not Ready", func() {
		BeforeEach(func() {
			// Create the compute
			instance := CreateNovaExternalCompute(computeName, GetDefaultNovaExternalComputeSpec(novaName.Name, computeName.Name))
			DeferCleanup(DeleteInstance, instance)
			compute := GetNovaExternalCompute(computeName)
			inventoryName := types.NamespacedName{
				Namespace: namespace,
				Name:      compute.Spec.InventoryConfigMapName,
			}
			CreateNovaExternalComputeInventoryConfigMap(inventoryName)
			DeferCleanup(DeleteSecret, inventoryName)

			sshSecretName := types.NamespacedName{
				Namespace: namespace,
				Name:      compute.Spec.SSHKeySecretName,
			}
			CreateNovaExternalComputeSSHSecret(sshSecretName)
			DeferCleanup(DeleteSecret, sshSecretName)
		})

		It("reports if NovaCell is missing", func() {
			th.ExpectConditionWithDetails(
				computeName,
				ConditionGetterFunc(NovaExternalComputeConditionGetter),
				novav1.NovaCellReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"Waiting for NovaCell "+novaCellName.Name+" to exists",
			)
		})

		It("reports if NovaCell is not Ready", func() {
			// Create the NovaCell but keep in unready by not simulating
			// deployment success
			spec := GetDefaultNovaCellSpec()
			spec["cellName"] = "cell1"
			instance := CreateNovaCell(novaCellName, spec)

			DeferCleanup(DeleteInstance, instance)

			th.ExpectConditionWithDetails(
				computeName,
				ConditionGetterFunc(NovaExternalComputeConditionGetter),
				novav1.NovaCellReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"Waiting for NovaCell "+novaCellName.Name+" to become Ready",
			)
		})
	})
	When("inventory is reconfigured to a non existing ConfigMap", func() {
		BeforeEach(func() {
			// Create the NovaCell the compute will belong to
			CreateNovaCellAndEnsureReady(namespace, novaCellName)
			// Create the compute
			instance := CreateNovaExternalCompute(computeName, GetDefaultNovaExternalComputeSpec(novaName.Name, computeName.Name))
			DeferCleanup(DeleteInstance, instance)
			compute := GetNovaExternalCompute(computeName)
			inventoryName := types.NamespacedName{
				Namespace: namespace,
				Name:      compute.Spec.InventoryConfigMapName,
			}
			CreateNovaExternalComputeInventoryConfigMap(inventoryName)
			DeferCleanup(DeleteConfigMap, inventoryName)

			sshSecretName := types.NamespacedName{
				Namespace: namespace,
				Name:      compute.Spec.SSHKeySecretName,
			}
			CreateNovaExternalComputeSSHSecret(sshSecretName)
			DeferCleanup(DeleteSecret, sshSecretName)

			libvirtAEEName := types.NamespacedName{
				Namespace: namespace,
				Name:      fmt.Sprintf("%s-%s-deploy-libvirt", compute.Spec.NovaInstance, compute.Name),
			}
			SimulateAEESucceded(libvirtAEEName)
			novaAEEName := types.NamespacedName{
				Namespace: namespace,
				Name:      fmt.Sprintf("%s-%s-deploy-nova", compute.Spec.NovaInstance, compute.Name),
			}
			SimulateAEESucceded(novaAEEName)

			th.ExpectCondition(
				computeName,
				ConditionGetterFunc(NovaExternalComputeConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("reports that the inventory is missing", func() {
			Eventually(func(g Gomega) {
				compute := GetNovaExternalCompute(computeName)
				compute.Spec.InventoryConfigMapName = "non-existent"
				err := k8sClient.Update(ctx, compute)
				g.Expect(err == nil || k8s_errors.IsConflict(err)).To(BeTrue())
			}, timeout, interval).Should(Succeed())

			th.ExpectConditionWithDetails(
				computeName,
				ConditionGetterFunc(NovaExternalComputeConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"Input data resources missing: configmap/non-existent",
			)
			th.ExpectCondition(
				computeName,
				ConditionGetterFunc(NovaExternalComputeConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})
	})
	When("created as unstructured and created from golang", func() {
		It("has the same defaults", func() {
			computeNameUnstructured := types.NamespacedName{Namespace: computeName.Namespace, Name: "compute-default-unstructured"}
			computeNameGolang := types.NamespacedName{Namespace: computeName.Namespace, Name: "compute-default-golang"}
			CreateNovaExternalCompute(
				computeNameUnstructured,
				map[string]interface{}{
					"inventoryConfigMapName": "foo-inventory-configmap",
					"sshKeySecretName":       "foo-ssh-key-secret",
				})
			computeFromUnstructured := GetNovaExternalCompute(computeNameUnstructured)
			DeferCleanup(DeleteInstance, computeFromUnstructured)

			spec := novav1.NewNovaExternalComputeSpec("foo-inventory-configmap", "foo-ssh-key-secret")
			err := k8sClient.Create(ctx, &novav1.NovaExternalCompute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      computeNameGolang.Name,
					Namespace: computeNameGolang.Namespace,
				},
				Spec: spec,
			})
			Expect(err).ShouldNot(HaveOccurred())
			computeFromGolang := GetNovaExternalCompute(computeNameGolang)
			DeferCleanup(DeleteInstance, computeFromGolang)

			Expect(computeFromUnstructured.Spec).To(Equal(computeFromGolang.Spec))
		})
	})
})
