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

	BeforeEach(func() {
		// NOTE(gibi): We need to create a unique namespace for each test run
		// as namespaces cannot be deleted in a locally running envtest. See
		// https://book.kubebuilder.io/reference/envtest.html#namespace-usage-limitation
		namespace = uuid.New().String()
		CreateNamespace(namespace)
		// We still request the delete of the Namespace in AfterEach to
		// properly cleanup if we run the test in an existing cluster.
		DeferCleanup(DeleteNamespace, namespace)
		// NOTE(gibi): ConfigMap generation looks up the local templates
		// directory via ENV, so provide it
		DeferCleanup(os.Setenv, "OPERATOR_TEMPLATES", os.Getenv("OPERATOR_TEMPLATES"))
		os.Setenv("OPERATOR_TEMPLATES", "../../templates")

		// Uncomment this if you need the full output in the logs from gomega
		// matchers
		// format.MaxLength = 0

	})

	When("A Nova CR instance is created without any input", func() {
		BeforeEach(func() {
			novaName = CreateNova(
				namespace, novav1.NovaSpec{
					CellTemplates: map[string]novav1.NovaCellTemplate{},
				},
			)
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

		It("is fails creating the API DB as MariaDB instance is missing", func() {
			ExpectConditionWithDetails(
				novaName,
				conditionGetterFunc(NovaConditionGetter),
				condition.DBReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"DBsync job error occured Error getting the DB service using "+
					"label map[app:mariadb cr:mariadb-openstack]: %!w(<nil>)",
			)
		})

		When("a DB Service is created", func() {
			var mariaDBDatabaseName types.NamespacedName

			BeforeEach(func() {
				DeferCleanup(
					DeleteDBService,
					CreateDBService(
						namespace,
						corev1.ServiceSpec{
							Ports: []corev1.ServicePort{{Port: 3306}},
						},
					),
				)
				mariaDBDatabaseName = types.NamespacedName{Namespace: namespace, Name: novaName.Name}
			})

			It("creates the MariaDBDatabase for the API DB and waits for it to be Ready", func() {
				ExpectConditionWithDetails(
					novaName,
					conditionGetterFunc(NovaConditionGetter),
					condition.DBReadyCondition,
					corev1.ConditionFalse,
					condition.RequestedReason,
					condition.DBReadyRunningMessage,
				)
				// this would fail if the MariaDBDatabase does not exist
				GetMariaDBDatabase(mariaDBDatabaseName)
			})

			When("the MariaDBDatabase instance becomes ready", func() {
				BeforeEach(func() {
					SimulateMariaDBDatabaseCompleted(mariaDBDatabaseName)
				})

				It("reports that the API Database is ready", func() {
					ExpectCondition(
						novaName,
						conditionGetterFunc(NovaConditionGetter),
						condition.DBReadyCondition,
						corev1.ConditionTrue,
					)
				})
			})
		})
	})

	When("Nova is created with NovaAPI definition", func() {
		var mariaDBDatabaseName types.NamespacedName
		var novaAPIName types.NamespacedName

		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateNovaAPISecret(namespace, SecretName),
			)

			novaName = CreateNova(
				namespace, novav1.NovaSpec{
					Secret: SecretName,
					APIServiceTemplate: novav1.NovaAPITemplate{
						ContainerImage: ContainerImage,
						Replicas:       1,
					},
					CellTemplates: map[string]novav1.NovaCellTemplate{},
				},
			)
			DeferCleanup(DeleteNova, novaName)

			DeferCleanup(
				DeleteDBService,
				CreateDBService(
					namespace,
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			mariaDBDatabaseName = types.NamespacedName{
				Namespace: namespace,
				Name:      novaName.Name,
			}
			SimulateMariaDBDatabaseCompleted(mariaDBDatabaseName)
			ExpectCondition(
				novaName,
				conditionGetterFunc(NovaConditionGetter),
				condition.DBReadyCondition,
				corev1.ConditionTrue,
			)

			novaAPIName = types.NamespacedName{
				Namespace: namespace,
				Name:      fmt.Sprintf("%s-api", novaName.Name),
			}
		})

		It("creates the NovaAPI and tracks its readiness", func() {
			GetNovaAPI(novaAPIName)
			ExpectCondition(
				novaName,
				conditionGetterFunc(NovaConditionGetter),
				novav1.NovaAPIReadyCondition,
				corev1.ConditionFalse,
			)
			nova := GetNova(novaName)
			Expect(nova.Status.APIServiceReadyCount).To(Equal(int32(0)))
		})

		When("NovaAPI is ready", func() {
			var novaAPIDBSyncJobName types.NamespacedName
			var novaAPIDeploymentName types.NamespacedName

			BeforeEach(func() {
				ExpectCondition(
					novaAPIName,
					conditionGetterFunc(NovaAPIConditionGetter),
					condition.DBSyncReadyCondition,
					corev1.ConditionFalse,
				)
				novaAPIDBSyncJobName = types.NamespacedName{
					Namespace: namespace,
					Name:      fmt.Sprintf("%s-api-db-sync", novaAPIName.Name)}
				SimulateJobSuccess(novaAPIDBSyncJobName)

				ExpectCondition(
					novaAPIName,
					conditionGetterFunc(NovaAPIConditionGetter),
					condition.DeploymentReadyCondition,
					corev1.ConditionFalse,
				)
				novaAPIDeploymentName = types.NamespacedName{
					Namespace: namespace,
					Name:      novaAPIName.Name,
				}
				SimulateDeploymentReplicaReady(novaAPIDeploymentName)

			})

			It("reports that NovaAPI is ready and mirrors its ReadyCount", func() {
				ExpectCondition(
					novaName,
					conditionGetterFunc(NovaConditionGetter),
					novav1.NovaAPIReadyCondition,
					corev1.ConditionTrue,
				)
				nova := GetNova(novaName)
				Expect(nova.Status.APIServiceReadyCount).To(Equal(int32(1)))

			})

			It("is Ready", func() {
				ExpectCondition(
					novaName,
					conditionGetterFunc(NovaConditionGetter),
					condition.ReadyCondition,
					corev1.ConditionTrue,
				)
			})
		})
	})
})
