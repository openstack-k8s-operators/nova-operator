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
	"os"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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

var _ = Describe("Nova controller", func() {
	var namespace string
	var novaName types.NamespacedName
	var mariaDBDatabaseNameForAPI types.NamespacedName
	var cell0 Cell
	var cell1 Cell
	var cell2 Cell
	var novaAPIName types.NamespacedName
	var novaAPIdeploymentName types.NamespacedName
	var novaKeystoneServiceName types.NamespacedName

	BeforeEach(func() {
		// NOTE(gibi): We need to create a unique namespace for each test run
		// as namespaces cannot be deleted in a locally running envtest. See
		// https://book.kubebuilder.io/reference/envtest.html#namespace-usage-limitation
		namespace = uuid.New().String()
		CreateNamespace(namespace)
		// We still request the delete of the Namespace to properly cleanup if
		// we run the test in an existing cluster.
		DeferCleanup(DeleteNamespace, namespace)
		// NOTE(gibi): ConfigMap generation looks up the local templates
		// directory via ENV, so provide it
		DeferCleanup(os.Setenv, "OPERATOR_TEMPLATES", os.Getenv("OPERATOR_TEMPLATES"))
		os.Setenv("OPERATOR_TEMPLATES", "../../templates")

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
			DeferCleanup(DeleteDBService, CreateDBService(namespace, "db-for-api", serviceSpec))
			DeferCleanup(DeleteDBService, CreateDBService(namespace, "db-for-cell1", serviceSpec))
			DeferCleanup(DeleteDBService, CreateDBService(namespace, "db-for-cell2", serviceSpec))

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

			CreateNova(novaName, spec)
			DeferCleanup(DeleteNova, novaName)
			DeferCleanup(DeleteKeystoneAPI, CreateKeystoneAPI(namespace))
			SimulateKeystoneServiceReady(novaKeystoneServiceName)
		})

		It("creates cell0 NovaCell", func() {
			SimulateMariaDBDatabaseCompleted(mariaDBDatabaseNameForAPI)
			SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			SimulateTransportURLReady(cell0.TransportURLName)

			// assert that cell related CRs are created pointing to the API MQ
			cell := GetNovaCell(cell0.CellName)
			Expect(cell.Spec.CellMessageBusSecretName).To(Equal("mq-for-api-secret"))
			conductor := GetNovaConductor(cell0.CellConductorName)
			Expect(conductor.Spec.CellMessageBusSecretName).To(Equal("mq-for-api-secret"))

			ExpectCondition(
				cell0.CellConductorName,
				conditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionFalse,
			)
			// assert that cell0 using the same DB as the API
			dbSync := GetJob(cell0.CellDBSyncJobName)
			dbSyncJobEnv := dbSync.Spec.Template.Spec.InitContainers[0].Env
			Expect(dbSyncJobEnv).To(
				ContainElements(
					[]corev1.EnvVar{
						{Name: "CellDatabaseHost", Value: "hostname-for-db-for-api"},
						{Name: "APIDatabaseHost", Value: "hostname-for-db-for-api"},
					},
				),
			)

			SimulateJobSuccess(cell0.CellDBSyncJobName)
			ExpectCondition(
				cell0.CellConductorName,
				conditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionTrue,
			)
			SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
			ExpectCondition(
				cell0.CellName,
				conditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaConductorReadyCondition,
				corev1.ConditionTrue,
			)
			ExpectCondition(
				novaName,
				conditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("creates NovaAPI", func() {
			SimulateMariaDBDatabaseCompleted(mariaDBDatabaseNameForAPI)
			SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			SimulateTransportURLReady(cell0.TransportURLName)
			SimulateJobSuccess(cell0.CellDBSyncJobName)
			SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)

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

			SimulateStatefulSetReplicaReady(novaAPIdeploymentName)

			ExpectCondition(
				novaAPIName,
				conditionGetterFunc(NovaAPIConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionTrue,
			)
			ExpectCondition(
				novaName,
				conditionGetterFunc(NovaConditionGetter),
				novav1.NovaAPIReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("creates all cell DBs", func() {
			SimulateMariaDBDatabaseCompleted(mariaDBDatabaseNameForAPI)
			SimulateTransportURLReady(cell0.TransportURLName)
			SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			SimulateJobSuccess(cell0.CellDBSyncJobName)
			SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
			SimulateStatefulSetReplicaReady(novaAPIdeploymentName)

			SimulateMariaDBDatabaseCompleted(cell1.MariaDBDatabaseName)
			ExpectCondition(
				novaName,
				conditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsDBReadyCondition,
				corev1.ConditionFalse,
			)
			SimulateMariaDBDatabaseCompleted(cell2.MariaDBDatabaseName)
			ExpectCondition(
				novaName,
				conditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsDBReadyCondition,
				corev1.ConditionTrue,
			)

		})

		It("creates all cell MQs", func() {
			SimulateTransportURLReady(cell0.TransportURLName)
			SimulateTransportURLReady(cell1.TransportURLName)
			ExpectCondition(
				novaName,
				conditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsMQReadyCondition,
				corev1.ConditionFalse,
			)
			SimulateTransportURLReady(cell2.TransportURLName)
			ExpectCondition(
				novaName,
				conditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsMQReadyCondition,
				corev1.ConditionTrue,
			)

		})

		It("creates cell1 NovaCell", func() {
			SimulateMariaDBDatabaseCompleted(mariaDBDatabaseNameForAPI)
			SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			SimulateTransportURLReady(cell0.TransportURLName)
			SimulateJobSuccess(cell0.CellDBSyncJobName)
			SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
			SimulateStatefulSetReplicaReady(novaAPIdeploymentName)
			SimulateMariaDBDatabaseCompleted(cell1.MariaDBDatabaseName)
			SimulateTransportURLReady(cell1.TransportURLName)

			// assert that cell related CRs are created pointing to the cell1 MQ
			c1 := GetNovaCell(cell1.CellName)
			Expect(c1.Spec.CellMessageBusSecretName).To(Equal("mq-for-cell1-secret"))
			c1Conductor := GetNovaConductor(cell1.CellConductorName)
			Expect(c1Conductor.Spec.CellMessageBusSecretName).To(Equal("mq-for-cell1-secret"))

			ExpectCondition(
				cell1.CellConductorName,
				conditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionFalse,
			)
			// assert that cell1 using its own DB but has access to the API DB
			dbSync := GetJob(cell1.CellDBSyncJobName)
			dbSyncJobEnv := dbSync.Spec.Template.Spec.InitContainers[0].Env
			Expect(dbSyncJobEnv).To(
				ContainElements(
					[]corev1.EnvVar{
						{Name: "CellDatabaseHost", Value: "hostname-for-db-for-cell1"},
						{Name: "APIDatabaseHost", Value: "hostname-for-db-for-api"},
					},
				),
			)
			SimulateJobSuccess(cell1.CellDBSyncJobName)
			ExpectCondition(
				cell1.CellConductorName,
				conditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionTrue,
			)
			SimulateStatefulSetReplicaReady(cell1.ConductorStatefulSetName)
			ExpectCondition(
				cell1.CellName,
				conditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaConductorReadyCondition,
				corev1.ConditionTrue,
			)
			ExpectCondition(
				novaName,
				conditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsReadyCondition,
				corev1.ConditionFalse,
			)
		})
		It("creates cell2 NovaCell", func() {
			SimulateMariaDBDatabaseCompleted(mariaDBDatabaseNameForAPI)
			SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			SimulateTransportURLReady(cell0.TransportURLName)
			SimulateJobSuccess(cell0.CellDBSyncJobName)
			SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
			SimulateStatefulSetReplicaReady(novaAPIdeploymentName)
			SimulateMariaDBDatabaseCompleted(cell1.MariaDBDatabaseName)
			SimulateTransportURLReady(cell1.TransportURLName)
			SimulateJobSuccess(cell1.CellDBSyncJobName)
			SimulateStatefulSetReplicaReady(cell1.ConductorStatefulSetName)

			SimulateMariaDBDatabaseCompleted(cell2.MariaDBDatabaseName)
			SimulateTransportURLReady(cell2.TransportURLName)

			// assert that cell related CRs are created pointing to the Cell 2 MQ
			c2 := GetNovaCell(cell2.CellName)
			Expect(c2.Spec.CellMessageBusSecretName).To(Equal("mq-for-cell2-secret"))
			c2Conductor := GetNovaConductor(cell2.CellConductorName)
			Expect(c2Conductor.Spec.CellMessageBusSecretName).To(Equal("mq-for-cell2-secret"))
			ExpectCondition(
				cell2.CellConductorName,
				conditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionFalse,
			)
			// assert that cell2 using its own DB but has *no* access to the API DB
			dbSync := GetJob(cell2.CellDBSyncJobName)
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
			SimulateJobSuccess(cell2.CellDBSyncJobName)
			ExpectCondition(
				cell2.CellConductorName,
				conditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionTrue,
			)
			SimulateStatefulSetReplicaReady(cell2.ConductorStatefulSetName)
			ExpectCondition(
				cell2.CellName,
				conditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaConductorReadyCondition,
				corev1.ConditionTrue,
			)
			// As cell2 was the last cell to deploy all cells is ready now and
			// Nova becomes ready
			ExpectCondition(
				novaName,
				conditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsReadyCondition,
				corev1.ConditionTrue,
			)
			ExpectCondition(
				novaName,
				conditionGetterFunc(NovaConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("creates cell2 NovaCell even if everthing else fails", func() {
			// Don't simulate any success for any other DBs MQs or Cells
			// just for cell2
			SimulateMariaDBDatabaseCompleted(cell2.MariaDBDatabaseName)
			SimulateTransportURLReady(cell2.TransportURLName)

			// assert that cell related CRs are created
			GetNovaCell(cell2.CellName)
			GetNovaConductor(cell2.CellConductorName)

			SimulateJobSuccess(cell2.CellDBSyncJobName)
			ExpectCondition(
				cell2.CellConductorName,
				conditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionTrue,
			)
			SimulateStatefulSetReplicaReady(cell2.ConductorStatefulSetName)
			ExpectCondition(
				cell2.CellName,
				conditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaConductorReadyCondition,
				corev1.ConditionTrue,
			)
			ExpectCondition(
				cell2.CellName,
				conditionGetterFunc(NovaCellConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
			// Only cell2 succeeded so Nova is not ready yet
			ExpectCondition(
				novaName,
				conditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsReadyCondition,
				corev1.ConditionFalse,
			)
		})
		It("creates Nova API even if cell1 and cell2 fails", func() {
			SimulateMariaDBDatabaseCompleted(mariaDBDatabaseNameForAPI)
			SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			SimulateTransportURLReady(cell0.TransportURLName)
			SimulateJobSuccess(cell0.CellDBSyncJobName)
			SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)

			// Simulate that cell1 DB sync failed and do not simulate
			// cell2 DB creation success so that will be in Creating state.
			SimulateMariaDBDatabaseCompleted(cell1.MariaDBDatabaseName)
			SimulateTransportURLReady(cell1.TransportURLName)
			SimulateJobFailure(cell1.CellDBSyncJobName)

			// NovaAPI is still created
			GetNovaAPI(novaAPIName)
			SimulateStatefulSetReplicaReady(novaAPIdeploymentName)
			ExpectCondition(
				novaName,
				conditionGetterFunc(NovaConditionGetter),
				novav1.NovaAPIReadyCondition,
				corev1.ConditionTrue,
			)
			ExpectCondition(
				novaName,
				conditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsReadyCondition,
				corev1.ConditionFalse,
			)
		})
		It("does not create cell1 if cell0 fails as cell1 needs API access", func() {
			SimulateMariaDBDatabaseCompleted(mariaDBDatabaseNameForAPI)
			SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			SimulateMariaDBDatabaseCompleted(cell1.MariaDBDatabaseName)
			SimulateTransportURLReady(cell0.TransportURLName)
			SimulateTransportURLReady(cell1.TransportURLName)

			SimulateJobFailure(cell0.CellDBSyncJobName)

			NovaCellNotExists(cell1.CellName)
		})
	})
})
