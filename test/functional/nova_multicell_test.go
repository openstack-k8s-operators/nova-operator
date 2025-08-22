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

	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	mariadbv1 "github.com/openstack-k8s-operators/mariadb-operator/api/v1beta1"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/nova-operator/controllers"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Nova multi cell", func() {
	BeforeEach(func() {
		apiMariaDBAccount, apiMariaDBSecret := mariadb.CreateMariaDBAccountAndSecret(
			novaNames.APIMariaDBDatabaseAccount, mariadbv1.MariaDBAccountSpec{})
		DeferCleanup(k8sClient.Delete, ctx, apiMariaDBAccount)
		DeferCleanup(k8sClient.Delete, ctx, apiMariaDBSecret)

		cell0Account, cell0Secret := mariadb.CreateMariaDBAccountAndSecret(
			cell0.MariaDBAccountName, mariadbv1.MariaDBAccountSpec{})
		DeferCleanup(k8sClient.Delete, ctx, cell0Account)
		DeferCleanup(k8sClient.Delete, ctx, cell0Secret)

		cell1Account, cell1Secret := mariadb.CreateMariaDBAccountAndSecret(
			cell1.MariaDBAccountName, mariadbv1.MariaDBAccountSpec{})
		DeferCleanup(k8sClient.Delete, ctx, cell1Account)
		DeferCleanup(k8sClient.Delete, ctx, cell1Secret)

		cell2Account, cell2Secret := mariadb.CreateMariaDBAccountAndSecret(
			cell2.MariaDBAccountName, mariadbv1.MariaDBAccountSpec{})
		DeferCleanup(k8sClient.Delete, ctx, cell2Account)
		DeferCleanup(k8sClient.Delete, ctx, cell2Secret)

	})
	When("Nova CR instance is created with 3 cells", func() {
		BeforeEach(func() {
			// TODO(bogdando): deduplicate this into CreateNovaWith3CellsAndEnsureReady()
			// DeferCleanup(k8sClient.Delete, ctx, CreateNovaSecretFor3Cells(novaNames.NovaName.Namespace, SecretName))
			DeferCleanup(k8sClient.Delete, ctx, CreateNovaSecret(novaNames.NovaName.Namespace, SecretName))
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
			cell0Template["cellDatabaseAccount"] = cell0.MariaDBAccountName.Name

			cell1Template := GetDefaultNovaCellTemplate()
			cell1Template["cellDatabaseInstance"] = cell1.MariaDBDatabaseName.Name
			cell1Template["cellDatabaseAccount"] = cell1.MariaDBAccountName.Name
			cell1Template["cellMessageBusInstance"] = cell1.TransportURLName.Name
			cell1Template["passwordSelectors"] = map[string]interface{}{
				"database": "NovaCell1DatabasePassword",
			}
			cell1Template["novaComputeTemplates"] = map[string]interface{}{
				ironicComputeName: GetDefaultNovaComputeTemplate(),
			}
			cell1Memcached := "memcached1"
			cell1Template["memcachedInstance"] = cell1Memcached

			cell2Template := GetDefaultNovaCellTemplate()
			cell2Template["cellDatabaseInstance"] = cell2.MariaDBDatabaseName.Name
			cell2Template["cellDatabaseAccount"] = cell2.MariaDBAccountName.Name
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
			memcachedSpecCell1 := infra.GetDefaultMemcachedSpec()
			memcachedNamespace := types.NamespacedName{
				Name:      cell1Memcached,
				Namespace: novaNames.NovaName.Namespace,
			}
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(novaNames.NovaName.Namespace, cell1Memcached, memcachedSpecCell1))
			infra.SimulateMemcachedReady(memcachedNamespace)

			memcachedSpec := infra.GetDefaultMemcachedSpec()

			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(novaNames.NovaName.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMemcachedReady(novaNames.MemcachedNamespace)
			keystone.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
		})

		It("creates cell0 NovaCell", func() {
			mariadb.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(novaNames.APIMariaDBDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell0.MariaDBAccountName)
			infra.SimulateTransportURLReady(cell0.TransportURLName)

			th.ExpectCondition(
				cell0.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionFalse,
			)
			// assert that cell0 conductor is using the same DB as the API
			dbSync := th.GetJob(cell0.DBSyncJobName)
			Expect(dbSync.Spec.Template.Spec.InitContainers).To(BeEmpty())

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
			mariadb.SimulateMariaDBAccountCompleted(novaNames.APIMariaDBDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell0.MariaDBAccountName)
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
			Expect(configData).ShouldNot(
				ContainSubstring(fmt.Sprintf("memcache_servers=memcached-0.memcached.%s.svc:11211,memcached-1.memcached.%s.svc:11211,memcached-2.memcached.%s.svc:11211",
					novaNames.Namespace, novaNames.Namespace, novaNames.Namespace)))
			Expect(configData).Should(
				ContainSubstring(fmt.Sprintf("memcached_servers=memcached-0.memcached.%s.svc:11211,memcached-1.memcached.%s.svc:11211,memcached-2.memcached.%s.svc:11211",
					novaNames.Namespace, novaNames.Namespace, novaNames.Namespace)))

			Expect(configData).To(ContainSubstring("transport_url=rabbit://cell0/fake"))

			SimulateReadyOfNovaTopServices()

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
		})

		It("creates all cell DBs", func() {
			mariadb.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(novaNames.APIMariaDBDatabaseAccount)
			infra.SimulateTransportURLReady(cell0.TransportURLName)
			mariadb.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell0.MariaDBAccountName)
			th.SimulateJobSuccess(cell0.DBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
			th.SimulateJobSuccess(cell0.CellMappingJobName)
			th.SimulateStatefulSetReplicaReady(novaNames.APIStatefulSetName)
			keystone.SimulateKeystoneEndpointReady(novaNames.APIKeystoneEndpointName)

			mariadb.SimulateMariaDBDatabaseCompleted(cell1.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell1.MariaDBAccountName)
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsDBReadyCondition,
				corev1.ConditionFalse,
			)
			mariadb.SimulateMariaDBDatabaseCompleted(cell2.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell2.MariaDBAccountName)
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
			mariadb.SimulateMariaDBAccountCompleted(novaNames.APIMariaDBDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell0.MariaDBAccountName)
			infra.SimulateTransportURLReady(cell0.TransportURLName)
			th.SimulateJobSuccess(cell0.DBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
			th.SimulateJobSuccess(cell0.CellMappingJobName)
			th.SimulateStatefulSetReplicaReady(novaNames.APIStatefulSetName)
			keystone.SimulateKeystoneEndpointReady(novaNames.APIKeystoneEndpointName)
			mariadb.SimulateMariaDBDatabaseCompleted(cell1.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell1.MariaDBAccountName)
			infra.SimulateTransportURLReady(cell1.TransportURLName)

			th.ExpectCondition(
				cell1.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionFalse,
			)
			// assert that cell1 using its own DB but has access to the API DB
			dbSync := th.GetJob(cell1.DBSyncJobName)
			Expect(dbSync.Spec.Template.Spec.InitContainers).To(BeEmpty())
			configDataMap := th.GetSecret(cell1.ConductorConfigDataName)
			Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
			configData := string(configDataMap.Data["01-nova.conf"])

			cell1Account := mariadb.GetMariaDBAccount(cell1.MariaDBAccountName)
			cell1Secret := th.GetSecret(types.NamespacedName{Name: cell1Account.Spec.Secret, Namespace: cell1.MariaDBAccountName.Namespace})

			Expect(configData).To(
				ContainSubstring(
					fmt.Sprintf(
						"[database]\nconnection = mysql+pymysql://%s:%s@hostname-for-%s.%s.svc/nova_cell1?read_default_file=/etc/my.cnf",
						cell1Account.Spec.UserName, cell1Secret.Data[mariadbv1.DatabasePasswordSelector],
						cell1.MariaDBDatabaseName.Name, novaNames.Namespace)),
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

			Expect(configData).To(ContainSubstring("transport_url=rabbit://cell1/fake"))
			Expect(configData).ShouldNot(
				ContainSubstring(fmt.Sprintf("memcache_servers=memcached1-0.memcached1.%s.svc:11211,memcached1-1.memcached1.%s.svc:11211,memcached1-2.memcached1.%s.svc:11211",
					novaNames.Namespace, novaNames.Namespace, novaNames.Namespace)))
			Expect(configData).Should(
				ContainSubstring(fmt.Sprintf("memcached_servers=memcached1-0.memcached1.%s.svc:11211,memcached1-1.memcached1.%s.svc:11211,memcached1-2.memcached1.%s.svc:11211",
					novaNames.Namespace, novaNames.Namespace, novaNames.Namespace)))
			Expect(configData).Should(
				ContainSubstring("tls_enabled=false"))

			myCnf := configDataMap.Data["my.cnf"]
			Expect(myCnf).To(
				ContainSubstring("[client]\nssl=0"))

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
				novav1.NovaAllControlPlaneComputesReadyCondition,
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
		It("creates cell2 NovaCell ready", func() {
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
			th.SimulateStatefulSetReplicaReady(novaNames.MetadataStatefulSetName)
			mariadb.SimulateMariaDBDatabaseCompleted(cell1.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell1.MariaDBAccountName)
			infra.SimulateTransportURLReady(cell1.TransportURLName)
			th.SimulateStatefulSetReplicaReady(cell1.NovaComputeStatefulSetName)
			th.SimulateStatefulSetReplicaReady(cell1.NoVNCProxyStatefulSetName)
			th.SimulateJobSuccess(cell1.DBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell1.ConductorStatefulSetName)
			th.SimulateJobSuccess(cell1.CellMappingJobName)
			th.SimulateJobSuccess(cell1.HostDiscoveryJobName)

			mariadb.SimulateMariaDBDatabaseCompleted(cell2.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell2.MariaDBAccountName)
			infra.SimulateTransportURLReady(cell2.TransportURLName)

			th.ExpectCondition(
				cell2.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionFalse,
			)
			// assert that cell2 using its own DB but has *no* access to the API DB
			dbSync := th.GetJob(cell2.DBSyncJobName)
			Expect(dbSync.Spec.Template.Spec.InitContainers).To(BeEmpty())
			configDataMap := th.GetSecret(cell2.ConductorConfigDataName)
			Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
			configData := string(configDataMap.Data["01-nova.conf"])

			cell2Account := mariadb.GetMariaDBAccount(cell2.MariaDBAccountName)
			cell2Secret := th.GetSecret(types.NamespacedName{Name: cell2Account.Spec.Secret, Namespace: cell2.MariaDBAccountName.Namespace})

			Expect(configData).To(
				ContainSubstring(
					fmt.Sprintf(
						"[database]\nconnection = mysql+pymysql://%s:%s@hostname-for-%s.%s.svc/nova_cell2?read_default_file=/etc/my.cnf",
						cell2Account.Spec.UserName, cell2Secret.Data[mariadbv1.DatabasePasswordSelector],
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
						"[database]\nconnection = mysql+pymysql://%s:%s@hostname-for-%s.%s.svc/nova_cell2?read_default_file=/etc/my.cnf",
						cell2Account.Spec.UserName, cell2Secret.Data[mariadbv1.DatabasePasswordSelector],
						cell2.MariaDBDatabaseName.Name, novaNames.Namespace)),
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

			th.SimulateStatefulSetReplicaReady(cell2.ConductorStatefulSetName)
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
			mariadb.SimulateMariaDBAccountCompleted(cell2.MariaDBAccountName)
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
			mariadb.SimulateMariaDBAccountCompleted(novaNames.APIMariaDBDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell0.MariaDBAccountName)
			infra.SimulateTransportURLReady(cell0.TransportURLName)
			th.SimulateJobSuccess(cell0.DBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)

			// Simulate that cell1 DB sync failed and do not simulate
			// cell2 DB creation success so that will be in Creating state.
			mariadb.SimulateMariaDBDatabaseCompleted(cell1.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell1.MariaDBAccountName)
			infra.SimulateTransportURLReady(cell1.TransportURLName)
			th.SimulateJobFailure(cell1.DBSyncJobName)

			// NovaAPI is still created
			GetNovaAPI(novaNames.APIName)
			th.SimulateStatefulSetReplicaReady(novaNames.APIStatefulSetName)
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
			mariadb.SimulateMariaDBAccountCompleted(novaNames.APIMariaDBDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell0.MariaDBAccountName)
			mariadb.SimulateMariaDBDatabaseCompleted(cell1.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell1.MariaDBAccountName)
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
			cell0Template["cellDatabaseAccount"] = cell0.MariaDBAccountName.Name
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
			cell1Template["cellDatabaseAccount"] = cell1.MariaDBAccountName.Name
			cell1Template["cellMessageBusInstance"] = cell0.TransportURLName.Name
			cell1Template["hasAPIAccess"] = true

			spec["cellTemplates"] = map[string]interface{}{
				"cell0": cell0Template,
				"cell1": cell1Template,
			}
			spec["apiDatabaseInstance"] = novaNames.APIMariaDBDatabaseName.Name
			spec["apiMessageBusInstance"] = cell0.TransportURLName.Name

			DeferCleanup(th.DeleteInstance, CreateNova(novaNames.NovaName, spec))
			memcachedSpec := infra.GetDefaultMemcachedSpec()

			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(novaNames.NovaName.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMemcachedReady(novaNames.MemcachedNamespace)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(novaNames.NovaName.Namespace))
			keystone.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)

		})
		It("cell0 becomes ready with 0 conductor replicas and the rest of nova is deployed", func() {
			mariadb.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(novaNames.APIMariaDBDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell0.MariaDBAccountName)
			mariadb.SimulateMariaDBDatabaseCompleted(cell1.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell1.MariaDBAccountName)
			infra.SimulateTransportURLReady(cell0.TransportURLName)
			infra.SimulateTransportURLReady(cell1.TransportURLName)

			// We requested 0 replicas from the cell0 conductor so the
			// conductor is ready even if 0 replicas is running but all
			// the necessary steps, i.e. db-sync is run successfully
			th.SimulateJobSuccess(cell0.DBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
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

			SimulateReadyOfNovaTopServices()
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
			cell0Template["cellDatabaseAccount"] = cell0.MariaDBAccountName.Name

			cell1Template := GetDefaultNovaCellTemplate()
			cell1Template["cellDatabaseInstance"] = cell1.MariaDBDatabaseName.Name
			cell1Template["cellDatabaseAccount"] = cell1.MariaDBAccountName.Name
			cell1Template["cellMessageBusInstance"] = cell1.TransportURLName.Name

			spec["cellTemplates"] = map[string]interface{}{
				"cell0": cell0Template,
				"cell1": cell1Template,
			}
			spec["apiDatabaseInstance"] = novaNames.APIMariaDBDatabaseName.Name
			spec["apiMessageBusInstance"] = cell0.TransportURLName.Name

			DeferCleanup(th.DeleteInstance, CreateNova(novaNames.NovaName, spec))
			memcachedSpec := infra.GetDefaultMemcachedSpec()

			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(novaNames.NovaName.Namespace, MemcachedInstance, memcachedSpec))
			infra.SimulateMemcachedReady(novaNames.MemcachedNamespace)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(novaNames.Namespace))
			keystone.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
		})

		It("waits for cell0 DB to be created", func() {
			mariadb.SimulateMariaDBDatabaseCompleted(cell1.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell1.MariaDBAccountName)
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
			novaSecret := th.CreateSecret(
				types.NamespacedName{Namespace: novaNames.NovaName.Namespace, Name: SecretName},
				map[string][]byte{
					"NovaPassword":                         []byte("service-password"),
					"MetadataSecret":                       []byte("metadata-secret"),
					"MetadataCellsSecret" + cell1.CellName: []byte("metadata-secret-cell1"),
				},
			)
			DeferCleanup(k8sClient.Delete, ctx, novaSecret)
			DeferCleanup(k8sClient.Delete, ctx, CreateNovaMessageBusSecret(cell0))
			DeferCleanup(k8sClient.Delete, ctx, CreateNovaMessageBusSecret(cell1))

			serviceSpec := corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 3306}}}
			DeferCleanup(mariadb.DeleteDBService, mariadb.CreateDBService(novaNames.APIMariaDBDatabaseName.Namespace, novaNames.APIMariaDBDatabaseName.Name, serviceSpec))
			DeferCleanup(mariadb.DeleteDBService, mariadb.CreateDBService(cell0.MariaDBDatabaseName.Namespace, cell0.MariaDBDatabaseName.Name, serviceSpec))
			DeferCleanup(mariadb.DeleteDBService, mariadb.CreateDBService(cell1.MariaDBDatabaseName.Namespace, cell1.MariaDBDatabaseName.Name, serviceSpec))

			spec := GetDefaultNovaSpec()
			cell0Template := GetDefaultNovaCellTemplate()
			cell0Template["cellDatabaseInstance"] = cell0.MariaDBDatabaseName.Name
			cell0Template["cellDatabaseAccount"] = cell0.MariaDBAccountName.Name

			cell1Template := GetDefaultNovaCellTemplate()
			cell1Template["cellDatabaseInstance"] = cell1.MariaDBDatabaseName.Name
			cell1Template["cellDatabaseAccount"] = cell1.MariaDBAccountName.Name
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
		It("cell0 becomes ready without metadata and the rest of nova is deployed", func() {
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

			cell1Secret := th.GetSecret(cell1.InternalCellSecretName)
			Expect(cell1Secret.Data).To(
				HaveKeyWithValue(controllers.MetadataSecretSelector, []byte("metadata-secret-cell1")))
			cell0Secret := th.GetSecret(cell0.InternalCellSecretName)
			Expect(cell0Secret.Data).NotTo(
				HaveKeyWithValue(controllers.MetadataSecretSelector, []byte("metadata-secret")))
			Expect(cell0Secret.Data).NotTo(
				HaveKeyWithValue(controllers.MetadataSecretSelector, []byte("metadata-secret-cell1")))
			configDataMap := th.GetSecret(cell1.MetadataConfigDataName)
			Expect(configDataMap).ShouldNot(BeNil())
			Expect(configDataMap.Data).Should(HaveKey("httpd.conf"))
			Expect(configDataMap.Data).Should(HaveKey("ssl.conf"))
			configData := string(configDataMap.Data["httpd.conf"])
			Expect(configData).Should(
				ContainSubstring("TimeOut 60"))

		})
	})
})

