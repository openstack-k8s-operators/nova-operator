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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	. "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
)

func CreateNovaCellAndEnsureReady(cell CellNames) {
	DeferCleanup(
		k8sClient.Delete, ctx, CreateNovaNoVNCProxySecret(cell.CellName.Namespace, SecretName))
	DeferCleanup(
		k8sClient.Delete, ctx, CreateNovaMessageBusSecret(cell.CellName.Namespace, MessageBusSecretName))

	spec := GetDefaultNovaCellSpec(cell.CellName.Name)
	DeferCleanup(th.DeleteInstance, CreateNovaCell(cell.CellName, spec))

	th.SimulateStatefulSetReplicaReady(cell.NoVNCProxyStatefulSetName)
	th.SimulateJobSuccess(cell.CellDBSyncJobName)
	th.SimulateStatefulSetReplicaReady(cell.ConductorStatefulSetName)

	//SimulateNoVNCProxyRouteIngress(cell1.CellName.Name, cell1.CellName.Namespace)

	th.ExpectCondition(
		cell.CellName,
		ConditionGetterFunc(NovaCellConditionGetter),
		condition.ReadyCondition,
		corev1.ConditionTrue,
	)
}

var _ = Describe("NovaExternalCompute", func() {
	When("created", func() {
		var libvirtAEEName types.NamespacedName
		var novaAEEName types.NamespacedName

		BeforeEach(func() {
			// Create the NovaCell the compute will belong to
			CreateNovaCellAndEnsureReady(cell1)
			// Create the external compute
			DeferCleanup(
				th.DeleteInstance,
				CreateNovaExternalCompute(
					novaNames.ComputeName,
					GetDefaultNovaExternalComputeSpec(novaNames.NovaName.Name, novaNames.ComputeName.Name)))

			compute := GetNovaExternalCompute(novaNames.ComputeName)
			// TODO(bogdando): move to novaNames.Compute*
			inventoryName := types.NamespacedName{
				Namespace: novaNames.ComputeName.Namespace,
				Name:      compute.Spec.InventoryConfigMapName,
			}
			CreateNovaExternalComputeInventoryConfigMap(inventoryName)
			DeferCleanup(th.DeleteConfigMap, inventoryName)

			sshSecretName := types.NamespacedName{
				Namespace: novaNames.ComputeName.Namespace,
				Name:      compute.Spec.SSHKeySecretName,
			}
			CreateNovaExternalComputeSSHSecret(sshSecretName)

			DeferCleanup(th.DeleteSecret, sshSecretName)
			libvirtAEEName = types.NamespacedName{
				Namespace: novaNames.ComputeName.Namespace,
				Name:      fmt.Sprintf("%s-%s-deploy-libvirt", compute.Spec.NovaInstance, compute.Name),
			}
			SimulateAEESucceeded(libvirtAEEName)
			novaAEEName = types.NamespacedName{
				Namespace: novaNames.ComputeName.Namespace,
				Name:      fmt.Sprintf("%s-%s-deploy-nova", compute.Spec.NovaInstance, compute.Name),
			}
			SimulateAEESucceeded(novaAEEName)

		})

		It("adds Finalizer to itself", func() {
			Eventually(func(g Gomega) {
				compute := GetNovaExternalCompute(novaNames.ComputeName)
				g.Expect(compute.Finalizers).To(ContainElement("NovaExternalCompute"))

			}, timeout, interval).Should(Succeed())
		})

		It("initializes Status", func() {
			Eventually(func(g Gomega) {
				compute := GetNovaExternalCompute(novaNames.ComputeName)
				g.Expect(compute.Status.Conditions).NotTo(BeEmpty())
				g.Expect(compute.Status.Hash).NotTo(BeNil())

			}, timeout, interval).Should(Succeed())
		})

		It("reports InputReady and stores that input hash", func() {
			th.ExpectCondition(
				novaNames.ComputeName,
				ConditionGetterFunc(NovaExternalComputeConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)
			compute := GetNovaExternalCompute(novaNames.ComputeName)
			Expect(compute.Status.Hash["input"]).NotTo(BeEmpty())
		})

		It("creates AnsibleEE for libvirt", func() {
			compute := GetNovaExternalCompute(novaNames.ComputeName)
			// TODO(gibi): assert more fields on AnsibleEE
			libvirtAEE := GetAEE(libvirtAEEName)
			Expect(libvirtAEE.Spec.ExtraMounts).To(HaveLen(1))
			extraMounts := libvirtAEE.Spec.ExtraMounts[0]
			configVol := &corev1.Volume{}
			Expect(extraMounts.Volumes).To(ContainElement(HaveField("Name", "compute-configs"), configVol))
			Expect(configVol.VolumeSource.Secret.SecretName).To(Equal(novaNames.ComputeName.Name + "-config-data"))
			Expect(libvirtAEE.Spec.NetworkAttachments).To(Equal(compute.Spec.NetworkAttachments))
		})

		It("creates AnsibleEE for nova", func() {
			compute := GetNovaExternalCompute(novaNames.ComputeName)
			// TODO(gibi): assert more fields on AnsibleEE
			novaAEE := GetAEE(novaAEEName)
			Expect(novaAEE.Spec.ExtraMounts).To(HaveLen(1))
			extraMounts := novaAEE.Spec.ExtraMounts[0]
			configVol := &corev1.Volume{}
			Expect(extraMounts.Volumes).To(ContainElement(HaveField("Name", "compute-configs"), configVol))
			Expect(configVol.VolumeSource.Secret.SecretName).To(Equal(novaNames.ComputeName.Name + "-config-data"))
			Expect(novaAEE.Spec.NetworkAttachments).To(Equal(compute.Spec.NetworkAttachments))
		})

		It("is Ready", func() {
			th.ExpectCondition(
				novaNames.ComputeName,
				ConditionGetterFunc(NovaExternalComputeConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("can be deleted as the finalizer is automatically removed", func() {
			th.ExpectCondition(
				novaNames.ComputeName,
				ConditionGetterFunc(NovaExternalComputeConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)

			// This would fail if the NovaExternalCompute CR is still exists
			// after the timeout. So if this passes then we know the the CR is
			// removed and that can only happen if the finalizer is removed from
			// it first
			compute := GetNovaExternalCompute(novaNames.ComputeName)
			th.DeleteInstance(compute)
		})
		It("generated configs successfully", func() {
			th.ExpectCondition(
				novaNames.ComputeName,
				ConditionGetterFunc(NovaExternalComputeConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)

			configDataMap := th.GetSecret(
				types.NamespacedName{
					Namespace: novaNames.ComputeName.Namespace,
					Name:      fmt.Sprintf("%s-config-data", novaNames.ComputeName.Name),
				},
			)
			Expect(configDataMap).ShouldNot(BeNil())
			Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
			configData := string(configDataMap.Data["01-nova.conf"])
			Expect(configData).To(ContainSubstring("transport_url=rabbit://rabbitmq-secret/fake"))
			// NOTE(sean) This  check prevents regressing bug #422 the password should be populated
			Expect(configData).To(ContainSubstring("username = nova\npassword = service-password\n"))
			Expect(configDataMap.Data).Should(HaveKey("02-nova-override.conf"))
			extraConfigData := string(configDataMap.Data["02-nova-override.conf"])
			Expect(extraConfigData).To(Equal(""))

		})
		It("generated vnc firewall configs successfully", func() {
			th.ExpectCondition(
				novaNames.ComputeName,
				ConditionGetterFunc(NovaExternalComputeConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)

			configDataMap := th.GetSecret(
				types.NamespacedName{
					Namespace: novaNames.ComputeName.Namespace,
					Name:      fmt.Sprintf("%s-config-data", novaNames.ComputeName.Name),
				},
			)
			Expect(configDataMap).ShouldNot(BeNil())
			Expect(configDataMap.Data).Should(HaveKey("firewall.yaml"))
			configData := string(configDataMap.Data["firewall.yaml"])
			Expect(configData).To(ContainSubstring("005 Allow vnc access on all networks."))
			Expect(configData).To(ContainSubstring("proto: tcp"))
			Expect(configData).To(ContainSubstring("5900-6923"))
		})
	})
	When("created but Secrets are missing or fields missing", func() {
		BeforeEach(func() {
			DeferCleanup(
				th.DeleteInstance,
				CreateNovaExternalCompute(
					novaNames.ComputeName,
					GetDefaultNovaExternalComputeSpec(novaNames.NovaName.Name, novaNames.ComputeName.Name)))
		})

		It("reports missing Inventory configmap", func() {
			compute := GetNovaExternalCompute(novaNames.ComputeName)
			th.ExpectConditionWithDetails(
				novaNames.ComputeName,
				ConditionGetterFunc(NovaExternalComputeConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"Input data resources missing: configmap/"+compute.Spec.InventoryConfigMapName,
			)
			compute = GetNovaExternalCompute(novaNames.ComputeName)
			Expect(compute.Status.Hash["input"]).To(BeEmpty())
		})

		It("reports missing field from Inventory configmap", func() {
			compute := GetNovaExternalCompute(novaNames.ComputeName)
			cm := th.CreateConfigMap(
				types.NamespacedName{Namespace: novaNames.ComputeName.Namespace, Name: compute.Spec.InventoryConfigMapName},
				map[string]interface{}{},
			)
			DeferCleanup(th.DeleteInstance, cm)
			th.ExpectConditionWithDetails(
				novaNames.ComputeName,
				ConditionGetterFunc(NovaExternalComputeConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"Input data error occurred field 'inventory' not found in configmap/"+compute.Spec.InventoryConfigMapName,
			)
			compute = GetNovaExternalCompute(novaNames.ComputeName)
			Expect(compute.Status.Hash["input"]).To(BeEmpty())
		})

		It("reports missing SSH key secret", func() {
			compute := GetNovaExternalCompute(novaNames.ComputeName)
			inventoryName := types.NamespacedName{Namespace: novaNames.ComputeName.Namespace, Name: compute.Spec.InventoryConfigMapName}
			CreateNovaExternalComputeInventoryConfigMap(inventoryName)
			DeferCleanup(th.DeleteConfigMap, inventoryName)
			th.ExpectConditionWithDetails(
				novaNames.ComputeName,
				ConditionGetterFunc(NovaExternalComputeConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"Input data resources missing: secret/"+compute.Spec.SSHKeySecretName,
			)
			compute = GetNovaExternalCompute(novaNames.ComputeName)
			Expect(compute.Status.Hash["input"]).To(BeEmpty())
		})

		It("reports missing field from SSH key secret", func() {
			compute := GetNovaExternalCompute(novaNames.ComputeName)
			CreateNovaExternalComputeInventoryConfigMap(
				types.NamespacedName{Namespace: novaNames.ComputeName.Namespace, Name: compute.Spec.InventoryConfigMapName})
			th.CreateEmptySecret(
				types.NamespacedName{Namespace: novaNames.ComputeName.Namespace, Name: compute.Spec.SSHKeySecretName})
			th.ExpectConditionWithDetails(
				novaNames.ComputeName,
				ConditionGetterFunc(NovaExternalComputeConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"Input data error occurred field 'ssh-privatekey' not found in secret/"+compute.Spec.SSHKeySecretName,
			)
			compute = GetNovaExternalCompute(novaNames.ComputeName)
			Expect(compute.Status.Hash["input"]).To(BeEmpty())
		})
	})

	When("created but NovaCell is not Ready", func() {
		BeforeEach(func() {
			// Create the compute
			DeferCleanup(
				th.DeleteInstance,
				CreateNovaExternalCompute(
					novaNames.ComputeName,
					GetDefaultNovaExternalComputeSpec(novaNames.NovaName.Name, novaNames.ComputeName.Name)))
			compute := GetNovaExternalCompute(novaNames.ComputeName)
			inventoryName := types.NamespacedName{
				Namespace: novaNames.ComputeName.Namespace,
				Name:      compute.Spec.InventoryConfigMapName,
			}
			CreateNovaExternalComputeInventoryConfigMap(inventoryName)
			DeferCleanup(th.DeleteSecret, inventoryName)

			sshSecretName := types.NamespacedName{
				Namespace: novaNames.ComputeName.Namespace,
				Name:      compute.Spec.SSHKeySecretName,
			}
			CreateNovaExternalComputeSSHSecret(sshSecretName)
			DeferCleanup(th.DeleteSecret, sshSecretName)
		})

		It("reports if NovaCell is missing", func() {
			th.ExpectConditionWithDetails(
				novaNames.ComputeName,
				ConditionGetterFunc(NovaExternalComputeConditionGetter),
				novav1.NovaCellReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"Waiting for NovaCell "+cell1.CellName.Name+" to exists",
			)
		})

		It("reports if NovaCell is not Ready", func() {
			// Create the NovaCell but keep in unready by not simulating
			// deployment success
			spec := GetDefaultNovaCellSpec("cell1")
			instance := CreateNovaCell(cell1.CellName, spec)

			DeferCleanup(th.DeleteInstance, instance)

			th.ExpectConditionWithDetails(
				novaNames.ComputeName,
				ConditionGetterFunc(NovaExternalComputeConditionGetter),
				novav1.NovaCellReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"Waiting for NovaCell "+cell1.CellName.Name+" to become Ready",
			)
		})
	})
	When("inventory is reconfigured to a non existing ConfigMap", func() {
		BeforeEach(func() {
			// Create the NovaCell the compute will belong to
			CreateNovaCellAndEnsureReady(cell1)
			// Create the compute
			DeferCleanup(
				th.DeleteInstance,
				CreateNovaExternalCompute(
					novaNames.ComputeName,
					GetDefaultNovaExternalComputeSpec(novaNames.NovaName.Name, novaNames.ComputeName.Name)))

			compute := GetNovaExternalCompute(novaNames.ComputeName)
			inventoryName := types.NamespacedName{
				Namespace: novaNames.ComputeName.Namespace,
				Name:      compute.Spec.InventoryConfigMapName,
			}
			CreateNovaExternalComputeInventoryConfigMap(inventoryName)
			DeferCleanup(th.DeleteConfigMap, inventoryName)

			sshSecretName := types.NamespacedName{
				Namespace: novaNames.ComputeName.Namespace,
				Name:      compute.Spec.SSHKeySecretName,
			}
			CreateNovaExternalComputeSSHSecret(sshSecretName)

			//SimulateNoVNCProxyRouteIngress(cell1.CellName.Name, cell1.CellName.Namespace)

			DeferCleanup(th.DeleteSecret, sshSecretName)

			libvirtAEEName := types.NamespacedName{
				Namespace: novaNames.ComputeName.Namespace,
				Name:      fmt.Sprintf("%s-%s-deploy-libvirt", compute.Spec.NovaInstance, compute.Name),
			}
			SimulateAEESucceeded(libvirtAEEName)
			novaAEEName := types.NamespacedName{
				Namespace: novaNames.ComputeName.Namespace,
				Name:      fmt.Sprintf("%s-%s-deploy-nova", compute.Spec.NovaInstance, compute.Name),
			}
			SimulateAEESucceeded(novaAEEName)

			th.ExpectCondition(
				novaNames.ComputeName,
				ConditionGetterFunc(NovaExternalComputeConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("reports that the inventory is missing", func() {
			Eventually(func(g Gomega) {
				compute := GetNovaExternalCompute(novaNames.ComputeName)
				compute.Spec.InventoryConfigMapName = "non-existent"
				g.Expect(k8sClient.Update(ctx, compute)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			th.ExpectConditionWithDetails(
				novaNames.ComputeName,
				ConditionGetterFunc(NovaExternalComputeConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"Input data resources missing: configmap/non-existent",
			)
			th.ExpectCondition(
				novaNames.ComputeName,
				ConditionGetterFunc(NovaExternalComputeConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})
	})
	When("created as unstructured and created from golang", func() {
		It("has the same defaults", func() {
			computeNameUnstructured := types.NamespacedName{Namespace: novaNames.ComputeName.Namespace, Name: "compute-default-unstructured"}
			computeNameGolang := types.NamespacedName{Namespace: novaNames.ComputeName.Namespace, Name: "compute-default-golang"}
			CreateNovaExternalCompute(
				computeNameUnstructured,
				map[string]interface{}{
					"inventoryConfigMapName": "foo-inventory-configmap",
					"sshKeySecretName":       "foo-ssh-key-secret",
				})
			computeFromUnstructured := GetNovaExternalCompute(computeNameUnstructured)
			DeferCleanup(th.DeleteInstance, computeFromUnstructured)

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
			DeferCleanup(th.DeleteInstance, computeFromGolang)

			Expect(computeFromUnstructured.Spec).To(Equal(computeFromGolang.Spec))
		})
	})
})
