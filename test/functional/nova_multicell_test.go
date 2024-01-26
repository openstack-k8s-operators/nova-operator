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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"

	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/nova-operator/controllers"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Nova multicell", func() {
	When("Nova CR instance is created with 3 cells", func() {
		BeforeEach(func() {
			// TODO(bogdando): deduplicate this into CreateNovaWith3CellsAndEnsureReady()
			DeferCleanup(k8sClient.Delete, ctx, CreateNovaSecretFor3Cells(novaNames.NovaName.Namespace, SecretName))
			DeferCleanup(k8sClient.Delete, ctx, CreateNovaMessageBusSecret(cell0))
			DeferCleanup(k8sClient.Delete, ctx, CreateNovaMessageBusSecret(cell1))
			DeferCleanup(k8sClient.Delete, ctx, CreateNovaMessageBusSecret(cell2))

			serviceSpec := corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 3306}}}
			DeferCleanup(mariadb.DeleteDBService, mariadb.CreateDBService(novaNames.APIMariaDBDatabaseName.Namespace, novaNames.APIMariaDBDatabaseName.Name, serviceSpec))
			DeferCleanup(mariadb.DeleteDBService, mariadb.CreateDBService(cell0.MariaDBDatabaseName.Namespace, cell0.MariaDBDatabaseName.Name, serviceSpec))
			DeferCleanup(mariadb.DeleteDBService, mariadb.CreateDBService(cell1.MariaDBDatabaseName.Namespace, cell1.MariaDBDatabaseName.Name, serviceSpec))
			DeferCleanup(mariadb.DeleteDBService, mariadb.CreateDBService(cell2.MariaDBDatabaseName.Namespace, cell2.MariaDBDatabaseName.Name, serviceSpec))

			spec := GetDefaultNovaSpec()
			cell0Template := GetDefaultNovaCellTemplate()
			cell0Template["cellDatabaseInstance"] = cell0.MariaDBDatabaseName.Name
			cell0Template["cellDatabaseUser"] = "nova_cell0"

			cell1Template := GetDefaultNovaCellTemplate()
			cell1Template["cellDatabaseInstance"] = cell1.MariaDBDatabaseName.Name
			cell1Template["cellDatabaseUser"] = "nova_cell1"
			cell1Template["cellMessageBusInstance"] = cell1.TransportURLName.Name
			cell1Template["passwordSelectors"] = map[string]interface{}{
				"database": "NovaCell1DatabasePassword",
			}
			cell1Template["novaComputeTemplates"] = map[string]interface{}{
				ironicComputeName: GetDefaultNovaComputeTemplate(),
			}

			cell2Template := GetDefaultNovaCellTemplate()
			cell2Template["cellDatabaseInstance"] = cell2.MariaDBDatabaseName.Name
			cell2Template["cellDatabaseUser"] = "nova_cell2"
			cell2Template["cellMessageBusInstance"] = cell2.TransportURLName.Name
			cell2Template["hasAPIAccess"] = false
			cell2Template["passwordSelectors"] = map[string]interface{}{
				"database": "NovaCell2DatabasePassword",
			}

			spec["cellTemplates"] = map[string]interface{}{
				"cell0": cell0Template,
				"cell1": cell1Template,
				"cell2": cell2Template,
			}
			spec["apiDatabaseInstance"] = novaNames.APIMariaDBDatabaseName.Name
			spec["apiMessageBusInstance"] = cell0.TransportURLName.Name

			DeferCleanup(th.DeleteInstance, CreateNova(novaNames.NovaName, spec))
			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(novaNames.NovaName.Namespace))
			keystone.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
		})

		It("creates cell0 NovaCell", func() {
			mariadb.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(novaNames.APIMariaDBDatabaseName)
			mariadb.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell0.MariaDBDatabaseName)
			infra.SimulateTransportURLReady(cell0.TransportURLName)

			th.ExpectCondition(
				cell0.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionFalse,
			)
			// assert that cell0 conductor is using the same DB as the API
			dbSync := th.GetJob(cell0.DBSyncJobName)
			Expect(dbSync.Spec.Template.Spec.InitContainers).To(HaveLen(0))

			configDataMap := th.GetSecret(cell0.ConductorConfigDataName)
			Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
			configData := string(configDataMap.Data["01-nova.conf"])

			Expect(configData).To(
				ContainSubstring(
					fmt.Sprintf(
						"[database]\nconnection = mysql+pymysql://nova_cell0:cell0-database-password@hostname-for-%s.%s.svc/nova_cell0",
						cell0.MariaDBDatabaseName.Name, novaNames.Namespace)),
			)
			Expect(configData).To(
				ContainSubstring(
					fmt.Sprintf(
						"[api_database]\nconnection = mysql+pymysql://nova_api:api-database-password@hostname-for-%s.%s.svc/nova_api",
						novaNames.APIMariaDBDatabaseName.Name, novaNames.Namespace)),
			)
			// and that it is using the top level MQ
			Expect(configData).To(ContainSubstring("transport_url=rabbit://cell0/fake"))

			th.SimulateJobSuccess(cell0.DBSyncJobName)
			th.ExpectCondition(
				cell0.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionTrue,
			)
			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
			th.SimulateJobSuccess(cell0.CellMappingJobName)

			th.ExpectCondition(
				cell0.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaConductorReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsReadyCondition,
				corev1.ConditionFalse,
			)
			Eventually(func(g Gomega) {
				nova := GetNova(novaNames.NovaName)
				g.Expect(nova.Status.RegisteredCells).To(
					HaveKeyWithValue(cell0.CellCRName.Name, Not(BeEmpty())))
			}, timeout, interval).Should(Succeed())
		})

		It("creates NovaAPI", func() {
			mariadb.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(novaNames.APIMariaDBDatabaseName)
			mariadb.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell0.MariaDBDatabaseName)
			infra.SimulateTransportURLReady(cell0.TransportURLName)
			th.SimulateJobSuccess(cell0.DBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
			th.SimulateJobSuccess(cell0.CellMappingJobName)

			api := GetNovaAPI(novaNames.APIName)
			one := int32(1)
			Expect(api.Spec.Replicas).Should(BeEquivalentTo(&one))
			Expect(api.Spec.Cell0DatabaseHostname).To(Equal(fmt.Sprintf("hostname-for-%s.%s.svc", cell0.MariaDBDatabaseName.Name, novaNames.Namespace)))
			Expect(api.Spec.APIDatabaseHostname).To(Equal(fmt.Sprintf("hostname-for-%s.%s.svc", novaNames.APIMariaDBDatabaseName.Name, novaNames.Namespace)))

			configDataMap := th.GetSecret(novaNames.APIConfigDataName)
			Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
			configData := string(configDataMap.Data["01-nova.conf"])
			Expect(configData).To(
				ContainSubstring(
					fmt.Sprintf(
						"[database]\nconnection = mysql+pymysql://nova_cell0:cell0-database-password@hostname-for-%s.%s.svc/nova_cell0",
						cell0.MariaDBDatabaseName.Name, novaNames.Namespace)),
			)
			Expect(configData).To(
				ContainSubstring(
					fmt.Sprintf(
						"[api_database]\nconnection = mysql+pymysql://nova_api:api-database-password@hostname-for-%s.%s.svc/nova_api",
						novaNames.APIMariaDBDatabaseName.Name, novaNames.Namespace)),
			)
			Expect(configData).To(ContainSubstring("transport_url=rabbit://cell0/fake"))

			th.SimulateStatefulSetReplicaReady(novaNames.APIDeploymentName)

			th.ExpectCondition(
				novaNames.APIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionTrue,
			)

			keystone.SimulateKeystoneEndpointReady(novaNames.APIKeystoneEndpointName)
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAPIReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("creates all cell DBs", func() {
			mariadb.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(novaNames.APIMariaDBDatabaseName)
			infra.SimulateTransportURLReady(cell0.TransportURLName)
			mariadb.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell0.MariaDBDatabaseName)
			th.SimulateJobSuccess(cell0.DBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
			th.SimulateJobSuccess(cell0.CellMappingJobName)
			th.SimulateStatefulSetReplicaReady(novaNames.APIDeploymentName)
			keystone.SimulateKeystoneEndpointReady(novaNames.APIKeystoneEndpointName)

			mariadb.SimulateMariaDBDatabaseCompleted(cell1.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell1.MariaDBDatabaseName)
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsDBReadyCondition,
				corev1.ConditionFalse,
			)
			mariadb.SimulateMariaDBDatabaseCompleted(cell2.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell2.MariaDBDatabaseName)
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsDBReadyCondition,
				corev1.ConditionTrue,
			)

		})

		It("creates all cell MQs", func() {
			infra.SimulateTransportURLReady(cell0.TransportURLName)
			infra.SimulateTransportURLReady(cell1.TransportURLName)
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsMQReadyCondition,
				corev1.ConditionFalse,
			)
			infra.SimulateTransportURLReady(cell2.TransportURLName)
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsMQReadyCondition,
				corev1.ConditionTrue,
			)

		})

		It("creates cell1 NovaCell", func() {
			mariadb.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(novaNames.APIMariaDBDatabaseName)
			mariadb.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell0.MariaDBDatabaseName)
			infra.SimulateTransportURLReady(cell0.TransportURLName)
			th.SimulateJobSuccess(cell0.DBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
			th.SimulateJobSuccess(cell0.CellMappingJobName)
			th.SimulateStatefulSetReplicaReady(novaNames.APIDeploymentName)
			keystone.SimulateKeystoneEndpointReady(novaNames.APIKeystoneEndpointName)
			mariadb.SimulateMariaDBDatabaseCompleted(cell1.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell1.MariaDBDatabaseName)
			infra.SimulateTransportURLReady(cell1.TransportURLName)

			th.ExpectCondition(
				cell1.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionFalse,
			)
			// assert that cell1 using its own DB but has access to the API DB
			dbSync := th.GetJob(cell1.DBSyncJobName)
			Expect(dbSync.Spec.Template.Spec.InitContainers).To(HaveLen(0))
			configDataMap := th.GetSecret(cell1.ConductorConfigDataName)
			Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
			configData := string(configDataMap.Data["01-nova.conf"])
			Expect(configData).To(
				ContainSubstring(
					fmt.Sprintf(
						"[database]\nconnection = mysql+pymysql://nova_cell1:cell1-database-password@hostname-for-%s.%s.svc/nova_cell1",
						cell1.MariaDBDatabaseName.Name, novaNames.Namespace)),
			)
			Expect(configData).To(
				ContainSubstring(
					fmt.Sprintf(
						"[api_database]\nconnection = mysql+pymysql://nova_api:api-database-password@hostname-for-%s.%s.svc/nova_api",
						novaNames.APIMariaDBDatabaseName.Name, novaNames.Namespace)),
			)
			Expect(configData).To(ContainSubstring("transport_url=rabbit://cell1/fake"))

			th.SimulateStatefulSetReplicaReady(cell1.NoVNCProxyStatefulSetName)
			th.SimulateJobSuccess(cell1.DBSyncJobName)
			th.ExpectCondition(
				cell1.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionTrue,
			)

			th.SimulateStatefulSetReplicaReady(cell1.NovaComputeStatefulSetName)
			th.SimulateStatefulSetReplicaReady(cell1.ConductorStatefulSetName)
			th.SimulateJobSuccess(cell1.CellMappingJobName)
			th.ExpectCondition(
				cell1.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaConductorReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				cell1.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaAllComputesReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsReadyCondition,
				corev1.ConditionFalse,
			)
			Eventually(func(g Gomega) {
				nova := GetNova(novaNames.NovaName)
				g.Expect(nova.Status.RegisteredCells).To(
					HaveKeyWithValue(cell0.CellCRName.Name, Not(BeEmpty())))
				g.Expect(nova.Status.RegisteredCells).To(
					HaveKeyWithValue(cell1.CellCRName.Name, Not(BeEmpty())))
			}, timeout, interval).Should(Succeed())

			// RegisteredCells are distributed
			nova := GetNova(novaNames.NovaName)
			api := GetNovaAPI(novaNames.APIName)
			Expect(api.Spec.RegisteredCells).To(Equal(nova.Status.RegisteredCells))
			scheduler := GetNovaScheduler(novaNames.SchedulerName)
			Expect(scheduler.Spec.RegisteredCells).To(Equal(nova.Status.RegisteredCells))
			metadata := GetNovaMetadata(novaNames.MetadataName)
			Expect(metadata.Spec.RegisteredCells).To(Equal(nova.Status.RegisteredCells))
		})
		It("creates cell2 NovaCell", func() {
			mariadb.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(novaNames.APIMariaDBDatabaseName)
			mariadb.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell0.MariaDBDatabaseName)
			infra.SimulateTransportURLReady(cell0.TransportURLName)
			th.SimulateJobSuccess(cell0.DBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
			th.SimulateJobSuccess(cell0.CellMappingJobName)
			th.SimulateStatefulSetReplicaReady(novaNames.APIDeploymentName)
			keystone.SimulateKeystoneEndpointReady(novaNames.APIKeystoneEndpointName)
			th.SimulateStatefulSetReplicaReady(novaNames.SchedulerStatefulSetName)
			th.SimulateStatefulSetReplicaReady(novaNames.MetadataStatefulSetName)
			mariadb.SimulateMariaDBDatabaseCompleted(cell1.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell1.MariaDBDatabaseName)
			infra.SimulateTransportURLReady(cell1.TransportURLName)
			th.SimulateStatefulSetReplicaReady(cell1.NovaComputeStatefulSetName)
			th.SimulateStatefulSetReplicaReady(cell1.NoVNCProxyStatefulSetName)
			th.SimulateJobSuccess(cell1.DBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell1.ConductorStatefulSetName)
			th.SimulateJobSuccess(cell1.CellMappingJobName)
			th.SimulateJobSuccess(cell1.HostDiscoveryJobName)

			mariadb.SimulateMariaDBDatabaseCompleted(cell2.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell2.MariaDBDatabaseName)
			infra.SimulateTransportURLReady(cell2.TransportURLName)

			th.ExpectCondition(
				cell2.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionFalse,
			)
			// assert that cell2 using its own DB but has *no* access to the API DB
			dbSync := th.GetJob(cell2.DBSyncJobName)
			Expect(dbSync.Spec.Template.Spec.InitContainers).To(HaveLen(0))
			configDataMap := th.GetSecret(cell2.ConductorConfigDataName)
			Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
			configData := string(configDataMap.Data["01-nova.conf"])
			Expect(configData).To(
				ContainSubstring(
					fmt.Sprintf(
						"[database]\nconnection = mysql+pymysql://nova_cell2:cell2-database-password@hostname-for-%s.%s.svc/nova_cell2",
						cell2.MariaDBDatabaseName.Name, novaNames.Namespace)),
			)
			Expect(configData).ToNot(
				ContainSubstring("[api_database]"),
			)
			Expect(configData).To(
				ContainSubstring("transport_url=rabbit://cell2/fake"),
			)
			th.SimulateStatefulSetReplicaReady(cell2.NoVNCProxyStatefulSetName)
			th.SimulateJobSuccess(cell2.DBSyncJobName)
			th.ExpectCondition(
				cell2.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionTrue,
			)
			th.SimulateStatefulSetReplicaReady(cell2.ConductorStatefulSetName)
			th.SimulateJobSuccess(cell2.CellMappingJobName)

			// Even though cell2 has no API access the cell2 mapping Job has
			// API access so that it can register cell2 to the API DB.
			mappingJobConfig := th.GetSecret(
				types.NamespacedName{
					Namespace: cell2.ConductorName.Namespace,
					Name:      fmt.Sprintf("%s-config-data", cell2.CellCRName.Name+"-manage"),
				},
			)
			Expect(mappingJobConfig.Data).Should(HaveKey("01-nova.conf"))
			configData = string(mappingJobConfig.Data["01-nova.conf"])
			Expect(configData).To(
				ContainSubstring(
					fmt.Sprintf(
						"[database]\nconnection = mysql+pymysql://nova_cell2:cell2-database-password@hostname-for-%s.%s.svc/nova_cell2",
						cell2.MariaDBDatabaseName.Name, novaNames.Namespace)),
			)
			Expect(configData).To(
				ContainSubstring(
					fmt.Sprintf(
						"[api_database]\nconnection = mysql+pymysql://nova_api:api-database-password@hostname-for-%s.%s.svc/nova_api",
						novaNames.APIMariaDBDatabaseName.Name, novaNames.Namespace)),
			)

			th.ExpectCondition(
				cell2.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaConductorReadyCondition,
				corev1.ConditionTrue,
			)
			// As cell2 was the last cell to deploy all cells is ready now and
			// Nova becomes ready
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
			Eventually(func(g Gomega) {
				nova := GetNova(novaNames.NovaName)
				g.Expect(nova.Status.RegisteredCells).To(
					HaveKeyWithValue(cell0.CellCRName.Name, Not(BeEmpty())))
				g.Expect(nova.Status.RegisteredCells).To(
					HaveKeyWithValue(cell1.CellCRName.Name, Not(BeEmpty())))
				g.Expect(nova.Status.RegisteredCells).To(
					HaveKeyWithValue(cell2.CellCRName.Name, Not(BeEmpty())))
			}, timeout, interval).Should(Succeed())

			// RegisteredCells are distributed
			nova := GetNova(novaNames.NovaName)
			api := GetNovaAPI(novaNames.APIName)
			Expect(api.Spec.RegisteredCells).To(Equal(nova.Status.RegisteredCells))
			scheduler := GetNovaScheduler(novaNames.SchedulerName)
			Expect(scheduler.Spec.RegisteredCells).To(Equal(nova.Status.RegisteredCells))
			metadata := GetNovaMetadata(novaNames.MetadataName)
			Expect(metadata.Spec.RegisteredCells).To(Equal(nova.Status.RegisteredCells))
		})
		It("creates cell2 NovaCell even if everything else fails", func() {
			// Don't simulate any success for any other DBs MQs or Cells
			// just for cell2
			mariadb.SimulateMariaDBDatabaseCompleted(cell2.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell2.MariaDBDatabaseName)
			infra.SimulateTransportURLReady(cell2.TransportURLName)

			// assert that cell related CRs are created
			GetNovaCell(cell2.CellCRName)
			GetNovaConductor(cell2.ConductorName)

			th.SimulateStatefulSetReplicaReady(cell2.NoVNCProxyStatefulSetName)
			th.SimulateJobSuccess(cell2.DBSyncJobName)
			th.ExpectCondition(
				cell2.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionTrue,
			)
			th.SimulateStatefulSetReplicaReady(cell2.ConductorStatefulSetName)
			th.ExpectCondition(
				cell2.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaConductorReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				cell2.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
			// Only cell2 succeeded so Nova is not ready yet
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsReadyCondition,
				corev1.ConditionFalse,
			)
		})
		It("creates Nova API even if cell1 and cell2 fails", func() {
			mariadb.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(novaNames.APIMariaDBDatabaseName)
			mariadb.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell0.MariaDBDatabaseName)
			infra.SimulateTransportURLReady(cell0.TransportURLName)
			th.SimulateJobSuccess(cell0.DBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)

			// Simulate that cell1 DB sync failed and do not simulate
			// cell2 DB creation success so that will be in Creating state.
			mariadb.SimulateMariaDBDatabaseCompleted(cell1.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell1.MariaDBDatabaseName)
			infra.SimulateTransportURLReady(cell1.TransportURLName)
			th.SimulateJobFailure(cell1.DBSyncJobName)

			// NovaAPI is still created
			GetNovaAPI(novaNames.APIName)
			th.SimulateStatefulSetReplicaReady(novaNames.APIDeploymentName)
			keystone.SimulateKeystoneEndpointReady(novaNames.APIKeystoneEndpointName)
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAPIReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsReadyCondition,
				corev1.ConditionFalse,
			)
		})
		It("does not create cell1 if cell0 fails as cell1 needs API access", func() {
			mariadb.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(novaNames.APIMariaDBDatabaseName)
			mariadb.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell0.MariaDBDatabaseName)
			mariadb.SimulateMariaDBDatabaseCompleted(cell1.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell1.MariaDBDatabaseName)
			infra.SimulateTransportURLReady(cell0.TransportURLName)
			infra.SimulateTransportURLReady(cell1.TransportURLName)

			th.SimulateJobFailure(cell0.DBSyncJobName)

			NovaCellNotExists(cell1.CellCRName)
		})
	})

	// NOTE(bogdando): a "collapsed" cell is:
	// * only one real cell, cell1 is created
	// * only one MariaDB DB service used and it host nova_api, nova_cell0, and nova_cell1 schemas
	// * only one RabbitMQ service is used and everything connects to that single MQ.
	When("Nova CR instance is created with collapsed cell deployment", func() {
		BeforeEach(func() {
			DeferCleanup(k8sClient.Delete, ctx, CreateNovaSecret(novaNames.NovaName.Namespace, SecretName))
			DeferCleanup(k8sClient.Delete, ctx, CreateNovaMessageBusSecret(cell0))

			serviceSpec := corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 3306}}}
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(novaNames.APIMariaDBDatabaseName.Namespace, novaNames.APIMariaDBDatabaseName.Name, serviceSpec))

			spec := GetDefaultNovaSpec()
			cell0Template := GetDefaultNovaCellTemplate()
			cell0Template["cellDatabaseInstance"] = novaNames.APIMariaDBDatabaseName.Name
			cell0Template["cellDatabaseUser"] = "nova_cell0"
			cell0Template["hasAPIAccess"] = true
			// disable cell0 conductor
			cell0Template["conductorServiceTemplate"] = map[string]interface{}{
				"replicas": 0,
			}

			cell1Template := GetDefaultNovaCellTemplate()
			// cell1 is configured to have API access and use the same
			// message bus as the top level services. Hence cell1 conductor
			// will act both as a super conductor and as cell1 conductor
			cell1Template["cellDatabaseInstance"] = novaNames.APIMariaDBDatabaseName.Name
			cell1Template["cellDatabaseUser"] = "nova_cell1"
			cell1Template["cellMessageBusInstance"] = cell0.TransportURLName.Name
			cell1Template["hasAPIAccess"] = true

			spec["cellTemplates"] = map[string]interface{}{
				"cell0": cell0Template,
				"cell1": cell1Template,
			}
			spec["apiDatabaseInstance"] = novaNames.APIMariaDBDatabaseName.Name
			spec["apiMessageBusInstance"] = cell0.TransportURLName.Name

			DeferCleanup(th.DeleteInstance, CreateNova(novaNames.NovaName, spec))
			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(novaNames.NovaName.Namespace))
			keystone.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
		})
		It("cell0 becomes ready with 0 conductor replicas and the rest of nova is deployed", func() {
			mariadb.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(novaNames.APIMariaDBDatabaseName)
			mariadb.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell0.MariaDBDatabaseName)
			mariadb.SimulateMariaDBDatabaseCompleted(cell1.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell1.MariaDBDatabaseName)
			infra.SimulateTransportURLReady(cell0.TransportURLName)
			infra.SimulateTransportURLReady(cell1.TransportURLName)

			// We requested 0 replicas from the cell0 conductor so the
			// conductor is ready even if 0 replicas is running but all
			// the necessary steps, i.e. db-sync is run successfully
			th.SimulateJobSuccess(cell0.DBSyncJobName)
			th.ExpectCondition(
				cell0.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)

			th.ExpectCondition(
				cell0.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaConductorReadyCondition,
				corev1.ConditionTrue,
			)
			ss := th.GetStatefulSet(cell0.ConductorStatefulSetName)
			Expect(ss.Status.Replicas).To(Equal(int32(0)))
			Expect(ss.Status.AvailableReplicas).To(Equal(int32(0)))

			th.SimulateJobSuccess(cell0.CellMappingJobName)

			// As cell0 is ready cell1 is deployed
			th.SimulateStatefulSetReplicaReady(cell1.NoVNCProxyStatefulSetName)
			th.SimulateJobSuccess(cell1.DBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell1.ConductorStatefulSetName)
			th.SimulateJobSuccess(cell1.CellMappingJobName)

			th.ExpectCondition(
				cell1.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaConductorReadyCondition,
				corev1.ConditionTrue,
			)

			th.SimulateStatefulSetReplicaReady(novaNames.MetadataStatefulSetName)
			// As cell0 is ready API is deployed
			th.SimulateStatefulSetReplicaReady(novaNames.APIDeploymentName)
			keystone.SimulateKeystoneEndpointReady(novaNames.APIKeystoneEndpointName)
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAPIReadyCondition,
				corev1.ConditionTrue,
			)

			// As cell0 is ready scheduler is deployed
			th.SimulateStatefulSetReplicaReady(novaNames.SchedulerStatefulSetName)
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaSchedulerReadyCondition,
				corev1.ConditionTrue,
			)

			// So the whole Nova deployment is ready
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})
	When("cell1 DB and MQ create finishes before cell0 DB create", func() {
		BeforeEach(func() {
			DeferCleanup(k8sClient.Delete, ctx, CreateNovaSecret(novaNames.Namespace, SecretName))
			DeferCleanup(k8sClient.Delete, ctx, CreateNovaMessageBusSecret(cell0))
			serviceSpec := corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 3306}}}
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					novaNames.APIMariaDBDatabaseName.Namespace,
					novaNames.APIMariaDBDatabaseName.Name,
					serviceSpec))
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					cell1.MariaDBDatabaseName.Namespace,
					cell1.MariaDBDatabaseName.Name,
					serviceSpec))

			spec := GetDefaultNovaSpec()
			cell0Template := GetDefaultNovaCellTemplate()
			cell0Template["cellDatabaseInstance"] = cell0.MariaDBDatabaseName.Name
			cell0Template["cellDatabaseUser"] = "nova_cell0"

			cell1Template := GetDefaultNovaCellTemplate()
			cell1Template["cellDatabaseInstance"] = cell1.MariaDBDatabaseName.Name
			cell1Template["cellDatabaseUser"] = "nova_cell1"
			cell1Template["cellMessageBusInstance"] = cell1.TransportURLName.Name

			spec["cellTemplates"] = map[string]interface{}{
				"cell0": cell0Template,
				"cell1": cell1Template,
			}
			spec["apiDatabaseInstance"] = novaNames.APIMariaDBDatabaseName.Name
			spec["apiMessageBusInstance"] = cell0.TransportURLName.Name

			DeferCleanup(th.DeleteInstance, CreateNova(novaNames.NovaName, spec))
			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(novaNames.Namespace))
			keystone.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
		})

		It("waits for cell0 DB to be created", func(ctx SpecContext) {
			mariadb.SimulateMariaDBDatabaseCompleted(cell1.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell1.MariaDBDatabaseName)
			infra.SimulateTransportURLReady(cell1.TransportURLName)
			// NOTE(gibi): before the fix https://github.com/openstack-k8s-operators/nova-operator/pull/356
			// nova-controller panic at this point and test would hang
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsReadyCondition,
				corev1.ConditionFalse,
			)
		})
	})
	When("Nova CR instance is created with metadata per cell", func() {
		BeforeEach(func() {
			DeferCleanup(k8sClient.Delete, ctx, CreateNovaSecret(novaNames.NovaName.Namespace, SecretName))
			DeferCleanup(k8sClient.Delete, ctx, CreateNovaMessageBusSecret(cell0))
			DeferCleanup(k8sClient.Delete, ctx, CreateNovaMessageBusSecret(cell1))

			serviceSpec := corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 3306}}}
			DeferCleanup(mariadb.DeleteDBService, mariadb.CreateDBService(novaNames.APIMariaDBDatabaseName.Namespace, novaNames.APIMariaDBDatabaseName.Name, serviceSpec))
			DeferCleanup(mariadb.DeleteDBService, mariadb.CreateDBService(cell0.MariaDBDatabaseName.Namespace, cell0.MariaDBDatabaseName.Name, serviceSpec))
			DeferCleanup(mariadb.DeleteDBService, mariadb.CreateDBService(cell1.MariaDBDatabaseName.Namespace, cell1.MariaDBDatabaseName.Name, serviceSpec))

			spec := GetDefaultNovaSpec()
			cell0Template := GetDefaultNovaCellTemplate()
			cell0Template["cellDatabaseInstance"] = cell0.MariaDBDatabaseName.Name
			cell0Template["cellDatabaseUser"] = "nova_cell0"

			cell1Template := GetDefaultNovaCellTemplate()
			cell1Template["cellDatabaseInstance"] = cell1.MariaDBDatabaseName.Name
			cell1Template["cellDatabaseUser"] = "nova_cell1"
			cell1Template["cellMessageBusInstance"] = cell1.TransportURLName.Name
			cell1Template["metadataServiceTemplate"] = map[string]interface{}{
				"enabled": true,
			}

			spec["cellTemplates"] = map[string]interface{}{
				"cell0": cell0Template,
				"cell1": cell1Template,
			}
			spec["metadataServiceTemplate"] = map[string]interface{}{
				"enabled": false,
			}
			spec["apiDatabaseInstance"] = novaNames.APIMariaDBDatabaseName.Name
			spec["apiMessageBusInstance"] = cell0.TransportURLName.Name

			DeferCleanup(th.DeleteInstance, CreateNova(novaNames.NovaName, spec))
			keystoneAPIName := keystone.CreateKeystoneAPI(novaNames.NovaName.Namespace)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystoneAPIName)
			keystoneAPI := keystone.GetKeystoneAPI(keystoneAPIName)
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Status().Update(ctx, keystoneAPI.DeepCopy())).Should(Succeed())
			}, timeout, interval).Should(Succeed())
			keystone.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
		})
		It("cell0 becomes ready without metadata and the rest of nova is deployed", func() {
			mariadb.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(novaNames.APIMariaDBDatabaseName)
			mariadb.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell0.MariaDBDatabaseName)
			mariadb.SimulateMariaDBDatabaseCompleted(cell1.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell1.MariaDBDatabaseName)
			infra.SimulateTransportURLReady(cell0.TransportURLName)
			infra.SimulateTransportURLReady(cell1.TransportURLName)
			th.SimulateJobSuccess(cell0.DBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
			th.SimulateJobSuccess(cell0.CellMappingJobName)

			cell0cond := NovaCellConditionGetter(cell0.CellCRName)
			Expect(cell0cond.Get(novav1.NovaMetadataReadyCondition)).Should(BeNil())

			// As cell0 is ready cell1 is deployed
			th.SimulateStatefulSetReplicaReady(cell1.NoVNCProxyStatefulSetName)
			th.SimulateJobSuccess(cell1.DBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell1.ConductorStatefulSetName)
			th.SimulateStatefulSetReplicaReady(cell1.MetadataStatefulSetName)
			th.SimulateJobSuccess(cell1.CellMappingJobName)

			th.ExpectCondition(
				cell1.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaMetadataReadyCondition,
				corev1.ConditionTrue,
			)
			AssertMetadataDoesNotExist(cell0.MetadataName)
		})
		It("puts the metadata secret to cell1 secret but not to cell0 secret", func() {
			mariadb.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(novaNames.APIMariaDBDatabaseName)
			mariadb.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell0.MariaDBDatabaseName)
			mariadb.SimulateMariaDBDatabaseCompleted(cell1.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell1.MariaDBDatabaseName)
			infra.SimulateTransportURLReady(cell0.TransportURLName)
			infra.SimulateTransportURLReady(cell1.TransportURLName)
			th.SimulateJobSuccess(cell0.DBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
			th.SimulateJobSuccess(cell0.CellMappingJobName)

			cell1Secret := th.GetSecret(cell1.InternalCellSecretName)
			Expect(cell1Secret.Data).To(
				HaveKeyWithValue(controllers.MetadataSecretSelector, []byte("metadata-secret")))
			cell0Secret := th.GetSecret(cell0.InternalCellSecretName)
			Expect(cell0Secret.Data).NotTo(
				HaveKeyWithValue(controllers.MetadataSecretSelector, []byte("metadata-secret")))
		})
	})
})
