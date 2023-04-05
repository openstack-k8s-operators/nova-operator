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

type Cell struct {
	CellName                 types.NamespacedName
	MariaDBDatabaseName      types.NamespacedName
	CellConductorName        types.NamespacedName
	CellDBSyncJobName        types.NamespacedName
	ConductorStatefulSetName types.NamespacedName
	TransportURLName         types.NamespacedName
}

func NewCell(novaName types.NamespacedName, cell string) Cell {
	cellName := types.NamespacedName{
		Namespace: novaName.Namespace,
		Name:      novaName.Name + "-" + cell,
	}
	c := Cell{
		CellName: cellName,
		MariaDBDatabaseName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      "nova-" + cell,
		},
		CellConductorName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      cellName.Name + "-conductor",
		},
		CellDBSyncJobName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      cellName.Name + "-conductor-db-sync",
		},
		ConductorStatefulSetName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      cellName.Name + "-conductor",
		},
		TransportURLName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      cell + "-transport",
		},
	}

	if cell == "cell0" {
		c.TransportURLName = types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      "nova-api-transport",
		}
	}

	return c
}

var _ = Describe("Nova multi cell", func() {
	var novaName types.NamespacedName
	var mariaDBDatabaseNameForAPI types.NamespacedName
	var cell0 Cell
	var cell1 Cell
	var cell2 Cell
	var novaAPIName types.NamespacedName
	var novaAPIdeploymentName types.NamespacedName
	var novaAPIKeystoneEndpointName types.NamespacedName
	var novaKeystoneServiceName types.NamespacedName
	var novaSchedulerName types.NamespacedName
	var novaSchedulerStatefulSetName types.NamespacedName
	var novaMetadataName types.NamespacedName
	var novaMetadataStatefulSetName types.NamespacedName

	BeforeEach(func() {
		// Uncomment this if you need the full output in the logs from gomega
		// matchers
		// format.MaxLength = 0

		novaName = types.NamespacedName{
			Namespace: namespace,
			Name:      uuid.New().String(),
		}
		mariaDBDatabaseNameForAPI = types.NamespacedName{
			Namespace: namespace,
			Name:      "nova-api",
		}
		novaAPIName = types.NamespacedName{
			Namespace: namespace,
			Name:      novaName.Name + "-api",
		}
		novaAPIdeploymentName = types.NamespacedName{
			Namespace: namespace,
			Name:      novaAPIName.Name,
		}
		novaKeystoneServiceName = types.NamespacedName{
			Namespace: namespace,
			Name:      "nova",
		}
		novaAPIKeystoneEndpointName = types.NamespacedName{
			Namespace: namespace,
			Name:      "nova",
		}
		novaSchedulerName = types.NamespacedName{
			Namespace: namespace,
			Name:      novaName.Name + "-scheduler",
		}
		novaSchedulerStatefulSetName = types.NamespacedName{
			Namespace: namespace,
			Name:      novaSchedulerName.Name,
		}
		novaMetadataName = types.NamespacedName{
			Namespace: namespace,
			Name:      novaName.Name + "-metadata",
		}
		novaMetadataStatefulSetName = types.NamespacedName{
			Namespace: namespace,
			Name:      novaMetadataName.Name,
		}
		cell0 = NewCell(novaName, "cell0")
		cell1 = NewCell(novaName, "cell1")
		cell2 = NewCell(novaName, "cell2")
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
			cell0["cellName"] = "cell0"
			cell0["cellDatabaseInstance"] = "db-for-api"
			cell0["cellDatabaseUser"] = "nova_cell0"

			cell1 := GetDefaultNovaCellTemplate()
			cell1["cellName"] = "cell1"
			cell1["cellDatabaseInstance"] = "db-for-cell1"
			cell1["cellDatabaseUser"] = "nova_cell1"
			cell1["cellMessageBusInstance"] = "mq-for-cell1"

			cell2 := GetDefaultNovaCellTemplate()
			cell2["cellName"] = "cell2"
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

			DeferCleanup(DeleteInstance, CreateNova(novaName, spec))
			keystoneAPIName := th.CreateKeystoneAPI(namespace)
			DeferCleanup(th.DeleteKeystoneAPI, keystoneAPIName)
			keystoneAPI := th.GetKeystoneAPI(keystoneAPIName)
			keystoneAPI.Status.APIEndpoints["internal"] = "http://keystone-internal-openstack.testing"
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Status().Update(ctx, keystoneAPI.DeepCopy())).Should(Succeed())
			}, timeout, interval).Should(Succeed())
			th.SimulateKeystoneServiceReady(novaKeystoneServiceName)
		})

		It("creates cell0 NovaCell", func() {
			th.SimulateMariaDBDatabaseCompleted(mariaDBDatabaseNameForAPI)
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
			// assert that cell0 using the same DB as the API
			dbSync := th.GetJob(cell0.CellDBSyncJobName)
			dbSyncJobEnv := dbSync.Spec.Template.Spec.InitContainers[0].Env
			Expect(dbSyncJobEnv).To(
				ContainElements(
					[]corev1.EnvVar{
						{Name: "CellDatabaseHost", Value: "hostname-for-db-for-api"},
						{Name: "APIDatabaseHost", Value: "hostname-for-db-for-api"},
					},
				),
			)

			th.SimulateJobSuccess(cell0.CellDBSyncJobName)
			th.ExpectCondition(
				cell0.CellConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionTrue,
			)
			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
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
		})

		It("creates NovaAPI", func() {
			th.SimulateMariaDBDatabaseCompleted(mariaDBDatabaseNameForAPI)
			th.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			th.SimulateTransportURLReady(cell0.TransportURLName)
			th.SimulateJobSuccess(cell0.CellDBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)

			api := GetNovaAPI(novaAPIName)
			Expect(api.Spec.Replicas).Should(BeEquivalentTo(1))
			Expect(api.Spec.Cell0DatabaseHostname).To(Equal("hostname-for-db-for-api"))
			Expect(api.Spec.Cell0DatabaseHostname).To(Equal(api.Spec.APIDatabaseHostname))
			Expect(api.Spec.APIMessageBusSecretName).To(Equal("mq-for-api-secret"))

			configDataMap := th.GetConfigMap(
				types.NamespacedName{
					Namespace: namespace,
					Name:      fmt.Sprintf("%s-config-data", novaAPIName.Name),
				},
			)
			Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
			Expect(configDataMap.Data["01-nova.conf"]).To(
				ContainSubstring("[database]\nconnection = mysql+pymysql://nova_cell0:12345678@hostname-for-db-for-api/nova_cell0"),
			)
			Expect(configDataMap.Data["01-nova.conf"]).To(
				ContainSubstring("[api_database]\nconnection = mysql+pymysql://nova_api:12345678@hostname-for-db-for-api/nova_api"),
			)

			th.SimulateStatefulSetReplicaReady(novaAPIdeploymentName)

			th.ExpectCondition(
				novaAPIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionTrue,
			)

			th.SimulateKeystoneEndpointReady(novaAPIKeystoneEndpointName)
			th.ExpectCondition(
				novaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAPIReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("creates all cell DBs", func() {
			th.SimulateMariaDBDatabaseCompleted(mariaDBDatabaseNameForAPI)
			th.SimulateTransportURLReady(cell0.TransportURLName)
			th.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			th.SimulateJobSuccess(cell0.CellDBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
			th.SimulateStatefulSetReplicaReady(novaAPIdeploymentName)
			th.SimulateKeystoneEndpointReady(novaAPIKeystoneEndpointName)

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
			th.SimulateMariaDBDatabaseCompleted(mariaDBDatabaseNameForAPI)
			th.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			th.SimulateTransportURLReady(cell0.TransportURLName)
			th.SimulateJobSuccess(cell0.CellDBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
			th.SimulateStatefulSetReplicaReady(novaAPIdeploymentName)
			th.SimulateKeystoneEndpointReady(novaAPIKeystoneEndpointName)
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
			dbSyncJobEnv := dbSync.Spec.Template.Spec.InitContainers[0].Env
			Expect(dbSyncJobEnv).To(
				ContainElements(
					[]corev1.EnvVar{
						{Name: "CellDatabaseHost", Value: "hostname-for-db-for-cell1"},
						{Name: "APIDatabaseHost", Value: "hostname-for-db-for-api"},
					},
				),
			)
			th.SimulateJobSuccess(cell1.CellDBSyncJobName)
			th.ExpectCondition(
				cell1.CellConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionTrue,
			)
			th.SimulateStatefulSetReplicaReady(cell1.ConductorStatefulSetName)
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
		})
		It("creates cell2 NovaCell", func() {
			th.SimulateMariaDBDatabaseCompleted(mariaDBDatabaseNameForAPI)
			th.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			th.SimulateTransportURLReady(cell0.TransportURLName)
			th.SimulateJobSuccess(cell0.CellDBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
			th.SimulateStatefulSetReplicaReady(novaAPIdeploymentName)
			th.SimulateKeystoneEndpointReady(novaAPIKeystoneEndpointName)
			th.SimulateStatefulSetReplicaReady(novaSchedulerStatefulSetName)
			th.SimulateStatefulSetReplicaReady(novaMetadataStatefulSetName)
			th.SimulateMariaDBDatabaseCompleted(cell1.MariaDBDatabaseName)
			th.SimulateTransportURLReady(cell1.TransportURLName)
			th.SimulateJobSuccess(cell1.CellDBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell1.ConductorStatefulSetName)

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
			dbSyncJobEnv := dbSync.Spec.Template.Spec.InitContainers[0].Env
			Expect(dbSyncJobEnv).To(
				ContainElements(
					[]corev1.EnvVar{
						{Name: "CellDatabaseHost", Value: "hostname-for-db-for-cell2"},
					},
				),
			)
			Expect(dbSyncJobEnv).NotTo(
				ContainElements(
					[]corev1.EnvVar{
						{Name: "APIDatabaseHost", Value: "hostname-for-db-for-api"},
					},
				),
			)
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
			th.SimulateMariaDBDatabaseCompleted(mariaDBDatabaseNameForAPI)
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
			GetNovaAPI(novaAPIName)
			th.SimulateStatefulSetReplicaReady(novaAPIdeploymentName)
			th.SimulateKeystoneEndpointReady(novaAPIKeystoneEndpointName)
			th.ExpectCondition(
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
			th.SimulateMariaDBDatabaseCompleted(mariaDBDatabaseNameForAPI)
			th.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			th.SimulateMariaDBDatabaseCompleted(cell1.MariaDBDatabaseName)
			th.SimulateTransportURLReady(cell0.TransportURLName)
			th.SimulateTransportURLReady(cell1.TransportURLName)

			th.SimulateJobFailure(cell0.CellDBSyncJobName)

			NovaCellNotExists(cell1.CellName)
		})
	})

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
			cell0["cellName"] = "cell0"
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
			cell1["cellName"] = "cell1"
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

			DeferCleanup(DeleteInstance, CreateNova(novaName, spec))
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

			// As cell0 is ready cell1 is deployed
			th.SimulateJobSuccess(cell1.CellDBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell1.ConductorStatefulSetName)

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
