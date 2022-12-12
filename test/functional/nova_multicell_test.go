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
	. "github.com/openstack-k8s-operators/lib-common/modules/test/helpers"

	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Nova multi cell", func() {
	var novaName types.NamespacedName
	var novaNames NovaNames
	var cell0 CellNames
	var cell1 CellNames
	var cell2 CellNames

	BeforeEach(func() {
		// Uncomment this if you need the full output in the logs from gomega
		// matchers
		// format.MaxLength = 0

		novaName = types.NamespacedName{
			Namespace: namespace,
			Name:      uuid.New().String(),
		}
		novaNames = GetNovaNames(novaName, []string{"cell0", "cell1", "cell2"})
		cell0 = novaNames.Cells["cell0"]
		cell1 = novaNames.Cells["cell1"]
		cell2 = novaNames.Cells["cell2"]
	})

	When("Nova CR instance is created with 3 cells", func() {
		BeforeEach(func() {
			DeferCleanup(k8sClient.Delete, ctx, CreateNovaSecret(namespace, SecretName))
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateNovaMessageBusSecret(namespace, "mq-for-api-secret"),
			)
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateNovaMessageBusSecret(namespace, "mq-for-cell1-secret"),
			)
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateNovaMessageBusSecret(namespace, "mq-for-cell2-secret"),
			)

			serviceSpec := corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 3306}}}
			DeferCleanup(th.DeleteDBService, th.CreateDBService(namespace, "db-for-api", serviceSpec))
			DeferCleanup(th.DeleteDBService, th.CreateDBService(namespace, "db-for-cell1", serviceSpec))
			DeferCleanup(th.DeleteDBService, th.CreateDBService(namespace, "db-for-cell2", serviceSpec))

			spec := GetDefaultNovaSpec()
			cell0 := GetDefaultNovaCellTemplate()
			cell0["cellDatabaseInstance"] = "db-for-api"
			cell0["cellDatabaseUser"] = "nova_cell0"

			cell1 := GetDefaultNovaCellTemplate()
			cell1["cellDatabaseInstance"] = "db-for-cell1"
			cell1["cellDatabaseUser"] = "nova_cell1"
			cell1["cellMessageBusInstance"] = "mq-for-cell1"

			cell2 := GetDefaultNovaCellTemplate()
			cell2["cellDatabaseInstance"] = "db-for-cell2"
			cell2["cellDatabaseUser"] = "nova_cell2"
			cell2["cellMessageBusInstance"] = "mq-for-cell2"
			cell2["hasAPIAccess"] = false

			spec["cellTemplates"] = map[string]interface{}{
				"cell0": cell0,
				"cell1": cell1,
				"cell2": cell2,
			}
			spec["apiDatabaseInstance"] = "db-for-api"
			spec["apiMessageBusInstance"] = "mq-for-api"

			DeferCleanup(th.DeleteInstance, CreateNova(novaName, spec))
			keystoneAPIName := th.CreateKeystoneAPI(namespace)
			DeferCleanup(th.DeleteKeystoneAPI, keystoneAPIName)
			keystoneAPI := th.GetKeystoneAPI(keystoneAPIName)
			keystoneAPI.Status.APIEndpoints["internal"] = "http://keystone-internal-openstack.testing"
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Status().Update(ctx, keystoneAPI.DeepCopy())).Should(Succeed())
			}, timeout, interval).Should(Succeed())
			th.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
		})

		It("creates cell0 NovaCell", func() {
			th.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			th.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			th.SimulateTransportURLReady(cell0.TransportURLName)

			// assert that cell related CRs are created pointing to the API MQ
			cell := GetNovaCell(cell0.CellName)
			Expect(cell.Spec.CellMessageBusSecretName).To(Equal("mq-for-api-secret"))
			conductor := GetNovaConductor(cell0.CellConductorName)
			Expect(conductor.Spec.CellMessageBusSecretName).To(Equal("mq-for-api-secret"))

			th.ExpectCondition(
				cell0.CellConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionFalse,
			)
			// assert that cell0 conductor is using the same DB as the API
			dbSync := th.GetJob(cell0.CellDBSyncJobName)
			Expect(dbSync.Spec.Template.Spec.InitContainers).To(HaveLen(0))

			configDataMap := th.GetConfigMap(
				types.NamespacedName{
					Namespace: namespace,
					Name:      fmt.Sprintf("%s-config-data", cell0.CellConductorName.Name),
				},
			)
			Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
			Expect(configDataMap.Data["01-nova.conf"]).To(
				ContainSubstring("[database]\nconnection = mysql+pymysql://nova_cell0:12345678@hostname-for-db-for-api/nova_cell0"),
			)
			Expect(configDataMap.Data["01-nova.conf"]).To(
				ContainSubstring("[api_database]\nconnection = mysql+pymysql://nova_api:12345678@hostname-for-db-for-api/nova_api"),
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
				novaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsReadyCondition,
				corev1.ConditionFalse,
			)
			Eventually(func(g Gomega) {
				nova := GetNova(novaName)
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
			Expect(api.Spec.Cell0DatabaseHostname).To(Equal("hostname-for-db-for-api"))
			Expect(api.Spec.Cell0DatabaseHostname).To(Equal(api.Spec.APIDatabaseHostname))
			Expect(api.Spec.APIMessageBusSecretName).To(Equal("mq-for-api-secret"))

			configDataMap := th.GetConfigMap(
				types.NamespacedName{
					Namespace: namespace,
					Name:      fmt.Sprintf("%s-config-data", novaNames.APIName.Name),
				},
			)
			Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
			Expect(configDataMap.Data["01-nova.conf"]).To(
				ContainSubstring("[database]\nconnection = mysql+pymysql://nova_cell0:12345678@hostname-for-db-for-api/nova_cell0"),
			)
			Expect(configDataMap.Data["01-nova.conf"]).To(
				ContainSubstring("[api_database]\nconnection = mysql+pymysql://nova_api:12345678@hostname-for-db-for-api/nova_api"),
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
				novaName,
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
				novaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsDBReadyCondition,
				corev1.ConditionFalse,
			)
			th.SimulateMariaDBDatabaseCompleted(cell2.MariaDBDatabaseName)
			th.ExpectCondition(
				novaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsDBReadyCondition,
				corev1.ConditionTrue,
			)

		})

		It("creates all cell MQs", func() {
			th.SimulateTransportURLReady(cell0.TransportURLName)
			th.SimulateTransportURLReady(cell1.TransportURLName)
			th.ExpectCondition(
				novaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsMQReadyCondition,
				corev1.ConditionFalse,
			)
			th.SimulateTransportURLReady(cell2.TransportURLName)
			th.ExpectCondition(
				novaName,
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
			Expect(c1.Spec.CellMessageBusSecretName).To(Equal("mq-for-cell1-secret"))
			c1Conductor := GetNovaConductor(cell1.CellConductorName)
			Expect(c1Conductor.Spec.CellMessageBusSecretName).To(Equal("mq-for-cell1-secret"))

			th.ExpectCondition(
				cell1.CellConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionFalse,
			)
			// assert that cell1 using its own DB but has access to the API DB
			dbSync := th.GetJob(cell1.CellDBSyncJobName)
			Expect(dbSync.Spec.Template.Spec.InitContainers).To(HaveLen(0))
			configDataMap := th.GetConfigMap(
				types.NamespacedName{
					Namespace: namespace,
					Name:      fmt.Sprintf("%s-config-data", cell1.CellConductorName.Name),
				},
			)
			Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
			Expect(configDataMap.Data["01-nova.conf"]).To(
				ContainSubstring("[database]\nconnection = mysql+pymysql://nova_cell1:12345678@hostname-for-db-for-cell1/nova_cell1"),
			)
			Expect(configDataMap.Data["01-nova.conf"]).To(
				ContainSubstring("[api_database]\nconnection = mysql+pymysql://nova_api:12345678@hostname-for-db-for-api/nova_api"),
			)
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
				novaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsReadyCondition,
				corev1.ConditionFalse,
			)
			Eventually(func(g Gomega) {
				nova := GetNova(novaName)
				g.Expect(nova.Status.RegisteredCells).To(
					HaveKeyWithValue(cell0.CellName.Name, Not(BeEmpty())))
				g.Expect(nova.Status.RegisteredCells).To(
					HaveKeyWithValue(cell1.CellName.Name, Not(BeEmpty())))
			}, timeout, interval).Should(Succeed())

			// RegisteredCells are distributed
			nova := GetNova(novaName)
			api := GetNovaAPI(novaAPIName)
			Expect(api.Spec.RegisteredCells).To(Equal(nova.Status.RegisteredCells))
			scheduler := GetNovaScheduler(novaSchedulerName)
			Expect(scheduler.Spec.RegisteredCells).To(Equal(nova.Status.RegisteredCells))
			metadata := GetNovaMetadata(novaMetadataName)
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
			th.SimulateJobSuccess(cell1.CellDBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell1.ConductorStatefulSetName)
			th.SimulateJobSuccess(cell1.CellMappingJobName)

			th.SimulateMariaDBDatabaseCompleted(cell2.MariaDBDatabaseName)
			th.SimulateTransportURLReady(cell2.TransportURLName)

			// assert that cell related CRs are created pointing to the Cell 2 MQ
			c2 := GetNovaCell(cell2.CellName)
			Expect(c2.Spec.CellMessageBusSecretName).To(Equal("mq-for-cell2-secret"))
			c2Conductor := GetNovaConductor(cell2.CellConductorName)
			Expect(c2Conductor.Spec.CellMessageBusSecretName).To(Equal("mq-for-cell2-secret"))
			th.ExpectCondition(
				cell2.CellConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionFalse,
			)
			// assert that cell2 using its own DB but has *no* access to the API DB
			dbSync := th.GetJob(cell2.CellDBSyncJobName)
			Expect(dbSync.Spec.Template.Spec.InitContainers).To(HaveLen(0))
			configDataMap := th.GetConfigMap(
				types.NamespacedName{
					Namespace: namespace,
					Name:      fmt.Sprintf("%s-config-data", cell2.CellConductorName.Name),
				},
			)
			Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
			Expect(configDataMap.Data["01-nova.conf"]).To(
				ContainSubstring("[database]\nconnection = mysql+pymysql://nova_cell2:12345678@hostname-for-db-for-cell2/nova_cell2"),
			)
			Expect(configDataMap.Data["01-nova.conf"]).ToNot(
				ContainSubstring("[api_database]"),
			)
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
			mappingJobConfig := th.GetConfigMap(
				types.NamespacedName{
					Namespace: namespace,
					Name:      fmt.Sprintf("%s-config-data", cell2.CellName.Name+"-manage"),
				},
			)
			Expect(mappingJobConfig.Data).Should(HaveKey("01-nova.conf"))
			Expect(mappingJobConfig.Data["01-nova.conf"]).To(
				ContainSubstring("[database]\nconnection = mysql+pymysql://nova_cell2:12345678@hostname-for-db-for-cell2/nova_cell2"),
			)
			Expect(mappingJobConfig.Data["01-nova.conf"]).To(
				ContainSubstring("[api_database]\nconnection = mysql+pymysql://nova_api:12345678@hostname-for-db-for-api/nova_api"),
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
				novaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				novaName,
				ConditionGetterFunc(NovaConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
			Eventually(func(g Gomega) {
				nova := GetNova(novaName)
				g.Expect(nova.Status.RegisteredCells).To(
					HaveKeyWithValue(cell0.CellName.Name, Not(BeEmpty())))
				g.Expect(nova.Status.RegisteredCells).To(
					HaveKeyWithValue(cell1.CellName.Name, Not(BeEmpty())))
				g.Expect(nova.Status.RegisteredCells).To(
					HaveKeyWithValue(cell2.CellName.Name, Not(BeEmpty())))
			}, timeout, interval).Should(Succeed())

			// RegisteredCells are distributed
			nova := GetNova(novaName)
			api := GetNovaAPI(novaAPIName)
			Expect(api.Spec.RegisteredCells).To(Equal(nova.Status.RegisteredCells))
			scheduler := GetNovaScheduler(novaSchedulerName)
			Expect(scheduler.Spec.RegisteredCells).To(Equal(nova.Status.RegisteredCells))
			metadata := GetNovaMetadata(novaMetadataName)
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
				novaName,
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
			ExpectCondition(
				novaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAPIReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				novaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsReadyCondition,
				corev1.ConditionFalse,
			)
		})
		It("does not create cell1 if cell0 fails as cell1 needs API access", func() {
			th.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabase)
			th.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			th.SimulateMariaDBDatabaseCompleted(cell1.MariaDBDatabaseName)
			th.SimulateTransportURLReady(cell0.TransportURLName)
			th.SimulateTransportURLReady(cell1.TransportURLName)

			th.SimulateJobFailure(cell0.CellDBSyncJobName)

			NovaCellNotExists(cell1.CellName)
		})
	})

        # TODO rewrite for novaNames
	When("Nova CR instance is created with collapsed cell deployment", func() {
		BeforeEach(func() {
			DeferCleanup(k8sClient.Delete, ctx, CreateNovaSecret(namespace, SecretName))
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateNovaMessageBusSecret(namespace, "mq-for-api-secret"),
			)

			serviceSpec := corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 3306}}}
			DeferCleanup(th.DeleteDBService, th.CreateDBService(namespace, "openstack", serviceSpec))

			spec := GetDefaultNovaSpec()
			cell0 := GetDefaultNovaCellTemplate()
			cell0["cellDatabaseInstance"] = "openstack"
			cell0["cellDatabaseUser"] = "nova_cell0"
			cell0["hasAPIAccess"] = true
			// disable cell0 conductor
			cell0["conductorServiceTemplate"] = map[string]interface{}{
				"replicas": 0,
			}

			cell1 := GetDefaultNovaCellTemplate()
			// cell1 is configured to have API access and use the same
			// message bus as the top level services. Hence cell1 conductor
			// will act both as a super conductor and as cell1 conductor
			cell1["cellDatabaseInstance"] = "openstack"
			cell1["cellDatabaseUser"] = "nova_cell1"
			cell1["cellMessageBusInstance"] = "mq-for-api"
			cell1["hasAPIAccess"] = true

			spec["cellTemplates"] = map[string]interface{}{
				"cell0": cell0,
				"cell1": cell1,
			}
			spec["apiDatabaseInstance"] = "openstack"
			spec["apiMessageBusInstance"] = "mq-for-api"

			DeferCleanup(th.DeleteInstance, CreateNova(novaName, spec))
			keystoneAPIName := th.CreateKeystoneAPI(namespace)
			DeferCleanup(th.DeleteKeystoneAPI, keystoneAPIName)
			keystoneAPI := th.GetKeystoneAPI(keystoneAPIName)
			keystoneAPI.Status.APIEndpoints["internal"] = "http://keystone-internal-openstack.testing"
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Status().Update(ctx, keystoneAPI.DeepCopy())).Should(Succeed())
			}, timeout, interval).Should(Succeed())
			th.SimulateKeystoneServiceReady(novaKeystoneServiceName)
		})
		It("cell0 becomes ready with 0 conductor replicas and the rest of nova is deployed", func() {
			th.SimulateMariaDBDatabaseCompleted(mariaDBDatabaseNameForAPI)
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
			th.SimulateJobSuccess(cell1.CellDBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell1.ConductorStatefulSetName)
			th.SimulateJobSuccess(cell1.CellMappingJobName)

			th.ExpectCondition(
				cell1.CellName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaConductorReadyCondition,
				corev1.ConditionTrue,
			)

			th.SimulateStatefulSetReplicaReady(novaMetadataStatefulSetName)
			// As cell0 is ready API is deployed
			th.SimulateStatefulSetReplicaReady(novaAPIdeploymentName)
			th.SimulateKeystoneEndpointReady(novaAPIKeystoneEndpointName)
			th.ExpectCondition(
				novaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAPIReadyCondition,
				corev1.ConditionTrue,
			)

			// As cell0 is ready scheduler is deployed
			th.SimulateStatefulSetReplicaReady(novaSchedulerStatefulSetName)
			th.ExpectCondition(
				novaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaSchedulerReadyCondition,
				corev1.ConditionTrue,
			)

			// So the whole Nova deployment is ready
			th.ExpectCondition(
				novaName,
				ConditionGetterFunc(NovaConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})
})
