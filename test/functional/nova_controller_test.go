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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
)

var _ = Describe("Nova controller", func() {
	var namespace string
	var novaName types.NamespacedName
	var novaNames NovaNames
	var cell0 CellNames

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
		novaNames = GetNovaNames(novaName, []string{"cell0"})
		cell0 = novaNames.Cells["cell0"]
	})

	When("Nova CR instance is created without cell0", func() {
		BeforeEach(func() {
			CreateNovaWithoutCell0(novaName)
			DeferCleanup(DeleteNova, novaName)
		})

		It("is not Ready", func() {
			ExpectCondition(
				novaName,
				conditionGetterFunc(NovaConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionUnknown,
			)
		})

		It("has no hash and no services ready", func() {
			instance := GetNova(novaName)
			Expect(instance.Status.Hash).To(BeEmpty())
			Expect(instance.Status.APIServiceReadyCount).To(Equal(int32(0)))
			Expect(instance.Status.SchedulerServiceReadyCount).To(Equal(int32(0)))
			Expect(instance.Status.MetadataServiceReadyCount).To(Equal(int32(0)))
		})

		It("reports that cell0 is missing from the spec", func() {
			ExpectConditionWithDetails(
				novaName,
				conditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NovaCell creation failed for cell0(missing cell0 specification from Spec.CellTemplates)",
			)
		})
	})

	When("Nova CR instance is created", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaSecret(namespace, SecretName))
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateNovaMessageBusSecret(namespace, MessageBusSecretName),
			)
			DeferCleanup(
				DeleteDBService,
				CreateDBService(
					namespace,
					"openstack",
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			DeferCleanup(DeleteKeystoneAPI, CreateKeystoneAPI(namespace))

			CreateNovaWithCell0(novaName)
			DeferCleanup(DeleteNova, novaName)
		})

		It("registers nova service to keystone", func() {
			// assert that the KeystoneService for nova is created
			GetKeystoneService(novaNames.KeystoneServiceName)
			// and simulate that it becomes ready i.e. the keystone-operator
			// did its job and registered the nova service
			SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)

			ExpectCondition(
				novaName,
				conditionGetterFunc(NovaConditionGetter),
				condition.KeystoneServiceReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("creates nova_api DB", func() {
			SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
			ExpectCondition(
				novaName,
				conditionGetterFunc(NovaConditionGetter),
				novav1.NovaAPIDBReadyCondition,
				corev1.ConditionFalse,
			)
			GetMariaDBDatabase(novaNames.APIMariaDBDatabaseName)

			SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			ExpectCondition(
				novaName,
				conditionGetterFunc(NovaConditionGetter),
				novav1.NovaAPIDBReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("creates nova-api MQ", func() {
			SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
			ExpectCondition(
				novaName,
				conditionGetterFunc(NovaConditionGetter),
				novav1.NovaAPIMQReadyCondition,
				corev1.ConditionFalse,
			)
			GetTransportURL(cell0.TransportURLName)

			SimulateTransportURLReady(cell0.TransportURLName)
			ExpectCondition(
				novaName,
				conditionGetterFunc(NovaConditionGetter),
				novav1.NovaAPIMQReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("creates nova_cell0 DB", func() {
			SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
			ExpectCondition(
				novaName,
				conditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsDBReadyCondition,
				corev1.ConditionFalse,
			)
			SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			GetMariaDBDatabase(cell0.MariaDBDatabaseName)
			SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			ExpectCondition(
				novaName,
				conditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsDBReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("creates cell0 NovaCell", func() {
			SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
			SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			SimulateTransportURLReady(cell0.TransportURLName)
			// assert that cell related CRs are created
			cell := GetNovaCell(cell0.CellName)
			Expect(cell.Spec.CellMessageBusSecretName).To(Equal("rabbitmq-secret"))
			Expect(cell.Spec.ServiceUser).To(Equal("nova"))

			conductor := GetNovaConductor(cell0.CellConductorName)
			Expect(conductor.Spec.CellMessageBusSecretName).To(Equal("rabbitmq-secret"))
			Expect(conductor.Spec.ServiceUser).To(Equal("nova"))

			ExpectCondition(
				cell0.CellConductorName,
				conditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionFalse,
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
				corev1.ConditionTrue,
			)
		})

		It("create NovaAPI", func() {
			SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
			SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			SimulateTransportURLReady(cell0.TransportURLName)
			SimulateJobSuccess(cell0.CellDBSyncJobName)
			SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)

			api := GetNovaAPI(novaNames.APIName)
			Expect(api.Spec.APIMessageBusSecretName).To(Equal("rabbitmq-secret"))
			Expect(api.Spec.ServiceUser).To(Equal("nova"))

			SimulateStatefulSetReplicaReady(novaNames.APIDeploymentName)

			ExpectCondition(
				novaNames.APIName,
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
			ExpectCondition(
				novaName,
				conditionGetterFunc(NovaConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})

	When("Nova CR instance is created but cell0 DB sync fails", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaSecret(namespace, SecretName))
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateNovaMessageBusSecret(namespace, MessageBusSecretName),
			)
			DeferCleanup(
				DeleteDBService,
				CreateDBService(
					namespace,
					"openstack",
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			DeferCleanup(DeleteKeystoneAPI, CreateKeystoneAPI(namespace))

			CreateNovaWithCell0(novaName)
			DeferCleanup(DeleteNova, novaName)
		})

		It("does not create NovaAPI", func() {
			SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
			SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			SimulateTransportURLReady(cell0.TransportURLName)
			GetNovaCell(cell0.CellName)
			GetNovaConductor(cell0.CellConductorName)

			SimulateJobFailure(cell0.CellDBSyncJobName)
			ExpectCondition(
				cell0.CellConductorName,
				conditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionFalse,
			)
			ExpectCondition(
				cell0.CellName,
				conditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaConductorReadyCondition,
				corev1.ConditionFalse,
			)
			ExpectCondition(
				novaName,
				conditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsReadyCondition,
				corev1.ConditionFalse,
			)

			NovaAPINotExists(novaNames.APIName)
		})
	})

	When("Nova CR instance with different DB Services for nova_api and cell0 DBs", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaSecret(namespace, SecretName))
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateNovaMessageBusSecret(namespace, MessageBusSecretName),
			)

			DeferCleanup(
				DeleteDBService,
				CreateDBService(
					namespace,
					"db-for-cell0",
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			DeferCleanup(
				DeleteDBService,
				CreateDBService(
					namespace,
					"db-for-api",
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			DeferCleanup(DeleteKeystoneAPI, CreateKeystoneAPI(namespace))

			spec := GetDefaultNovaSpec()
			cell0 := GetDefaultNovaCellTemplate()
			cell0["cellDatabaseInstance"] = "db-for-cell0"
			spec["cellTemplates"] = map[string]interface{}{"cell0": cell0}
			spec["apiDatabaseInstance"] = "db-for-api"
			CreateNova(novaName, spec)

			DeferCleanup(DeleteNova, novaName)
		})

		It("uses the correct hostnames to access the different DB services", func() {
			SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
			SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			SimulateTransportURLReady(cell0.TransportURLName)

			cell0DBSync := GetJob(cell0.CellDBSyncJobName)
			cell0DBSyncJobEnv := cell0DBSync.Spec.Template.Spec.InitContainers[0].Env
			Expect(cell0DBSyncJobEnv).To(
				ContainElements(
					[]corev1.EnvVar{
						{Name: "CellDatabaseHost", Value: "hostname-for-db-for-cell0"},
						{Name: "APIDatabaseHost", Value: "hostname-for-db-for-api"},
					},
				),
			)

			SimulateJobSuccess(cell0.CellDBSyncJobName)
			SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)

			configDataMap := th.GetConfigMap(
				types.NamespacedName{
					Namespace: namespace,
					Name:      fmt.Sprintf("%s-config-data", novaNames.APIName.Name),
				},
			)
			Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
			Expect(configDataMap.Data["01-nova.conf"]).To(
				ContainSubstring("[database]\nconnection = mysql+pymysql://nova_cell0:12345678@hostname-for-db-for-cell0/nova_cell0"),
			)
			Expect(configDataMap.Data["01-nova.conf"]).To(
				ContainSubstring("[api_database]\nconnection = mysql+pymysql://nova_api:12345678@hostname-for-db-for-api/nova_api"),
			)

			SimulateStatefulSetReplicaReady(novaNames.APIName)

			ExpectCondition(
				cell0.CellConductorName,
				conditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionTrue,
			)
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
				corev1.ConditionTrue,
			)
			ExpectCondition(
				novaName,
				conditionGetterFunc(NovaConditionGetter),
				novav1.NovaAPIReadyCondition,
				corev1.ConditionTrue,
			)
			ExpectCondition(
				novaName,
				conditionGetterFunc(NovaConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})
	When("Nova CR instance is deleted", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaSecret(namespace, SecretName))
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateNovaMessageBusSecret(namespace, MessageBusSecretName),
			)
			DeferCleanup(
				DeleteDBService,
				CreateDBService(
					namespace,
					"openstack",
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			DeferCleanup(DeleteKeystoneAPI, CreateKeystoneAPI(namespace))

			CreateNovaWithCell0(novaName)
			DeferCleanup(DeleteNova, novaName)
		})

		It("removes the finalizer from KeystoneService", func() {
			SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
			ExpectCondition(
				novaName,
				conditionGetterFunc(NovaConditionGetter),
				condition.KeystoneServiceReadyCondition,
				corev1.ConditionTrue,
			)

			service := GetKeystoneService(novaNames.KeystoneServiceName)
			Expect(service.Finalizers).To(ContainElement("Nova"))

			DeleteNova(novaName)
			service = GetKeystoneService(novaNames.KeystoneServiceName)
			Expect(service.Finalizers).NotTo(ContainElement("Nova"))
		})
	})
})