var _ = Describe("Nova multi cell deletion", func() {
	BeforeEach(func() {
		CreateNovaWithNCellsAndEnsureReady(4, &novaNames)
	})

	When("Nova CR instance is created with 4 cells", func() {
		It("delete cell2 and cell3, verify for cell2", func() {

			nova := GetNova(novaNames.NovaName)
			Expect(nova.Status.RegisteredCells).To(HaveKey(cell0.CellCRName.Name), "cell0 is not in the RegisteredCells", nova.Status.RegisteredCells)
			Expect(nova.Status.RegisteredCells).To(HaveKey(cell1.CellCRName.Name), "cell1 is not in the RegisteredCells", nova.Status.RegisteredCells)
			Expect(nova.Status.RegisteredCells).To(HaveKey(cell2.CellCRName.Name), "cell2 is not in the RegisteredCells", nova.Status.RegisteredCells)
			Expect(nova.Status.RegisteredCells).To(HaveKey(cell3.CellCRName.Name), "cell3 is not in the RegisteredCells", nova.Status.RegisteredCells)

			cell2Account := mariadb.GetMariaDBAccount(cell2.MariaDBAccountName)
			cell2Account.Spec.Secret = ""
			Expect(k8sClient.Update(ctx, cell2Account)).To(Succeed())

			Eventually(func(g Gomega) {
				// remove from cells CR
				nova := GetNova(novaNames.NovaName)
				delete(nova.Spec.CellTemplates, "cell2")
				delete(nova.Spec.CellTemplates, "cell3")
				g.Expect(k8sClient.Update(ctx, nova)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			th.ExpectConditionWithDetails(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaCellsDeletionCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NovaCells deletion in progress: cell2, cell3",
			)
			th.SimulateJobSuccess(cell2.CellDeleteJobName)
			th.SimulateJobSuccess(cell3.CellDeleteJobName)
			Eventually(func(g Gomega) {
				nova := GetNova(novaNames.NovaName)
				g.Expect(nova.Status.RegisteredCells).To(HaveKey(cell2.CellCRName.Name))
				g.Expect(nova.Status.RegisteredCells).NotTo(HaveKey(cell3.CellCRName.Name))
			}, timeout, interval).Should(Succeed())

			GetNovaCell(cell2.CellCRName)
			NovaCellNotExists(cell3.CellCRName)
			th.ExpectConditionWithDetails(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaCellsDeletionCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NovaCells deletion in progress: cell2",
			)

		})

		It("retrieve sorted cells", func() {
			cellList := &novav1.NovaCellList{
				Items: []novav1.NovaCell{
					{ObjectMeta: metav1.ObjectMeta{Name: "cell-3"}},
					{ObjectMeta: metav1.ObjectMeta{Name: "cell-2"}},
					{ObjectMeta: metav1.ObjectMeta{Name: "cell-1"}},
				},
			}

			expectedList := []string{
				"cell-1",
				"cell-2",
				"cell-3",
			}

			controllers.SortNovaCellListByName(cellList)

			actualList := []string{}
			for _, cell := range cellList.Items {
				actualList = append(actualList, cell.ObjectMeta.Name)
			}

			Expect(actualList).To(Equal(expectedList))
		})
	})

})
