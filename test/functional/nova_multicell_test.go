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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Nova multicell", func() {
	When("Nova CR instance is created with 3 cells", func() {
		BeforeEach(func() {
			// TODO(bogdando): deduplicate this into CreateNovaWith3CellsAndEnsureReady()
			DeferCleanup(k8sClient.Delete, ctx, CreateNovaSecretFor3Cells(novaNames.NovaName.Namespace, SecretName))
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateNovaMessageBusSecret(cell0.CellName.Namespace, fmt.Sprintf("%s-secret", cell0.TransportURLName.Name)),
			)
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateNovaMessageBusSecret(cell1.CellName.Namespace, fmt.Sprintf("%s-secret", cell1.TransportURLName.Name)),
			)
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateNovaMessageBusSecret(cell2.CellName.Namespace, fmt.Sprintf("%s-secret", cell2.TransportURLName.Name)),
			)

			serviceSpec := corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 3306}}}
			DeferCleanup(th.DeleteDBService, th.CreateDBService(novaNames.APIMariaDBDatabaseName.Namespace, novaNames.APIMariaDBDatabaseName.Name, serviceSpec))
			DeferCleanup(th.DeleteDBService, th.CreateDBService(cell0.MariaDBDatabaseName.Namespace, cell0.MariaDBDatabaseName.Name, serviceSpec))
			DeferCleanup(th.DeleteDBService, th.CreateDBService(cell1.MariaDBDatabaseName.Namespace, cell1.MariaDBDatabaseName.Name, serviceSpec))
			DeferCleanup(th.DeleteDBService, th.CreateDBService(cell2.MariaDBDatabaseName.Namespace, cell2.MariaDBDatabaseName.Name, serviceSpec))

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
			DeferCleanup(th.DeleteKeystoneAPI, th.CreateKeystoneAPI(novaNames.NovaName.Namespace))
			th.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
		})

		It("creates cell0 NovaCell", func() {
			th.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			th.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			th.SimulateTransportURLReady(cell0.TransportURLName)

			// assert that cell related CRs are created pointing to the API MQ
			cell := GetNovaCell(cell0.CellName)
			Expect(cell.Spec.CellMessageBusSecretName).To(Equal(fmt.Sprintf("%s-secret", cell0.TransportURLName.Name)))
			conductor := GetNovaConductor(cell0.CellConductorName)
			Expect(conductor.Spec.CellMessageBusSecretName).To(Equal(fmt.Sprintf("%s-secret", cell0.TransportURLName.Name)))

			th.ExpectCondition(
				cell0.CellConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionFalse,
			)
			// assert that cell0 conductor is using the same DB as the API
			dbSync := th.GetJob(cell0.CellDBSyncJobName)
			Expect(dbSync.Spec.Template.Spec.InitContainers).To(HaveLen(0))

			configDataMap := th.GetSecret(cell0.CellConductorConfigDataName)
			Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
			configData := string(configDataMap.Data["01-nova.conf"])
			Expect(configData).To(
				ContainSubstring(
					fmt.Sprintf(
						"[database]\nconnection = mysql+pymysql://nova_cell0:cell0-database-password@hostname-for-%s/nova_cell0",
						cell0.MariaDBDatabaseName.Name)),
			)
			Expect(configData).To(
				ContainSubstring(
					fmt.Sprintf(
						"[api_database]\nconnection = mysql+pymysql://nova_api:api-database-password@hostname-for-%s/nova_api",
						novaNames.APIMariaDBDatabaseName.Name)),
			)

			th.SimulateJobSuccess(cell0.CellDBSyncJobName)
			th.ExpectCondition(
				cell0.CellConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionTrue,
			)
			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
			th.SimulateJobSuccess(cell0.CellMappingJobName)

			th.ExpectCondition(
				cell0.CellName,
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
					HaveKeyWithValue(cell0.CellName.Name, Not(BeEmpty())))
			}, timeout, interval).Should(Succeed())
		})

		It("creates NovaAPI", func() {
			th.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			th.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			th.SimulateTransportURLReady(cell0.TransportURLName)
			th.SimulateJobSuccess(cell0.CellDBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
			th.SimulateJobSuccess(cell0.CellMappingJobName)

			api := GetNovaAPI(novaNames.APIName)
			one := int32(1)
			Expect(api.Spec.Replicas).Should(BeEquivalentTo(&one))
			Expect(api.Spec.Cell0DatabaseHostname).To(Equal(fmt.Sprintf("hostname-for-%s", cell0.MariaDBDatabaseName.Name)))
			Expect(api.Spec.APIDatabaseHostname).To(Equal(fmt.Sprintf("hostname-for-%s", novaNames.APIMariaDBDatabaseName.Name)))
			Expect(api.Spec.APIMessageBusSecretName).To(Equal(fmt.Sprintf("%s-secret", cell0.TransportURLName.Name)))

			configDataMap := th.GetSecret(novaNames.APIConfigDataName)
			Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
			configData := string(configDataMap.Data["01-nova.conf"])
			Expect(configData).To(
				ContainSubstring(
					fmt.Sprintf(
						"[database]\nconnection = mysql+pymysql://nova_cell0:cell0-database-password@hostname-for-%s/nova_cell0",
						cell0.MariaDBDatabaseName.Name)),
			)
			Expect(configData).To(
				ContainSubstring(
					fmt.Sprintf(
						"[api_database]\nconnection = mysql+pymysql://nova_api:api-database-password@hostname-for-%s/nova_api",
						novaNames.APIMariaDBDatabaseName.Name)),
			)

			th.SimulateStatefulSetReplicaReady(novaNames.APIDeploymentName)

			th.ExpectCondition(
				novaNames.APIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionTrue,
			)

			th.SimulateKeystoneEndpointReady(novaNames.APIKeystoneEndpointName)
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAPIReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("creates all cell DBs", func() {
			th.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			th.SimulateTransportURLReady(cell0.TransportURLName)
			th.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			th.SimulateJobSuccess(cell0.CellDBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
			th.SimulateJobSuccess(cell0.CellMappingJobName)
			th.SimulateStatefulSetReplicaReady(novaNames.APIDeploymentName)
			th.SimulateKeystoneEndpointReady(novaNames.APIKeystoneEndpointName)

			th.SimulateMariaDBDatabaseCompleted(cell1.MariaDBDatabaseName)
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsDBReadyCondition,
				corev1.ConditionFalse,
			)
			th.SimulateMariaDBDatabaseCompleted(cell2.MariaDBDatabaseName)
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsDBReadyCondition,
				corev1.ConditionTrue,
			)

		})

		It("creates all cell MQs", func() {
			th.SimulateTransportURLReady(cell0.TransportURLName)
			th.SimulateTransportURLReady(cell1.TransportURLName)
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsMQReadyCondition,
				corev1.ConditionFalse,
			)
			th.SimulateTransportURLReady(cell2.TransportURLName)
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsMQReadyCondition,
				corev1.ConditionTrue,
			)

		})

		It("creates cell1 NovaCell", func() {
			th.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			th.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			th.SimulateTransportURLReady(cell0.TransportURLName)
			th.SimulateJobSuccess(cell0.CellDBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
			th.SimulateJobSuccess(cell0.CellMappingJobName)
			th.SimulateStatefulSetReplicaReady(novaNames.APIDeploymentName)
			th.SimulateKeystoneEndpointReady(novaNames.APIKeystoneEndpointName)
			th.SimulateMariaDBDatabaseCompleted(cell1.MariaDBDatabaseName)
			th.SimulateTransportURLReady(cell1.TransportURLName)

			// assert that cell related CRs are created pointing to the cell1 MQ
			c1 := GetNovaCell(cell1.CellName)
			Expect(c1.Spec.CellMessageBusSecretName).To(Equal(fmt.Sprintf("%s-secret", cell1.TransportURLName.Name)))
			c1Conductor := GetNovaConductor(cell1.CellConductorName)
			Expect(c1Conductor.Spec.CellMessageBusSecretName).To(Equal(fmt.Sprintf("%s-secret", cell1.TransportURLName.Name)))

			th.ExpectCondition(
				cell1.CellConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionFalse,
			)
			// assert that cell1 using its own DB but has access to the API DB
			dbSync := th.GetJob(cell1.CellDBSyncJobName)
			Expect(dbSync.Spec.Template.Spec.InitContainers).To(HaveLen(0))
			configDataMap := th.GetSecret(cell1.CellConductorConfigDataName)
			Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
			configData := string(configDataMap.Data["01-nova.conf"])
			Expect(configData).To(
				ContainSubstring(
					fmt.Sprintf(
						"[database]\nconnection = mysql+pymysql://nova_cell1:cell1-database-password@hostname-for-%s/nova_cell1",
						cell1.MariaDBDatabaseName.Name)),
			)
			Expect(configData).To(
				ContainSubstring(
					fmt.Sprintf(
						"[api_database]\nconnection = mysql+pymysql://nova_api:api-database-password@hostname-for-%s/nova_api",
						novaNames.APIMariaDBDatabaseName.Name)),
			)
			th.SimulateStatefulSetReplicaReady(cell1.NoVNCProxyStatefulSetName)
			th.SimulateJobSuccess(cell1.CellDBSyncJobName)
			th.ExpectCondition(
				cell1.CellConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionTrue,
			)
			th.SimulateStatefulSetReplicaReady(cell1.ConductorStatefulSetName)
			th.SimulateJobSuccess(cell1.CellMappingJobName)
			th.ExpectCondition(
				cell1.CellName,
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
					HaveKeyWithValue(cell0.CellName.Name, Not(BeEmpty())))
				g.Expect(nova.Status.RegisteredCells).To(
					HaveKeyWithValue(cell1.CellName.Name, Not(BeEmpty())))
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
			th.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			th.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			th.SimulateTransportURLReady(cell0.TransportURLName)
			th.SimulateJobSuccess(cell0.CellDBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
			th.SimulateJobSuccess(cell0.CellMappingJobName)
			th.SimulateStatefulSetReplicaReady(novaNames.APIDeploymentName)
			th.SimulateKeystoneEndpointReady(novaNames.APIKeystoneEndpointName)
			th.SimulateStatefulSetReplicaReady(novaNames.SchedulerStatefulSetName)
			th.SimulateStatefulSetReplicaReady(novaNames.MetadataStatefulSetName)
			th.SimulateMariaDBDatabaseCompleted(cell1.MariaDBDatabaseName)
			th.SimulateTransportURLReady(cell1.TransportURLName)
			th.SimulateStatefulSetReplicaReady(cell1.NoVNCProxyStatefulSetName)
			th.SimulateJobSuccess(cell1.CellDBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell1.ConductorStatefulSetName)
			th.SimulateJobSuccess(cell1.CellMappingJobName)

			th.SimulateMariaDBDatabaseCompleted(cell2.MariaDBDatabaseName)
			th.SimulateTransportURLReady(cell2.TransportURLName)

			// assert that cell related CRs are created pointing to the Cell 2 MQ
			c2 := GetNovaCell(cell2.CellName)
			Expect(c2.Spec.CellMessageBusSecretName).To(Equal(fmt.Sprintf("%s-secret", cell2.TransportURLName.Name)))
			c2Conductor := GetNovaConductor(cell2.CellConductorName)
			Expect(c2Conductor.Spec.CellMessageBusSecretName).To(Equal(fmt.Sprintf("%s-secret", cell2.TransportURLName.Name)))
			th.ExpectCondition(
				cell2.CellConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionFalse,
			)
			// assert that cell2 using its own DB but has *no* access to the API DB
			dbSync := th.GetJob(cell2.CellDBSyncJobName)
			Expect(dbSync.Spec.Template.Spec.InitContainers).To(HaveLen(0))
			configDataMap := th.GetSecret(cell2.CellConductorConfigDataName)
			Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
			configData := string(configDataMap.Data["01-nova.conf"])
			Expect(configData).To(
				ContainSubstring(
					fmt.Sprintf(
						"[database]\nconnection = mysql+pymysql://nova_cell2:cell2-database-password@hostname-for-%s/nova_cell2",
						cell2.MariaDBDatabaseName.Name)),
			)
			Expect(configData).ToNot(
				ContainSubstring("[api_database]"),
			)
			th.SimulateStatefulSetReplicaReady(cell2.NoVNCProxyStatefulSetName)
			th.SimulateJobSuccess(cell2.CellDBSyncJobName)
			th.ExpectCondition(
				cell2.CellConductorName,
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
					Namespace: cell2.CellConductorName.Namespace,
					Name:      fmt.Sprintf("%s-config-data", cell2.CellName.Name+"-manage"),
				},
			)
			Expect(mappingJobConfig.Data).Should(HaveKey("01-nova.conf"))
			configData = string(mappingJobConfig.Data["01-nova.conf"])
			Expect(configData).To(
				ContainSubstring(
					fmt.Sprintf(
						"[database]\nconnection = mysql+pymysql://nova_cell2:cell2-database-password@hostname-for-%s/nova_cell2",
						cell2.MariaDBDatabaseName.Name)),
			)
			Expect(configData).To(
				ContainSubstring(
					fmt.Sprintf(
						"[api_database]\nconnection = mysql+pymysql://nova_api:api-database-password@hostname-for-%s/nova_api",
						novaNames.APIMariaDBDatabaseName.Name)),
			)

			th.ExpectCondition(
				cell2.CellName,
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
					HaveKeyWithValue(cell0.CellName.Name, Not(BeEmpty())))
				g.Expect(nova.Status.RegisteredCells).To(
					HaveKeyWithValue(cell1.CellName.Name, Not(BeEmpty())))
				g.Expect(nova.Status.RegisteredCells).To(
					HaveKeyWithValue(cell2.CellName.Name, Not(BeEmpty())))
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
			th.SimulateMariaDBDatabaseCompleted(cell2.MariaDBDatabaseName)
			th.SimulateTransportURLReady(cell2.TransportURLName)

			// assert that cell related CRs are created
			GetNovaCell(cell2.CellName)
			GetNovaConductor(cell2.CellConductorName)

			th.SimulateStatefulSetReplicaReady(cell2.NoVNCProxyStatefulSetName)
			th.SimulateJobSuccess(cell2.CellDBSyncJobName)
			th.ExpectCondition(
				cell2.CellConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionTrue,
			)
			th.SimulateStatefulSetReplicaReady(cell2.ConductorStatefulSetName)
			th.ExpectCondition(
				cell2.CellName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaConductorReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				cell2.CellName,
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
			th.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			th.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			th.SimulateTransportURLReady(cell0.TransportURLName)
			th.SimulateJobSuccess(cell0.CellDBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)

			// Simulate that cell1 DB sync failed and do not simulate
			// cell2 DB creation success so that will be in Creating state.
			th.SimulateMariaDBDatabaseCompleted(cell1.MariaDBDatabaseName)
			th.SimulateTransportURLReady(cell1.TransportURLName)
			th.SimulateJobFailure(cell1.CellDBSyncJobName)

			// NovaAPI is still created
			GetNovaAPI(novaNames.APIName)
			th.SimulateStatefulSetReplicaReady(novaNames.APIDeploymentName)
			th.SimulateKeystoneEndpointReady(novaNames.APIKeystoneEndpointName)
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
			th.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			th.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			th.SimulateMariaDBDatabaseCompleted(cell1.MariaDBDatabaseName)
			th.SimulateTransportURLReady(cell0.TransportURLName)
			th.SimulateTransportURLReady(cell1.TransportURLName)

			th.SimulateJobFailure(cell0.CellDBSyncJobName)

			NovaCellNotExists(cell1.CellName)
		})
	})

	// NOTE(bogdando): a "collapsed" cell is:
	// * only one real cell, cell1 is created
	// * only one MariaDB DB service used and it host nova_api, nova_cell0, and nova_cell1 schemas
	// * only one RabbitMQ service is used and everything connects to that single MQ.
	When("Nova CR instance is created with collapsed cell deployment", func() {
		BeforeEach(func() {
			DeferCleanup(k8sClient.Delete, ctx, CreateNovaSecret(novaNames.NovaName.Namespace, SecretName))
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateNovaMessageBusSecret(cell0.CellName.Namespace, fmt.Sprintf("%s-secret", cell0.TransportURLName.Name)),
			)

			serviceSpec := corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 3306}}}
			DeferCleanup(
				th.DeleteDBService,
				th.CreateDBService(novaNames.APIMariaDBDatabaseName.Namespace, novaNames.APIMariaDBDatabaseName.Name, serviceSpec))

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
			DeferCleanup(th.DeleteKeystoneAPI, th.CreateKeystoneAPI(novaNames.NovaName.Namespace))
			th.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
		})
		It("cell0 becomes ready with 0 conductor replicas and the rest of nova is deployed", func() {
			th.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			th.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			th.SimulateMariaDBDatabaseCompleted(cell1.MariaDBDatabaseName)
			th.SimulateTransportURLReady(cell0.TransportURLName)
			th.SimulateTransportURLReady(cell1.TransportURLName)

			// We requested 0 replicas from the cell0 conductor so the
			// conductor is ready even if 0 replicas is running but all
			// the necessary steps, i.e. db-sync is run successfully
			th.SimulateJobSuccess(cell0.CellDBSyncJobName)
			th.ExpectCondition(
				cell0.CellConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)

			th.ExpectCondition(
				cell0.CellName,
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
			th.SimulateJobSuccess(cell1.CellDBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell1.ConductorStatefulSetName)
			th.SimulateJobSuccess(cell1.CellMappingJobName)

			th.ExpectCondition(
				cell1.CellName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaConductorReadyCondition,
				corev1.ConditionTrue,
			)

			th.SimulateStatefulSetReplicaReady(novaNames.MetadataStatefulSetName)
			// As cell0 is ready API is deployed
			th.SimulateStatefulSetReplicaReady(novaNames.APIDeploymentName)
			th.SimulateKeystoneEndpointReady(novaNames.APIKeystoneEndpointName)
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
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateNovaMessageBusSecret(cell0.CellName.Namespace, fmt.Sprintf("%s-secret", cell0.TransportURLName.Name)),
			)
			serviceSpec := corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 3306}}}
			DeferCleanup(
				th.DeleteDBService,
				th.CreateDBService(
					novaNames.APIMariaDBDatabaseName.Namespace,
					novaNames.APIMariaDBDatabaseName.Name,
					serviceSpec))
			DeferCleanup(
				th.DeleteDBService,
				th.CreateDBService(
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
			DeferCleanup(th.DeleteKeystoneAPI, th.CreateKeystoneAPI(novaNames.Namespace))
			th.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
		})

		It("waits for cell0 DB to be created", func(ctx SpecContext) {
			th.SimulateMariaDBDatabaseCompleted(cell1.MariaDBDatabaseName)
			th.SimulateTransportURLReady(cell1.TransportURLName)
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
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateNovaMessageBusSecret(cell0.CellName.Namespace, fmt.Sprintf("%s-secret", cell0.TransportURLName.Name)),
			)
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateNovaMessageBusSecret(cell1.CellName.Namespace, fmt.Sprintf("%s-secret", cell1.TransportURLName.Name)),
			)

			serviceSpec := corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 3306}}}
			DeferCleanup(th.DeleteDBService, th.CreateDBService(novaNames.APIMariaDBDatabaseName.Namespace, novaNames.APIMariaDBDatabaseName.Name, serviceSpec))
			DeferCleanup(th.DeleteDBService, th.CreateDBService(cell0.MariaDBDatabaseName.Namespace, cell0.MariaDBDatabaseName.Name, serviceSpec))
			DeferCleanup(th.DeleteDBService, th.CreateDBService(cell1.MariaDBDatabaseName.Namespace, cell1.MariaDBDatabaseName.Name, serviceSpec))

			spec := GetDefaultNovaSpec()
			cell0Template := GetDefaultNovaCellTemplate()
			cell0Template["cellDatabaseInstance"] = cell0.MariaDBDatabaseName.Name
			cell0Template["cellDatabaseUser"] = "nova_cell0"

			cell0Template["metadataServiceTemplate"] = map[string]interface{}{
				"replicas": 0,
			}

			cell1Template := GetDefaultNovaCellTemplate()
			cell1Template["cellDatabaseInstance"] = cell1.MariaDBDatabaseName.Name
			cell1Template["cellDatabaseUser"] = "nova_cell1"
			cell1Template["cellMessageBusInstance"] = cell1.TransportURLName.Name
			cell1Template["metadataServiceTemplate"] = map[string]interface{}{
				"replicas": 1,
			}

			spec["cellTemplates"] = map[string]interface{}{
				"cell0": cell0Template,
				"cell1": cell1Template,
			}
			spec["metadataServiceTemplate"] = map[string]interface{}{
				"replicas": 0,
			}
			spec["apiDatabaseInstance"] = novaNames.APIMariaDBDatabaseName.Name
			spec["apiMessageBusInstance"] = cell0.TransportURLName.Name

			DeferCleanup(th.DeleteInstance, CreateNova(novaNames.NovaName, spec))
			keystoneAPIName := th.CreateKeystoneAPI(novaNames.NovaName.Namespace)
			DeferCleanup(th.DeleteKeystoneAPI, keystoneAPIName)
			keystoneAPI := th.GetKeystoneAPI(keystoneAPIName)
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Status().Update(ctx, keystoneAPI.DeepCopy())).Should(Succeed())
			}, timeout, interval).Should(Succeed())
			th.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
		})
		It("cell0 becomes ready with 0 metadata replicas and the rest of nova is deployed", func() {
			th.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			th.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			th.SimulateMariaDBDatabaseCompleted(cell1.MariaDBDatabaseName)
			th.SimulateTransportURLReady(cell0.TransportURLName)
			th.SimulateTransportURLReady(cell1.TransportURLName)
			th.SimulateJobSuccess(cell0.CellDBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
			th.SimulateJobSuccess(cell0.CellMappingJobName)

			cell0cond := NovaCellConditionGetter(cell0.CellName)
			Expect(cell0cond.Get(novav1.NovaMetadataReadyCondition)).Should(BeNil())

			// As cell0 is ready cell1 is deployed
			th.SimulateStatefulSetReplicaReady(cell1.NoVNCProxyStatefulSetName)
			th.SimulateJobSuccess(cell1.CellDBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell1.ConductorStatefulSetName)
			th.SimulateStatefulSetReplicaReady(cell1.MetadataStatefulSetName)
			th.SimulateJobSuccess(cell1.CellMappingJobName)

			th.ExpectCondition(
				cell1.CellName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaMetadataReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})
})
