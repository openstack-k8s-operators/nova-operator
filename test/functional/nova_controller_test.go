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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/nova-operator/controllers"
)

var _ = Describe("Nova controller", func() {
	When("Nova CR instance is created without cell0", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateNovaWithoutCell0(novaNames.NovaName))
		})

		It("is not Ready", func() {
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("has no hash and no services ready", func() {
			instance := GetNova(novaNames.NovaName)
			Expect(instance.Status.Hash).To(BeEmpty())
			Expect(instance.Status.APIServiceReadyCount).To(Equal(int32(0)))
			Expect(instance.Status.SchedulerServiceReadyCount).To(Equal(int32(0)))
			Expect(instance.Status.MetadataServiceReadyCount).To(Equal(int32(0)))
		})

		It("reports that cell0 is missing from the spec", func() {
			th.ExpectConditionWithDetails(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NovaCell creation failed for cell0(missing cell0 specification from Spec.CellTemplates)",
			)
		})
	})

	When("Nova CR instance is created without a proper secret", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, th.CreateSecret(
					types.NamespacedName{Namespace: novaNames.Namespace, Name: SecretName},
					map[string][]byte{
						"NovaPassword": []byte("service-password"),
					},
				))
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateNovaMessageBusSecret(novaNames.NovaName.Namespace, MessageBusSecretName),
			)
			DeferCleanup(
				th.DeleteDBService,
				th.CreateDBService(
					novaNames.NovaName.Namespace,
					"openstack",
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			DeferCleanup(th.DeleteKeystoneAPI, th.CreateKeystoneAPI(novaNames.NovaName.Namespace))

			DeferCleanup(th.DeleteInstance, CreateNovaWithCell0(novaNames.NovaName))
		})

		It("is not Ready", func() {
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("reports that the inputs are not ready", func() {
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionFalse,
			)
		})
	})

	When("Nova CR instance is created", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaSecret(novaNames.NovaName.Namespace, SecretName))
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateNovaMessageBusSecret(novaNames.NovaName.Namespace, MessageBusSecretName),
			)
			DeferCleanup(
				th.DeleteDBService,
				th.CreateDBService(
					novaNames.NovaName.Namespace,
					"openstack",
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			DeferCleanup(th.DeleteKeystoneAPI, th.CreateKeystoneAPI(novaNames.NovaName.Namespace))

			DeferCleanup(th.DeleteInstance, CreateNovaWithCell0(novaNames.NovaName))
		})

		It("creates service account, role and rolebindig", func() {
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				condition.ServiceAccountReadyCondition,
				corev1.ConditionTrue,
			)
			sa := th.GetServiceAccount(novaNames.ServiceAccountName)

			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				condition.RoleReadyCondition,
				corev1.ConditionTrue,
			)
			role := th.GetRole(novaNames.RoleName)
			Expect(role.Rules).To(HaveLen(2))
			Expect(role.Rules[0].Resources).To(Equal([]string{"securitycontextconstraints"}))
			Expect(role.Rules[1].Resources).To(Equal([]string{"pods"}))

			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				condition.RoleBindingReadyCondition,
				corev1.ConditionTrue,
			)
			binding := th.GetRoleBinding(novaNames.RoleBindingName)
			Expect(binding.RoleRef.Name).To(Equal(role.Name))
			Expect(binding.Subjects).To(HaveLen(1))
			Expect(binding.Subjects[0].Name).To(Equal(sa.Name))
		})

		It("initializes Status fields", func() {
			instance := GetNova(novaNames.NovaName)
			Expect(instance.Status.Hash).To(BeEmpty())
			Expect(instance.Status.APIServiceReadyCount).To(Equal(int32(0)))
			Expect(instance.Status.SchedulerServiceReadyCount).To(Equal(int32(0)))
			Expect(instance.Status.MetadataServiceReadyCount).To(Equal(int32(0)))
			Expect(instance.Status.RegisteredCells).To(BeEmpty())
		})

		It("registers nova service to keystone", func() {
			// assert that the KeystoneService for nova is created
			th.GetKeystoneService(novaNames.KeystoneServiceName)
			// and simulate that it becomes ready i.e. the keystone-operator
			// did its job and registered the nova service
			th.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
			keystone := th.GetKeystoneService(novaNames.KeystoneServiceName)
			Expect(keystone.Status.Conditions).ToNot(BeNil())

			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				condition.KeystoneServiceReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("creates nova_api DB", func() {
			th.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAPIDBReadyCondition,
				corev1.ConditionFalse,
			)
			th.GetMariaDBDatabase(novaNames.APIMariaDBDatabaseName)

			th.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAPIDBReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("creates nova-api MQ", func() {
			th.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAPIMQReadyCondition,
				corev1.ConditionFalse,
			)
			th.GetTransportURL(cell0.TransportURLName)

			th.SimulateTransportURLReady(cell0.TransportURLName)
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAPIMQReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("creates nova_cell0 DB", func() {
			th.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsDBReadyCondition,
				corev1.ConditionFalse,
			)
			th.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			th.GetMariaDBDatabase(cell0.MariaDBDatabaseName)
			th.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsDBReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("creates cell0 NovaCell", func() {
			th.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
			th.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			th.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			th.SimulateTransportURLReady(cell0.TransportURLName)
			// assert that cell related CRs are created
			cell := GetNovaCell(cell0.CellName)
			Expect(cell.Spec.CellMessageBusSecretName).To(Equal("rabbitmq-secret"))
			Expect(cell.Spec.ServiceUser).To(Equal("nova"))
			Expect(cell.Spec.ServiceAccount).To(Equal(novaNames.ServiceAccountName.Name))

			conductor := GetNovaConductor(cell0.CellConductorName)
			Expect(conductor.Spec.CellMessageBusSecretName).To(Equal("rabbitmq-secret"))
			Expect(conductor.Spec.ServiceUser).To(Equal("nova"))
			Expect(conductor.Spec.ServiceAccount).To(Equal(novaNames.ServiceAccountName.Name))

			// assert that a cell specific internal secret is created with the
			// proper content and the cell subCRs are configured to use the
			// internal secret
			internalCellSecret := th.GetSecret(cell0.InternalCellSecretName)
			Expect(internalCellSecret.Data).To(HaveLen(3))
			Expect(internalCellSecret.Data).To(
				HaveKeyWithValue(controllers.APIDatabasePasswordSelector, []byte("api-database-password")))
			Expect(internalCellSecret.Data).To(
				HaveKeyWithValue(controllers.CellDatabasePasswordSelector, []byte("cell0-database-password")))
			Expect(internalCellSecret.Data).To(
				HaveKeyWithValue(controllers.ServicePasswordSelector, []byte("service-password")))

			Expect(cell.Spec.Secret).To(Equal(cell0.InternalCellSecretName.Name))
			Expect(conductor.Spec.Secret).To(Equal(cell0.InternalCellSecretName.Name))

			th.ExpectCondition(
				cell0.CellConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionFalse,
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

			th.SimulateJobSuccess(cell0.CellMappingJobName)

			// TODO(bogdando): move to CellNames.MappingJob*
			mappingJobConfig := th.GetSecret(
				types.NamespacedName{
					Namespace: cell0.CellName.Namespace,
					Name:      fmt.Sprintf("%s-config-data", cell0.CellName.Name+"-manage"),
				},
			)
			Expect(mappingJobConfig.Data).Should(HaveKey("01-nova.conf"))
			configData := string(mappingJobConfig.Data["01-nova.conf"])
			Expect(configData).To(
				ContainSubstring("[database]\nconnection = mysql+pymysql://nova_cell0:cell0-database-password@hostname-for-openstack/nova_cell0"),
			)
			Expect(configData).To(
				ContainSubstring("[api_database]\nconnection = mysql+pymysql://nova_api:api-database-password@hostname-for-openstack/nova_api"),
			)
			// NOTE(gibi): cell mapping for cell0 should not have transport_url
			// configured. As the nova-manage command used to create the mapping
			// uses the transport_url from the nova.conf provided to the job
			// we need to make sure that it is empty.
			Expect(configData).NotTo(ContainSubstring("transport_url"))

			mappingJobScript := th.GetSecret(
				types.NamespacedName{
					Namespace: cell0.CellName.Namespace,
					Name:      fmt.Sprintf("%s-scripts", cell0.CellName.Name+"-manage"),
				},
			)
			Expect(mappingJobScript.Data).Should(HaveKey("ensure_cell_mapping.sh"))
			scriptData := string(mappingJobScript.Data["ensure_cell_mapping.sh"])
			Expect(scriptData).To(ContainSubstring("nova-manage cell_v2 update_cell"))
			Expect(scriptData).To(ContainSubstring("nova-manage cell_v2 map_cell0"))

			Eventually(func(g Gomega) {
				nova := GetNova(novaNames.NovaName)
				g.Expect(nova.Status.RegisteredCells).To(
					HaveKeyWithValue(cell0.CellName.Name, Not(BeEmpty())))
			}, timeout, interval).Should(Succeed())

			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("creates an internal Secret for the top level services", func() {
			th.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
			th.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			th.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			th.SimulateTransportURLReady(cell0.TransportURLName)
			th.SimulateJobSuccess(cell0.CellDBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
			th.SimulateJobSuccess(cell0.CellMappingJobName)
			th.SimulateStatefulSetReplicaReady(novaNames.SchedulerStatefulSetName)
			th.SimulateStatefulSetReplicaReady(novaNames.MetadataStatefulSetName)

			// assert that a the top level internal internal secret is created
			// with the proper data
			internalTopLevelSecret := th.GetSecret(novaNames.InternalTopLevelSecretName)
			Expect(internalTopLevelSecret.Data).To(HaveLen(4))
			Expect(internalTopLevelSecret.Data).To(
				HaveKeyWithValue(controllers.APIDatabasePasswordSelector, []byte("api-database-password")))
			Expect(internalTopLevelSecret.Data).To(
				HaveKeyWithValue(controllers.CellDatabasePasswordSelector, []byte("cell0-database-password")))
			Expect(internalTopLevelSecret.Data).To(
				HaveKeyWithValue(controllers.ServicePasswordSelector, []byte("service-password")))
			Expect(internalTopLevelSecret.Data).To(
				HaveKeyWithValue(controllers.MetadataSecretSelector, []byte("metadata-secret")))
		})

		It("creates NovaAPI", func() {
			th.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
			th.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			th.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			th.SimulateTransportURLReady(cell0.TransportURLName)
			th.SimulateJobSuccess(cell0.CellDBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
			th.SimulateJobSuccess(cell0.CellMappingJobName)
			th.SimulateStatefulSetReplicaReady(novaNames.SchedulerStatefulSetName)
			th.SimulateStatefulSetReplicaReady(novaNames.MetadataStatefulSetName)

			api := GetNovaAPI(novaNames.APIName)
			Expect(api.Spec.APIMessageBusSecretName).To(Equal("rabbitmq-secret"))
			Expect(api.Spec.ServiceUser).To(Equal("nova"))
			Expect(api.Spec.ServiceAccount).To(Equal(novaNames.ServiceAccountName.Name))
			Expect(api.Spec.Secret).To(Equal(novaNames.InternalTopLevelSecretName.Name))

			th.SimulateStatefulSetReplicaReady(novaNames.APIDeploymentName)
			th.SimulateKeystoneEndpointReady(novaNames.APIKeystoneEndpointName)
			th.ExpectCondition(
				novaNames.APIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAPIReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("creates NovaScheduler", func() {
			th.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
			th.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			th.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			th.SimulateTransportURLReady(cell0.TransportURLName)
			th.SimulateJobSuccess(cell0.CellDBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
			th.SimulateJobSuccess(cell0.CellMappingJobName)
			th.SimulateStatefulSetReplicaReady(novaNames.APIDeploymentName)
			th.SimulateKeystoneEndpointReady(novaNames.APIKeystoneEndpointName)
			th.SimulateStatefulSetReplicaReady(novaNames.MetadataStatefulSetName)

			scheduler := GetNovaScheduler(novaNames.SchedulerName)
			Expect(scheduler.Spec.APIMessageBusSecretName).To(Equal("rabbitmq-secret"))
			Expect(scheduler.Spec.ServiceAccount).To(Equal(novaNames.ServiceAccountName.Name))
			Expect(scheduler.Spec.Secret).To(Equal(novaNames.InternalTopLevelSecretName.Name))

			th.SimulateStatefulSetReplicaReady(novaNames.SchedulerStatefulSetName)

			th.ExpectCondition(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaSchedulerReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("creates NovaMetadata", func() {
			th.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
			th.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			th.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			th.SimulateTransportURLReady(cell0.TransportURLName)
			th.SimulateJobSuccess(cell0.CellDBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
			th.SimulateJobSuccess(cell0.CellMappingJobName)
			th.SimulateStatefulSetReplicaReady(novaNames.APIDeploymentName)
			th.SimulateKeystoneEndpointReady(novaNames.APIKeystoneEndpointName)
			th.SimulateStatefulSetReplicaReady(novaNames.SchedulerStatefulSetName)

			metadata := GetNovaMetadata(novaNames.MetadataName)
			Expect(metadata.Spec.APIMessageBusSecretName).To(Equal("rabbitmq-secret"))
			Expect(metadata.Spec.ServiceAccount).To(Equal(novaNames.ServiceAccountName.Name))
			Expect(metadata.Spec.Secret).To(Equal(novaNames.InternalTopLevelSecretName.Name))

			th.SimulateStatefulSetReplicaReady(novaNames.MetadataStatefulSetName)

			th.ExpectCondition(
				novaNames.MetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaMetadataReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})

	When("Nova CR instance is created but cell0 DB sync fails", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaSecret(novaNames.NovaName.Namespace, SecretName))
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateNovaMessageBusSecret(novaNames.NovaName.Namespace, MessageBusSecretName),
			)
			DeferCleanup(
				th.DeleteDBService,
				th.CreateDBService(
					novaNames.NovaName.Namespace,
					"openstack",
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			DeferCleanup(th.DeleteKeystoneAPI, th.CreateKeystoneAPI(novaNames.NovaName.Namespace))

			DeferCleanup(th.DeleteInstance, CreateNovaWithCell0(novaNames.NovaName))

			th.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
			th.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			th.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			th.SimulateTransportURLReady(cell0.TransportURLName)
			GetNovaCell(cell0.CellName)
			GetNovaConductor(cell0.CellConductorName)

			th.SimulateJobFailure(cell0.CellDBSyncJobName)
		})

		It("does not set the cell db sync ready condtion to true", func() {
			th.ExpectCondition(
				cell0.CellConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("does not set the cell0 ready condition to true", func() {
			th.ExpectCondition(
				cell0.CellName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaConductorReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("does not set the all cell ready condition", func() {
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("does not create NovaAPI", func() {
			NovaAPINotExists(novaNames.APIName)
		})

		It("does not create NovaScheduler", func() {
			NovaSchedulerNotExists(novaNames.SchedulerName)
		})
	})

	When("Nova CR instance is created but cell0 cell registration fails", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaSecret(novaNames.NovaName.Namespace, SecretName))
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateNovaMessageBusSecret(novaNames.NovaName.Namespace, MessageBusSecretName),
			)
			DeferCleanup(
				th.DeleteDBService,
				th.CreateDBService(
					novaNames.NovaName.Namespace,
					"openstack",
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			DeferCleanup(th.DeleteKeystoneAPI, th.CreateKeystoneAPI(novaNames.NovaName.Namespace))

			DeferCleanup(th.DeleteInstance, CreateNovaWithCell0(novaNames.NovaName))

			th.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
			th.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			th.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			th.SimulateTransportURLReady(cell0.TransportURLName)
			th.SimulateJobSuccess(cell0.CellDBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)

			th.SimulateJobFailure(cell0.CellMappingJobName)
		})

		It("does not set the all cell ready condition", func() {
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("still creates the top level services", func() {
			GetNovaAPI(novaNames.APIName)
			GetNovaScheduler(novaNames.SchedulerName)
			GetNovaMetadata(novaNames.MetadataName)
		})

	})

	When("Nova CR instance with different DB Services for nova_api and cell0 DBs", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaSecret(novaNames.NovaName.Namespace, SecretName))
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateNovaMessageBusSecret(novaNames.NovaName.Namespace, MessageBusSecretName),
			)

			DeferCleanup(
				th.DeleteDBService,
				th.CreateDBService(
					cell0.MariaDBDatabaseName.Namespace,
					cell0.MariaDBDatabaseName.Name,
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			DeferCleanup(
				th.DeleteDBService,
				th.CreateDBService(
					novaNames.APIMariaDBDatabaseName.Namespace,
					novaNames.APIMariaDBDatabaseName.Name,
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			DeferCleanup(th.DeleteKeystoneAPI, th.CreateKeystoneAPI(novaNames.NovaName.Namespace))

			spec := GetDefaultNovaSpec()
			cell0template := GetDefaultNovaCellTemplate()
			cell0template["cellDatabaseInstance"] = cell0.MariaDBDatabaseName.Name
			spec["cellTemplates"] = map[string]interface{}{"cell0": cell0template}
			spec["apiDatabaseInstance"] = novaNames.APIMariaDBDatabaseName.Name

			DeferCleanup(th.DeleteInstance, CreateNova(novaNames.NovaName, spec))
		})

		It("uses the correct hostnames to access the different DB services", func() {
			th.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
			th.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			th.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			th.SimulateTransportURLReady(cell0.TransportURLName)

			cell0DBSync := th.GetJob(cell0.CellDBSyncJobName)
			Expect(len(cell0DBSync.Spec.Template.Spec.InitContainers)).To(Equal(0))
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
			Expect(configData).To(ContainSubstring("password = service-password"))

			th.SimulateJobSuccess(cell0.CellDBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
			th.SimulateJobSuccess(cell0.CellMappingJobName)
			th.SimulateStatefulSetReplicaReady(novaNames.SchedulerStatefulSetName)
			th.SimulateStatefulSetReplicaReady(novaNames.MetadataStatefulSetName)

			configDataMap = th.GetSecret(novaNames.APIConfigDataName)
			Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
			configData = string(configDataMap.Data["01-nova.conf"])
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
			Expect(configData).To(ContainSubstring("password = service-password"))

			configDataMap = th.GetSecret(novaNames.SchedulerConfigDataName)
			Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
			configData = string(configDataMap.Data["01-nova.conf"])
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
			Expect(configData).To(ContainSubstring("password = service-password"))

			th.SimulateStatefulSetReplicaReady(novaNames.APIName)
			th.SimulateKeystoneEndpointReady(novaNames.APIKeystoneEndpointName)

			th.ExpectCondition(
				cell0.CellConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionTrue,
			)
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
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAPIReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaSchedulerReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})

	When("Nova CR instance is deleted", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaSecret(novaNames.NovaName.Namespace, SecretName))
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateNovaMessageBusSecret(novaNames.NovaName.Namespace, MessageBusSecretName),
			)
			DeferCleanup(
				th.DeleteDBService,
				th.CreateDBService(
					novaNames.NovaName.Namespace,
					"openstack",
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			DeferCleanup(th.DeleteKeystoneAPI, th.CreateKeystoneAPI(novaNames.NovaName.Namespace))

			DeferCleanup(th.DeleteInstance, CreateNovaWithCell0(novaNames.NovaName))
		})

		It("removes the finalizer from KeystoneService", func() {
			th.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				condition.KeystoneServiceReadyCondition,
				corev1.ConditionTrue,
			)

			service := th.GetKeystoneService(novaNames.KeystoneServiceName)
			Expect(service.Finalizers).To(ContainElement("Nova"))

			th.DeleteInstance(GetNova(novaNames.NovaName))
			service = th.GetKeystoneService(novaNames.KeystoneServiceName)
			Expect(service.Finalizers).NotTo(ContainElement("Nova"))
		})

		It("removes the finalizers from the nova dbs", func() {
			th.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)

			apiDB := th.GetMariaDBDatabase(novaNames.APIMariaDBDatabaseName)
			Expect(apiDB.Finalizers).To(ContainElement("Nova"))
			cell0DB := th.GetMariaDBDatabase(cell0.MariaDBDatabaseName)
			Expect(cell0DB.Finalizers).To(ContainElement("Nova"))

			th.DeleteInstance(GetNova(novaNames.NovaName))

			apiDB = th.GetMariaDBDatabase(novaNames.APIMariaDBDatabaseName)
			Expect(apiDB.Finalizers).NotTo(ContainElement("Nova"))
			cell0DB = th.GetMariaDBDatabase(cell0.MariaDBDatabaseName)
			Expect(cell0DB.Finalizers).NotTo(ContainElement("Nova"))
		})
	})

	When("Nova CR instance is created with NetworkAttachment and ExternalEndpoints", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaSecret(novaNames.NovaName.Namespace, SecretName))
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateNovaMessageBusSecret(novaNames.NovaName.Namespace, MessageBusSecretName),
			)
			DeferCleanup(
				th.DeleteDBService,
				th.CreateDBService(
					novaNames.NovaName.Namespace,
					"openstack",
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			DeferCleanup(th.DeleteKeystoneAPI, th.CreateKeystoneAPI(novaNames.NovaName.Namespace))

			nad := th.CreateNetworkAttachmentDefinition(novaNames.InternalAPINetworkNADName)
			DeferCleanup(th.DeleteInstance, nad)

			var externalEndpoints []interface{}
			externalEndpoints = append(
				externalEndpoints, map[string]interface{}{
					"endpoint":        "internal",
					"ipAddressPool":   "osp-internalapi",
					"loadBalancerIPs": []string{"10.1.0.1", "10.1.0.2"},
				},
			)
			rawSpec := map[string]interface{}{
				"secret": SecretName,
				"cellTemplates": map[string]interface{}{
					"cell0": map[string]interface{}{
						"cellDatabaseUser": "nova_cell0",
						"hasAPIAccess":     true,
						"conductorServiceTemplate": map[string]interface{}{
							"networkAttachments": []string{"internalapi"},
						},
					},
				},
				"apiServiceTemplate": map[string]interface{}{
					"networkAttachments": []string{"internalapi"},
					"externalEndpoints":  externalEndpoints,
				},
				"schedulerServiceTemplate": map[string]interface{}{
					"networkAttachments": []string{"internalapi"},
				},
				"metadataServiceTemplate": map[string]interface{}{
					"networkAttachments": []string{"internalapi"},
				},
			}
			DeferCleanup(th.DeleteInstance, CreateNova(novaNames.NovaName, rawSpec))

			th.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
			th.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			th.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			th.SimulateTransportURLReady(cell0.TransportURLName)
			th.SimulateJobSuccess(cell0.CellDBSyncJobName)
		})

		It("creates all the sub CRs and passes down the network parameters", func() {
			th.SimulateStatefulSetReplicaReadyWithPods(
				cell0.ConductorStatefulSetName,
				map[string][]string{novaNames.NovaName.Namespace + "/internalapi": {"10.0.0.1"}},
			)
			th.SimulateJobSuccess(cell0.CellMappingJobName)

			th.SimulateStatefulSetReplicaReadyWithPods(
				novaNames.SchedulerStatefulSetName,
				map[string][]string{novaNames.NovaName.Namespace + "/internalapi": {"10.0.0.1"}},
			)
			th.SimulateStatefulSetReplicaReadyWithPods(
				novaNames.APIDeploymentName,
				map[string][]string{novaNames.NovaName.Namespace + "/internalapi": {"10.0.0.1"}},
			)
			th.SimulateStatefulSetReplicaReadyWithPods(
				novaNames.MetadataStatefulSetName,
				map[string][]string{novaNames.NovaName.Namespace + "/internalapi": {"10.0.0.1"}},
			)
			th.SimulateKeystoneEndpointReady(novaNames.APIKeystoneEndpointName)

			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)

			nova := GetNova(novaNames.NovaName)

			conductor := GetNovaConductor(cell0.CellConductorName)
			Expect(conductor.Spec.NetworkAttachments).To(
				Equal(nova.Spec.CellTemplates["cell0"].ConductorServiceTemplate.NetworkAttachments))

			api := GetNovaAPI(novaNames.APIName)
			Expect(api.Spec.NetworkAttachments).To(Equal(nova.Spec.APIServiceTemplate.NetworkAttachments))
			Expect(api.Spec.Override).To(Equal(nova.Spec.APIServiceTemplate.Override))

			scheduler := GetNovaScheduler(novaNames.SchedulerName)
			Expect(scheduler.Spec.NetworkAttachments).To(Equal(nova.Spec.APIServiceTemplate.NetworkAttachments))
		})

	})

	When("Nova CR is created without container images defined", func() {
		BeforeEach(func() {
			spec := GetDefaultNovaSpec()
			cell0 := GetDefaultNovaCellTemplate()
			spec["cellTemplates"] = map[string]interface{}{"cell0": cell0}
			// This nova is created without any container image is specified in
			// the request
			DeferCleanup(th.DeleteInstance, CreateNova(novaNames.NovaName, spec))
		})
		It("has the expected container image defaults", func() {
			novaDefault := GetNova(novaNames.NovaName)

			Expect(novaDefault.Spec.APIServiceTemplate.ContainerImage).To(Equal(util.GetEnvVar("RELATED_IMAGE_NOVA_API_IMAGE_URL_DEFAULT", novav1.NovaAPIContainerImage)))
			Expect(novaDefault.Spec.MetadataServiceTemplate.ContainerImage).To(Equal(util.GetEnvVar("RELATED_IMAGE_NOVA_METADATA_IMAGE_URL_DEFAULT", novav1.NovaMetadataContainerImage)))
			Expect(novaDefault.Spec.SchedulerServiceTemplate.ContainerImage).To(Equal(util.GetEnvVar("RELATED_IMAGE_NOVA_SCHEDULER_IMAGE_URL_DEFAULT", novav1.NovaSchedulerContainerImage)))

			for _, cell := range novaDefault.Spec.CellTemplates {
				Expect(cell.ConductorServiceTemplate.ContainerImage).To(Equal(util.GetEnvVar("RELATED_IMAGE_NOVA_CONDUCTOR_IMAGE_URL_DEFAULT", novav1.NovaConductorContainerImage)))
				Expect(cell.MetadataServiceTemplate.ContainerImage).To(Equal(util.GetEnvVar("RELATED_IMAGE_NOVA_METADATA_IMAGE_URL_DEFAULT", novav1.NovaMetadataContainerImage)))
				Expect(cell.NoVNCProxyServiceTemplate.ContainerImage).To(Equal(util.GetEnvVar("RELATED_IMAGE_NOVA_NOVNC_IMAGE_URL_DEFAULT", novav1.NovaNoVNCContainerImage)))
			}
		})
	})
})
