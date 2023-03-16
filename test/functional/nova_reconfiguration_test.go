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
	"os"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/openstack-k8s-operators/lib-common/modules/test/helpers"

	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

func CreateNovaWith3CellsAndEnsureReady(namespace string) types.NamespacedName {
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
	novaAPIKeystoneEndpointName = types.NamespacedName{
		Namespace: namespace,
		Name:      "nova",
	}
	novaKeystoneServiceName = types.NamespacedName{
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
	cell0 = NewCell(novaName, "cell0")
	cell1 = NewCell(novaName, "cell1")
	cell2 = NewCell(novaName, "cell2")

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
	cell0Template := GetDefaultNovaCellTemplate()
	cell0Template["cellName"] = "cell0"
	cell0Template["cellDatabaseInstance"] = "db-for-api"
	cell0Template["cellDatabaseUser"] = "nova_cell0"

	cell1Template := GetDefaultNovaCellTemplate()
	cell1Template["cellName"] = "cell1"
	cell1Template["cellDatabaseInstance"] = "db-for-cell1"
	cell1Template["cellDatabaseUser"] = "nova_cell1"
	cell1Template["cellMessageBusInstance"] = "mq-for-cell1"

	cell2Template := GetDefaultNovaCellTemplate()
	cell2Template["cellName"] = "cell2"
	cell2Template["cellDatabaseInstance"] = "db-for-cell2"
	cell2Template["cellDatabaseUser"] = "nova_cell2"
	cell2Template["cellMessageBusInstance"] = "mq-for-cell2"
	cell2Template["hasAPIAccess"] = false

	spec["cellTemplates"] = map[string]interface{}{
		"cell0": cell0Template,
		"cell1": cell1Template,
		"cell2": cell2Template,
	}
	spec["apiDatabaseInstance"] = "db-for-api"
	spec["apiMessageBusInstance"] = "mq-for-api"

	DeferCleanup(DeleteInstance, CreateNova(novaName, spec))
	keystoneApiName := th.CreateKeystoneAPI(namespace)
	DeferCleanup(th.DeleteKeystoneAPI, keystoneApiName)
	keystoneApi := th.GetKeystoneAPI(keystoneApiName)
	keystoneApi.Status.APIEndpoints["internal"] = "http://keystone-internal-openstack.testing"
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Status().Update(ctx, keystoneApi.DeepCopy())).Should(Succeed())
	}, timeout, interval).Should(Succeed())

	th.SimulateKeystoneServiceReady(novaKeystoneServiceName)

	th.SimulateMariaDBDatabaseCompleted(mariaDBDatabaseNameForAPI)
	th.SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
	th.SimulateMariaDBDatabaseCompleted(cell1.MariaDBDatabaseName)
	th.SimulateMariaDBDatabaseCompleted(cell2.MariaDBDatabaseName)

	th.SimulateTransportURLReady(cell0.TransportURLName)
	th.SimulateTransportURLReady(cell1.TransportURLName)
	th.SimulateTransportURLReady(cell2.TransportURLName)

	th.SimulateJobSuccess(cell0.CellDBSyncJobName)
	th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)

	th.SimulateStatefulSetReplicaReady(novaAPIdeploymentName)
	th.SimulateKeystoneEndpointReady(novaAPIKeystoneEndpointName)

	th.SimulateJobSuccess(cell1.CellDBSyncJobName)
	th.SimulateStatefulSetReplicaReady(cell1.ConductorStatefulSetName)

	th.SimulateJobSuccess(cell2.CellDBSyncJobName)
	th.SimulateStatefulSetReplicaReady(cell2.ConductorStatefulSetName)
	th.SimulateStatefulSetReplicaReady(novaSchedulerStatefulSetName)
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
	return novaName
}

var _ = Describe("Nova reconfiguration", func() {
	var namespace string
	var novaName types.NamespacedName

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

		novaName = CreateNovaWith3CellsAndEnsureReady(namespace)

	})
	When("cell0 conductor replicas is set to 0", func() {
		It("sets the deployment replicas to 0", func() {
			cell0DeploymentName := NewCell(novaName, "cell0").ConductorStatefulSetName

			deployment := th.GetStatefulSet(cell0DeploymentName)
			one := int32(1)
			Expect(deployment.Spec.Replicas).To(Equal(&one))

			// We need this big Eventually block because the Update() call might
			// return a Conflict and then we have to retry by re-reading Nova,
			// and updating the Replicas again.
			Eventually(func(g Gomega) {
				nova := GetNova(novaName)

				// TODO(gibi): Is there a simpler way to achieve this update
				// in golang?
				cell0 := nova.Spec.CellTemplates["cell0"]
				(&cell0).ConductorServiceTemplate.Replicas = int32(0)
				nova.Spec.CellTemplates["cell0"] = cell0

				err := k8sClient.Update(ctx, nova)
				g.Expect(err == nil || k8s_errors.IsConflict(err)).To(BeTrue())

				deployment = &appsv1.StatefulSet{}
				g.Expect(k8sClient.Get(ctx, cell0DeploymentName, deployment)).Should(Succeed())
				zero := int32(0)
				g.Expect(deployment.Spec.Replicas).To(Equal(&zero))
			}, timeout, interval).Should(Succeed())
		})
	})
	When("networkAttachemnt is added to a conductor while the definition is missing", func() {
		It("applys new NetworkAttachments configuration to that Conductor", func() {
			cell1Names := NewCell(novaName, "cell1")

			Eventually(func(g Gomega) {
				nova := GetNova(novaName)

				cell1 := nova.Spec.CellTemplates["cell1"]
				attachments := cell1.ConductorServiceTemplate.NetworkAttachments
				attachments = append(attachments, "internalapi")
				(&cell1).ConductorServiceTemplate.NetworkAttachments = attachments
				nova.Spec.CellTemplates["cell1"] = cell1

				err := k8sClient.Update(ctx, nova)
				g.Expect(err == nil || k8s_errors.IsConflict(err)).To(BeTrue())
			}, timeout, interval).Should(Succeed())

			th.ExpectConditionWithDetails(
				cell1Names.CellConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NetworkAttachment resources missing: internalapi",
			)
			th.ExpectConditionWithDetails(
				cell1Names.CellConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NetworkAttachment resources missing: internalapi",
			)

			th.ExpectConditionWithDetails(
				cell1Names.CellName,
				ConditionGetterFunc(NovaCellConditionGetter),
				novav1.NovaConductorReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NetworkAttachment resources missing: internalapi",
			)
			th.ExpectConditionWithDetails(
				cell1Names.CellName,
				ConditionGetterFunc(NovaCellConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NetworkAttachment resources missing: internalapi",
			)

			th.ExpectConditionWithDetails(
				novaName,
				ConditionGetterFunc(NovaConditionGetter),
				novav1.NovaAllCellsReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NovaCell cell1 is not Ready",
			)
			th.ExpectConditionWithDetails(
				novaName,
				ConditionGetterFunc(NovaConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NovaCell cell1 is not Ready",
			)

			internalAPINADName := types.NamespacedName{Namespace: namespace, Name: "internalapi"}
			DeferCleanup(DeleteInstance, CreateNetworkAttachmentDefinition(internalAPINADName))

			th.ExpectConditionWithDetails(
				novaName,
				ConditionGetterFunc(NovaConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NovaCell cell1 is not Ready",
			)

			SimulateStatefulSetReplicaReadyWithPods(
				cell1Names.ConductorStatefulSetName,
				map[string][]string{namespace + "/internalapi": {"10.0.0.1"}},
			)

			th.ExpectCondition(
				novaName,
				ConditionGetterFunc(NovaConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})

})
