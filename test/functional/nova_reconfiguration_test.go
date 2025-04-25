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
	"time"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo/v2" //revive:disable:dot-imports
	. "github.com/onsi/gomega"    //revive:disable:dot-imports
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"

	//revive:disable-next-line:dot-imports
	. "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"

	rabbitmqv1 "github.com/openstack-k8s-operators/infra-operator/apis/rabbitmq/v1beta1"
	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	mariadbv1 "github.com/openstack-k8s-operators/mariadb-operator/api/v1beta1"

	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
)

func CreateNovaWith3CellsAndEnsureReady(novaNames NovaNames) {
	cell0 := novaNames.Cells["cell0"]
	cell1 := novaNames.Cells["cell1"]
	cell2 := novaNames.Cells["cell2"]

	DeferCleanup(k8sClient.Delete, ctx, CreateNovaSecret(novaNames.NovaName.Namespace, SecretName))
	DeferCleanup(k8sClient.Delete, ctx, CreateNovaMessageBusSecret(cell0))
	DeferCleanup(k8sClient.Delete, ctx, CreateNovaMessageBusSecret(cell1))
	DeferCleanup(k8sClient.Delete, ctx, CreateNovaMessageBusSecret(cell2))

	serviceSpec := corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 3306}}}
	DeferCleanup(
		mariadb.DeleteDBService,
		mariadb.CreateDBService(novaNames.APIMariaDBDatabaseName.Namespace, novaNames.APIMariaDBDatabaseName.Name, serviceSpec))
	DeferCleanup(mariadb.DeleteDBService, mariadb.CreateDBService(cell0.MariaDBDatabaseName.Namespace, cell0.MariaDBDatabaseName.Name, serviceSpec))
	DeferCleanup(mariadb.DeleteDBService, mariadb.CreateDBService(cell1.MariaDBDatabaseName.Namespace, cell1.MariaDBDatabaseName.Name, serviceSpec))
	DeferCleanup(mariadb.DeleteDBService, mariadb.CreateDBService(cell2.MariaDBDatabaseName.Namespace, cell2.MariaDBDatabaseName.Name, serviceSpec))

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
	DeferCleanup(th.DeleteInstance, cell1Account)
	DeferCleanup(
		th.DeleteSecret,
		types.NamespacedName{Name: cell1Secret.Name, Namespace: cell1Secret.Namespace})

	cell2Account, cell2Secret := mariadb.CreateMariaDBAccountAndSecret(
		cell2.MariaDBAccountName, mariadbv1.MariaDBAccountSpec{})
	DeferCleanup(k8sClient.Delete, ctx, cell2Account)
	DeferCleanup(k8sClient.Delete, ctx, cell2Secret)

	spec := GetDefaultNovaSpec()
	cell0Template := GetDefaultNovaCellTemplate()
	cell0Template["cellDatabaseInstance"] = cell0.MariaDBDatabaseName.Name
	cell0Template["cellDatabaseAccount"] = cell0Account.Name

	cell1Template := GetDefaultNovaCellTemplate()
	cell1Template["cellDatabaseInstance"] = cell1.MariaDBDatabaseName.Name
	cell1Template["cellDatabaseAccount"] = cell1Account.Name
	cell1Template["cellMessageBusInstance"] = cell1.TransportURLName.Name
	cell1Template["novaComputeTemplates"] = map[string]interface{}{
		ironicComputeName: GetDefaultNovaComputeTemplate(),
	}

	cell2Template := GetDefaultNovaCellTemplate()
	cell2Template["cellDatabaseInstance"] = cell2.MariaDBDatabaseName.Name
	cell2Template["cellDatabaseAccount"] = cell2Account.Name
	cell2Template["cellMessageBusInstance"] = cell2.TransportURLName.Name
	cell2Template["hasAPIAccess"] = false

	spec["cellTemplates"] = map[string]interface{}{
		"cell0": cell0Template,
		"cell1": cell1Template,
		"cell2": cell2Template,
	}
	spec["apiDatabaseInstance"] = novaNames.APIMariaDBDatabaseName.Name
	spec["apiMessageBusInstance"] = cell0.TransportURLName.Name

	DeferCleanup(th.DeleteInstance, CreateNova(novaNames.NovaName, spec))
	DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(novaNames.NovaName.Namespace))
	memcachedSpec := GetDefaultMemcachedSpec()

	DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(novaNames.NovaName.Namespace, MemcachedInstance, memcachedSpec))
	infra.SimulateMemcachedReady(novaNames.MemcachedNamespace)
	keystone.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
	// END of common logic with Nova multi cell test

	mariadb.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
	mariadb.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
	mariadb.SimulateMariaDBDatabaseCompleted(cell1.MariaDBDatabaseName)
	mariadb.SimulateMariaDBDatabaseCompleted(cell2.MariaDBDatabaseName)

	mariadb.SimulateMariaDBAccountCompleted(novaNames.APIMariaDBDatabaseAccount)
	mariadb.SimulateMariaDBAccountCompleted(cell0.MariaDBAccountName)
	mariadb.SimulateMariaDBAccountCompleted(cell1.MariaDBAccountName)
	mariadb.SimulateMariaDBAccountCompleted(cell2.MariaDBAccountName)

	infra.SimulateTransportURLReady(cell0.TransportURLName)
	infra.SimulateTransportURLReady(cell1.TransportURLName)
	infra.SimulateTransportURLReady(cell2.TransportURLName)

	th.SimulateJobSuccess(cell0.DBSyncJobName)
	th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
	th.SimulateJobSuccess(cell0.CellMappingJobName)

	th.SimulateStatefulSetReplicaReady(cell1.NoVNCProxyStatefulSetName)
	th.SimulateJobSuccess(cell1.DBSyncJobName)
	th.SimulateStatefulSetReplicaReady(cell1.ConductorStatefulSetName)
	th.SimulateStatefulSetReplicaReady(cell1.NovaComputeStatefulSetName)
	th.SimulateJobSuccess(cell1.CellMappingJobName)
	th.SimulateJobSuccess(cell1.HostDiscoveryJobName)

	th.SimulateStatefulSetReplicaReady(cell2.NoVNCProxyStatefulSetName)
	th.SimulateJobSuccess(cell2.DBSyncJobName)
	th.SimulateStatefulSetReplicaReady(cell2.ConductorStatefulSetName)
	th.SimulateJobSuccess(cell2.CellMappingJobName)

	th.ExpectCondition(
		novaNames.NovaName,
		ConditionGetterFunc(NovaConditionGetter),
		novav1.NovaAllCellsReadyCondition,
		corev1.ConditionTrue,
	)
	SimulateReadyOfNovaTopServices()
	th.ExpectCondition(
		novaNames.NovaName,
		ConditionGetterFunc(NovaConditionGetter),
		condition.ReadyCondition,
		corev1.ConditionTrue,
	)
}

