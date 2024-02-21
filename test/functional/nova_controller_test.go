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
	memcachedv1 "github.com/openstack-k8s-operators/infra-operator/apis/memcached/v1beta1"
	. "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/nova-operator/controllers"
)

var _ = Describe("Nova controller", func() {
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
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					novaNames.NovaName.Namespace,
					"openstack",
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(novaNames.NovaName.Namespace))

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
				k8sClient.Delete, ctx, CreateNovaMessageBusSecret(cell0))
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					novaNames.NovaName.Namespace,
					"openstack",
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			memcachedSpec := memcachedv1.MemcachedSpec{
				Replicas: ptr.To(int32(3)),
			}

			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(novaNames.NovaName.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMemcachedReady(novaNames.MemcachedNamespace)

			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(novaNames.NovaName.Namespace))

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
			Expect(instance.Status.APIServiceReadyCount).To(Equal(int32(0)))
			Expect(instance.Status.SchedulerServiceReadyCount).To(Equal(int32(0)))
			Expect(instance.Status.MetadataServiceReadyCount).To(Equal(int32(0)))
			Expect(instance.Status.RegisteredCells).To(BeEmpty())
		})

		It("defaults Spec fields", func() {
			nova := GetNova(novaNames.NovaName)
			cell0Template := nova.Spec.CellTemplates["cell0"]
			Expect(cell0Template.DBPurge.Schedule).To(Equal(ptr.To("1 0 * * *")))
			Expect(cell0Template.DBPurge.ArchiveAge).To(Equal(ptr.To(30)))
			Expect(cell0Template.DBPurge.PurgeAge).To(Equal(ptr.To(90)))
		})

		It("registers nova service to keystone", func() {
			// assert that the KeystoneService for nova is created
			keystone.GetKeystoneService(novaNames.KeystoneServiceName)
			// and simulate that it becomes ready i.e. the keystone-operator
			// did its job and registered the nova service
			keystone.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
			keystone := keystone.GetKeystoneService(novaNames.KeystoneServiceName)
			Expect(keystone.Status.Conditions).ToNot(BeNil())

			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				condition.KeystoneServiceReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("creates nova_api DB", func() {
			keystone.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAPIDBReadyCondition,
				corev1.ConditionFalse,
			)
			mariadb.GetMariaDBDatabase(novaNames.APIMariaDBDatabaseName)

			mariadb.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(novaNames.APIMariaDBDatabaseName)
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAPIDBReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("creates nova-api MQ", func() {
			keystone.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAPIMQReadyCondition,
				corev1.ConditionFalse,
			)
			infra.GetTransportURL(cell0.TransportURLName)

			infra.SimulateTransportURLReady(cell0.TransportURLName)
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAPIMQReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("creates nova_cell0 DB", func() {
			keystone.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsDBReadyCondition,
				corev1.ConditionFalse,
			)
			mariadb.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(novaNames.APIMariaDBDatabaseName)
			mariadb.GetMariaDBDatabase(cell0.MariaDBDatabaseName)
			mariadb.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell0.MariaDBDatabaseName)
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsDBReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("creates cell0 NovaCell", func() {
			keystone.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
			mariadb.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(novaNames.APIMariaDBDatabaseName)
			mariadb.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell0.MariaDBDatabaseName)
			infra.SimulateTransportURLReady(cell0.TransportURLName)
			// assert that cell related CRs are created
			cell := GetNovaCell(cell0.CellCRName)
			Expect(cell.Spec.ServiceUser).To(Equal("nova"))
			Expect(cell.Spec.ServiceAccount).To(Equal(novaNames.ServiceAccountName.Name))
			Expect(cell.Spec.DBPurge.Schedule).To(Equal(ptr.To("1 0 * * *")))
			Expect(cell.Spec.DBPurge.ArchiveAge).To(Equal(ptr.To(30)))
			Expect(cell.Spec.DBPurge.PurgeAge).To(Equal(ptr.To(90)))

			conductor := GetNovaConductor(cell0.ConductorName)
			Expect(conductor.Spec.ServiceUser).To(Equal("nova"))
			Expect(conductor.Spec.ServiceAccount).To(Equal(novaNames.ServiceAccountName.Name))
			Expect(conductor.Spec.DBPurge.Schedule).To(Equal(ptr.To("1 0 * * *")))
			Expect(conductor.Spec.DBPurge.ArchiveAge).To(Equal(ptr.To(30)))
			Expect(conductor.Spec.DBPurge.PurgeAge).To(Equal(ptr.To(90)))

			// assert that a cell specific internal secret is created with the
			// proper content and the cell subCRs are configured to use the
			// internal secret
			internalCellSecret := th.GetSecret(cell0.InternalCellSecretName)
			Expect(internalCellSecret.Data).To(HaveLen(4))
			Expect(internalCellSecret.Data).To(
				HaveKeyWithValue(controllers.APIDatabasePasswordSelector, []byte("api-database-password")))
			Expect(internalCellSecret.Data).To(
				HaveKeyWithValue(controllers.CellDatabasePasswordSelector, []byte("cell0-database-password")))
			Expect(internalCellSecret.Data).To(
				HaveKeyWithValue(controllers.ServicePasswordSelector, []byte("service-password")))
			Expect(internalCellSecret.Data).To(
				HaveKeyWithValue("transport_url", []byte("rabbit://cell0/fake")))

			Expect(cell.Spec.Secret).To(Equal(cell0.InternalCellSecretName.Name))
			Expect(conductor.Spec.Secret).To(Equal(cell0.InternalCellSecretName.Name))

			th.ExpectCondition(
				cell0.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionFalse,
			)

			th.SimulateJobSuccess(cell0.DBSyncJobName)
			th.ExpectCondition(
				cell0.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionTrue,
			)
			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
			th.ExpectCondition(
				cell0.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaConductorReadyCondition,
				corev1.ConditionTrue,
			)

			mappingJob := th.GetJob(cell0.CellMappingJobName)
			Expect(mappingJob.Spec.Template.Spec.ServiceAccountName).To(
				Equal(novaNames.ServiceAccountName.Name))

			th.SimulateJobSuccess(cell0.CellMappingJobName)

			// TODO(bogdando): move to CellNames.MappingJob*
			mappingJobConfig := th.GetSecret(
				types.NamespacedName{
					Namespace: cell0.CellCRName.Namespace,
					Name:      fmt.Sprintf("%s-config-data", cell0.CellCRName.Name+"-manage"),
				},
			)
			Expect(mappingJobConfig.Data).Should(HaveKey("01-nova.conf"))
			configData := string(mappingJobConfig.Data["01-nova.conf"])
			Expect(configData).To(
				ContainSubstring(fmt.Sprintf("[database]\nconnection = mysql+pymysql://nova_cell0:cell0-database-password@hostname-for-openstack.%s.svc/nova_cell0?read_default_file=/etc/my.cnf", novaNames.Namespace)),
			)
			Expect(configData).To(
				ContainSubstring(fmt.Sprintf("[api_database]\nconnection = mysql+pymysql://nova_api:api-database-password@hostname-for-openstack.%s.svc/nova_api?read_default_file=/etc/my.cnf", novaNames.Namespace)),
			)
			// NOTE(gibi): cell mapping for cell0 should not have transport_url
			// configured. As the nova-manage command used to create the mapping
			// uses the transport_url from the nova.conf provided to the job
			// we need to make sure that it is empty.
			Expect(configData).NotTo(ContainSubstring("transport_url"))

			myCnf := mappingJobConfig.Data["my.cnf"]
			Expect(myCnf).To(
				ContainSubstring("[client]\nssl=0"))

			mappingJobScript := th.GetSecret(
				types.NamespacedName{
					Namespace: cell0.CellCRName.Namespace,
					Name:      fmt.Sprintf("%s-scripts", cell0.CellCRName.Name+"-manage"),
				},
			)
			Expect(mappingJobScript.Data).Should(HaveKey("ensure_cell_mapping.sh"))
			scriptData := string(mappingJobScript.Data["ensure_cell_mapping.sh"])
			Expect(scriptData).To(ContainSubstring("nova-manage cell_v2 update_cell"))
			Expect(scriptData).To(ContainSubstring("nova-manage cell_v2 map_cell0"))

			Eventually(func(g Gomega) {
				nova := GetNova(novaNames.NovaName)
				g.Expect(nova.Status.RegisteredCells).To(
					HaveKeyWithValue(cell0.CellCRName.Name, Not(BeEmpty())))
			}, timeout, interval).Should(Succeed())

			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("creates an internal Secret for the top level services", func() {
			keystone.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
			mariadb.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(novaNames.APIMariaDBDatabaseName)
			mariadb.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell0.MariaDBDatabaseName)
			infra.SimulateTransportURLReady(cell0.TransportURLName)
			th.SimulateJobSuccess(cell0.DBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
			th.SimulateJobSuccess(cell0.CellMappingJobName)
			th.SimulateStatefulSetReplicaReady(novaNames.SchedulerStatefulSetName)
			th.SimulateStatefulSetReplicaReady(novaNames.MetadataStatefulSetName)

			// assert that a the top level internal internal secret is created
			// with the proper data
			internalTopLevelSecret := th.GetSecret(novaNames.InternalTopLevelSecretName)
			Expect(internalTopLevelSecret.Data).To(HaveLen(5))
			Expect(internalTopLevelSecret.Data).To(
				HaveKeyWithValue(controllers.APIDatabasePasswordSelector, []byte("api-database-password")))
			Expect(internalTopLevelSecret.Data).To(
				HaveKeyWithValue(controllers.CellDatabasePasswordSelector, []byte("cell0-database-password")))
			Expect(internalTopLevelSecret.Data).To(
				HaveKeyWithValue(controllers.ServicePasswordSelector, []byte("service-password")))
			Expect(internalTopLevelSecret.Data).To(
				HaveKeyWithValue(controllers.MetadataSecretSelector, []byte("metadata-secret")))
			Expect(internalTopLevelSecret.Data).To(
				HaveKeyWithValue("transport_url", []byte("rabbit://cell0/fake")))
		})

		It("creates NovaAPI", func() {
			keystone.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
			mariadb.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(novaNames.APIMariaDBDatabaseName)
			mariadb.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell0.MariaDBDatabaseName)
			infra.SimulateTransportURLReady(cell0.TransportURLName)
			th.SimulateJobSuccess(cell0.DBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
			th.SimulateJobSuccess(cell0.CellMappingJobName)
			th.SimulateStatefulSetReplicaReady(novaNames.SchedulerStatefulSetName)
			th.SimulateStatefulSetReplicaReady(novaNames.MetadataStatefulSetName)

			api := GetNovaAPI(novaNames.APIName)
			Expect(api.Spec.ServiceUser).To(Equal("nova"))
			Expect(api.Spec.ServiceAccount).To(Equal(novaNames.ServiceAccountName.Name))
			Expect(api.Spec.Secret).To(Equal(novaNames.InternalTopLevelSecretName.Name))

			th.SimulateStatefulSetReplicaReady(novaNames.APIDeploymentName)
			keystone.SimulateKeystoneEndpointReady(novaNames.APIKeystoneEndpointName)
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
			Expect(GetNova(novaNames.NovaName).Status.APIServiceReadyCount).To(Equal(int32(1)))
		})

		It("creates NovaScheduler", func() {
			keystone.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
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
			th.SimulateStatefulSetReplicaReady(novaNames.MetadataStatefulSetName)

			scheduler := GetNovaScheduler(novaNames.SchedulerName)
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
			Expect(GetNova(novaNames.NovaName).Status.SchedulerServiceReadyCount).To(Equal(int32(1)))
		})

		It("creates NovaMetadata", func() {
			keystone.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
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

			metadata := GetNovaMetadata(novaNames.MetadataName)
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
			Expect(GetNova(novaNames.NovaName).Status.MetadataServiceReadyCount).To(Equal(int32(1)))
		})
	})

	When("Nova CR instance is created but cell0 DB sync fails", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaSecret(novaNames.NovaName.Namespace, SecretName))
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaMessageBusSecret(cell0))
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					novaNames.NovaName.Namespace,
					"openstack",
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)

			memcachedSpec := memcachedv1.MemcachedSpec{
				Replicas: ptr.To(int32(3)),
			}

			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(novaNames.NovaName.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMemcachedReady(novaNames.MemcachedNamespace)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(novaNames.NovaName.Namespace))

			DeferCleanup(th.DeleteInstance, CreateNovaWithCell0(novaNames.NovaName))

			keystone.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
			mariadb.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(novaNames.APIMariaDBDatabaseName)
			mariadb.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell0.MariaDBDatabaseName)
			infra.SimulateTransportURLReady(cell0.TransportURLName)
			GetNovaCell(cell0.CellCRName)
			GetNovaConductor(cell0.ConductorName)

			th.SimulateJobFailure(cell0.DBSyncJobName)
		})

		It("does not set the cell db sync ready condtion to true", func() {
			th.ExpectCondition(
				cell0.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("does not set the cell0 ready condition to true", func() {
			th.ExpectCondition(
				cell0.CellCRName,
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
				k8sClient.Delete, ctx, CreateNovaMessageBusSecret(cell0))
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					novaNames.NovaName.Namespace,
					"openstack",
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			memcachedSpec := memcachedv1.MemcachedSpec{
				Replicas: ptr.To(int32(3)),
			}

			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(novaNames.NovaName.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMemcachedReady(novaNames.MemcachedNamespace)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(novaNames.NovaName.Namespace))

			DeferCleanup(th.DeleteInstance, CreateNovaWithCell0(novaNames.NovaName))

			keystone.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
			mariadb.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(novaNames.APIMariaDBDatabaseName)
			mariadb.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell0.MariaDBDatabaseName)
			infra.SimulateTransportURLReady(cell0.TransportURLName)
			th.SimulateJobSuccess(cell0.DBSyncJobName)
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
				k8sClient.Delete, ctx, CreateNovaMessageBusSecret(cell0))

			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					cell0.MariaDBDatabaseName.Namespace,
					cell0.MariaDBDatabaseName.Name,
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					novaNames.APIMariaDBDatabaseName.Namespace,
					novaNames.APIMariaDBDatabaseName.Name,
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			memcachedSpec := memcachedv1.MemcachedSpec{
				Replicas: ptr.To(int32(3)),
			}

			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(novaNames.NovaName.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMemcachedReady(novaNames.MemcachedNamespace)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(novaNames.NovaName.Namespace))

			spec := GetDefaultNovaSpec()
			cell0template := GetDefaultNovaCellTemplate()
			cell0template["cellDatabaseInstance"] = cell0.MariaDBDatabaseName.Name
			spec["cellTemplates"] = map[string]interface{}{"cell0": cell0template}
			spec["apiDatabaseInstance"] = novaNames.APIMariaDBDatabaseName.Name

			DeferCleanup(th.DeleteInstance, CreateNova(novaNames.NovaName, spec))
		})

		It("uses the correct hostnames to access the different DB services", func() {
			keystone.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
			mariadb.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(novaNames.APIMariaDBDatabaseName)
			mariadb.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell0.MariaDBDatabaseName)
			infra.SimulateTransportURLReady(cell0.TransportURLName)

			cell0DBSync := th.GetJob(cell0.DBSyncJobName)
			Expect(len(cell0DBSync.Spec.Template.Spec.InitContainers)).To(Equal(0))
			configDataMap := th.GetSecret(cell0.ConductorConfigDataName)
			Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
			configData := string(configDataMap.Data["01-nova.conf"])
			Expect(configData).To(
				ContainSubstring(
					fmt.Sprintf(
						"[database]\nconnection = mysql+pymysql://nova_cell0:cell0-database-password@hostname-for-%s.%s.svc/nova_cell0?read_default_file=/etc/my.cnf",
						cell0.MariaDBDatabaseName.Name, novaNames.Namespace)),
			)
			Expect(configData).To(
				ContainSubstring(
					fmt.Sprintf(
						"[api_database]\nconnection = mysql+pymysql://nova_api:api-database-password@hostname-for-%s.%s.svc/nova_api?read_default_file=/etc/my.cnf",
						novaNames.APIMariaDBDatabaseName.Name, novaNames.Namespace)),
			)
			Expect(configData).To(ContainSubstring("password = service-password"))

			th.SimulateJobSuccess(cell0.DBSyncJobName)
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
						"[database]\nconnection = mysql+pymysql://nova_cell0:cell0-database-password@hostname-for-%s.%s.svc/nova_cell0?read_default_file=/etc/my.cnf",
						cell0.MariaDBDatabaseName.Name, novaNames.Namespace)),
			)
			Expect(configData).To(
				ContainSubstring(
					fmt.Sprintf(
						"[api_database]\nconnection = mysql+pymysql://nova_api:api-database-password@hostname-for-%s.%s.svc/nova_api?read_default_file=/etc/my.cnf",
						novaNames.APIMariaDBDatabaseName.Name, novaNames.Namespace)),
			)
			Expect(configData).To(ContainSubstring("password = service-password"))

			configDataMap = th.GetSecret(novaNames.SchedulerConfigDataName)
			Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
			configData = string(configDataMap.Data["01-nova.conf"])
			Expect(configData).To(
				ContainSubstring(
					fmt.Sprintf(
						"[database]\nconnection = mysql+pymysql://nova_cell0:cell0-database-password@hostname-for-%s.%s.svc/nova_cell0?read_default_file=/etc/my.cnf",
						cell0.MariaDBDatabaseName.Name, novaNames.Namespace)),
			)
			Expect(configData).To(
				ContainSubstring(
					fmt.Sprintf(
						"[api_database]\nconnection = mysql+pymysql://nova_api:api-database-password@hostname-for-%s.%s.svc/nova_api?read_default_file=/etc/my.cnf",
						novaNames.APIMariaDBDatabaseName.Name, novaNames.Namespace)),
			)
			Expect(configData).To(ContainSubstring("password = service-password"))

			th.SimulateStatefulSetReplicaReady(novaNames.APIName)
			keystone.SimulateKeystoneEndpointReady(novaNames.APIKeystoneEndpointName)

			th.ExpectCondition(
				cell0.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionTrue,
			)
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
				k8sClient.Delete, ctx, CreateNovaMessageBusSecret(cell0))
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					novaNames.NovaName.Namespace,
					"openstack",
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			memcachedSpec := memcachedv1.MemcachedSpec{
				Replicas: ptr.To(int32(3)),
			}

			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(novaNames.NovaName.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMemcachedReady(novaNames.MemcachedNamespace)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(novaNames.NovaName.Namespace))

			DeferCleanup(th.DeleteInstance, CreateNovaWithCell0(novaNames.NovaName))
		})

		It("removes the finalizer from KeystoneService", func() {
			keystone.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				condition.KeystoneServiceReadyCondition,
				corev1.ConditionTrue,
			)

			service := keystone.GetKeystoneService(novaNames.KeystoneServiceName)
			Expect(service.Finalizers).To(ContainElement("Nova"))

			th.DeleteInstance(GetNova(novaNames.NovaName))
			service = keystone.GetKeystoneService(novaNames.KeystoneServiceName)
			Expect(service.Finalizers).NotTo(ContainElement("Nova"))
		})

		It("removes the finalizers from the nova dbs", func() {
			keystone.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)

			apiDB := mariadb.GetMariaDBDatabase(novaNames.APIMariaDBDatabaseName)
			Expect(apiDB.Finalizers).To(ContainElement("Nova"))
			cell0DB := mariadb.GetMariaDBDatabase(cell0.MariaDBDatabaseName)
			Expect(cell0DB.Finalizers).To(ContainElement("Nova"))

			apiAcc := mariadb.GetMariaDBAccount(novaNames.APIMariaDBDatabaseName)
			Expect(apiAcc.Finalizers).To(ContainElement("Nova"))
			cell0Acc := mariadb.GetMariaDBAccount(cell0.MariaDBDatabaseName)
			Expect(cell0Acc.Finalizers).To(ContainElement("Nova"))

			th.DeleteInstance(GetNova(novaNames.NovaName))

			apiDB = mariadb.GetMariaDBDatabase(novaNames.APIMariaDBDatabaseName)
			Expect(apiDB.Finalizers).NotTo(ContainElement("Nova"))
			cell0DB = mariadb.GetMariaDBDatabase(cell0.MariaDBDatabaseName)
			Expect(cell0DB.Finalizers).NotTo(ContainElement("Nova"))

			apiAcc = mariadb.GetMariaDBAccount(novaNames.APIMariaDBDatabaseName)
			Expect(apiAcc.Finalizers).NotTo(ContainElement("Nova"))
			cell0Acc = mariadb.GetMariaDBAccount(cell0.MariaDBDatabaseName)
			Expect(cell0Acc.Finalizers).NotTo(ContainElement("Nova"))

		})
	})

	When("Nova CR instance is created with NetworkAttachment and ExternalEndpoints", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaSecret(novaNames.NovaName.Namespace, SecretName))
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaMessageBusSecret(cell0))
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					novaNames.NovaName.Namespace,
					"openstack",
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			memcachedSpec := memcachedv1.MemcachedSpec{
				Replicas: ptr.To(int32(3)),
			}

			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(novaNames.NovaName.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMemcachedReady(novaNames.MemcachedNamespace)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(novaNames.NovaName.Namespace))

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
				"secret":                SecretName,
				"apiMessageBusInstance": cell0.TransportURLName.Name,
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

			keystone.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
			mariadb.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(novaNames.APIMariaDBDatabaseName)
			mariadb.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell0.MariaDBDatabaseName)
			infra.SimulateTransportURLReady(cell0.TransportURLName)
			th.SimulateJobSuccess(cell0.DBSyncJobName)
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
			keystone.SimulateKeystoneEndpointReady(novaNames.APIKeystoneEndpointName)

			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)

			nova := GetNova(novaNames.NovaName)

			conductor := GetNovaConductor(cell0.ConductorName)
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
			Expect(novaDefault.Spec.MetadataServiceTemplate.ContainerImage).To(Equal(util.GetEnvVar("RELATED_IMAGE_NOVA_API_IMAGE_URL_DEFAULT", novav1.NovaMetadataContainerImage)))
			Expect(novaDefault.Spec.SchedulerServiceTemplate.ContainerImage).To(Equal(util.GetEnvVar("RELATED_IMAGE_NOVA_SCHEDULER_IMAGE_URL_DEFAULT", novav1.NovaSchedulerContainerImage)))

			for _, cell := range novaDefault.Spec.CellTemplates {
				Expect(cell.ConductorServiceTemplate.ContainerImage).To(Equal(util.GetEnvVar("RELATED_IMAGE_NOVA_CONDUCTOR_IMAGE_URL_DEFAULT", novav1.NovaConductorContainerImage)))
				Expect(cell.MetadataServiceTemplate.ContainerImage).To(Equal(util.GetEnvVar("RELATED_IMAGE_NOVA_API_IMAGE_URL_DEFAULT", novav1.NovaMetadataContainerImage)))
				Expect(cell.NoVNCProxyServiceTemplate.ContainerImage).To(Equal(util.GetEnvVar("RELATED_IMAGE_NOVA_NOVNC_IMAGE_URL_DEFAULT", novav1.NovaNoVNCContainerImage)))
			}
		})
	})
})

var _ = Describe("Nova controller without memcached", func() {
	BeforeEach(func() {
		DeferCleanup(
			mariadb.DeleteDBService,
			mariadb.CreateDBService(
				novaNames.NovaName.Namespace,
				"openstack",
				corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Port: 3306}},
				},
			),
		)
		DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(novaNames.NovaName.Namespace))

		DeferCleanup(th.DeleteInstance, CreateNovaWithCell0(novaNames.NovaName))
		DeferCleanup(
			k8sClient.Delete, ctx, CreateNovaSecret(novaNames.NovaName.Namespace, SecretName))
		DeferCleanup(
			k8sClient.Delete, ctx, CreateNovaMessageBusSecret(cell0))
	})
	It("memcached failed", func() {
		th.ExpectCondition(
			novaNames.NovaName,
			ConditionGetterFunc(NovaConditionGetter),
			condition.ServiceAccountReadyCondition,
			corev1.ConditionTrue,
		)
		th.ExpectConditionWithDetails(
			novaNames.NovaName,
			ConditionGetterFunc(NovaConditionGetter),
			condition.MemcachedReadyCondition,
			corev1.ConditionFalse,
			condition.RequestedReason,
			" Memcached instance has not been provisioned",
		)
	})
})
