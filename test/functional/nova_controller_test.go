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
	. "github.com/openstack-k8s-operators/lib-common/modules/test/helpers"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
)

var _ = Describe("Nova controller", func() {
	var namespace string
	var novaName types.NamespacedName
	var mariaDBDatabaseNameForAPI types.NamespacedName
	var mariaDBDatabaseNameForCell0 types.NamespacedName
	var cell0Name types.NamespacedName
	var cell0ConductorName types.NamespacedName
	var cell0DBSyncJobName types.NamespacedName
	var novaAPIName types.NamespacedName
	var novaAPIdeploymentName types.NamespacedName
	var novaKeystoneServiceName types.NamespacedName
	var novaCell0ConductorStatefulSetName types.NamespacedName
	var apiTransportURLName types.NamespacedName
	var novaSchedulerName types.NamespacedName
	var novaSchedulerStatefulSetName types.NamespacedName

	BeforeEach(func() {
		// NOTE(gibi): We need to create a unique namespace for each test run
		// as namespaces cannot be deleted in a locally running envtest. See
		// https://book.kubebuilder.io/reference/envtest.html#namespace-usage-limitation
		namespace = uuid.New().String()
		th.CreateNamespace(namespace)
		// We still request the delete of the Namespace to properly cleanup if
		// we run the test in an existing cluster.
		DeferCleanup(th.DeleteNamespace, namespace)
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
		mariaDBDatabaseNameForCell0 = types.NamespacedName{
			Namespace: namespace,
			Name:      "nova-cell0",
		}
		cell0Name = types.NamespacedName{
			Namespace: namespace,
			Name:      novaName.Name + "-cell0",
		}
		cell0ConductorName = types.NamespacedName{
			Namespace: namespace,
			Name:      cell0Name.Name + "-conductor",
		}
		cell0DBSyncJobName = types.NamespacedName{
			Namespace: namespace,
			Name:      cell0ConductorName.Name + "-db-sync",
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
		novaCell0ConductorStatefulSetName = types.NamespacedName{
			Namespace: namespace,
			Name:      cell0ConductorName.Name,
		}
		apiTransportURLName = types.NamespacedName{
			Namespace: namespace,
			Name:      "nova-api-transport",
		}
		novaSchedulerName = types.NamespacedName{
			Namespace: namespace,
			Name:      novaName.Name + "-scheduler",
		}
		novaSchedulerStatefulSetName = types.NamespacedName{
			Namespace: namespace,
			Name:      novaSchedulerName.Name,
		}
	})

	When("Nova CR instance is created without cell0", func() {
		BeforeEach(func() {
			CreateNovaWithoutCell0(novaName)
			DeferCleanup(DeleteNova, novaName)
		})

		It("is not Ready", func() {
			th.ExpectCondition(
				novaName,
				ConditionGetterFunc(NovaConditionGetter),
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
			th.ExpectConditionWithDetails(
				novaName,
				ConditionGetterFunc(NovaConditionGetter),
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
			GetKeystoneService(novaKeystoneServiceName)
			// and simulate that it becomes ready i.e. the keystone-operator
			// did its job and registered the nova service
			SimulateKeystoneServiceReady(novaKeystoneServiceName)

			th.ExpectCondition(
				novaName,
				ConditionGetterFunc(NovaConditionGetter),
				condition.KeystoneServiceReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("creates nova_api DB", func() {
			SimulateKeystoneServiceReady(novaKeystoneServiceName)
			th.ExpectCondition(
				novaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAPIDBReadyCondition,
				corev1.ConditionFalse,
			)
			GetMariaDBDatabase(mariaDBDatabaseNameForAPI)

			SimulateMariaDBDatabaseCompleted(mariaDBDatabaseNameForAPI)
			th.ExpectCondition(
				novaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAPIDBReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("creates nova-api MQ", func() {
			SimulateKeystoneServiceReady(novaKeystoneServiceName)
			th.ExpectCondition(
				novaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAPIMQReadyCondition,
				corev1.ConditionFalse,
			)
			GetTransportURL(apiTransportURLName)

			SimulateTransportURLReady(apiTransportURLName)
			th.ExpectCondition(
				novaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAPIMQReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("creates nova_cell0 DB", func() {
			SimulateKeystoneServiceReady(novaKeystoneServiceName)
			th.ExpectCondition(
				novaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsDBReadyCondition,
				corev1.ConditionFalse,
			)
			SimulateMariaDBDatabaseCompleted(mariaDBDatabaseNameForAPI)
			GetMariaDBDatabase(mariaDBDatabaseNameForCell0)
			SimulateMariaDBDatabaseCompleted(mariaDBDatabaseNameForCell0)
			th.ExpectCondition(
				novaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsDBReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("creates cell0 NovaCell", func() {
			SimulateKeystoneServiceReady(novaKeystoneServiceName)
			SimulateMariaDBDatabaseCompleted(mariaDBDatabaseNameForAPI)
			SimulateMariaDBDatabaseCompleted(mariaDBDatabaseNameForCell0)
			SimulateTransportURLReady(apiTransportURLName)
			// assert that cell related CRs are created
			cell := GetNovaCell(cell0Name)
			Expect(cell.Spec.CellMessageBusSecretName).To(Equal("rabbitmq-secret"))
			Expect(cell.Spec.ServiceUser).To(Equal("nova"))

			conductor := GetNovaConductor(cell0ConductorName)
			Expect(conductor.Spec.CellMessageBusSecretName).To(Equal("rabbitmq-secret"))
			Expect(conductor.Spec.ServiceUser).To(Equal("nova"))

			th.ExpectCondition(
				cell0ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionFalse,
			)

			th.SimulateJobSuccess(cell0DBSyncJobName)
			th.ExpectCondition(
				cell0ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionTrue,
			)
			th.SimulateStatefulSetReplicaReady(novaCell0ConductorStatefulSetName)
			th.ExpectCondition(
				cell0Name,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaConductorReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				novaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("create NovaAPI", func() {
			SimulateKeystoneServiceReady(novaKeystoneServiceName)
			SimulateMariaDBDatabaseCompleted(mariaDBDatabaseNameForAPI)
			SimulateMariaDBDatabaseCompleted(mariaDBDatabaseNameForCell0)
			SimulateTransportURLReady(apiTransportURLName)
			th.SimulateJobSuccess(cell0DBSyncJobName)
			th.SimulateStatefulSetReplicaReady(novaCell0ConductorStatefulSetName)
			th.SimulateStatefulSetReplicaReady(novaSchedulerStatefulSetName)

			api := GetNovaAPI(novaAPIName)
			Expect(api.Spec.APIMessageBusSecretName).To(Equal("rabbitmq-secret"))
			Expect(api.Spec.ServiceUser).To(Equal("nova"))

			th.SimulateStatefulSetReplicaReady(novaAPIdeploymentName)

			th.ExpectCondition(
				novaAPIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				novaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAPIReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				novaName,
				ConditionGetterFunc(NovaConditionGetter),
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
			SimulateKeystoneServiceReady(novaKeystoneServiceName)
			SimulateMariaDBDatabaseCompleted(mariaDBDatabaseNameForAPI)
			SimulateMariaDBDatabaseCompleted(mariaDBDatabaseNameForCell0)
			SimulateTransportURLReady(apiTransportURLName)
			GetNovaCell(cell0Name)
			GetNovaConductor(cell0ConductorName)

			th.SimulateJobFailure(cell0DBSyncJobName)
			th.ExpectCondition(
				cell0ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionFalse,
			)
			th.ExpectCondition(
				cell0Name,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaConductorReadyCondition,
				corev1.ConditionFalse,
			)
			th.ExpectCondition(
				novaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsReadyCondition,
				corev1.ConditionFalse,
			)

			NovaAPINotExists(novaAPIName)
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
			SimulateKeystoneServiceReady(novaKeystoneServiceName)
			SimulateMariaDBDatabaseCompleted(mariaDBDatabaseNameForAPI)
			SimulateMariaDBDatabaseCompleted(mariaDBDatabaseNameForCell0)
			SimulateTransportURLReady(apiTransportURLName)

			cell0DBSync := th.GetJob(cell0DBSyncJobName)
			cell0DBSyncJobEnv := cell0DBSync.Spec.Template.Spec.InitContainers[0].Env
			Expect(cell0DBSyncJobEnv).To(
				ContainElements(
					[]corev1.EnvVar{
						{Name: "CellDatabaseHost", Value: "hostname-for-db-for-cell0"},
						{Name: "APIDatabaseHost", Value: "hostname-for-db-for-api"},
					},
				),
			)

			th.SimulateJobSuccess(cell0DBSyncJobName)
			th.SimulateStatefulSetReplicaReady(novaCell0ConductorStatefulSetName)
			th.SimulateStatefulSetReplicaReady(novaSchedulerStatefulSetName)

			configDataMap := th.GetConfigMap(
				types.NamespacedName{
					Namespace: namespace,
					Name:      fmt.Sprintf("%s-config-data", novaAPIName.Name),
				},
			)
			Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
			Expect(configDataMap.Data["01-nova.conf"]).To(
				ContainSubstring("[database]\nconnection = mysql+pymysql://nova_cell0:12345678@hostname-for-db-for-cell0/nova_cell0"),
			)
			Expect(configDataMap.Data["01-nova.conf"]).To(
				ContainSubstring("[api_database]\nconnection = mysql+pymysql://nova_api:12345678@hostname-for-db-for-api/nova_api"),
			)

			th.SimulateStatefulSetReplicaReady(novaAPIdeploymentName)

			th.ExpectCondition(
				cell0ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				cell0Name,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaConductorReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				novaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				novaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAPIReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				novaName,
				ConditionGetterFunc(NovaConditionGetter),
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
			SimulateKeystoneServiceReady(novaKeystoneServiceName)
			th.ExpectCondition(
				novaName,
				ConditionGetterFunc(NovaConditionGetter),
				condition.KeystoneServiceReadyCondition,
				corev1.ConditionTrue,
			)

			service := GetKeystoneService(novaKeystoneServiceName)
			Expect(service.Finalizers).To(ContainElement("Nova"))

			DeleteNova(novaName)
			service = GetKeystoneService(novaKeystoneServiceName)
			Expect(service.Finalizers).NotTo(ContainElement("Nova"))
		})
	})
})