var _ = Describe("Nova reconfiguration", func() {
	BeforeEach(func() {
		// Uncomment this if you need the full output in the logs from gomega
		// matchers
		// format.MaxLength = 0

		CreateNovaWith3CellsAndEnsureReady(novaNames)
	})
	When("cell1 is deleted", func() {
		It("cell cr is deleted", func() {
			// Cells are created so NovaCellsDeletionCondition is set to
			// True because there is no cell to delete
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaCellsDeletionCondition,
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

				delete(nova.Spec.CellTemplates, "cell1")

				g.Expect(k8sClient.Update(ctx, nova)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				mappingJob := th.GetJob(cell1.CellDeleteJobName)
				newJobInputHash := GetEnvVarValue(
					mappingJob.Spec.Template.Spec.Containers[0].Env, "INPUT_HASH", "")
				g.Expect(newJobInputHash).NotTo(BeNil())
			}, timeout, interval).Should(Succeed())

			// Check that the cell status is not deleted
			Consistently(func(g Gomega) {
				nova := GetNova(novaNames.NovaName)
				g.Expect(nova.Status.RegisteredCells).To(HaveKey(cell1.CellCRName.Name))
			}, timeout, interval).Should(Succeed())

			//
			th.ExpectConditionWithDetails(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaCellsDeletionCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NovaCells deletion in progress: cell1",
			)
			// Simulate the cell delete job success
			th.SimulateJobSuccess(cell1.CellDeleteJobName)
			Eventually(func(g Gomega) {
				nova := GetNova(novaNames.NovaName)
				g.Expect(nova.Status.RegisteredCells).NotTo(HaveKey(cell1.CellCRName.Name))
			}, timeout, interval).Should(Succeed())

			NovaCellNotExists(cell1.CellCRName)

			th.AssertSecretDoesNotExist(cell1.InternalCellSecretName)

			Eventually(func(g Gomega) {
				instance := &rabbitmqv1.TransportURL{}
				err := k8sClient.Get(ctx, cell1.TransportURLName, instance)
				g.Expect(k8s_errors.IsNotFound(err)).To(BeTrue())
			}, timeout, interval).Should(Succeed())
			th.ExpectCondition(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaCellsDeletionCondition,
				corev1.ConditionTrue,
			)
		})
	})
	When("cell0 conductor replicas is set to 0", func() {
		It("sets the deployment replicas to 0", func() {
			cell0DeploymentName := cell0.ConductorStatefulSetName

			deployment := th.GetStatefulSet(cell0DeploymentName)
			one := int32(1)
			Expect(deployment.Spec.Replicas).To(Equal(&one))

			// We need this big Eventually block because the Update() call might
			// return a Conflict and then we have to retry by re-reading Nova,
			// and updating the Replicas again.
			Eventually(func(g Gomega) {
				nova := GetNova(novaNames.NovaName)

				// TODO(gibi): Is there a simpler way to achieve this update
				// in golang?
				zero := int32(0)
				cell0 := nova.Spec.CellTemplates["cell0"]
				(&cell0).ConductorServiceTemplate.Replicas = &zero
				nova.Spec.CellTemplates["cell0"] = cell0

				g.Expect(k8sClient.Update(ctx, nova)).To(Succeed())

				deployment = &appsv1.StatefulSet{}
				g.Expect(k8sClient.Get(ctx, cell0DeploymentName, deployment)).Should(Succeed())
				g.Expect(deployment.Spec.Replicas).To(Equal(&zero))
			}, timeout, interval).Should(Succeed())
		})
	})
	When("networkAttachment is added to a conductor while the definition is missing", func() {
		It("applies new NetworkAttachments configuration to that Conductor", func() {
			Eventually(func(g Gomega) {
				nova := GetNova(novaNames.NovaName)

				cell1 := nova.Spec.CellTemplates["cell1"]
				attachments := cell1.ConductorServiceTemplate.NetworkAttachments
				attachments = append(attachments, "internalapi")
				(&cell1).ConductorServiceTemplate.NetworkAttachments = attachments
				nova.Spec.CellTemplates["cell1"] = cell1

				g.Expect(k8sClient.Update(ctx, nova)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			th.ExpectConditionWithDetails(
				cell1.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NetworkAttachment resources missing: internalapi",
			)
			th.ExpectConditionWithDetails(
				cell1.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NetworkAttachment resources missing: internalapi",
			)

			th.ExpectConditionWithDetails(
				cell1.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaConductorReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NetworkAttachment resources missing: internalapi",
			)
			th.ExpectConditionWithDetails(
				cell1.CellCRName,
				ConditionGetterFunc(NovaCellConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NetworkAttachment resources missing: internalapi",
			)

			th.ExpectConditionWithDetails(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NovaCell cell1 is not Ready",
			)
			th.ExpectConditionWithDetails(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NovaCell cell1 is not Ready",
			)

			DeferCleanup(th.DeleteInstance, th.CreateNetworkAttachmentDefinition(novaNames.InternalAPINetworkNADName))

			th.ExpectConditionWithDetails(
				novaNames.NovaName,
				ConditionGetterFunc(NovaConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NovaCell cell1 is not Ready",
			)

			th.SimulateStatefulSetReplicaReadyWithPods(
				cell1.ConductorStatefulSetName,
				map[string][]string{novaNames.NovaName.Namespace + "/internalapi": {"10.0.0.1"}},
			)
			th.ExpectCondition(
				cell1.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})

	When("Nova CR instance is created with topology that is later removed", func() {
		var topologySpec map[string]interface{}
		var defaultTopologyRef topologyv1.TopoRef
		BeforeEach(func() {
			// Create Test Topologies
			for _, t := range novaNames.NovaTopologies {
				// Build the topology Spec
				topologySpec, _ = GetSampleTopologySpec(t.Name)
				infra.CreateTopology(t, topologySpec)
			}
			keystone.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
			mariadb.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(novaNames.APIMariaDBDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(cell0.MariaDBAccountName)
			infra.SimulateTransportURLReady(cell0.TransportURLName)
		})
		It("updates topologyRef", func() {
			// topologyRef is added after the default Nova CR is created
			Eventually(func(g Gomega) {
				nova := GetNova(novaNames.NovaName)
				defaultTopologyRef = topologyv1.TopoRef{
					Name:      novaNames.NovaTopologies[0].Name,
					Namespace: novaNames.Namespace,
				}
				nova.Spec.TopologyRef = &defaultTopologyRef
				g.Expect(k8sClient.Update(ctx, nova)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			SimulateReadyOfNovaTopServices()
			th.SimulateJobSuccess(cell0.DBSyncJobName)
			th.SimulateJobSuccess(cell1.DBSyncJobName)
			th.SimulateJobSuccess(cell2.DBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)

			for _, cell := range []types.NamespacedName{cell0.ConductorName, cell1.ConductorName, cell2.ConductorName} {
				Eventually(func(g Gomega) {
					cond := GetNovaConductor(cell)
					g.Expect(cond.Status.LastAppliedTopology).ToNot(BeNil())
					g.Expect(cond.Status.LastAppliedTopology).To(Equal(&defaultTopologyRef))
				}, timeout, interval).Should(Succeed())
			}

			Eventually(func(g Gomega) {
				api := GetNovaAPI(novaNames.APIName)
				g.Expect(api.Status.LastAppliedTopology).ToNot(BeNil())
				g.Expect(api.Status.LastAppliedTopology).To(Equal(&defaultTopologyRef))
			}, timeout, interval).Should(Succeed())

			th.ExpectCondition(
				novaNames.APIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.TopologyReadyCondition,
				corev1.ConditionTrue,
			)

			Eventually(func(g Gomega) {
				sch := GetNovaScheduler(novaNames.SchedulerName)
				g.Expect(sch.Status.LastAppliedTopology).ToNot(BeNil())
				g.Expect(sch.Status.LastAppliedTopology).To(Equal(&defaultTopologyRef))
			}, timeout, interval).Should(Succeed())

			th.ExpectCondition(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.TopologyReadyCondition,
				corev1.ConditionTrue,
			)

			Eventually(func(g Gomega) {
				metadata := GetNovaMetadata(novaNames.MetadataName)
				g.Expect(metadata.Status.LastAppliedTopology).ToNot(BeNil())
				g.Expect(metadata.Status.LastAppliedTopology).To(Equal(&defaultTopologyRef))
			}, timeout, interval).Should(Succeed())

			th.ExpectCondition(
				novaNames.MetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.TopologyReadyCondition,
				corev1.ConditionTrue,
			)
			// Get the referenced topology
			tp := infra.GetTopology(types.NamespacedName{
				Name:      defaultTopologyRef.Name,
				Namespace: defaultTopologyRef.Namespace,
			})
			// Check finalizers for top-level resources
			for topology, component := range map[string]types.NamespacedName{
				novaNames.NovaTopologies[1].Name: novaNames.APIName,
				novaNames.NovaTopologies[2].Name: novaNames.SchedulerName,
				novaNames.NovaTopologies[3].Name: novaNames.MetadataName,
			} {
				Eventually(func(g Gomega) {
					g.Expect(tp.GetFinalizers()).To(ContainElement(
						fmt.Sprintf("openstack.org/%s-%s", topology, component.Name)))
				}, timeout, interval).Should(Succeed())
			}
			// Check finalizers for cell Conductors
			for _, cell := range []types.NamespacedName{cell0.ConductorName, cell1.ConductorName, cell2.ConductorName} {
				Eventually(func(g Gomega) {
					g.Expect(tp.GetFinalizers()).To(ContainElement(
						fmt.Sprintf("openstack.org/novaconductor-%s", cell.Name)))
				}, timeout, interval).Should(Succeed())
			}
		})
		It("overrides topology when the reference changes", func() {
			Eventually(func(g Gomega) {
				nova := GetNova(novaNames.NovaName)
				//Patch API Spec
				newAPI := GetNovaAPI(novaNames.APIName)
				newAPI.Spec.TopologyRef = &topologyv1.TopoRef{
					Name:      novaNames.NovaTopologies[1].Name,
					Namespace: novaNames.Namespace,
				}
				nova.Spec.APIServiceTemplate = novav1.NovaAPITemplate{
					TopologyRef: newAPI.Spec.TopologyRef,
				}
				//Patch Scheduler Spec
				newSch := GetNovaScheduler(novaNames.SchedulerName)
				newSch.Spec.TopologyRef = &topologyv1.TopoRef{
					Name:      novaNames.NovaTopologies[2].Name,
					Namespace: novaNames.Namespace,
				}
				nova.Spec.SchedulerServiceTemplate = novav1.NovaSchedulerTemplate{
					TopologyRef: newSch.Spec.TopologyRef,
				}
				//Patch Metadata Spec
				newMeta := GetNovaMetadata(novaNames.MetadataName)
				newMeta.Spec.TopologyRef = &topologyv1.TopoRef{
					Name:      novaNames.NovaTopologies[3].Name,
					Namespace: novaNames.Namespace,
				}
				nova.Spec.MetadataServiceTemplate = novav1.NovaMetadataTemplate{
					TopologyRef: newMeta.Spec.TopologyRef,
				}
				//Patch cell0 template
				c0 := nova.Spec.CellTemplates["cell0"]
				c0.TopologyRef = &topologyv1.TopoRef{
					Name:      novaNames.NovaTopologies[4].Name,
					Namespace: novaNames.Namespace,
				}
				nova.Spec.CellTemplates["cell0"] = c0
				g.Expect(k8sClient.Update(ctx, nova)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			SimulateReadyOfNovaTopServices()
			th.SimulateJobSuccess(cell0.DBSyncJobName)
			th.SimulateJobSuccess(cell1.DBSyncJobName)
			th.SimulateJobSuccess(cell2.DBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)

			Eventually(func(g Gomega) {
				api := GetNovaAPI(novaNames.APIName)
				g.Expect(api.Status.LastAppliedTopology).ToNot(BeNil())
				g.Expect(api.Status.LastAppliedTopology.Name).To(Equal(novaNames.NovaTopologies[1].Name))
			}, timeout, interval).Should(Succeed())

			th.ExpectCondition(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.TopologyReadyCondition,
				corev1.ConditionTrue,
			)
			Eventually(func(g Gomega) {
				sch := GetNovaScheduler(novaNames.SchedulerName)
				g.Expect(sch.Status.LastAppliedTopology).ToNot(BeNil())
				g.Expect(sch.Status.LastAppliedTopology.Name).To(Equal(novaNames.NovaTopologies[2].Name))
			}, timeout, interval).Should(Succeed())

			th.ExpectCondition(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.TopologyReadyCondition,
				corev1.ConditionTrue,
			)

			Eventually(func(g Gomega) {
				metadata := GetNovaMetadata(novaNames.MetadataName)
				g.Expect(metadata.Status.LastAppliedTopology).ToNot(BeNil())
				g.Expect(metadata.Status.LastAppliedTopology.Name).To(Equal(novaNames.NovaTopologies[3].Name))
			}, timeout, interval).Should(Succeed())

			th.ExpectCondition(
				novaNames.MetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.TopologyReadyCondition,
				corev1.ConditionTrue,
			)

			Eventually(func(g Gomega) {
				cond := GetNovaConductor(cell0.ConductorName)
				g.Expect(cond.Status.LastAppliedTopology).ToNot(BeNil())
				g.Expect(cond.Status.LastAppliedTopology.Name).To(Equal(novaNames.NovaTopologies[4].Name))
			}, timeout, interval).Should(Succeed())

			// Check the resulting StatefulSets
			for index, component := range []types.NamespacedName{
				novaNames.APIName,
				novaNames.SchedulerName,
				novaNames.MetadataName,
				cell0.ConductorName,
			} {
				Eventually(func(g Gomega) {
					_, componentTopologySpec := GetSampleTopologySpec(novaNames.NovaTopologies[index+1].Name)
					ss := th.GetStatefulSet(component)
					podTemplate := ss.Spec.Template.Spec
					g.Expect(podTemplate.TopologySpreadConstraints).To(Equal(componentTopologySpec))
				}, timeout, interval).Should(Succeed())
			}

			// Check the updated finalizers for top-level resources
			previousTopology := infra.GetTopology(types.NamespacedName{
				Name:      novaNames.NovaTopologies[0].Name,
				Namespace: novaNames.Namespace,
			})
			for topology, component := range map[string]types.NamespacedName{
				novaNames.NovaTopologies[1].Name: novaNames.APIName,
				novaNames.NovaTopologies[2].Name: novaNames.SchedulerName,
				novaNames.NovaTopologies[3].Name: novaNames.MetadataName,
			} {
				Eventually(func(g Gomega) {
					tp := infra.GetTopology(types.NamespacedName{
						Name:      topology,
						Namespace: novaNames.Namespace,
					})
					finalizers := tp.GetFinalizers()
					g.Expect(finalizers).To(ContainElement(
						fmt.Sprintf("openstack.org/%s-%s", tp.Name, component.Name)))
					// The finalizer has been removed from the previously referenced
					// topology
					g.Expect(previousTopology.GetFinalizers()).ToNot(ContainElement(
						fmt.Sprintf("openstack.org/%s-%s", tp.Name, component.Name)))
				}, timeout, interval).Should(Succeed())
			}
			// Check finalizers for cell Conductors
			Eventually(func(g Gomega) {
				cell0Topology := infra.GetTopology(types.NamespacedName{
					Name:      novaNames.NovaTopologies[4].Name,
					Namespace: novaNames.NovaTopologies[4].Namespace,
				})
				g.Expect(cell0Topology.GetFinalizers()).To(ContainElement(
					fmt.Sprintf("openstack.org/novaconductor-%s", cell0.ConductorName.Name)))
				// The finalizer is not present in the previously referenced
				// topology
				g.Expect(previousTopology.GetFinalizers()).ToNot(ContainElement(
					fmt.Sprintf("openstack.org/novaconductor-%s", cell0.ConductorName.Name)))
			}, timeout, interval).Should(Succeed())
		})
		It("removes topologyRef from the spec", func() {
			Eventually(func(g Gomega) {
				nova := GetNova(novaNames.NovaName)
				// Remove the TopologyRef from the existing Nova .Spec
				nova.Spec.TopologyRef = nil
				g.Expect(k8sClient.Update(ctx, nova)).To(Succeed())
			}, timeout, interval).Should(Succeed())
			Eventually(func(g Gomega) {
				api := GetNovaAPI(novaNames.APIName)
				g.Expect(api.Status.LastAppliedTopology).Should(BeNil())
				scheduler := GetNovaScheduler(novaNames.SchedulerName)
				g.Expect(scheduler.Status.LastAppliedTopology).Should(BeNil())
				metadata := GetNovaMetadata(novaNames.MetadataName)
				g.Expect(metadata.Status.LastAppliedTopology).Should(BeNil())
				cond := GetNovaConductor(cell0.ConductorName)
				g.Expect(cond.Status.LastAppliedTopology).Should(BeNil())
			}, timeout, interval).Should(Succeed())

			for _, component := range []types.NamespacedName{
				novaNames.APIName,
				novaNames.SchedulerName,
				novaNames.MetadataName,
				cell0.ConductorName,
			} {
				Eventually(func(g Gomega) {
					ss := th.GetStatefulSet(component)
					podTemplate := ss.Spec.Template.Spec
					g.Expect(podTemplate.TopologySpreadConstraints).To(BeNil())
					// Default Pod AntiAffinity is applied
					g.Expect(podTemplate.Affinity).ToNot(BeNil())
				}, timeout, interval).Should(Succeed())
			}
		})
		It("removes all finalizers from the referenced topologies", func() {
			for _, topologyName := range novaNames.NovaTopologies {
				Eventually(func(g Gomega) {
					tp := infra.GetTopology(topologyName)
					finalizers := tp.GetFinalizers()
					g.Expect(finalizers).To(BeEmpty())
				}, timeout, interval).Should(Succeed())
			}
		})
	})

	When("global NodeSelector is set", func() {
		DescribeTable("it is propagated to", func(serviceNameFunc func() types.NamespacedName) {
			// We need this big Eventually block because the Update() call might
			// return a Conflict and then we have to retry by re-reading Nova,
			// and updating it again.
			Eventually(func(g Gomega) {
				nova := GetNova(novaNames.NovaName)

				newSelector := map[string]string{"foo": "bar"}
				nova.Spec.NodeSelector = &newSelector

				g.Expect(k8sClient.Update(ctx, nova)).To(Succeed())
				SimulateReadyOfNovaTopServices()
				th.SimulateJobSuccess(cell1.DBSyncJobName)
				th.SimulateJobSuccess(cell2.DBSyncJobName)

				serviceDeployment := th.GetStatefulSet(serviceNameFunc())
				g.Expect(serviceDeployment.Spec.Template.Spec.NodeSelector).To(Equal(newSelector))

				g.Expect(th.GetJob(cell0.DBSyncJobName).Spec.Template.Spec.NodeSelector).To(Equal(newSelector))
				g.Expect(th.GetJob(cell1.DBSyncJobName).Spec.Template.Spec.NodeSelector).To(Equal(newSelector))
				g.Expect(th.GetJob(cell2.DBSyncJobName).Spec.Template.Spec.NodeSelector).To(Equal(newSelector))

			}, timeout, interval).Should(Succeed())

			// Now reset it back to empty and see that it is propagates too
			Eventually(func(g Gomega) {
				nova := GetNova(novaNames.NovaName)

				newSelector := map[string]string{}
				nova.Spec.NodeSelector = &newSelector

				g.Expect(k8sClient.Update(ctx, nova)).To(Succeed())

				serviceDeploymentName := serviceNameFunc()
				th.SimulateJobSuccess(cell0.DBSyncJobName)
				th.SimulateJobSuccess(cell1.DBSyncJobName)
				th.SimulateJobSuccess(cell2.DBSyncJobName)
				th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
				th.SimulateStatefulSetReplicaReady(serviceDeploymentName)
				serviceDeployment := th.GetStatefulSet(serviceDeploymentName)
				g.Expect(serviceDeployment.Spec.Template.Spec.NodeSelector).To(BeNil())

				g.Expect(th.GetJob(cell0.DBSyncJobName).Spec.Template.Spec.NodeSelector).To(BeNil())
				g.Expect(th.GetJob(cell1.DBSyncJobName).Spec.Template.Spec.NodeSelector).To(BeNil())
				g.Expect(th.GetJob(cell2.DBSyncJobName).Spec.Template.Spec.NodeSelector).To(BeNil())
			}, timeout, interval).Should(Succeed())
		},
			Entry("the nova api pods",
				func() types.NamespacedName {
					return novaNames.APIName
				}),
			Entry("the nova scheduler pods", func() types.NamespacedName {
				return novaNames.SchedulerName
			}),
			Entry("the nova metadata pods", func() types.NamespacedName {
				return novaNames.MetadataName
			}),
			Entry("the nova cell0 conductor", func() types.NamespacedName {
				return cell0.ConductorStatefulSetName
			}),
			Entry("the nova cell1 conductor", func() types.NamespacedName {
				return cell1.ConductorStatefulSetName
			}),
			Entry("the nova cell2 conductor", func() types.NamespacedName {
				return cell2.ConductorStatefulSetName
			}),
		)

		It("does not override NodeSelector defined in the service template", func() {
			serviceSelector := map[string]string{"foo": "api"}
			conductorSelector := map[string]string{"foo": "conductor"}
			globalSelector := map[string]string{"foo": "global"}
			emptySelector := map[string]string{}

			// Set the service specific NodeSelector first
			Eventually(func(g Gomega) {
				nova := GetNova(novaNames.NovaName)

				nova.Spec.APIServiceTemplate.NodeSelector = &serviceSelector
				nova.Spec.MetadataServiceTemplate.NodeSelector = &serviceSelector
				nova.Spec.SchedulerServiceTemplate.NodeSelector = &serviceSelector
				for _, cell := range []string{"cell0", "cell1", "cell2"} {
					cellTemplate := nova.Spec.CellTemplates[cell]
					cellTemplate.ConductorServiceTemplate.NodeSelector = &conductorSelector
					if cell == "cell2" {
						cellTemplate.ConductorServiceTemplate.NodeSelector = &emptySelector
					}
					nova.Spec.CellTemplates[cell] = cellTemplate

				}
				g.Expect(k8sClient.Update(ctx, nova)).To(Succeed())
				th.SimulateJobSuccess(cell0.DBSyncJobName)
				th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
				th.SimulateJobSuccess(cell1.DBSyncJobName)
				th.SimulateStatefulSetReplicaReady(cell1.ConductorStatefulSetName)
				th.SimulateJobSuccess(cell2.DBSyncJobName)
				th.SimulateStatefulSetReplicaReady(cell2.ConductorStatefulSetName)

				apiDeployment := th.GetStatefulSet(novaNames.APIStatefulSetName)
				g.Expect(apiDeployment.Spec.Template.Spec.NodeSelector).To(Equal(serviceSelector))
				schedulerDeployment := th.GetStatefulSet(novaNames.SchedulerStatefulSetName)
				g.Expect(schedulerDeployment.Spec.Template.Spec.NodeSelector).To(Equal(serviceSelector))
				metadataDeployment := th.GetStatefulSet(novaNames.MetadataStatefulSetName)
				g.Expect(metadataDeployment.Spec.Template.Spec.NodeSelector).To(Equal(serviceSelector))

				conductorDeployment := th.GetStatefulSet(cell0.ConductorStatefulSetName)
				g.Expect(conductorDeployment.Spec.Template.Spec.NodeSelector).To(Equal(conductorSelector))
				conductorDeployment = th.GetStatefulSet(cell1.ConductorStatefulSetName)
				g.Expect(conductorDeployment.Spec.Template.Spec.NodeSelector).To(Equal(conductorSelector))
				conductorDeployment = th.GetStatefulSet(cell2.ConductorStatefulSetName)
				g.Expect(conductorDeployment.Spec.Template.Spec.NodeSelector).To(BeNil())

				g.Expect(th.GetJob(cell0.DBSyncJobName).Spec.Template.Spec.NodeSelector).To(Equal(conductorSelector))
				g.Expect(th.GetJob(cell1.DBSyncJobName).Spec.Template.Spec.NodeSelector).To(Equal(conductorSelector))
				g.Expect(th.GetJob(cell2.DBSyncJobName).Spec.Template.Spec.NodeSelector).To(BeNil())
			}, timeout, interval).Should(Succeed())
			SimulateReadyOfNovaTopServices()

			// Set the global NodeSelector and assert that it is propagated
			// except to the NovaService's
			Eventually(func(g Gomega) {
				nova := GetNova(novaNames.NovaName)
				nova.Spec.NodeSelector = &globalSelector

				g.Expect(k8sClient.Update(ctx, nova)).To(Succeed())

				// NovaService's deployment keeps it own selector
				apiDeployment := th.GetStatefulSet(novaNames.APIStatefulSetName)
				g.Expect(apiDeployment.Spec.Template.Spec.NodeSelector).To(Equal(serviceSelector))
				schedulerDeployment := th.GetStatefulSet(novaNames.SchedulerStatefulSetName)
				g.Expect(schedulerDeployment.Spec.Template.Spec.NodeSelector).To(Equal(serviceSelector))
				metadataDeployment := th.GetStatefulSet(novaNames.MetadataStatefulSetName)
				g.Expect(metadataDeployment.Spec.Template.Spec.NodeSelector).To(Equal(serviceSelector))

				// and cell conductors keep their own selector
				conductorDeployment := th.GetStatefulSet(cell0.ConductorStatefulSetName)
				g.Expect(conductorDeployment.Spec.Template.Spec.NodeSelector).To(Equal(conductorSelector))
				conductorDeployment = th.GetStatefulSet(cell1.ConductorStatefulSetName)
				g.Expect(conductorDeployment.Spec.Template.Spec.NodeSelector).To(Equal(conductorSelector))
				conductorDeployment = th.GetStatefulSet(cell2.ConductorStatefulSetName)
				g.Expect(conductorDeployment.Spec.Template.Spec.NodeSelector).To(BeNil())

				g.Expect(th.GetJob(cell0.DBSyncJobName).Spec.Template.Spec.NodeSelector).To(Equal(conductorSelector))
				g.Expect(th.GetJob(cell1.DBSyncJobName).Spec.Template.Spec.NodeSelector).To(Equal(conductorSelector))
				g.Expect(th.GetJob(cell2.DBSyncJobName).Spec.Template.Spec.NodeSelector).To(BeNil())
			}, timeout, interval).Should(Succeed())
		})
	})
	When("CellMessageBusInstance is reconfigured for a cell", func() {
		It("updates the cell, re-runs the cell mapping job and updates the cell hash", func() {
			mappingJob := th.GetJob(cell1.CellMappingJobName)
			oldJobInputHash := GetEnvVarValue(
				mappingJob.Spec.Template.Spec.Containers[0].Env, "INPUT_HASH", "")

			oldCell1Hash := GetNova(novaNames.NovaName).Status.RegisteredCells[cell1.CellCRName.Name]
			oldComputeConfigHash := GetNovaCell(cell1.CellCRName).Status.Hash[cell1.ComputeConfigSecretName.Name]

			Eventually(func(g Gomega) {
				nova := GetNova(novaNames.NovaName)

				cell1 := nova.Spec.CellTemplates["cell1"]
				cell1.CellMessageBusInstance = "alternate-mq-for-cell1"
				nova.Spec.CellTemplates["cell1"] = cell1

				g.Expect(k8sClient.Update(ctx, nova)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			// The new TransportURL will point to a new secret so we need to
			// simulate that is created by the infra-operator.
			s := th.CreateSecret(
				types.NamespacedName{Namespace: cell1.CellCRName.Namespace, Name: "alternate-mq-for-cell1-secret"},
				map[string][]byte{
					"transport_url": []byte("rabbit://alternate-mq-for-cell1/fake"),
				},
			)
			DeferCleanup(k8sClient.Delete, ctx, s)

			// Expect that nova controller updates the TransportURL to point to
			// the new rabbit cluster
			Eventually(func(g Gomega) {
				transport := infra.GetTransportURL(cell1.TransportURLName)
				g.Expect(transport.Spec.RabbitmqClusterName).To(Equal("alternate-mq-for-cell1"))
			}, timeout, interval).Should(Succeed())

			infra.SimulateTransportURLReady(cell1.TransportURLName)

			th.SimulateStatefulSetReplicaReady(cell1.NoVNCProxyStatefulSetName)
			// Expect that the NovaConductor config is updated with the new transport URL
			Eventually(func(g Gomega) {
				configDataMap := th.GetSecret(cell1.ConductorConfigDataName)
				g.Expect(configDataMap).ShouldNot(BeNil())
				g.Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
				configData := string(configDataMap.Data["01-nova.conf"])
				g.Expect(configData).Should(ContainSubstring("transport_url=rabbit://alternate-mq-for-cell1/fake"))
			}, timeout, interval).Should(Succeed())

			// As the NoVNCProxy config is updated its StatefulSet is also updated,
			// so the test needs to simulate that the new StatefulSet Generation is Ready
			th.SimulateStatefulSetReplicaReady(cell1.NoVNCProxyStatefulSetName)
			// Expect that the NovaNoVNCProxy config is updated with the new transport URL
			Eventually(func(g Gomega) {
				configDataMap := th.GetSecret(cell1.CellNoVNCProxyNameConfigDataName)
				g.Expect(configDataMap).ShouldNot(BeNil())
				g.Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
				configData := string(configDataMap.Data["01-nova.conf"])
				g.Expect(configData).Should(ContainSubstring("transport_url=rabbit://alternate-mq-for-cell1/fake"))
			}, timeout, interval).Should(Succeed())

			// Expect that the compute config is updated with the new transport URL
			Eventually(func(g Gomega) {
				configDataMap := th.GetSecret(cell1.ComputeConfigSecretName)
				g.Expect(configDataMap).ShouldNot(BeNil())
				g.Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
				configData := string(configDataMap.Data["01-nova.conf"])
				g.Expect(configData).Should(ContainSubstring("transport_url=rabbit://alternate-mq-for-cell1/fake"))
			}, timeout, interval).Should(Succeed())
			Expect(GetNovaCell(cell1.CellCRName).Status.Hash[cell1.ComputeConfigSecretName.Name]).NotTo(Equal(oldComputeConfigHash))
			// and therefore the statefulset is also updated with a new config
			// hash so the test needs to make the Generation of the StatefulSet
			// Ready
			th.SimulateStatefulSetReplicaReady(cell1.NovaComputeStatefulSetName)
			th.SimulateStatefulSetReplicaReady(cell1.ConductorStatefulSetName)
			// Expect that nova controller updates the mapping Job to re-run that
			// to update the CellMapping table in the nova_api DB.
			Eventually(func(g Gomega) {
				mappingJob := th.GetJob(cell1.CellMappingJobName)
				newJobInputHash := GetEnvVarValue(
					mappingJob.Spec.Template.Spec.Containers[0].Env, "INPUT_HASH", "")
				g.Expect(newJobInputHash).NotTo(Equal(oldJobInputHash))
			}, timeout, interval).Should(Succeed())

			th.SimulateJobSuccess(cell1.CellMappingJobName)

			// Expect that the new config results in a new cell1 hash
			Eventually(func(g Gomega) {
				newCell1Hash := GetNova(novaNames.NovaName).Status.RegisteredCells[cell1.CellCRName.Name]
				g.Expect(newCell1Hash).NotTo(Equal(oldCell1Hash))
			}, timeout, interval).Should(Succeed())

			// Expect that the compute config is updated with the new transport URL
			Eventually(func(g Gomega) {
				configDataMap := th.GetSecret(cell1.ComputeConfigSecretName)
				g.Expect(configDataMap).ShouldNot(BeNil())
				g.Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
				configData := string(configDataMap.Data["01-nova.conf"])
				g.Expect(configData).Should(ContainSubstring("transport_url=rabbit://alternate-mq-for-cell1/fake"))
			}, timeout, interval).Should(Succeed())
			Expect(GetNovaCell(cell1.CellCRName).Status.Hash[cell1.ComputeConfigSecretName.Name]).NotTo(Equal(oldComputeConfigHash))
		})
	})

	When("computeReplica is reconfigured for a cell", func() {
		It("updates the cell, re-runs the cell discover job", func() {
			discoverJob := th.GetJob(cell1.HostDiscoveryJobName)
			oldJobInputHash := GetEnvVarValue(
				discoverJob.Spec.Template.Spec.Containers[0].Env, "INPUT_HASH", "")

			Eventually(func(g Gomega) {
				nova := GetNova(novaNames.NovaName)
				cell1 := nova.Spec.CellTemplates["cell1"]
				ironicTemplate := cell1.NovaComputeTemplates[ironicComputeName]
				// we change replicas to rerun job.In that case replica can be only set to 0
				// because ironicDriver can't have more than 1 replica
				ironicTemplate.Replicas = ptr.To(int32(0))
				cell1.NovaComputeTemplates[ironicComputeName] = ironicTemplate
				nova.Spec.CellTemplates["cell1"] = cell1

				g.Expect(k8sClient.Update(ctx, nova)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			// Expect that nova controller updates the mapping Job to re-run that
			// to update the CellMapping table in the nova_api DB.
			Eventually(func(g Gomega) {
				discoverJob := th.GetJob(cell1.HostDiscoveryJobName)
				th.SimulateStatefulSetReplicaReady(cell1.NovaComputeStatefulSetName)
				newJobInputHash := GetEnvVarValue(
					discoverJob.Spec.Template.Spec.Containers[0].Env, "INPUT_HASH", "")
				g.Expect(newJobInputHash).NotTo(Equal(oldJobInputHash))
			}, timeout, interval).Should(Succeed())
		})
	})

	When("the service password in the osp secret is changed", func() {
		It("reconfigures every nova service", func() {
			secretName := types.NamespacedName{Namespace: novaNames.NovaName.Namespace, Name: SecretName}
			th.UpdateSecret(secretName, "NovaPassword", []byte("new-service-password"))

			// Expect that every service config is updated with the new service password
			for _, cmName := range []types.NamespacedName{
				cell0.ConductorConfigDataName,
				cell1.ConductorConfigDataName,
				cell1.CellNoVNCProxyNameConfigDataName,
				cell1.ComputeConfigSecretName,
				cell2.ConductorConfigDataName,
				cell2.CellNoVNCProxyNameConfigDataName,
				cell2.ComputeConfigSecretName,
				novaNames.APIConfigDataName,
				novaNames.SchedulerConfigDataName,
				novaNames.MetadataConfigDataName,
			} {
				Eventually(func(g Gomega) {
					configDataMap := th.GetSecret(cmName)

					g.Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
					conf := string(configDataMap.Data["01-nova.conf"])
					g.Expect(conf).Should(ContainSubstring(("password = new-service-password")))
					g.Expect(conf).ShouldNot(ContainSubstring(("password = 12345678")))

				}, timeout, interval).Should(Succeed(), fmt.Sprintf("Failed on %s", cmName))
			}
		})
		It("updates the hash in the statefulsets to trigger the restart with the new config", func() {
			var ssNames = []types.NamespacedName{
				cell0.ConductorStatefulSetName,
				cell1.ConductorStatefulSetName,
				cell2.ConductorStatefulSetName,
				cell1.NoVNCProxyStatefulSetName,
				cell2.NoVNCProxyStatefulSetName,
				novaNames.APIStatefulSetName,
				novaNames.SchedulerStatefulSetName,
				novaNames.MetadataStatefulSetName,
			}
			var originalHashes []string = []string{}

			// Grab the current statefulset config hashes
			for _, ss := range ssNames {
				originalHash := GetEnvVarValue(
					th.GetStatefulSet(ss).Spec.Template.Spec.Containers[0].Env, "CONFIG_HASH", "")
				Expect(originalHash).NotTo(BeEmpty())
				originalHashes = append(originalHashes, originalHash)
			}

			secretName := types.NamespacedName{Namespace: novaNames.NovaName.Namespace, Name: SecretName}
			th.UpdateSecret(secretName, "NovaPassword", []byte("new-service-password"))

			// Assert that the config hash is updated in each stateful set
			for i, ss := range ssNames {
				Eventually(func(g Gomega) {
					newHash := GetEnvVarValue(
						th.GetStatefulSet(ss).Spec.Template.Spec.Containers[0].Env, "CONFIG_HASH", "")
					g.Expect(newHash).NotTo(BeEmpty())
					g.Expect(newHash).NotTo(Equal(originalHashes[i]))
				}, timeout, interval).Should(Succeed())
			}
		})
	})
	It("deletes NovaMetadata if it is disabled", func() {
		Eventually(func(g Gomega) {
			nova := GetNova(novaNames.NovaName)
			nova.Spec.MetadataServiceTemplate.Enabled = ptr.To(false)

			g.Expect(k8sClient.Update(ctx, nova)).To(Succeed())
		}, timeout, interval).Should(Succeed())

		AssertMetadataDoesNotExist(novaNames.MetadataName)
		th.ExpectCondition(
			novaNames.NovaName,
			ConditionGetterFunc(NovaConditionGetter),
			condition.ReadyCondition,
			corev1.ConditionTrue,
		)
	})
	It("creates NovaMetadata if it is enabled", func() {
		//disable it first
		Eventually(func(g Gomega) {
			nova := GetNova(novaNames.NovaName)
			nova.Spec.MetadataServiceTemplate.Enabled = ptr.To(false)

			g.Expect(k8sClient.Update(ctx, nova)).To(Succeed())
		}, timeout, interval).Should(Succeed())

		AssertMetadataDoesNotExist(novaNames.MetadataName)
		th.ExpectCondition(
			novaNames.NovaName,
			ConditionGetterFunc(NovaConditionGetter),
			condition.ReadyCondition,
			corev1.ConditionTrue,
		)
		nova := GetNova(novaNames.NovaName)
		Expect(nova.Status.MetadataServiceReadyCount).To(Equal(int32(0)))
		// NOTE(gibi): This only needed in envtest, in a real k8s
		// deployment the garbage collector would delete the StatefulSet
		// when its parents, the NovaMetadata, is deleted, but that garbage
		// collector does not run in envtest. So we manually clean up here
		th.DeleteInstance(th.GetStatefulSet(novaNames.MetadataStatefulSetName))

		// then enable it again
		Eventually(func(g Gomega) {
			nova := GetNova(novaNames.NovaName)
			nova.Spec.MetadataServiceTemplate.Enabled = ptr.To(true)

			g.Expect(k8sClient.Update(ctx, nova)).To(Succeed())
		}, timeout, interval).Should(Succeed())

		th.ExpectCondition(
			novaNames.NovaName,
			ConditionGetterFunc(NovaConditionGetter),
			condition.ReadyCondition,
			corev1.ConditionFalse,
		)

		th.SimulateStatefulSetReplicaReady(novaNames.MetadataStatefulSetName)

		th.ExpectCondition(
			novaNames.NovaName,
			ConditionGetterFunc(NovaConditionGetter),
			condition.ReadyCondition,
			corev1.ConditionTrue,
		)
		nova = GetNova(novaNames.NovaName)
		Expect(nova.Status.MetadataServiceReadyCount).To(Equal(int32(1)))
	})

	It("reconfigures nova-metadata service if metadata shared secret is changed", func() {
		originalHash := GetEnvVarValue(
			th.GetStatefulSet(novaNames.MetadataStatefulSetName).Spec.Template.Spec.Containers[0].Env, "CONFIG_HASH", "")
		Expect(originalHash).NotTo(BeEmpty())

		originalComputeHash := GetNovaMetadata(
			novaNames.MetadataName).Status.Hash[novaNames.MetadataNeutronConfigDataName.Name]
		Expect(originalComputeHash).NotTo(BeEmpty())

		secretName := types.NamespacedName{Namespace: novaNames.NovaName.Namespace, Name: SecretName}
		th.UpdateSecret(secretName, "MetadataSecret", []byte("new-metadata-secret"))

		Eventually(func(g Gomega) {
			configDataMap := th.GetSecret(novaNames.MetadataConfigDataName)

			g.Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
			conf := string(configDataMap.Data["01-nova.conf"])
			g.Expect(conf).Should(ContainSubstring(("metadata_proxy_shared_secret = new-metadata-secret")))
		}, timeout, interval).Should(Succeed())

		// Assert that the config hash is updated in each stateful set
		Eventually(func(g Gomega) {
			newHash := GetEnvVarValue(
				th.GetStatefulSet(novaNames.MetadataStatefulSetName).Spec.Template.Spec.Containers[0].Env, "CONFIG_HASH", "")
			g.Expect(newHash).NotTo(BeEmpty())
			g.Expect(newHash).NotTo(Equal(originalHash))

			//Assert that the compute config is updated too
			computeConfigData := th.GetSecret(novaNames.MetadataNeutronConfigDataName)
			g.Expect(computeConfigData).ShouldNot(BeNil())
			g.Expect(computeConfigData.Data).Should(HaveKey("05-nova-metadata.conf"))
			configData := string(computeConfigData.Data["05-nova-metadata.conf"])
			g.Expect(configData).To(ContainSubstring("metadata_proxy_shared_secret = new-metadata-secret"))

			newComputeHash := GetNovaMetadata(
				novaNames.MetadataName).Status.Hash[novaNames.MetadataNeutronConfigDataName.Name]
			g.Expect(originalComputeHash).NotTo(Equal(newComputeHash))
		}, timeout, interval).Should(Succeed())
	})

	It("reconfigures memcached service and check", func() {
		configDataMap := th.GetSecret(novaNames.SchedulerConfigDataName)
		Expect(configDataMap).ShouldNot(BeNil())
		Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
		configData := string(configDataMap.Data["01-nova.conf"])
		Expect(configData).ShouldNot(
			ContainSubstring(fmt.Sprintf("memcache_servers=memcached-0.memcached.%s.svc:11211,memcached-1.memcached.%s.svc:11211,memcached-2.memcached.%s.svc:11211",
				novaNames.Namespace, novaNames.Namespace, novaNames.Namespace)))
		Expect(configData).Should(
			ContainSubstring(fmt.Sprintf("memcached_servers=inet:[memcached-0.memcached.%s.svc]:11211,inet:[memcached-1.memcached.%s.svc]:11211,inet:[memcached-2.memcached.%s.svc]:11211",
				novaNames.Namespace, novaNames.Namespace, novaNames.Namespace)))
		Expect(configData).Should(
			ContainSubstring("tls_enabled=false"))

		Eventually(func(g Gomega) {
			memcached := infra.GetMemcached(novaNames.MemcachedNamespace)
			memcached.Status.ServerList = []string{"new"}
			memcached.Status.ServerListWithInet = []string{"inet_new"}
			g.Expect(k8sClient.Status().Update(ctx, memcached)).To(Succeed())
		}, timeout, interval).Should(Succeed())

		Eventually(func(g Gomega) {
			configDataMap = th.GetSecret(novaNames.SchedulerConfigDataName)
			g.Expect(configDataMap).ShouldNot(BeNil())
			g.Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
			configData = string(configDataMap.Data["01-nova.conf"])
			g.Expect(configData).ToNot(ContainSubstring("memcache_servers=new"))
			g.Expect(configData).To(ContainSubstring("memcached_servers=inet_new"))
		}, timeout, interval).Should(Succeed())
	})

	It("reconfigures DB Purge job", func() {
		Eventually(func(g Gomega) {
			nova := GetNova(novaNames.NovaName)
			cell0 := nova.Spec.CellTemplates["cell0"]
			(&cell0).DBPurge.Schedule = ptr.To("3 0 * * *")
			(&cell0).DBPurge.ArchiveAge = ptr.To(33)
			(&cell0).DBPurge.PurgeAge = ptr.To(99)

			nova.Spec.CellTemplates["cell0"] = cell0

			g.Expect(k8sClient.Update(ctx, nova)).To(Succeed())
		}, timeout, interval).Should(Succeed())

		Eventually(func(g Gomega) {
			cron := GetCronJob(cell0.DBPurgeCronJobName)

			g.Expect(cron.Spec.Schedule).To(Equal("3 0 * * *"))
			jobEnv := cron.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Env
			g.Expect(GetEnvVarValue(jobEnv, "ARCHIVE_AGE", "")).To(Equal("33"))
			g.Expect(GetEnvVarValue(jobEnv, "PURGE_AGE", "")).To(Equal("99"))
		}, timeout, interval).Should(Succeed())
	})
	It("does not change status if label is added", func() {
		// ensure that any newly generated timestamp i.e. in the condition list
		// will result in a different string representation
		time.Sleep(time.Second)

		var oldStatus *novav1.NovaStatus
		Eventually(func(g Gomega) {
			nova := GetNova(novaNames.NovaName)
			oldStatus = nova.Status.DeepCopy()

			nova.Labels = map[string]string{
				"foo": "bar",
			}
			g.Expect(k8sClient.Update(ctx, nova)).To(Succeed())
		}, timeout, interval).Should(Succeed())

		Consistently(func(g Gomega) {
			newStatus := &GetNova(novaNames.NovaName).Status
			diff := cmp.Diff(oldStatus, newStatus)
			g.Expect(diff).To(BeEmpty())
		}, timeout, interval).Should(Succeed())
	})
})
