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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	mariadb_test "github.com/openstack-k8s-operators/mariadb-operator/api/test/helpers"
	mariadbv1 "github.com/openstack-k8s-operators/mariadb-operator/api/v1beta1"

	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/nova-operator/controllers"
)

var _ = Describe("Nova controller - notifications", func() {

	When("Nova CR instance is created", func() {
		BeforeEach(func() {
			CreateNovaWithNCellsAndEnsureReady(1, &novaNames)
		})
		It("notification transport url is not set", func() {

			// assert that a the top level internal internal secret is created
			// with the proper data
			internalTopLevelSecret := th.GetSecret(novaNames.InternalTopLevelSecretName)
			// verify if nova secret has notification-transport-url
			Expect(internalTopLevelSecret.Data).To(HaveKey("notification_transport_url"))
			Expect(internalTopLevelSecret.Data).To(
				HaveKeyWithValue("notification_transport_url", []byte("")))

			// verify if confs are updated with notification-transport-url under oslo_messaging_notifications
			// assert in nova-api conf
			configDataMap := th.GetSecret(novaNames.APIConfigDataName)
			Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
			configData := string(configDataMap.Data["01-nova.conf"])
			AssertNotHaveNotificationTransportURL(configData)

			// assert in sch conf
			configDataMap = th.GetSecret(novaNames.SchedulerConfigDataName)
			Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
			configData = string(configDataMap.Data["01-nova.conf"])
			AssertNotHaveNotificationTransportURL(configData)

			// assert in cell0-conductor conf
			configDataMap = th.GetSecret(cell0.ConductorConfigDataName)
			Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
			configData = string(configDataMap.Data["01-nova.conf"])
			AssertNotHaveNotificationTransportURL(configData)

		})

		It("notification transport url is set with new rabbit", func() {

			// add new-rabbit in Nova CR
			notificationsBus := GetNotificationsBusNames(novaNames.NovaName)
			DeferCleanup(k8sClient.Delete, ctx, CreateNotificationTransportURLSecret(notificationsBus))

			Eventually(func(g Gomega) {
				nova := GetNova(novaNames.NovaName)
				nova.Spec.NotificationsBusInstance = &notificationsBus.BusName
				g.Expect(k8sClient.Update(ctx, nova)).Should(Succeed())
			}, timeout, interval).Should(Succeed())

			// as new-rabbit already exists in cluster, infra operator will create transporturl for it
			// simulating same
			infra.SimulateTransportURLReady(notificationsBus.TransportURLName)
			transportURLName := infra.GetTransportURL(notificationsBus.TransportURLName)
			Expect(transportURLName.Spec.RabbitmqClusterName).To(Equal(notificationsBus.BusName))

			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaNotificationMQReadyCondition,
				corev1.ConditionTrue,
			)

			// the nova secret(i.e top level secret) should have notification_transport_url value set
			internalTopLevelSecret := th.GetSecret(novaNames.InternalTopLevelSecretName)
			Expect(internalTopLevelSecret.Data).To(HaveKey("notification_transport_url"))
			Expect(internalTopLevelSecret.Data["notification_transport_url"]).ShouldNot(BeEmpty())

			// assert in nova-api conf
			configDataMap := th.GetSecret(novaNames.APIConfigDataName)
			Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
			configData := string(configDataMap.Data["01-nova.conf"])
			AssertHaveNotificationTransportURL(notificationsBus.TransportURLName.Name, configData)

			// assert in sch conf
			configDataMap = th.GetSecret(novaNames.SchedulerConfigDataName)
			Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
			configData = string(configDataMap.Data["01-nova.conf"])
			AssertHaveNotificationTransportURL(notificationsBus.TransportURLName.Name, configData)

			// assert in cell0-conductor conf
			configDataMap = th.GetSecret(cell0.ConductorConfigDataName)
			Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
			configData = string(configDataMap.Data["01-nova.conf"])
			AssertHaveNotificationTransportURL(notificationsBus.TransportURLName.Name, configData)

			// cleanup notifications transporturl
			Eventually(func(g Gomega) {
				nova := GetNova(novaNames.NovaName)
				nova.Spec.NotificationsBusInstance = nil
				g.Expect(k8sClient.Update(ctx, nova)).Should(Succeed())
			}, timeout, interval).Should(Succeed())

			infra.AssertTransportURLDoesNotExist(notificationsBus.TransportURLName)
		})
	})

})

