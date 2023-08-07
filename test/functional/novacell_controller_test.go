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
	"fmt"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
)

var _ = Describe("NovaCell controller", func() {
	When("A NovaCell CR instance is created without any input", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateNovaCell(cell0.CellName, GetDefaultNovaCellSpec("cell0")))
		})

		It("reports that input is not ready", func() {
			th.ExpectConditionWithDetails(
				cell0.CellName,
				ConditionGetterFunc(NovaCellConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"Input data resources missing: secret/test-secret",
			)
		})

		It("is not Ready", func() {
			th.ExpectCondition(
				cell0.CellName,
				ConditionGetterFunc(NovaCellConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("has no hash and no services ready", func() {
			instance := GetNovaCell(cell0.CellName)
			Expect(instance.Status.Hash).To(BeEmpty())
			Expect(instance.Status.ConductorServiceReadyCount).To(Equal(int32(0)))
			Expect(instance.Status.MetadataServiceReadyCount).To(Equal(int32(0)))
			Expect(instance.Status.NoVNCPRoxyServiceReadyCount).To(Equal(int32(0)))
		})
	})

	When("A NovaCell/cell0 CR instance is created", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateNovaConductorSecret(cell0.CellName.Namespace, SecretName),
			)
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateNovaMessageBusSecret(cell0.CellName.Namespace, MessageBusSecretName),
			)

			DeferCleanup(th.DeleteInstance, CreateNovaCell(cell0.CellName, GetDefaultNovaCellSpec("cell0")))
		})

		It("creates the NovaConductor and tracks its readiness", func() {
			GetNovaConductor(cell0.CellConductorName)
			th.ExpectCondition(
				cell0.CellName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaConductorReadyCondition,
				corev1.ConditionFalse,
			)
			novaCell := GetNovaCell(cell0.CellName)
			Expect(novaCell.Status.ConductorServiceReadyCount).To(Equal(int32(0)))
		})

		When("NovaConductor is ready", func() {
			BeforeEach(func() {
				th.ExpectCondition(
					cell0.CellConductorName,
					ConditionGetterFunc(NovaConductorConditionGetter),
					condition.DBSyncReadyCondition,
					corev1.ConditionFalse,
				)
				th.SimulateJobSuccess(cell0.CellDBSyncJobName)

				th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)

				th.ExpectCondition(
					cell0.CellConductorName,
					ConditionGetterFunc(NovaConductorConditionGetter),
					condition.DBSyncReadyCondition,
					corev1.ConditionTrue,
				)
			})

			It("reports that NovaConductor is ready", func() {
				th.ExpectCondition(
					cell0.CellName,
					ConditionGetterFunc(NovaCellConditionGetter),
					novav1.NovaConductorReadyCondition,
					corev1.ConditionTrue,
				)
				// TODO(gibi): this is a bug in the cell controller
				// novaCell = GetNovaCell(cell0.CellName)
				// Expect(novaCell.Status.ConductorServiceReadyCount).To(Equal(int32(1)))
			})

			It("does not create Metadata or NoVNCProxy services in cell0", func() {
				th.ExpectCondition(
					cell0.CellName,
					ConditionGetterFunc(NovaCellConditionGetter),
					condition.ReadyCondition,
					corev1.ConditionTrue,
				)
				AssertMetadataDoesNotExist(cell0.MetadataName)
				AssertNoVNCProxyDoesNotExist(cell0.MetadataName)
			})

			It("is Ready", func() {
				th.ExpectCondition(
					cell0.CellName,
					ConditionGetterFunc(NovaCellConditionGetter),
					condition.ReadyCondition,
					corev1.ConditionTrue,
				)
			})
		})
	})
	When("A NovaCell/cell1 CR instance is created", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateNovaMetadataSecret(cell1.CellName.Namespace, SecretName),
			)
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateNovaMessageBusSecret(cell1.CellName.Namespace, MessageBusSecretName),
			)
			spec := GetDefaultNovaCellSpec("cell1")
			spec["metadataServiceTemplate"] = map[string]interface{}{
				"replicas": 1,
			}
			DeferCleanup(th.DeleteInstance, CreateNovaCell(cell1.CellName, spec))
		})

		It("creates the NovaConductor and tracks its readiness", func() {
			GetNovaConductor(cell1.CellConductorName)
			th.ExpectCondition(
				cell1.CellName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaConductorReadyCondition,
				corev1.ConditionFalse,
			)
			novaCell := GetNovaCell(cell1.CellName)
			Expect(novaCell.Status.ConductorServiceReadyCount).To(Equal(int32(0)))

			th.ExpectCondition(
				cell1.CellConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionFalse,
			)

			// make conductor ready
			th.SimulateJobSuccess(cell1.CellDBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell1.ConductorStatefulSetName)

			th.ExpectCondition(
				cell1.CellConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				cell1.CellName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaConductorReadyCondition,
				corev1.ConditionTrue,
			)
			// TODO(gibi): this is a bug in the cell controller
			// novaCell = GetNovaCell(cell1.CellName)
			// Expect(novaCell.Status.ConductorServiceReadyCount).To(Equal(int32(1)))
		})

		It("creates the NovaNoVNCProxy and tracks its readiness", func() {
			GetNovaNoVNCProxy(cell1.NoVNCProxyName)
			th.ExpectCondition(
				cell1.CellName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaNoVNCProxyReadyCondition,
				corev1.ConditionFalse,
			)
			novaCell := GetNovaCell(cell1.CellName)
			Expect(novaCell.Status.NoVNCPRoxyServiceReadyCount).To(Equal(int32(0)))

			// make novncproxy ready
			th.SimulateStatefulSetReplicaReady(cell1.NoVNCProxyStatefulSetName)

			th.ExpectCondition(
				cell1.CellName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaNoVNCProxyReadyCondition,
				corev1.ConditionTrue,
			)
			// TODO(gibi): this is a bug in the cell controller
			// novaCell = GetNovaCell(cell1.CellName)
			// Expect(novaCell.Status.NoVNCPRoxyServiceReadyCount).To(Equal(int32(1)))
		})

		It("creates the NovaMetadata and tracks its readiness", func() {
			GetNovaMetadata(cell1.MetadataName)
			th.ExpectCondition(
				cell1.CellName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaMetadataReadyCondition,
				corev1.ConditionFalse,
			)
			novaCell := GetNovaCell(cell1.CellName)
			Expect(novaCell.Status.MetadataServiceReadyCount).To(Equal(int32(0)))

			// make metadata ready
			th.SimulateStatefulSetReplicaReady(cell1.MetadataStatefulSetName)

			th.ExpectCondition(
				cell1.CellName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaMetadataReadyCondition,
				corev1.ConditionTrue,
			)
			// TODO(gibi): this is a bug in the cell controller
			// novaCell = GetNovaCell(cell1.CellName)
			// Expect(novaCell.Status.MetadataServiceReadyCount).To(Equal(int32(1)))
		})

		It("creates the compute config secret", func() {
			th.ExpectCondition(
				cell1.CellName,
				ConditionGetterFunc(NovaCellConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)
			// compute config only generated after VNCProxy is ready,
			// so make novncproxy ready
			th.SimulateStatefulSetReplicaReady(cell1.NoVNCProxyStatefulSetName)
			host := SimulateNoVNCProxyService("cell1", cell1.CellName.Namespace)
			th.ExpectCondition(
				cell1.CellName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaNoVNCProxyReadyCondition,
				corev1.ConditionTrue,
			)

			computeConfigData := th.GetSecret(cell1.ComputeConfigSecretName)
			Expect(computeConfigData).ShouldNot(BeNil())
			Expect(computeConfigData.Data).Should(HaveKey("01-nova.conf"))
			configData := string(computeConfigData.Data["01-nova.conf"])
			Expect(configData).To(ContainSubstring("transport_url=rabbit://rabbitmq-secret/fake"))
			Expect(configData).To(ContainSubstring("username = nova\npassword = service-password\n"))
			vncUrlConfig := fmt.Sprintf("novncproxy_base_url = http://%s/vnc_lite.html", host)
			Expect(configData).To(ContainSubstring(vncUrlConfig))

			th.ExpectCondition(
				cell1.CellName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaComputeServiceConfigReady,
				corev1.ConditionTrue,
			)

			Expect(GetNovaCell(cell1.CellName).Status.Hash).To(HaveKey(cell1.ComputeConfigSecretName.Name))
		})

		It("is Ready when all cell services is ready", func() {
			th.SimulateJobSuccess(cell1.CellDBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell1.ConductorStatefulSetName)
			th.SimulateStatefulSetReplicaReady(cell1.NoVNCProxyStatefulSetName)
			th.SimulateStatefulSetReplicaReady(cell1.MetadataStatefulSetName)

			th.ExpectCondition(
				cell1.CellName,
				ConditionGetterFunc(NovaCellConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})
	When("A NovaCell/cell2 CR instance is created without VNCProxy", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateNovaMetadataSecret(cell2.CellName.Namespace, SecretName),
			)
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateNovaMessageBusSecret(cell2.CellName.Namespace, MessageBusSecretName),
			)
			spec := GetDefaultNovaCellSpec("cell2")
			spec["noVNCProxyServiceTemplate"] = map[string]interface{}{
				"replicas": 0,
			}
			DeferCleanup(th.DeleteInstance, CreateNovaCell(cell2.CellName, spec))
		})

		It("creates the compute config secret", func() {
			th.ExpectCondition(
				cell2.CellName,
				ConditionGetterFunc(NovaCellConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)

			computeConfigData := th.GetSecret(cell2.ComputeConfigSecretName)
			Expect(computeConfigData).ShouldNot(BeNil())
			Expect(computeConfigData.Data).Should(HaveKey("01-nova.conf"))
			configData := string(computeConfigData.Data["01-nova.conf"])
			Expect(configData).To(ContainSubstring("transport_url=rabbit://rabbitmq-secret/fake"))
			Expect(configData).To(ContainSubstring("username = nova\npassword = service-password\n"))
			// There is no VNCProxy created but we still get a compute config just
			// without any vnc proxy url
			Expect(configData).NotTo(ContainSubstring("novncproxy_base_url "))

			th.ExpectCondition(
				cell2.CellName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaComputeServiceConfigReady,
				corev1.ConditionTrue,
			)

			Expect(GetNovaCell(cell2.CellName).Status.Hash).To(HaveKey(cell2.ComputeConfigSecretName.Name))
		})

		It("is Ready when all cell services is ready", func() {
			th.SimulateJobSuccess(cell2.CellDBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell2.ConductorStatefulSetName)

			th.ExpectCondition(
				cell2.CellName,
				ConditionGetterFunc(NovaCellConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})
	When("NovaCell is reconfigured", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateNovaConductorSecret(cell0.CellName.Namespace, SecretName),
			)
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateNovaMessageBusSecret(cell0.CellName.Namespace, MessageBusSecretName),
			)

			DeferCleanup(th.DeleteInstance, CreateNovaCell(cell0.CellName, GetDefaultNovaCellSpec("cell0")))
			th.SimulateJobSuccess(cell0.CellDBSyncJobName)

			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
			th.ExpectCondition(
				cell0.CellName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaConductorReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("applies new NetworkAttachments configuration to its Conductor", func() {
			Eventually(func(g Gomega) {
				novaCell := GetNovaCell(cell0.CellName)
				novaCell.Spec.ConductorServiceTemplate.NetworkAttachments = append(
					novaCell.Spec.ConductorServiceTemplate.NetworkAttachments, "internalapi")

				g.Expect(k8sClient.Update(ctx, novaCell)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			th.ExpectConditionWithDetails(
				cell0.CellConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NetworkAttachment resources missing: internalapi",
			)

			th.ExpectConditionWithDetails(
				cell0.CellName,
				ConditionGetterFunc(NovaCellConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NetworkAttachment resources missing: internalapi",
			)

			DeferCleanup(th.DeleteInstance, th.CreateNetworkAttachmentDefinition(cell0.InternalAPINetworkNADName))

			th.ExpectConditionWithDetails(
				cell0.CellConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occurred "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
			)

			th.ExpectConditionWithDetails(
				cell0.CellName,
				ConditionGetterFunc(NovaCellConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occurred "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
			)

			th.SimulateStatefulSetReplicaReadyWithPods(
				cell0.ConductorStatefulSetName,
				map[string][]string{cell0.CellName.Namespace + "/internalapi": {"10.0.0.1"}},
			)

			th.ExpectCondition(
				cell0.CellConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionTrue,
			)

			th.ExpectCondition(
				cell0.CellName,
				ConditionGetterFunc(NovaCellConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})
})

var _ = Describe("NovaCell controller webhook", func() {
	It("name is too long", func() {
		DeferCleanup(
			k8sClient.Delete, ctx, CreateNovaConductorSecret(cell1.CellName.Namespace, SecretName))
		DeferCleanup(
			k8sClient.Delete, ctx, CreateNovaMessageBusSecret(cell1.CellName.Namespace, MessageBusSecretName))

		spec := GetDefaultNovaCellSpec(uuid.New().String())
		rawObj := map[string]interface{}{
			"apiVersion": "nova.openstack.org/v1beta1",
			"kind":       "NovaCell",
			"metadata": map[string]interface{}{
				"name":      cell1.CellName.Name,
				"namespace": cell1.CellName.Namespace,
			},
			"spec": spec,
		}
		th.Logger.Info("Creating", "raw", rawObj)
		unstructuredObj := &unstructured.Unstructured{Object: rawObj}
		_, err := controllerutil.CreateOrPatch(
			th.Ctx, th.K8sClient, unstructuredObj, func() error { return nil })
		Expect(err).Should(HaveOccurred())
	})
})
