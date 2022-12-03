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
	var novaKeystoneServiceName types.NamespacedName

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

	CreateNova(novaName, spec)
	DeferCleanup(DeleteNova, novaName)
	DeferCleanup(DeleteKeystoneAPI, CreateKeystoneAPI(namespace))

	SimulateKeystoneServiceReady(novaKeystoneServiceName)

	SimulateMariaDBDatabaseCompleted(mariaDBDatabaseNameForAPI)
	SimulateMariaDBDatabaseCompleted(cell0.MariaDBDatabaseName)
	SimulateMariaDBDatabaseCompleted(cell1.MariaDBDatabaseName)
	SimulateMariaDBDatabaseCompleted(cell2.MariaDBDatabaseName)

	SimulateTransportURLReady(cell0.TransportURLName)
	SimulateTransportURLReady(cell1.TransportURLName)
	SimulateTransportURLReady(cell2.TransportURLName)

	SimulateJobSuccess(cell0.CellDBSyncJobName)
	SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)

	SimulateStatefulSetReplicaReady(novaAPIdeploymentName)

	SimulateJobSuccess(cell1.CellDBSyncJobName)
	SimulateStatefulSetReplicaReady(cell1.ConductorStatefulSetName)

	SimulateJobSuccess(cell2.CellDBSyncJobName)
	SimulateStatefulSetReplicaReady(cell2.ConductorStatefulSetName)

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

		novaName = CreateNovaWith3CellsAndEnsureReady(namespace)

	})
	When("cell0 conductor replicas is set to 0", func() {
		It("sets the deployment replicas to 0", func() {
			cell0DeploymentName := NewCell(novaName, "cell0").ConductorStatefulSetName

			deployment := GetStatefulSet(cell0DeploymentName)
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
})