var _ = Describe("Nova controller - quorum queues", func() {

	When("Nova CR instance is created with quorum queues disabled", func() {
		BeforeEach(func() {
			CreateNovaWithNCellsAndEnsureReady(1, &novaNames)
		})

		It("should have quorum queues disabled in configuration", func() {
			// Check that internal secrets have quorum queues set to false
			internalTopLevelSecret := th.GetSecret(novaNames.InternalTopLevelSecretName)
			Expect(internalTopLevelSecret.Data).To(HaveKey(controllers.QuorumQueuesSelector))
			Expect(internalTopLevelSecret.Data).To(
				HaveKeyWithValue(controllers.QuorumQueuesSelector, []byte("false")))

			// Check cell0 internal secret
			cell0InternalSecret := th.GetSecret(cell0.InternalCellSecretName)
			Expect(cell0InternalSecret.Data).To(HaveKey(controllers.QuorumQueuesSelector))
			Expect(cell0InternalSecret.Data).To(
				HaveKeyWithValue(controllers.QuorumQueuesSelector, []byte("false")))

			// Verify nova-api configuration
			configDataMap := th.GetSecret(novaNames.APIConfigDataName)
			Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
			configData := string(configDataMap.Data["01-nova.conf"])

			// Should have standard non-quorum queue settings
			Expect(configData).To(ContainSubstring("amqp_durable_queues=false"))
			Expect(configData).To(ContainSubstring("amqp_auto_delete=false"))
			Expect(configData).ToNot(ContainSubstring("rabbit_quorum_queue=true"))
			Expect(configData).ToNot(ContainSubstring("rabbit_transient_quorum_queue=true"))

			// Verify nova-scheduler configuration
			configDataMap = th.GetSecret(novaNames.SchedulerConfigDataName)
			Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
			configData = string(configDataMap.Data["01-nova.conf"])

			Expect(configData).To(ContainSubstring("amqp_durable_queues=false"))
			Expect(configData).To(ContainSubstring("amqp_auto_delete=false"))
			Expect(configData).ToNot(ContainSubstring("rabbit_quorum_queue=true"))

			// Verify nova-conductor configuration
			configDataMap = th.GetSecret(cell0.ConductorConfigDataName)
			Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
			configData = string(configDataMap.Data["01-nova.conf"])

			Expect(configData).To(ContainSubstring("amqp_durable_queues=false"))
			Expect(configData).To(ContainSubstring("amqp_auto_delete=false"))
			Expect(configData).ToNot(ContainSubstring("rabbit_quorum_queue=true"))
		})
	})

	When("Nova CR instance is created with quorum queues enabled", func() {
		BeforeEach(func() {
			CreateNovaWithNCellsAndEnsureReady(1, &novaNames)
		})

		It("should have quorum queues enabled in configuration when TransportURL secret has quorumqueues=true", func() {
			// Get the TransportURL resource to find the actual secret name that the controller uses
			transportURL := infra.GetTransportURL(cell0.TransportURLName)

			// Update the correct TransportURL secret (the one the controller actually checks)
			transportSecret := th.GetSecret(types.NamespacedName{
				Namespace: cell0.TransportURLName.Namespace,
				Name:      transportURL.Status.SecretName,
			})
			transportSecret.Data[controllers.QuorumQueuesSelector] = []byte("true")
			Expect(k8sClient.Update(ctx, &transportSecret)).Should(Succeed())

			// Trigger Nova reconciliation by updating a field (force a reconcile)
			nova := GetNova(novaNames.NovaName)
			if nova.Annotations == nil {
				nova.Annotations = make(map[string]string)
			}
			nova.Annotations["test-trigger"] = "force-reconcile"
			Expect(k8sClient.Update(ctx, nova)).Should(Succeed())

			// Wait for Nova to reconcile and update internal secrets
			Eventually(func(g Gomega) {
				internalSecret := th.GetSecret(cell0.InternalCellSecretName)
				g.Expect(internalSecret.Data).To(HaveKey(controllers.QuorumQueuesSelector))
				g.Expect(internalSecret.Data).To(
					HaveKeyWithValue(controllers.QuorumQueuesSelector, []byte("true")))
			}, timeout, interval).Should(Succeed())

			// Check that internal secrets have quorum queues set to true
			internalSecret := th.GetSecret(cell0.InternalCellSecretName)
			Expect(internalSecret.Data).To(
				HaveKeyWithValue(controllers.QuorumQueuesSelector, []byte("true")))

			// Verify nova-api configuration has quorum queue settings
			Eventually(func(g Gomega) {
				configDataMap := th.GetSecret(novaNames.APIConfigDataName)
				g.Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
				configData := string(configDataMap.Data["01-nova.conf"])

				g.Expect(configData).To(ContainSubstring("rabbit_quorum_queue=true"))
				g.Expect(configData).To(ContainSubstring("rabbit_transient_quorum_queue=true"))
				g.Expect(configData).To(ContainSubstring("amqp_durable_queues=true"))
				g.Expect(configData).ToNot(ContainSubstring("amqp_durable_queues=false"))
				g.Expect(configData).ToNot(ContainSubstring("amqp_auto_delete=false"))
			}, timeout, interval).Should(Succeed())

			// Verify nova-scheduler configuration
			Eventually(func(g Gomega) {
				configDataMap := th.GetSecret(novaNames.SchedulerConfigDataName)
				g.Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
				configData := string(configDataMap.Data["01-nova.conf"])

				g.Expect(configData).To(ContainSubstring("rabbit_quorum_queue=true"))
				g.Expect(configData).To(ContainSubstring("rabbit_transient_quorum_queue=true"))
				g.Expect(configData).To(ContainSubstring("amqp_durable_queues=true"))
			}, timeout, interval).Should(Succeed())

			// Verify nova-conductor configuration
			Eventually(func(g Gomega) {
				configDataMap := th.GetSecret(cell0.ConductorConfigDataName)
				g.Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
				configData := string(configDataMap.Data["01-nova.conf"])

				g.Expect(configData).To(ContainSubstring("rabbit_quorum_queue=true"))
				g.Expect(configData).To(ContainSubstring("rabbit_transient_quorum_queue=true"))
				g.Expect(configData).To(ContainSubstring("amqp_durable_queues=true"))
			}, timeout, interval).Should(Succeed())
		})
	})

	When("TransportURL secret is updated after Nova deployment", func() {
		BeforeEach(func() {
			CreateNovaWithNCellsAndEnsureReady(1, &novaNames)
		})

		It("should update configuration when quorum queues are enabled later", func() {
			// Initially verify quorum queues are disabled
			configDataMap := th.GetSecret(novaNames.APIConfigDataName)
			configData := string(configDataMap.Data["01-nova.conf"])
			Expect(configData).To(ContainSubstring("amqp_durable_queues=false"))
			Expect(configData).ToNot(ContainSubstring("rabbit_quorum_queue=true"))

			// Update the TransportURL secret to enable quorum queues
			transportSecret := th.GetSecret(types.NamespacedName{
				Namespace: cell0.TransportURLName.Namespace,
				Name:      fmt.Sprintf("%s-secret", cell0.TransportURLName.Name),
			})
			transportSecret.Data[controllers.QuorumQueuesSelector] = []byte("true")
			Expect(k8sClient.Update(ctx, &transportSecret)).Should(Succeed())

			// Trigger Nova reconciliation by updating a field (force a reconcile)
			nova := GetNova(novaNames.NovaName)
			if nova.Annotations == nil {
				nova.Annotations = make(map[string]string)
			}
			nova.Annotations["test-trigger"] = "force-reconcile"
			Expect(k8sClient.Update(ctx, nova)).Should(Succeed())

			// Wait for Nova to reconcile and update configurations
			Eventually(func(g Gomega) {
				configDataMap := th.GetSecret(novaNames.APIConfigDataName)
				configData := string(configDataMap.Data["01-nova.conf"])

				g.Expect(configData).To(ContainSubstring("rabbit_quorum_queue=true"))
				g.Expect(configData).To(ContainSubstring("rabbit_transient_quorum_queue=true"))
				g.Expect(configData).To(ContainSubstring("amqp_durable_queues=true"))
				g.Expect(configData).ToNot(ContainSubstring("amqp_durable_queues=false"))
			}, timeout, interval).Should(Succeed())
		})
	})
})

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
			memcachedSpec := infra.GetDefaultMemcachedSpec()

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
			mariadb.SimulateMariaDBAccountCompleted(novaNames.APIMariaDBDatabaseAccount)
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
			mariadb.SimulateMariaDBAccountCompleted(novaNames.APIMariaDBDatabaseAccount)
			mariadb.GetMariaDBDatabase(cell0.MariaDBDatabaseName)
			mariadb.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell0.MariaDBAccountName)
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
			mariadb.SimulateMariaDBAccountCompleted(novaNames.APIMariaDBDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell0.MariaDBAccountName)
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
				HaveKeyWithValue(controllers.ServicePasswordSelector, []byte("service-password")))
			Expect(internalCellSecret.Data).To(
				HaveKeyWithValue("transport_url", []byte("rabbit://cell0/fake")))
			Expect(internalCellSecret.Data).To(
				HaveKeyWithValue("notification_transport_url", []byte("")))

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

			cell0Account := mariadb.GetMariaDBAccount(cell0.MariaDBAccountName)
			cell0Secret := th.GetSecret(types.NamespacedName{Name: cell0Account.Spec.Secret, Namespace: cell0.MariaDBAccountName.Namespace})

			Expect(configData).To(
				ContainSubstring(fmt.Sprintf("[database]\nconnection = mysql+pymysql://%s:%s@hostname-for-openstack.%s.svc/nova_cell0?read_default_file=/etc/my.cnf",
					cell0Account.Spec.UserName, cell0Secret.Data[mariadbv1.DatabasePasswordSelector],
					novaNames.Namespace)),
			)

			apiAccount := mariadb.GetMariaDBAccount(novaNames.APIMariaDBDatabaseAccount)
			apiSecret := th.GetSecret(types.NamespacedName{Name: apiAccount.Spec.Secret, Namespace: novaNames.APIMariaDBDatabaseAccount.Namespace})

			Expect(configData).To(
				ContainSubstring(fmt.Sprintf("[api_database]\nconnection = mysql+pymysql://%s:%s@hostname-for-openstack.%s.svc/nova_api?read_default_file=/etc/my.cnf",
					apiAccount.Spec.UserName, apiSecret.Data[mariadbv1.DatabasePasswordSelector], novaNames.Namespace)),
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
			mariadb.SimulateMariaDBAccountCompleted(novaNames.APIMariaDBDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell0.MariaDBAccountName)
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
				HaveKeyWithValue(controllers.ServicePasswordSelector, []byte("service-password")))
			Expect(internalTopLevelSecret.Data).To(
				HaveKeyWithValue(controllers.MetadataSecretSelector, []byte("metadata-secret")))
			Expect(internalTopLevelSecret.Data).To(
				HaveKeyWithValue("transport_url", []byte("rabbit://cell0/fake")))
			Expect(internalTopLevelSecret.Data).To(
				HaveKeyWithValue("notification_transport_url", []byte("")))
		})

		It("creates NovaAPI", func() {
			keystone.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
			mariadb.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(novaNames.APIMariaDBDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell0.MariaDBAccountName)
			infra.SimulateTransportURLReady(cell0.TransportURLName)
			th.SimulateJobSuccess(cell0.DBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
			th.SimulateJobSuccess(cell0.CellMappingJobName)
			SimulateReadyOfNovaTopServices()

			api := GetNovaAPI(novaNames.APIName)
			Expect(api.Spec.ServiceUser).To(Equal("nova"))
			Expect(api.Spec.ServiceAccount).To(Equal(novaNames.ServiceAccountName.Name))
			Expect(api.Spec.Secret).To(Equal(novaNames.InternalTopLevelSecretName.Name))

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
			mariadb.SimulateMariaDBAccountCompleted(novaNames.APIMariaDBDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell0.MariaDBAccountName)
			infra.SimulateTransportURLReady(cell0.TransportURLName)
			th.SimulateJobSuccess(cell0.DBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
			th.SimulateJobSuccess(cell0.CellMappingJobName)
			SimulateReadyOfNovaTopServices()

			scheduler := GetNovaScheduler(novaNames.SchedulerName)
			Expect(scheduler.Spec.ServiceAccount).To(Equal(novaNames.ServiceAccountName.Name))
			Expect(scheduler.Spec.Secret).To(Equal(novaNames.InternalTopLevelSecretName.Name))

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
			mariadb.SimulateMariaDBAccountCompleted(novaNames.APIMariaDBDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell0.MariaDBAccountName)
			infra.SimulateTransportURLReady(cell0.TransportURLName)
			th.SimulateJobSuccess(cell0.DBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
			th.SimulateJobSuccess(cell0.CellMappingJobName)
			th.SimulateStatefulSetReplicaReady(novaNames.APIStatefulSetName)
			keystone.SimulateKeystoneEndpointReady(novaNames.APIKeystoneEndpointName)
			th.SimulateStatefulSetReplicaReady(novaNames.SchedulerStatefulSetName)

			metadata := GetNovaMetadata(novaNames.MetadataName)
			Expect(metadata.Spec.ServiceAccount).To(Equal(novaNames.ServiceAccountName.Name))
			Expect(metadata.Spec.Secret).To(Equal(novaNames.InternalTopLevelSecretName.Name))

			SimulateReadyOfNovaTopServices()

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

			memcachedSpec := infra.GetDefaultMemcachedSpec()

			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(novaNames.NovaName.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMemcachedReady(novaNames.MemcachedNamespace)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(novaNames.NovaName.Namespace))

			DeferCleanup(th.DeleteInstance, CreateNovaWithCell0(novaNames.NovaName))

			keystone.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
			mariadb.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(novaNames.APIMariaDBDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell0.MariaDBAccountName)
			infra.SimulateTransportURLReady(cell0.TransportURLName)
			GetNovaCell(cell0.CellCRName)
			GetNovaConductor(cell0.ConductorName)

			th.SimulateJobFailure(cell0.DBSyncJobName)
		})

		It("does not set the cell db sync ready condition to true", func() {
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
			memcachedSpec := infra.GetDefaultMemcachedSpec()

			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(novaNames.NovaName.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMemcachedReady(novaNames.MemcachedNamespace)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(novaNames.NovaName.Namespace))

			DeferCleanup(th.DeleteInstance, CreateNovaWithCell0(novaNames.NovaName))

			keystone.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
			mariadb.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(novaNames.APIMariaDBDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell0.MariaDBAccountName)
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
			memcachedSpec := infra.GetDefaultMemcachedSpec()

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
			mariadb.SimulateMariaDBAccountCompleted(novaNames.APIMariaDBDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell0.MariaDBAccountName)
			infra.SimulateTransportURLReady(cell0.TransportURLName)

			cell0DBSync := th.GetJob(cell0.DBSyncJobName)
			Expect(cell0DBSync.Spec.Template.Spec.InitContainers).To(BeEmpty())
			configDataMap := th.GetSecret(cell0.ConductorConfigDataName)
			Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
			configData := string(configDataMap.Data["01-nova.conf"])

			cell0Account := mariadb.GetMariaDBAccount(cell0.MariaDBAccountName)
			cell0Secret := th.GetSecret(types.NamespacedName{Name: cell0Account.Spec.Secret, Namespace: cell0.MariaDBAccountName.Namespace})

			Expect(configData).To(
				ContainSubstring(
					fmt.Sprintf(
						"[database]\nconnection = mysql+pymysql://%s:%s@hostname-for-%s.%s.svc/nova_cell0?read_default_file=/etc/my.cnf",
						cell0Account.Spec.UserName, cell0Secret.Data[mariadbv1.DatabasePasswordSelector],
						cell0.MariaDBDatabaseName.Name, novaNames.Namespace)),
			)

			apiAccount := mariadb.GetMariaDBAccount(novaNames.APIMariaDBDatabaseAccount)
			apiSecret := th.GetSecret(types.NamespacedName{Name: apiAccount.Spec.Secret, Namespace: novaNames.APIMariaDBDatabaseAccount.Namespace})

			Expect(configData).To(
				ContainSubstring(
					fmt.Sprintf(
						"[api_database]\nconnection = mysql+pymysql://%s:%s@hostname-for-%s.%s.svc/nova_api?read_default_file=/etc/my.cnf",
						apiAccount.Spec.UserName, apiSecret.Data[mariadbv1.DatabasePasswordSelector],
						novaNames.APIMariaDBDatabaseName.Name, novaNames.Namespace)),
			)
			Expect(configData).To(ContainSubstring("password = service-password"))

			th.SimulateJobSuccess(cell0.DBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
			th.SimulateJobSuccess(cell0.CellMappingJobName)
			SimulateReadyOfNovaTopServices()

			configDataMap = th.GetSecret(novaNames.APIConfigDataName)
			Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
			configData = string(configDataMap.Data["01-nova.conf"])
			Expect(configData).To(
				ContainSubstring(
					fmt.Sprintf(
						"[database]\nconnection = mysql+pymysql://%s:%s@hostname-for-%s.%s.svc/nova_cell0?read_default_file=/etc/my.cnf",
						cell0Account.Spec.UserName, cell0Secret.Data[mariadbv1.DatabasePasswordSelector],
						cell0.MariaDBDatabaseName.Name, novaNames.Namespace)),
			)
			Expect(configData).To(
				ContainSubstring(
					fmt.Sprintf(
						"[api_database]\nconnection = mysql+pymysql://%s:%s@hostname-for-%s.%s.svc/nova_api?read_default_file=/etc/my.cnf",
						apiAccount.Spec.UserName, apiSecret.Data[mariadbv1.DatabasePasswordSelector],
						novaNames.APIMariaDBDatabaseName.Name, novaNames.Namespace)),
			)
			Expect(configData).To(ContainSubstring("password = service-password"))

			configDataMap = th.GetSecret(novaNames.SchedulerConfigDataName)
			Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
			configData = string(configDataMap.Data["01-nova.conf"])
			Expect(configData).To(
				ContainSubstring(
					fmt.Sprintf(
						"[database]\nconnection = mysql+pymysql://%s:%s@hostname-for-%s.%s.svc/nova_cell0?read_default_file=/etc/my.cnf",
						cell0Account.Spec.UserName, cell0Secret.Data[mariadbv1.DatabasePasswordSelector],
						cell0.MariaDBDatabaseName.Name, novaNames.Namespace)),
			)
			Expect(configData).To(
				ContainSubstring(
					fmt.Sprintf(
						"[api_database]\nconnection = mysql+pymysql://%s:%s@hostname-for-%s.%s.svc/nova_api?read_default_file=/etc/my.cnf",
						apiAccount.Spec.UserName, apiSecret.Data[mariadbv1.DatabasePasswordSelector],
						novaNames.APIMariaDBDatabaseName.Name, novaNames.Namespace)),
			)
			Expect(configData).To(ContainSubstring("password = service-password"))

			SimulateReadyOfNovaTopServices()

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
			memcachedSpec := infra.GetDefaultMemcachedSpec()

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
			Expect(service.Finalizers).To(ContainElement("openstack.org/nova"))

			th.DeleteInstance(GetNova(novaNames.NovaName))
			service = keystone.GetKeystoneService(novaNames.KeystoneServiceName)
			Expect(service.Finalizers).NotTo(ContainElement("openstack.org/nova"))
		})

		It("removes the finalizers from the nova dbs", func() {
			keystone.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)

			apiDB := mariadb.GetMariaDBDatabase(novaNames.APIMariaDBDatabaseName)
			Expect(apiDB.Finalizers).To(ContainElement("openstack.org/nova"))
			cell0DB := mariadb.GetMariaDBDatabase(cell0.MariaDBDatabaseName)
			Expect(cell0DB.Finalizers).To(ContainElement("openstack.org/nova"))

			apiAcc := mariadb.GetMariaDBAccount(novaNames.APIMariaDBDatabaseAccount)
			Expect(apiAcc.Finalizers).To(ContainElement("openstack.org/nova"))
			cell0Acc := mariadb.GetMariaDBAccount(cell0.MariaDBAccountName)
			Expect(cell0Acc.Finalizers).To(ContainElement("openstack.org/nova"))

			th.DeleteInstance(GetNova(novaNames.NovaName))

			apiDB = mariadb.GetMariaDBDatabase(novaNames.APIMariaDBDatabaseName)
			Expect(apiDB.Finalizers).NotTo(ContainElement("openstack.org/nova"))
			cell0DB = mariadb.GetMariaDBDatabase(cell0.MariaDBDatabaseName)
			Expect(cell0DB.Finalizers).NotTo(ContainElement("openstack.org/nova"))

			apiAcc = mariadb.GetMariaDBAccount(novaNames.APIMariaDBDatabaseAccount)
			Expect(apiAcc.Finalizers).NotTo(ContainElement("openstack.org/nova"))
			cell0Acc = mariadb.GetMariaDBAccount(cell0.MariaDBAccountName)
			Expect(cell0Acc.Finalizers).NotTo(ContainElement("openstack.org/nova"))

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
			memcachedSpec := infra.GetDefaultMemcachedSpec()

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
				"apiDatabaseAccount":    novaNames.APIMariaDBDatabaseAccount.Name,
				"apiMessageBusInstance": cell0.TransportURLName.Name,
				"cellTemplates": map[string]interface{}{
					"cell0": map[string]interface{}{
						"apiDatabaseAccount":  novaNames.APIMariaDBDatabaseAccount.Name,
						"cellDatabaseAccount": cell0.MariaDBAccountName.Name,
						"hasAPIAccess":        true,
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
			mariadb.SimulateMariaDBAccountCompleted(novaNames.APIMariaDBDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell0.MariaDBAccountName)
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
				novaNames.APIStatefulSetName,
				map[string][]string{novaNames.NovaName.Namespace + "/internalapi": {"10.0.0.1"}},
			)
			th.SimulateStatefulSetReplicaReadyWithPods(
				novaNames.MetadataStatefulSetName,
				map[string][]string{novaNames.NovaName.Namespace + "/internalapi": {"10.0.0.1"}},
			)
			SimulateReadyOfNovaTopServices()

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
	When("Nova CR instance is created with a wrong topology", func() {
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

			memcachedSpec := infra.GetDefaultMemcachedSpec()
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(novaNames.NovaName.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMemcachedReady(novaNames.MemcachedNamespace)

			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(novaNames.NovaName.Namespace))

			spec := GetDefaultNovaSpec()
			cell0template := GetDefaultNovaCellTemplate()
			cell0template["cellDatabaseInstance"] = cell0.MariaDBDatabaseName.Name
			spec["cellTemplates"] = map[string]interface{}{"cell0": cell0template}
			spec["apiDatabaseInstance"] = novaNames.APIMariaDBDatabaseName.Name
			spec["apiDatabaseAccount"] = novaNames.APIMariaDBDatabaseAccount.Name
			spec["topologyRef"] = map[string]interface{}{"name": "foo"}

			DeferCleanup(th.DeleteInstance, CreateNova(novaNames.NovaName, spec))

			keystone.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
			mariadb.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(novaNames.APIMariaDBDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell0.MariaDBAccountName)
			infra.SimulateTransportURLReady(cell0.TransportURLName)
			th.SimulateJobSuccess(cell0.DBSyncJobName)
		})
		It("points to a non existing topology CR", func() {
			// Reconciliation does not succeed because TopologyReadyCondition
			// is not marked as True for cell0 conductor.
			// Top level resources are therefore not created because nova
			//controller waits for cell0 to be ready first
			th.ExpectCondition(
				cell0.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.TopologyReadyCondition,
				corev1.ConditionFalse,
			)
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})
	})
	When("Nova CR instance is created with topology", func() {
		var topologyRef topologyv1.TopoRef
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

			memcachedSpec := infra.GetDefaultMemcachedSpec()
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(novaNames.NovaName.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMemcachedReady(novaNames.MemcachedNamespace)

			// Build the topology Spec
			topologySpec, _ := GetSampleTopologySpec(novaNames.NovaName.Name)
			// Create a global Test Topology
			_, topologyRef = infra.CreateTopology(novaNames.NovaTopologies[0], topologySpec)

			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(novaNames.NovaName.Namespace))

			spec := GetDefaultNovaSpec()
			cell0template := GetDefaultNovaCellTemplate()
			cell0template["cellDatabaseInstance"] = cell0.MariaDBDatabaseName.Name
			spec["cellTemplates"] = map[string]interface{}{"cell0": cell0template}
			spec["apiDatabaseInstance"] = novaNames.APIMariaDBDatabaseName.Name
			spec["apiDatabaseAccount"] = novaNames.APIMariaDBDatabaseAccount.Name

			// We reference the global topology and is inherited by the sub components
			spec["topologyRef"] = map[string]interface{}{"name": topologyRef.Name}

			DeferCleanup(th.DeleteInstance, CreateNova(novaNames.NovaName, spec))

			keystone.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
			mariadb.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(novaNames.APIMariaDBDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell0.MariaDBAccountName)
			infra.SimulateTransportURLReady(cell0.TransportURLName)
			th.SimulateJobSuccess(cell0.DBSyncJobName)
		})
		It("propagates topology to the Nova components", func() {
			SimulateReadyOfNovaTopServices()
			tp := infra.GetTopology(types.NamespacedName{
				Name:      topologyRef.Name,
				Namespace: topologyRef.Namespace,
			})
			Expect(tp.GetFinalizers()).To(HaveLen(4))
			finalizers := tp.GetFinalizers()

			api := GetNovaAPI(novaNames.APIName)
			Expect(api.Status.LastAppliedTopology).ToNot(BeNil())
			Expect(api.Status.LastAppliedTopology).To(Equal(&topologyRef))
			Expect(finalizers).To(ContainElement(
				fmt.Sprintf("openstack.org/novaapi-%s", novaNames.APIName.Name)))

			scheduler := GetNovaScheduler(novaNames.SchedulerName)
			Expect(scheduler.Status.LastAppliedTopology).ToNot(BeNil())
			Expect(scheduler.Status.LastAppliedTopology).To(Equal(&topologyRef))
			Expect(finalizers).To(ContainElement(
				fmt.Sprintf("openstack.org/novascheduler-%s", novaNames.SchedulerName.Name)))

			metadata := GetNovaMetadata(novaNames.MetadataName)
			Expect(metadata.Status.LastAppliedTopology).ToNot(BeNil())
			Expect(metadata.Status.LastAppliedTopology).To(Equal(&topologyRef))
			Expect(finalizers).To(ContainElement(
				fmt.Sprintf("openstack.org/novametadata-%s", novaNames.MetadataName.Name)))

			cond := GetNovaConductor(cell0.ConductorName)
			Expect(cond.Status.LastAppliedTopology).ToNot(BeNil())
			Expect(cond.Status.LastAppliedTopology).To(Equal(&topologyRef))
			Expect(finalizers).To(ContainElement(
				fmt.Sprintf("openstack.org/novaconductor-%s", cell0.ConductorName.Name)))

			th.ExpectCondition(
				novaNames.APIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.TopologyReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.TopologyReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				novaNames.MetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.TopologyReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				cell0.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.TopologyReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})
	When("Nova CR instance is created with topology and cell1", func() {
		var topologyRefCell topologyv1.TopoRef
		var topologyRefTopLevel topologyv1.TopoRef
		BeforeEach(func() {
			novaSecret := th.CreateSecret(
				types.NamespacedName{Namespace: novaNames.NovaName.Namespace, Name: SecretName},
				map[string][]byte{
					"NovaPassword":                         []byte("service-password"),
					"MetadataSecret":                       []byte("metadata-secret"),
					"MetadataCellsSecret" + cell1.CellName: []byte("metadata-secret-cell1"),
				},
			)
			// Build two topologies with the same content (the content of
			// TopologySpec is not relevant for this test
			topologySpec, _ := GetSampleTopologySpec(novaNames.NovaName.Name)
			// Create a global Test Topology
			_, topologyRefTopLevel = infra.CreateTopology(novaNames.NovaTopologies[0], topologySpec)
			_, topologyRefCell = infra.CreateTopology(novaNames.NovaTopologies[5], topologySpec)

			DeferCleanup(k8sClient.Delete, ctx, novaSecret)
			DeferCleanup(k8sClient.Delete, ctx, CreateNovaMessageBusSecret(cell0))
			DeferCleanup(k8sClient.Delete, ctx, CreateNovaMessageBusSecret(cell1))

			serviceSpec := corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 3306}}}
			DeferCleanup(mariadb.DeleteDBService, mariadb.CreateDBService(novaNames.APIMariaDBDatabaseName.Namespace, novaNames.APIMariaDBDatabaseName.Name, serviceSpec))
			DeferCleanup(mariadb.DeleteDBService, mariadb.CreateDBService(cell0.MariaDBDatabaseName.Namespace, cell0.MariaDBDatabaseName.Name, serviceSpec))
			DeferCleanup(mariadb.DeleteDBService, mariadb.CreateDBService(cell1.MariaDBDatabaseName.Namespace, cell1.MariaDBDatabaseName.Name, serviceSpec))
			// cell0
			spec := GetDefaultNovaSpec()
			cell0Template := GetDefaultNovaCellTemplate()
			cell0Template["cellDatabaseInstance"] = cell0.MariaDBDatabaseName.Name
			cell0Template["cellDatabaseAccount"] = cell0.MariaDBAccountName.Name
			// cell1
			cell1Template := GetDefaultNovaCellTemplate()
			cell1Template["cellDatabaseInstance"] = cell1.MariaDBDatabaseName.Name
			cell1Template["cellDatabaseAccount"] = cell1.MariaDBAccountName.Name
			cell1Template["cellMessageBusInstance"] = cell1.TransportURLName.Name
			// We reference the cell1 topology that is inherited by the cell1 conductor,
			// metadata, and novncproxy
			cell1Template["topologyRef"] = map[string]interface{}{"name": topologyRefCell.Name}
			cell1Template["metadataServiceTemplate"] = map[string]interface{}{
				"enabled": true,
			}
			spec["cellTemplates"] = map[string]interface{}{
				"cell0": cell0Template,
				"cell1": cell1Template,
			}
			// disable top-level metadata as we enabled the instance in cell1
			spec["metadataServiceTemplate"] = map[string]interface{}{
				"enabled": false,
			}
			spec["apiDatabaseInstance"] = novaNames.APIMariaDBDatabaseName.Name
			spec["apiMessageBusInstance"] = cell0.TransportURLName.Name
			// We reference the global topology and is inherited by the sub components
			// except cell1 that has an override
			spec["topologyRef"] = map[string]interface{}{"name": topologyRefTopLevel.Name}
			DeferCleanup(th.DeleteInstance, CreateNova(novaNames.NovaName, spec))
			memcachedSpec := infra.GetDefaultMemcachedSpec()
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(novaNames.NovaName.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMemcachedReady(novaNames.MemcachedNamespace)
			keystoneAPIName := keystone.CreateKeystoneAPI(novaNames.NovaName.Namespace)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystoneAPIName)
			keystoneAPI := keystone.GetKeystoneAPI(keystoneAPIName)
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Status().Update(ctx, keystoneAPI.DeepCopy())).Should(Succeed())
			}, timeout, interval).Should(Succeed())
			keystone.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
		})
		It("propagates topology to the Nova components except cell1 that has an override", func() {
			mariadb.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(novaNames.APIMariaDBDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell0.MariaDBAccountName)
			mariadb.SimulateMariaDBDatabaseCompleted(cell1.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell1.MariaDBAccountName)
			infra.SimulateTransportURLReady(cell0.TransportURLName)
			infra.SimulateTransportURLReady(cell1.TransportURLName)
			th.SimulateJobSuccess(cell0.DBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
			th.SimulateJobSuccess(cell0.CellMappingJobName)

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
			th.ExpectCondition(
				novaNames.APIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.TopologyReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.TopologyReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				cell1.MetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.TopologyReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				cell1.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.TopologyReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				cell1.NoVNCProxyName,
				ConditionGetterFunc(NoVNCProxyConditionGetter),
				condition.TopologyReadyCondition,
				corev1.ConditionTrue,
			)
			// Retrieve topology and check finalizers
			tpCell := infra.GetTopology(types.NamespacedName{
				Name:      topologyRefCell.Name,
				Namespace: topologyRefCell.Namespace,
			})
			tpTopTLevel := infra.GetTopology(types.NamespacedName{
				Name:      topologyRefTopLevel.Name,
				Namespace: topologyRefTopLevel.Namespace,
			})
			Expect(tpCell.GetFinalizers()).To(HaveLen(3))
			Expect(tpTopTLevel.GetFinalizers()).To(HaveLen(3))
			finalizers := tpCell.GetFinalizers()

			metadata := GetNovaMetadata(cell1.MetadataName)
			Expect(metadata.Status.LastAppliedTopology).ToNot(BeNil())
			Expect(metadata.Status.LastAppliedTopology).To(Equal(&topologyRefCell))
			Expect(finalizers).To(ContainElement(
				fmt.Sprintf("openstack.org/novametadata-%s", cell1.MetadataName.Name)))

			cond := GetNovaConductor(cell1.ConductorName)
			Expect(cond.Status.LastAppliedTopology).ToNot(BeNil())
			Expect(cond.Status.LastAppliedTopology).To(Equal(&topologyRefCell))
			Expect(finalizers).To(ContainElement(
				fmt.Sprintf("openstack.org/novaconductor-%s", cell1.ConductorName.Name)))

			novnc := GetNovaNoVNCProxy(cell1.NoVNCProxyName)
			Expect(novnc.Status.LastAppliedTopology).ToNot(BeNil())
			Expect(novnc.Status.LastAppliedTopology).To(Equal(&topologyRefCell))
			Expect(finalizers).To(ContainElement(
				fmt.Sprintf("openstack.org/novanovncproxy-%s", cell1.NoVNCProxyName.Name)))

			finalizers = tpTopTLevel.GetFinalizers()
			api := GetNovaAPI(novaNames.APIName)
			Expect(api.Status.LastAppliedTopology).ToNot(BeNil())
			Expect(api.Status.LastAppliedTopology).To(Equal(&topologyRefTopLevel))
			Expect(finalizers).To(ContainElement(
				fmt.Sprintf("openstack.org/novaapi-%s", novaNames.APIName.Name)))

			scheduler := GetNovaScheduler(novaNames.SchedulerName)
			Expect(scheduler.Status.LastAppliedTopology).ToNot(BeNil())
			Expect(scheduler.Status.LastAppliedTopology).To(Equal(&topologyRefTopLevel))
			Expect(finalizers).To(ContainElement(
				fmt.Sprintf("openstack.org/novascheduler-%s", novaNames.SchedulerName.Name)))
		})
	})

	When("Nova CR is created without container images defined", func() {
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
			memcachedSpec := infra.GetDefaultMemcachedSpec()

			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(novaNames.NovaName.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMemcachedReady(novaNames.MemcachedNamespace)

			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(novaNames.NovaName.Namespace))

			spec := GetDefaultNovaSpec()
			cell0 := GetDefaultNovaCellTemplate()
			spec["cellTemplates"] = map[string]interface{}{"cell0": cell0}
			// This nova is created without any container image is specified in
			// the request
			DeferCleanup(th.DeleteInstance, CreateNova(novaNames.NovaName, spec))
		})
		It("has the expected container image defaults", func() {
			novaDefault := GetNova(novaNames.NovaName)

			Expect(novaDefault.Spec.APIContainerImageURL).To(Equal(util.GetEnvVar("RELATED_IMAGE_NOVA_API_IMAGE_URL_DEFAULT", novav1.NovaAPIContainerImage)))

			Expect(novaDefault.Spec.MetadataContainerImageURL).To(Equal(util.GetEnvVar("RELATED_IMAGE_NOVA_API_IMAGE_URL_DEFAULT", novav1.NovaMetadataContainerImage)))
			Expect(novaDefault.Spec.SchedulerContainerImageURL).To(Equal(util.GetEnvVar("RELATED_IMAGE_NOVA_SCHEDULER_IMAGE_URL_DEFAULT", novav1.NovaSchedulerContainerImage)))
			Expect(novaDefault.Spec.ConductorContainerImageURL).To(Equal(util.GetEnvVar("RELATED_IMAGE_NOVA_CONDUCTOR_IMAGE_URL_DEFAULT", novav1.NovaConductorContainerImage)))
			Expect(novaDefault.Spec.MetadataContainerImageURL).To(Equal(util.GetEnvVar("RELATED_IMAGE_NOVA_API_IMAGE_URL_DEFAULT", novav1.NovaMetadataContainerImage)))
			Expect(novaDefault.Spec.NoVNCContainerImageURL).To(Equal(util.GetEnvVar("RELATED_IMAGE_NOVA_NOVNC_IMAGE_URL_DEFAULT", novav1.NovaNoVNCContainerImage)))

			// do the prerequisites for cell0 to be created
			keystone.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
			mariadb.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(novaNames.APIMariaDBDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell0.MariaDBAccountName)
			infra.SimulateTransportURLReady(cell0.TransportURLName)

			cell := GetNovaCell(cell0.CellCRName)
			Expect(cell.Spec.ConductorContainerImageURL).To(Equal(util.GetEnvVar("RELATED_IMAGE_NOVA_CONDUCTOR_IMAGE_URL_DEFAULT", novav1.NovaConductorContainerImage)))
			Expect(cell.Spec.MetadataContainerImageURL).To(Equal(util.GetEnvVar("RELATED_IMAGE_NOVA_API_IMAGE_URL_DEFAULT", novav1.NovaMetadataContainerImage)))
			Expect(cell.Spec.NoVNCContainerImageURL).To(Equal(util.GetEnvVar("RELATED_IMAGE_NOVA_NOVNC_IMAGE_URL_DEFAULT", novav1.NovaNoVNCContainerImage)))
		})
	})

	// Run MariaDBAccount suite tests for NovaAPI database.  these are pre-packaged ginkgo tests
	// that exercise standard account create / update patterns that should be
	// common to all controllers that ensure MariaDBAccount CRs.
	mariadbAPISuite := &mariadb_test.MariaDBTestHarness{
		PopulateHarness: func(harness *mariadb_test.MariaDBTestHarness) {
			harness.Setup(
				"Nova API",
				novaNames.Namespace,
				novaNames.APIMariaDBDatabaseName.Name,
				"openstack.org/nova",
				mariadb,
				timeout,
				interval,
			)
		},
		// Generate a fully running Nova service given an accountName
		// needs to make it all the way to the end where the mariadb finalizers
		// are removed from unused accounts since that's part of what we are testing
		SetupCR: func(accountName types.NamespacedName) {
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

			memcachedSpec := infra.GetDefaultMemcachedSpec()
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(novaNames.NovaName.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMemcachedReady(novaNames.MemcachedNamespace)

			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(novaNames.NovaName.Namespace))

			spec := GetDefaultNovaSpec()
			cell0template := GetDefaultNovaCellTemplate()
			cell0template["cellDatabaseInstance"] = cell0.MariaDBDatabaseName.Name
			spec["cellTemplates"] = map[string]interface{}{"cell0": cell0template}
			spec["apiDatabaseInstance"] = novaNames.APIMariaDBDatabaseName.Name
			spec["apiDatabaseAccount"] = accountName.Name

			DeferCleanup(th.DeleteInstance, CreateNova(novaNames.NovaName, spec))

			keystone.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
			mariadb.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(accountName)
			mariadb.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell0.MariaDBAccountName)
			infra.SimulateTransportURLReady(cell0.TransportURLName)
			th.SimulateJobSuccess(cell0.DBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
			th.SimulateJobSuccess(cell0.CellMappingJobName)

			th.SimulateStatefulSetReplicaReady(novaNames.APIStatefulSetName)
			keystone.SimulateKeystoneEndpointReady(novaNames.APIKeystoneEndpointName)
			th.SimulateStatefulSetReplicaReady(novaNames.SchedulerStatefulSetName)
			th.SimulateStatefulSetReplicaReady(novaNames.MetadataStatefulSetName)

			// ensure api/scheduler/metadata all complete
			Eventually(func(g Gomega) {
				nova := GetNova(novaNames.NovaName)
				g.Expect(nova.Status.APIServiceReadyCount).To(Equal(int32(1)))
				g.Expect(nova.Status.SchedulerServiceReadyCount).To(Equal(int32(1)))
				g.Expect(nova.Status.MetadataServiceReadyCount).To(Equal(int32(1)))
			}, timeout, interval).Should(Succeed())
		},
		// update to a new account name
		UpdateAccount: func(accountName types.NamespacedName) {
			Eventually(func(g Gomega) {
				nova := GetNova(novaNames.NovaName)
				nova.Spec.APIDatabaseAccount = accountName.Name
				g.Expect(th.K8sClient.Update(ctx, nova)).Should(Succeed())
			}, timeout, interval).Should(Succeed())

		},
		SwitchToNewAccount: func() {
			SimulateReadyOfNovaTopServices()

		},
		// delete the CR, allowing tests that exercise finalizer removal
		DeleteCR: func() {
			th.DeleteInstance(GetNova(novaNames.NovaName))
		},
	}

	mariadbAPISuite.RunBasicSuite()

	// Run MariaDBAccount suite tests for NovaCell0 database.  these are pre-packaged ginkgo tests
	// that exercise standard account create / update patterns that should be
	// common to all controllers that ensure MariaDBAccount CRs.
	mariadbCellSuite := &mariadb_test.MariaDBTestHarness{
		PopulateHarness: func(harness *mariadb_test.MariaDBTestHarness) {
			harness.Setup(
				"Nova Cell",
				novaNames.Namespace,
				cell0.MariaDBDatabaseName.Name,
				"openstack.org/nova",
				mariadb,
				timeout,
				interval,
			)
		},
		// Generate a fully running Nova service given an accountName
		// needs to make it all the way to the end where the mariadb finalizers
		// are removed from unused accounts since that's part of what we are testing
		SetupCR: func(accountName types.NamespacedName) {
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

			memcachedSpec := infra.GetDefaultMemcachedSpec()
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(novaNames.NovaName.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMemcachedReady(novaNames.MemcachedNamespace)

			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(novaNames.NovaName.Namespace))

			spec := GetDefaultNovaSpec()
			cell0template := GetDefaultNovaCellTemplate()
			cell0template["cellDatabaseInstance"] = cell0.MariaDBDatabaseName.Name
			cell0template["cellDatabaseAccount"] = accountName.Name
			spec["cellTemplates"] = map[string]interface{}{"cell0": cell0template}
			spec["apiDatabaseInstance"] = novaNames.APIMariaDBDatabaseName.Name

			DeferCleanup(th.DeleteInstance, CreateNova(novaNames.NovaName, spec))

			keystone.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
			mariadb.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(novaNames.APIMariaDBDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(accountName)
			infra.SimulateTransportURLReady(cell0.TransportURLName)
			th.SimulateJobSuccess(cell0.DBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
			th.SimulateJobSuccess(cell0.CellMappingJobName)

			SimulateReadyOfNovaTopServices()

			// ensure api/scheduler/metadata all complete so that the Nova
			// CR is not expected to have subsequent status changes from the controller
			Eventually(func(g Gomega) {
				nova := GetNova(novaNames.NovaName)
				g.Expect(nova.Status.APIServiceReadyCount).To(Equal(int32(1)))
				g.Expect(nova.Status.SchedulerServiceReadyCount).To(Equal(int32(1)))
				g.Expect(nova.Status.MetadataServiceReadyCount).To(Equal(int32(1)))
			}, timeout, interval).Should(Succeed())
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				cell0.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		},
		// update to a new account name
		UpdateAccount: func(accountName types.NamespacedName) {

			Eventually(func(g Gomega) {
				nova := GetNova(novaNames.NovaName)

				if entry, ok := nova.Spec.CellTemplates[cell0.CellName]; ok {
					entry.CellDatabaseAccount = accountName.Name
					nova.Spec.CellTemplates[cell0.CellName] = entry
				}

				g.Expect(th.K8sClient.Update(ctx, nova)).Should(Succeed())
			}, timeout, interval).Should(Succeed())

		},
		SwitchToNewAccount: func() {
			SimulateReadyOfNovaTopServices()
			Eventually(func(g Gomega) {
				th.SimulateJobSuccess(cell0.DBSyncJobName)
				th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
				th.SimulateJobSuccess(cell0.CellMappingJobName)
				nova := GetNova(novaNames.NovaName)
				g.Expect(nova.Status.APIServiceReadyCount).To(Equal(int32(1)))
				g.Expect(nova.Status.SchedulerServiceReadyCount).To(Equal(int32(1)))
				g.Expect(nova.Status.MetadataServiceReadyCount).To(Equal(int32(1)))
				cell := GetNovaCell(cell0.CellCRName)
				g.Expect(cell.Status.ConductorServiceReadyCount).To(Equal(int32(1)))
			}, timeout, interval).Should(Succeed())

		},
		// delete the CR, allowing tests that exercise finalizer removal
		DeleteCR: func() {
			th.DeleteInstance(GetNova(novaNames.NovaName))
		},
	}

	mariadbCellSuite.RunBasicSuite()
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
