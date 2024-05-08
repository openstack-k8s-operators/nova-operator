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

	. "github.com/onsi/ginkgo/v2" //revive:disable:dot-imports
	. "github.com/onsi/gomega"    //revive:disable:dot-imports

	//revive:disable-next-line:dot-imports
	. "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"

	"github.com/google/uuid"
	memcachedv1 "github.com/openstack-k8s-operators/infra-operator/apis/memcached/v1beta1"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/service"
	mariadbv1 "github.com/openstack-k8s-operators/mariadb-operator/api/v1beta1"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var _ = Describe("NovaCell controller", func() {
	BeforeEach(func() {
		apiMariaDBAccount, apiMariaDBSecret := mariadb.CreateMariaDBAccountAndSecret(
			cell0.APIDatabaseAccountName, mariadbv1.MariaDBAccountSpec{})
		DeferCleanup(k8sClient.Delete, ctx, apiMariaDBAccount)
		DeferCleanup(k8sClient.Delete, ctx, apiMariaDBSecret)

		cell0Account, cell0Secret := mariadb.CreateMariaDBAccountAndSecret(
			cell0.MariaDBAccountName, mariadbv1.MariaDBAccountSpec{})
		DeferCleanup(k8sClient.Delete, ctx, cell0Account)
		DeferCleanup(k8sClient.Delete, ctx, cell0Secret)

		cell1Account, cell1Secret := mariadb.CreateMariaDBAccountAndSecret(
			cell1.MariaDBAccountName, mariadbv1.MariaDBAccountSpec{})
		DeferCleanup(k8sClient.Delete, ctx, cell1Account)
		DeferCleanup(k8sClient.Delete, ctx, cell1Secret)

		cell2Account, cell2Secret := mariadb.CreateMariaDBAccountAndSecret(
			cell2.MariaDBAccountName, mariadbv1.MariaDBAccountSpec{})
		DeferCleanup(k8sClient.Delete, ctx, cell2Account)
		DeferCleanup(k8sClient.Delete, ctx, cell2Secret)

		memcachedSpec := memcachedv1.MemcachedSpec{
			MemcachedSpecCore: memcachedv1.MemcachedSpecCore{
				Replicas: ptr.To(int32(3)),
			},
		}
		DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(novaNames.NovaName.Namespace, MemcachedInstance, memcachedSpec))
		infra.SimulateMemcachedReady(novaNames.MemcachedNamespace)
	})
	When("A NovaCell CR instance is created without any input", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateNovaCell(cell0.CellCRName, GetDefaultNovaCellSpec(cell0)))
		})

		It("reports that input is not ready", func() {
			th.ExpectConditionWithDetails(
				cell0.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				fmt.Sprintf("Input data resources missing: secret/%s", cell0.InternalCellSecretName.Name),
			)
		})

		It("defaults Spec fields", func() {
			cell0 := GetNovaCell(cell0.CellCRName)
			Expect(cell0.Spec.DBPurge.Schedule).To(Equal(ptr.To("0 0 * * *")))
			Expect(cell0.Spec.DBPurge.ArchiveAge).To(Equal(ptr.To(30)))
			Expect(cell0.Spec.DBPurge.PurgeAge).To(Equal(ptr.To(90)))
		})

		It("is not Ready", func() {
			th.ExpectCondition(
				cell0.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("has no hash and no services ready", func() {
			instance := GetNovaCell(cell0.CellCRName)
			Expect(instance.Status.Hash).To(BeEmpty())
			Expect(instance.Status.ConductorServiceReadyCount).To(Equal(int32(0)))
			Expect(instance.Status.MetadataServiceReadyCount).To(Equal(int32(0)))
			Expect(instance.Status.NoVNCPRoxyServiceReadyCount).To(Equal(int32(0)))
			Expect(instance.Status.NovaComputesStatus).To(HaveLen(int(0)))
		})
	})

	When("A NovaCell/cell0 CR instance is created", func() {
		BeforeEach(func() {
			mariadb.CreateMariaDBDatabase(cell0.MariaDBDatabaseName.Namespace, cell0.MariaDBDatabaseName.Name, mariadbv1.MariaDBDatabaseSpec{})
			DeferCleanup(k8sClient.Delete, ctx, mariadb.GetMariaDBDatabase(cell0.MariaDBDatabaseName))

			DeferCleanup(k8sClient.Delete, ctx, CreateDefaultCellInternalSecret(cell0))
			DeferCleanup(th.DeleteInstance, CreateNovaCell(cell0.CellCRName, GetDefaultNovaCellSpec(cell0)))
		})

		It("creates the NovaConductor and tracks its readiness", func() {
			GetNovaConductor(cell0.ConductorName)
			th.ExpectCondition(
				cell0.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaConductorReadyCondition,
				corev1.ConditionFalse,
			)
			novaCell := GetNovaCell(cell0.CellCRName)
			Expect(novaCell.Status.ConductorServiceReadyCount).To(Equal(int32(0)))
		})

		When("NovaConductor is ready", func() {
			BeforeEach(func() {
				th.ExpectCondition(
					cell0.ConductorName,
					ConditionGetterFunc(NovaConductorConditionGetter),
					condition.DBSyncReadyCondition,
					corev1.ConditionFalse,
				)
				th.SimulateJobSuccess(cell0.DBSyncJobName)

				th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)

				th.ExpectCondition(
					cell0.ConductorName,
					ConditionGetterFunc(NovaConductorConditionGetter),
					condition.DBSyncReadyCondition,
					corev1.ConditionTrue,
				)
			})

			It("reports that NovaConductor is ready", func() {
				th.ExpectCondition(
					cell0.CellCRName,
					ConditionGetterFunc(NovaCellConditionGetter),
					novav1.NovaConductorReadyCondition,
					corev1.ConditionTrue,
				)
				novaCell := GetNovaCell(cell0.CellCRName)
				Expect(novaCell.Status.ConductorServiceReadyCount).To(Equal(int32(1)))
			})

			It("does not create Metadata or NoVNCProxy services in cell0", func() {
				th.ExpectCondition(
					cell0.CellCRName,
					ConditionGetterFunc(NovaCellConditionGetter),
					condition.ReadyCondition,
					corev1.ConditionTrue,
				)
				AssertMetadataDoesNotExist(cell0.MetadataName)
				AssertNoVNCProxyDoesNotExist(cell0.NoVNCProxyName)
			})

			It("is Ready", func() {
				th.ExpectCondition(
					cell0.CellCRName,
					ConditionGetterFunc(NovaCellConditionGetter),
					condition.ReadyCondition,
					corev1.ConditionTrue,
				)
			})
		})
	})
	When("A NovaCell/cell1 CR instance is created", func() {
		BeforeEach(func() {
			mariadb.CreateMariaDBDatabase(cell1.MariaDBDatabaseName.Namespace, cell1.MariaDBDatabaseName.Name, mariadbv1.MariaDBDatabaseSpec{})
			DeferCleanup(k8sClient.Delete, ctx, mariadb.GetMariaDBDatabase(cell1.MariaDBDatabaseName))

			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateMetadataCellInternalSecret(cell1),
			)

			spec := GetDefaultNovaCellSpec(cell1)
			spec["metadataServiceTemplate"] = map[string]interface{}{
				"enabled": true,
			}
			spec["novaComputeTemplates"] = map[string]interface{}{
				ironicComputeName: GetDefaultNovaComputeTemplate(),
			}
			DeferCleanup(th.DeleteInstance, CreateNovaCell(cell1.CellCRName, spec))
		})

		It("creates the NovaConductor and tracks its readiness", func() {
			GetNovaConductor(cell1.ConductorName)
			th.ExpectCondition(
				cell1.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaConductorReadyCondition,
				corev1.ConditionFalse,
			)
			novaCell := GetNovaCell(cell1.CellCRName)
			Expect(novaCell.Status.ConductorServiceReadyCount).To(Equal(int32(0)))

			th.ExpectCondition(
				cell1.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionFalse,
			)

			// make conductor ready
			th.SimulateJobSuccess(cell1.DBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell1.ConductorStatefulSetName)

			th.ExpectCondition(
				cell1.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				cell1.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaConductorReadyCondition,
				corev1.ConditionTrue,
			)
			novaCell = GetNovaCell(cell1.CellCRName)
			Expect(novaCell.Status.ConductorServiceReadyCount).To(Equal(int32(1)))
		})

		It("creates the NovaNoVNCProxy and tracks its readiness", func() {
			GetNovaNoVNCProxy(cell1.NoVNCProxyName)
			th.ExpectCondition(
				cell1.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaNoVNCProxyReadyCondition,
				corev1.ConditionFalse,
			)
			novaCell := GetNovaCell(cell1.CellCRName)
			Expect(novaCell.Status.NoVNCPRoxyServiceReadyCount).To(Equal(int32(0)))

			// make novncproxy ready
			th.SimulateStatefulSetReplicaReady(cell1.NoVNCProxyStatefulSetName)

			th.ExpectCondition(
				cell1.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaNoVNCProxyReadyCondition,
				corev1.ConditionTrue,
			)
			novaCell = GetNovaCell(cell1.CellCRName)
			Expect(novaCell.Status.NoVNCPRoxyServiceReadyCount).To(Equal(int32(1)))
		})

		It("creates the NovaMetadata and tracks its readiness", func() {
			metadata := GetNovaMetadata(cell1.MetadataName)
			th.ExpectCondition(
				cell1.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaMetadataReadyCondition,
				corev1.ConditionFalse,
			)
			novaCell := GetNovaCell(cell1.CellCRName)
			Expect(novaCell.Status.MetadataServiceReadyCount).To(Equal(int32(0)))
			Expect(metadata.Spec.ServiceAccount).To(Equal(novaCell.Spec.ServiceAccount))

			// make metadata ready
			th.SimulateStatefulSetReplicaReady(cell1.MetadataStatefulSetName)

			th.ExpectCondition(
				cell1.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaMetadataReadyCondition,
				corev1.ConditionTrue,
			)
			novaCell = GetNovaCell(cell1.CellCRName)
			Expect(novaCell.Status.MetadataServiceReadyCount).To(Equal(int32(1)))
		})

		It("creates the compute config secret", func() {
			th.ExpectCondition(
				cell1.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)
			th.SimulateJobSuccess(cell1.DBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell1.ConductorStatefulSetName)
			// compute config only generated after VNCProxy is ready,
			// so make novncproxy ready
			th.SimulateStatefulSetReplicaReady(cell1.NoVNCProxyStatefulSetName)
			th.ExpectCondition(
				cell1.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaNoVNCProxyReadyCondition,
				corev1.ConditionTrue,
			)

			computeConfigData := th.GetSecret(cell1.ComputeConfigSecretName)
			Expect(computeConfigData).ShouldNot(BeNil())
			Expect(computeConfigData.Data).Should(HaveKey("01-nova.conf"))
			configData := string(computeConfigData.Data["01-nova.conf"])
			Expect(configData).To(ContainSubstring("transport_url=rabbit://cell1/fake"))
			Expect(configData).To(ContainSubstring("username = nova\npassword = service-password\n"))
			vncURLConfig := fmt.Sprintf("novncproxy_base_url = http://%s/vnc_lite.html",
				fmt.Sprintf("nova-novncproxy-%s-public.%s.svc:6080", cell1.CellName, cell1.CellCRName.Namespace))
			Expect(configData).To(ContainSubstring(vncURLConfig))
			Expect(configData).To(ContainSubstring("[vnc]\nenabled = True"))
			Expect(configData).Should(
				ContainSubstring("[upgrade_levels]\ncompute = auto"))
			Expect(configData).To(
				ContainSubstring(
					"live_migration_uri = qemu+ssh://nova@%s/system?keyfile=/var/lib/nova/.ssh/ssh-privatekey"))
			Expect(configData).To(ContainSubstring("cpu_power_management=true"))
			// The nova compute agent is expected to log to stdout. On edpm nodes this allows podman to
			// capture the logs and make them available via `podman logs` while also redirecting the logs
			// to the systemd journal. For openshift compute services, the logs are captured by the openshift
			// logging infrastructure.
			Expect(configData).To(Not(ContainSubstring("log_file = /var/log/nova/nova-compute.log")))

			th.ExpectCondition(
				cell1.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaComputeServiceConfigReady,
				corev1.ConditionTrue,
			)

			Expect(GetNovaCell(cell1.CellCRName).Status.Hash).To(HaveKey(cell1.ComputeConfigSecretName.Name))
		})

		It("updates the novncproxy_base_url in the compute config secret when VNCProxy endpointURL is set", func() {
			// Update the VNCProxy endpointURL
			Eventually(func(g Gomega) {
				novaCell := GetNovaCell(cell1.CellCRName)
				novaCell.Spec.NoVNCProxyServiceTemplate.Override.Service = &service.RoutedOverrideSpec{
					EndpointURL: ptr.To("http://foo"),
				}

				g.Expect(k8sClient.Update(ctx, novaCell)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			th.ExpectCondition(
				cell1.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)
			th.SimulateJobSuccess(cell1.DBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell1.ConductorStatefulSetName)
			// compute config only generated after VNCProxy is ready,
			// so make novncproxy ready
			th.SimulateStatefulSetReplicaReady(cell1.NoVNCProxyStatefulSetName)
			th.ExpectCondition(
				cell1.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaNoVNCProxyReadyCondition,
				corev1.ConditionTrue,
			)

			computeConfigData := th.GetSecret(cell1.ComputeConfigSecretName)
			Expect(computeConfigData).ShouldNot(BeNil())
			Expect(computeConfigData.Data).Should(HaveKey("01-nova.conf"))
			configData := string(computeConfigData.Data["01-nova.conf"])
			Expect(configData).To(ContainSubstring("transport_url=rabbit://cell1/fake"))
			Expect(configData).To(ContainSubstring("username = nova\npassword = service-password\n"))
			vncURLConfig := "novncproxy_base_url = http://foo/vnc_lite.html"
			Expect(configData).To(ContainSubstring(vncURLConfig))
			Expect(configData).To(ContainSubstring("[vnc]\nenabled = True"))

			th.ExpectCondition(
				cell1.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaComputeServiceConfigReady,
				corev1.ConditionTrue,
			)

			Expect(GetNovaCell(cell1.CellCRName).Status.Hash).To(HaveKey(cell1.ComputeConfigSecretName.Name))
		})

		It("is Ready when all cell services is ready", func() {
			th.SimulateJobSuccess(cell1.DBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell1.ConductorStatefulSetName)
			th.SimulateStatefulSetReplicaReady(cell1.NovaComputeStatefulSetName)
			th.SimulateStatefulSetReplicaReady(cell1.NoVNCProxyStatefulSetName)
			th.SimulateStatefulSetReplicaReady(cell1.MetadataStatefulSetName)
			cell := GetNovaCell(cell1.CellCRName)
			th.ExpectCondition(
				cell1.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)

			Expect(cell.Status.NovaComputesStatus).To(HaveKey(ironicComputeName))
		})

		It("deletes NoVNCProxy if it is disabled later", func() {
			th.SimulateJobSuccess(cell1.DBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell1.ConductorStatefulSetName)
			th.SimulateStatefulSetReplicaReady(cell1.NovaComputeStatefulSetName)
			th.SimulateStatefulSetReplicaReady(cell1.NoVNCProxyStatefulSetName)
			th.SimulateStatefulSetReplicaReady(cell1.MetadataStatefulSetName)

			th.ExpectCondition(
				cell1.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)

			// Cell is ready. Now disable NoVNCProxy in it
			Eventually(func(g Gomega) {
				novaCell := GetNovaCell(cell1.CellCRName)
				novaCell.Spec.NoVNCProxyServiceTemplate.Enabled = ptr.To(false)

				g.Expect(k8sClient.Update(ctx, novaCell)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			// Assert that the NoVNCProxy is deleted
			AssertNoVNCProxyDoesNotExist(cell1.NoVNCProxyName)
			th.ExpectCondition(
				cell1.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)

			// updates the compute config and disables VNC there
			computeConfigData := th.GetSecret(cell1.ComputeConfigSecretName)
			Expect(computeConfigData).ShouldNot(BeNil())
			Expect(computeConfigData.Data).Should(HaveKey("01-nova.conf"))
			configData := string(computeConfigData.Data["01-nova.conf"])
			Expect(configData).NotTo(ContainSubstring("novncproxy_base_url"))
			Expect(configData).To(ContainSubstring("[vnc]\nenabled = False"))

		})
		It("deletes NovaMetadata if it is disabled later", func() {
			th.SimulateJobSuccess(cell1.DBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell1.ConductorStatefulSetName)
			th.SimulateStatefulSetReplicaReady(cell1.NovaComputeStatefulSetName)
			th.SimulateStatefulSetReplicaReady(cell1.NoVNCProxyStatefulSetName)
			th.SimulateStatefulSetReplicaReady(cell1.MetadataStatefulSetName)

			th.ExpectCondition(
				cell1.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)

			// Cell is ready. Now disable NovaMetadata in it
			Eventually(func(g Gomega) {
				novaCell := GetNovaCell(cell1.CellCRName)
				novaCell.Spec.MetadataServiceTemplate.Enabled = ptr.To(false)

				g.Expect(k8sClient.Update(ctx, novaCell)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			AssertMetadataDoesNotExist(cell1.MetadataName)
			th.ExpectCondition(
				cell1.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("deletes NovaCompute if removed from the template", func() {
			th.SimulateJobSuccess(cell1.DBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell1.ConductorStatefulSetName)
			th.SimulateStatefulSetReplicaReady(cell1.NovaComputeStatefulSetName)
			th.SimulateStatefulSetReplicaReady(cell1.NoVNCProxyStatefulSetName)
			th.SimulateStatefulSetReplicaReady(cell1.MetadataStatefulSetName)

			th.ExpectCondition(
				cell1.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)

			novaCell := GetNovaCell(cell1.CellCRName)
			Expect(novaCell.Status.NovaComputesStatus[ironicComputeName]).To(Equal(
				novav1.NovaComputeCellStatus{Deployed: true, Errors: false}))

			// Cell is ready. Now remove the compute definition
			Eventually(func(g Gomega) {
				novaCell := GetNovaCell(cell1.CellCRName)
				novaCell.Spec.NovaComputeTemplates = map[string]novav1.NovaComputeTemplate{}

				g.Expect(k8sClient.Update(ctx, novaCell)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			AssertComputeDoesNotExist(cell1.NovaComputeName)
			th.ExpectCondition(
				cell1.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
			Eventually(func(g Gomega) {
				novaCell = GetNovaCell(cell1.CellCRName)
				g.Expect(novaCell.Status.NovaComputesStatus).To(BeEmpty())
			}, timeout, interval).Should(Succeed())

		})
	})
	When("A NovaCell/cell2 CR instance is created without VNCProxy", func() {
		BeforeEach(func() {
			mariadb.CreateMariaDBDatabase(cell2.MariaDBDatabaseName.Namespace, cell2.MariaDBDatabaseName.Name, mariadbv1.MariaDBDatabaseSpec{})
			DeferCleanup(k8sClient.Delete, ctx, mariadb.GetMariaDBDatabase(cell2.MariaDBDatabaseName))

			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateMetadataCellInternalSecret(cell2),
			)
			spec := GetDefaultNovaCellSpec(cell2)
			spec["noVNCProxyServiceTemplate"] = map[string]interface{}{
				"enabled": false,
			}
			DeferCleanup(th.DeleteInstance, CreateNovaCell(cell2.CellCRName, spec))
		})

		It("creates the compute config secret", func() {
			th.ExpectCondition(
				cell2.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)

			computeConfigData := th.GetSecret(cell2.ComputeConfigSecretName)
			Expect(computeConfigData).ShouldNot(BeNil())
			Expect(computeConfigData.Data).Should(HaveKey("01-nova.conf"))
			configData := string(computeConfigData.Data["01-nova.conf"])
			Expect(configData).To(ContainSubstring("transport_url=rabbit://cell2/fake"))
			Expect(configData).To(ContainSubstring("username = nova\npassword = service-password\n"))
			// There is no VNCProxy created but we still get a compute config just
			// without any vnc proxy url and therefore vnc disabled
			Expect(configData).NotTo(ContainSubstring("novncproxy_base_url"))
			Expect(configData).To(ContainSubstring("[vnc]\nenabled = False"))

			th.ExpectCondition(
				cell2.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaComputeServiceConfigReady,
				corev1.ConditionTrue,
			)

			Expect(GetNovaCell(cell2.CellCRName).Status.Hash).To(HaveKey(cell2.ComputeConfigSecretName.Name))
		})

		It("is Ready without NoVNCProxy", func() {
			th.SimulateJobSuccess(cell2.DBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell2.ConductorStatefulSetName)

			th.ExpectCondition(
				cell2.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
			AssertNoVNCProxyDoesNotExist(cell2.NoVNCProxyName)
		})

		It("deploys NoVNCProxy if it is later enabled", func() {
			th.SimulateJobSuccess(cell2.DBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell2.ConductorStatefulSetName)

			th.ExpectCondition(
				cell2.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
			oldComputeConfigHash := GetNovaCell(cell2.CellCRName).Status.Hash[cell2.ComputeConfigSecretName.Name]

			// Now that the cell is deployed without VNCProxy, enabled the
			// VNCProxy for this cell
			Eventually(func(g Gomega) {
				novaCell := GetNovaCell(cell2.CellCRName)
				novaCell.Spec.NoVNCProxyServiceTemplate.Enabled = ptr.To(true)

				g.Expect(k8sClient.Update(ctx, novaCell)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			// Check that the NVCProxy is now deployed
			GetNovaNoVNCProxy(cell2.NoVNCProxyName)
			th.ExpectCondition(
				cell2.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaNoVNCProxyReadyCondition,
				corev1.ConditionFalse,
			)
			novaCell := GetNovaCell(cell2.CellCRName)
			Expect(novaCell.Status.NoVNCPRoxyServiceReadyCount).To(Equal(int32(0)))

			// make novncproxy ready
			th.SimulateStatefulSetReplicaReady(cell2.NoVNCProxyStatefulSetName)

			th.ExpectCondition(
				cell2.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaNoVNCProxyReadyCondition,
				corev1.ConditionTrue,
			)
			novaCell = GetNovaCell(cell2.CellCRName)
			Expect(novaCell.Status.NoVNCPRoxyServiceReadyCount).To(Equal(int32(1)))

			// And the compute config is updated with the VNCProxy url
			th.ExpectCondition(
				cell2.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaComputeServiceConfigReady,
				corev1.ConditionTrue,
			)

			computeConfigData := th.GetSecret(cell2.ComputeConfigSecretName)
			Expect(computeConfigData).ShouldNot(BeNil())
			Expect(computeConfigData.Data).Should(HaveKey("01-nova.conf"))
			configData := string(computeConfigData.Data["01-nova.conf"])
			vncURLConfig := fmt.Sprintf("novncproxy_base_url = http://%s/vnc_lite.html",
				fmt.Sprintf("nova-novncproxy-%s-public.%s.svc:6080", cell2.CellName, cell2.CellCRName.Namespace))
			Expect(configData).To(ContainSubstring(vncURLConfig))
			Expect(configData).To(ContainSubstring("[vnc]\nenabled = True"))

			Expect(GetNovaCell(cell2.CellCRName).Status.Hash[cell2.ComputeConfigSecretName.Name]).NotTo(BeNil())
			Expect(GetNovaCell(cell2.CellCRName).Status.Hash[cell2.ComputeConfigSecretName.Name]).NotTo(Equal(oldComputeConfigHash))
		})
		It("fails if VNC is enabled later while a manually created VNC already exists until that is deleted", func() {
			th.SimulateJobSuccess(cell2.DBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell2.ConductorStatefulSetName)

			th.ExpectCondition(
				cell2.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)

			// Now that the cell is deployed without VNCProxy, create one
			// manually not owned by the cell to simulate some advanced user
			spec := GetDefaultNovaNoVNCProxySpec(cell2)
			// We don't need to make this deployed successfully for this test
			CreateNovaNoVNCProxy(cell2.NoVNCProxyName, spec)

			// NOTE(gibi): The manually created NoVNCProxy CR should not
			// trigger any NovaCell reconciliation but if for other reasons
			// the NovaCell is reconciled then that will see the NovaNoVNCProxy
			// instance exists. So lets trigger a NovaCell reconciliation to
			// make sure that does not try to mess with the manually created
			// NoVNCProxy.
			Eventually(func(g Gomega) {
				novaCell := GetNovaCell(cell2.CellCRName)
				novaCell.Spec.ConductorServiceTemplate.Replicas = ptr.To[int32](3)

				g.Expect(k8sClient.Update(ctx, novaCell)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			// Just ensure that it is not automatically gets owned or deleted
			// by the cell
			Consistently(func(g Gomega) {
				vnc := GetNovaNoVNCProxy(cell2.NoVNCProxyName)
				g.Expect(vnc.OwnerReferences).To(BeEmpty())
			}, consistencyTimeout, interval).Should(Succeed())

			// Now enable VNCProxy in the cell config
			Eventually(func(g Gomega) {
				novaCell := GetNovaCell(cell2.CellCRName)
				novaCell.Spec.NoVNCProxyServiceTemplate.Enabled = ptr.To(true)

				g.Expect(k8sClient.Update(ctx, novaCell)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			// The cell goes to error state as the NoVNCProxy is not owned by
			// it
			th.ExpectConditionWithDetails(
				cell2.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaNoVNCProxyReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				fmt.Sprintf(
					"NovaNoVNCProxy error occurred cannot update "+
						"NovaNoVNCProxy/%s as the cell is not owning it",
					cell2.NoVNCProxyName.Name,
				),
			)

			// Now simulate that the user follows our documentation and deletes
			// the manually created NoVNCPRoxy CR
			th.DeleteInstance(GetNovaNoVNCProxy(cell2.NoVNCProxyName))
			// NOTE(gibi): This only needed in envtest, in a real k8s
			// deployment the garbage collector would delete the StatefulSet
			// when its parents, the NoVNCProxy, is deleted, but that garbage
			// collector does not run in envtest. So we manually clean up here
			th.DeleteInstance(th.GetStatefulSet(cell2.NoVNCProxyStatefulSetName))

			// As the manually created NoVNCProxy is deleted the controller is
			// unblocked to deploy its own NoVNCProxy CR
			th.ExpectConditionWithDetails(
				cell2.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaNoVNCProxyReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"Deployment in progress",
			)
			th.SimulateStatefulSetReplicaReady(cell2.NoVNCProxyStatefulSetName)
			th.ExpectCondition(
				cell2.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaNoVNCProxyReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				cell2.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("deploys NovaMetadata if it is later enabled", func() {
			th.SimulateJobSuccess(cell2.DBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell2.ConductorStatefulSetName)

			th.ExpectCondition(
				cell2.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)

			// Now that the cell is deployed, enabled the Metadata for this
			// cell
			Eventually(func(g Gomega) {
				novaCell := GetNovaCell(cell2.CellCRName)
				novaCell.Spec.MetadataServiceTemplate.Enabled = ptr.To(true)

				g.Expect(k8sClient.Update(ctx, novaCell)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			// Check that the Metadata is now deployed
			GetNovaMetadata(cell2.MetadataName)
			th.ExpectCondition(
				cell2.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaMetadataReadyCondition,
				corev1.ConditionFalse,
			)
			novaCell := GetNovaCell(cell2.CellCRName)
			Expect(novaCell.Status.MetadataServiceReadyCount).To(Equal(int32(0)))

			// make metadata ready
			th.SimulateStatefulSetReplicaReady(cell2.MetadataStatefulSetName)

			th.ExpectCondition(
				cell2.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaMetadataReadyCondition,
				corev1.ConditionTrue,
			)
			novaCell = GetNovaCell(cell2.CellCRName)
			Expect(novaCell.Status.MetadataServiceReadyCount).To(Equal(int32(1)))
		})
		It("fails if Metadata is enabled later while a manually created Metadata already exists until that is deleted", func() {
			th.SimulateJobSuccess(cell2.DBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell2.ConductorStatefulSetName)

			th.ExpectCondition(
				cell2.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)

			// Now that the cell is deployed, create one Metadata
			// manually not owned by the cell to simulate some advanced user
			spec := GetDefaultNovaMetadataSpec(cell2.InternalCellSecretName)
			// We don't need to make this deployed successfully for this test
			CreateNovaMetadata(cell2.MetadataName, spec)

			// NOTE(gibi): The manually created NovaMetadata CR should not
			// trigger any NovaCell reconciliation but if for other reasons
			// the NovaCell is reconciled then that will see the NovaMetadata
			// instance exists. So lets trigger a NovaCell reconciliation to
			// make sure that does not try to mess with the manually created
			// NovaMetadata.
			Eventually(func(g Gomega) {
				novaCell := GetNovaCell(cell2.CellCRName)
				novaCell.Spec.ConductorServiceTemplate.Replicas = ptr.To[int32](3)

				g.Expect(k8sClient.Update(ctx, novaCell)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			// Just ensure that it is not automatically gets owned or deleted
			// by the cell
			Consistently(func(g Gomega) {
				metadata := GetNovaMetadata(cell2.MetadataName)
				g.Expect(metadata.OwnerReferences).To(BeEmpty())
			}, consistencyTimeout, interval).Should(Succeed())

			// Now enable Metadata in the cell config
			Eventually(func(g Gomega) {
				novaCell := GetNovaCell(cell2.CellCRName)
				novaCell.Spec.MetadataServiceTemplate.Enabled = ptr.To(true)

				g.Expect(k8sClient.Update(ctx, novaCell)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			// The cell goes to error state as the NovaMetadata is not owned by
			// it
			th.ExpectConditionWithDetails(
				cell2.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaMetadataReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				fmt.Sprintf(
					"NovaMetadata error occurred cannot update "+
						"NovaMetadata/%s as the cell is not owning it",
					cell2.MetadataName.Name,
				),
			)

			// Now simulate that the user follows our documentation and deletes
			// the manually created NovaMetadata CR
			th.DeleteInstance(GetNovaMetadata(cell2.MetadataName))
			// NOTE(gibi): This only needed in envtest, in a real k8s
			// deployment the garbage collector would delete the StatefulSet
			// when its parents, the NovaMetadata, is deleted, but that garbage
			// collector does not run in envtest. So we manually clean up here
			th.DeleteInstance(th.GetStatefulSet(cell2.MetadataStatefulSetName))

			th.ExpectConditionWithDetails(
				cell2.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaMetadataReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"Deployment in progress",
			)

			// As the manually created Metadata is deleted the controller is
			// unblocked to deploy its own NovaMetadata CR
			th.SimulateStatefulSetReplicaReady(cell2.MetadataStatefulSetName)
			th.ExpectCondition(
				cell2.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaMetadataReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				cell2.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})
	When("NovaCell/cell0 is reconfigured", func() {
		BeforeEach(func() {
			mariadb.CreateMariaDBDatabase(cell0.MariaDBDatabaseName.Namespace, cell0.MariaDBDatabaseName.Name, mariadbv1.MariaDBDatabaseSpec{})
			DeferCleanup(k8sClient.Delete, ctx, mariadb.GetMariaDBDatabase(cell0.MariaDBDatabaseName))

			DeferCleanup(k8sClient.Delete, ctx, CreateDefaultCellInternalSecret(cell0))
			DeferCleanup(th.DeleteInstance, CreateNovaCell(cell0.CellCRName, GetDefaultNovaCellSpec(cell0)))
			th.SimulateJobSuccess(cell0.DBSyncJobName)

			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)

			cell := GetNovaCell(cell0.CellCRName)
			Expect(cell.Spec.MetadataServiceTemplate.Enabled).To(Equal(ptr.To(false)))
			th.ExpectCondition(
				cell0.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaConductorReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("applies new NetworkAttachments configuration to its Conductor", func() {
			Eventually(func(g Gomega) {
				novaCell := GetNovaCell(cell0.CellCRName)
				novaCell.Spec.ConductorServiceTemplate.NetworkAttachments = append(
					novaCell.Spec.ConductorServiceTemplate.NetworkAttachments, "internalapi")

				g.Expect(k8sClient.Update(ctx, novaCell)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			th.ExpectConditionWithDetails(
				cell0.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NetworkAttachment resources missing: internalapi",
			)

			th.ExpectConditionWithDetails(
				cell0.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NetworkAttachment resources missing: internalapi",
			)

			DeferCleanup(th.DeleteInstance, th.CreateNetworkAttachmentDefinition(cell0.InternalAPINetworkNADName))

			th.ExpectConditionWithDetails(
				cell0.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occurred "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
			)

			th.ExpectConditionWithDetails(
				cell0.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occurred "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
			)

			th.SimulateStatefulSetReplicaReadyWithPods(
				cell0.ConductorStatefulSetName,
				map[string][]string{cell0.CellCRName.Namespace + "/internalapi": {"10.0.0.1"}},
			)

			th.ExpectCondition(
				cell0.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionTrue,
			)

			th.ExpectCondition(
				cell0.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})

	When("NovaCell/cell1 with metadata is reconfigured", func() {
		BeforeEach(func() {
			mariadb.CreateMariaDBDatabase(cell1.MariaDBDatabaseName.Namespace, cell1.MariaDBDatabaseName.Name, mariadbv1.MariaDBDatabaseSpec{})
			DeferCleanup(k8sClient.Delete, ctx, mariadb.GetMariaDBDatabase(cell1.MariaDBDatabaseName))

			DeferCleanup(k8sClient.Delete, ctx, CreateMetadataCellInternalSecret(cell1))

			spec := GetDefaultNovaCellSpec(cell1)
			spec["metadataServiceTemplate"] = map[string]interface{}{
				"enabled": true,
			}
			DeferCleanup(th.DeleteInstance, CreateNovaCell(cell1.CellCRName, spec))
			th.SimulateJobSuccess(cell1.DBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell1.ConductorStatefulSetName)
			th.SimulateStatefulSetReplicaReady(cell1.NoVNCProxyStatefulSetName)
			th.SimulateStatefulSetReplicaReady(cell1.MetadataStatefulSetName)

			cell := GetNovaCell(cell1.CellCRName)
			Expect(cell.Spec.MetadataServiceTemplate.Replicas).To(Equal(ptr.To[int32](1)))
			th.ExpectCondition(
				cell1.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaConductorReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				cell1.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaNoVNCProxyReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				cell1.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaMetadataReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				cell1.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("applies zero replicas to NovaNoVNCProxy if requested", func() {
			Eventually(func(g Gomega) {
				novaCell := GetNovaCell(cell1.CellCRName)
				novaCell.Spec.NoVNCProxyServiceTemplate.Replicas = ptr.To[int32](0)

				g.Expect(k8sClient.Update(ctx, novaCell)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				th.SimulateStatefulSetReplicaReady(cell1.NoVNCProxyStatefulSetName)
				ss := th.GetStatefulSet(cell1.NoVNCProxyStatefulSetName)
				g.Expect(ss.Spec.Replicas).To(Equal(ptr.To[int32](0)))
			}, timeout, interval).Should(Succeed())
			th.ExpectCondition(
				cell1.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaNoVNCProxyReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				cell1.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("applies zero replicas to NovaMetadata if requested", func() {
			Eventually(func(g Gomega) {
				novaCell := GetNovaCell(cell1.CellCRName)
				novaCell.Spec.MetadataServiceTemplate.Replicas = ptr.To[int32](0)

				g.Expect(k8sClient.Update(ctx, novaCell)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				ss := th.GetStatefulSet(cell1.MetadataStatefulSetName)
				g.Expect(ss.Spec.Replicas).To(Equal(ptr.To[int32](0)))
			}, timeout, interval).Should(Succeed())
			th.ExpectCondition(
				cell1.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaMetadataReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				cell1.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})
})

var _ = Describe("NovaCell controller webhook", func() {
	BeforeEach(func() {
		memcachedSpec := memcachedv1.MemcachedSpec{
			MemcachedSpecCore: memcachedv1.MemcachedSpecCore{
				Replicas: ptr.To(int32(3)),
			},
		}
		DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(novaNames.NovaName.Namespace, MemcachedInstance, memcachedSpec))
		infra.SimulateMemcachedReady(novaNames.MemcachedNamespace)

	})
	It("name is too long", func() {
		cell := GetCellNames(novaNames.NovaName, uuid.New().String())
		DeferCleanup(k8sClient.Delete, ctx, CreateDefaultCellInternalSecret(cell))

		spec := GetDefaultNovaCellSpec(cell)
		rawObj := map[string]interface{}{
			"apiVersion": "nova.openstack.org/v1beta1",
			"kind":       "NovaCell",
			"metadata": map[string]interface{}{
				"name":      cell.CellCRName.Name,
				"namespace": cell.CellCRName.Namespace,
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
