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
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
)

var _ = Describe("NovaAPI controller", func() {
	var namespace string
	var novaAPIName types.NamespacedName

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

	When("A NovaAPI CR instance is created without any input", func() {
		BeforeEach(func() {
			novaAPIName = CreateNovaAPI(namespace, novav1.NovaAPISpec{})
			DeferCleanup(DeleteNovaAPI, novaAPIName)
		})

		It("is not Ready", func() {
			ExpectCondition(
				novaAPIName,
				conditionGetterFunc(NovaAPIConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionUnknown,
			)
		})

		It("has empty Status fields", func() {
			instance := GetNovaAPI(novaAPIName)
			// NOTE(gibi): Hash and Endpoints have `omitempty` tags so while
			// they are initialized to {} that value is omited from the output
			// when sent to the client. So we see nils here.
			Expect(instance.Status.Hash).To(BeEmpty())
			Expect(instance.Status.APIEndpoints).To(BeEmpty())
			Expect(instance.Status.ReadyCount).To(Equal(int32(0)))
			Expect(instance.Status.ServiceID).To(Equal(""))
		})

	})

	When("a NovaAPI CR is created pointing to a non existent Secret", func() {
		BeforeEach(func() {
			novaAPIName = CreateNovaAPI(
				namespace, novav1.NovaAPISpec{Secret: SecretName})
			DeferCleanup(DeleteNovaAPI, novaAPIName)
		})

		It("is not Ready", func() {
			ExpectCondition(
				novaAPIName,
				conditionGetterFunc(NovaAPIConditionGetter),
				condition.ReadyCondition, corev1.ConditionUnknown,
			)
		})

		It("is missing the secret", func() {
			ExpectCondition(
				novaAPIName,
				conditionGetterFunc(NovaAPIConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionFalse,
			)
		})

		When("an unrealated Secret is created the CR state does not change", func() {
			BeforeEach(func() {
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "not-relevant-secret",
						Namespace: namespace,
					},
				}
				Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
				DeferCleanup(k8sClient.Delete, ctx, secret)
			})

			It("is not Ready", func() {
				ExpectCondition(
					novaAPIName,
					conditionGetterFunc(NovaAPIConditionGetter),
					condition.ReadyCondition,
					corev1.ConditionUnknown,
				)
			})

			It("is missing the secret", func() {
				ExpectCondition(
					novaAPIName,
					conditionGetterFunc(NovaAPIConditionGetter),
					condition.InputReadyCondition,
					corev1.ConditionFalse,
				)
			})

		})

		When("the Secret is created but some fields are missing", func() {
			BeforeEach(func() {
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      SecretName,
						Namespace: namespace,
					},
					Data: map[string][]byte{
						"NovaPassword": []byte("12345678"),
					},
				}
				Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
				DeferCleanup(k8sClient.Delete, ctx, secret)
			})

			It("is not Ready", func() {
				ExpectCondition(
					novaAPIName,
					conditionGetterFunc(NovaAPIConditionGetter),
					condition.ReadyCondition,
					corev1.ConditionUnknown,
				)
			})

			It("reports that the inputes are not ready", func() {
				ExpectCondition(
					novaAPIName,
					conditionGetterFunc(NovaAPIConditionGetter),
					condition.InputReadyCondition,
					corev1.ConditionFalse,
				)
			})
		})

		When("the Secret is created with all the expected fields", func() {
			BeforeEach(func() {
				DeferCleanup(
					k8sClient.Delete, ctx, CreateNovaAPISecret(namespace, SecretName))
			})

			It("reports that input is ready", func() {
				ExpectCondition(
					novaAPIName,
					conditionGetterFunc(NovaAPIConditionGetter),
					condition.InputReadyCondition,
					corev1.ConditionTrue,
				)
			})

			It("generated configs successfully", func() {
				// NOTE(gibi): NovaAPI has no external dependency right now to
				// generate the configs.
				ExpectCondition(
					novaAPIName,
					conditionGetterFunc(NovaAPIConditionGetter),
					condition.ServiceConfigReadyCondition,
					corev1.ConditionTrue,
				)

				configDataMap := GetConfigMap(
					types.NamespacedName{
						Namespace: namespace,
						Name:      fmt.Sprintf("%s-config-data", novaAPIName.Name),
					},
				)
				Expect(configDataMap.Data).Should(
					HaveKeyWithValue("custom.conf", "# add your customization here"))

				scriptMap := GetConfigMap(
					types.NamespacedName{
						Namespace: namespace,
						Name:      fmt.Sprintf("%s-scripts", novaAPIName.Name),
					},
				)
				// This is explicitly added to the map by the controller
				Expect(scriptMap.Data).Should(HaveKeyWithValue(
					"common.sh", ContainSubstring("function merge_config_dir")))
				// Everything under templates/novaapi are added automatically by
				// lib-common
				Expect(scriptMap.Data).Should(HaveKeyWithValue(
					"init.sh", ContainSubstring("api_database connection mysql+pymysql")))
			})

			It("stored the input hash in the Status", func() {
				Eventually(func(g Gomega) {
					novaAPI := GetNovaAPI(novaAPIName)
					g.Expect(novaAPI.Status.Hash).Should(HaveKeyWithValue("input", Not(BeEmpty())))
				}, timeout, interval).Should(Succeed())

			})

			When("the NovaAPI is deleted", func() {
				It("deletes the generated ConfigMaps", func() {
					ExpectCondition(
						novaAPIName,
						conditionGetterFunc(NovaAPIConditionGetter),
						condition.ServiceConfigReadyCondition,
						corev1.ConditionTrue,
					)

					DeleteNovaAPI(novaAPIName)

					Eventually(func() []corev1.ConfigMap {
						return ListConfigMaps(novaAPIName.Name).Items
					}, timeout, interval).Should(BeEmpty())
				})
			})
		})
	})

	When("NovAPI is created with a proper Secret", func() {
		var jobName types.NamespacedName

		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaAPISecret(namespace, SecretName))

			novaAPIName = CreateNovaAPI(
				namespace,
				novav1.NovaAPISpec{
					Secret: SecretName,
					NovaServiceBase: novav1.NovaServiceBase{
						ContainerImage: ContainerImage,
					},
				},
			)
			DeferCleanup(DeleteNovaAPI, novaAPIName)

			ExpectCondition(
				novaAPIName,
				conditionGetterFunc(NovaAPIConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)

			jobName = types.NamespacedName{
				Namespace: namespace,
				Name:      fmt.Sprintf("%s-api-db-sync", novaAPIName.Name),
			}

		})

		// NOTE(gibi): This could be racy when run against a real cluster
		// as the job might finish / fail automatically before this test can
		// assert the in progress state. Fortunately the real env is slow so
		// this actually passes.
		It("started the dbsync job and it reports waiting for that job to finish", func() {
			ExpectConditionWithDetails(
				novaAPIName,
				conditionGetterFunc(NovaAPIConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				condition.DBSyncReadyRunningMessage,
			)
			job := GetJob(jobName)
			// TODO(gibi): We could verify a lot of fields but should we?
			Expect(job.Spec.Template.Spec.Volumes).To(HaveLen(3))
			Expect(job.Spec.Template.Spec.InitContainers).To(HaveLen(1))
			initContainer := job.Spec.Template.Spec.InitContainers[0]
			Expect(initContainer.VolumeMounts).To(HaveLen(3))
			Expect(initContainer.Image).To(Equal(ContainerImage))

			Expect(job.Spec.Template.Spec.Containers).To(HaveLen(1))
			container := job.Spec.Template.Spec.Containers[0]
			Expect(container.VolumeMounts).To(HaveLen(2))
			Expect(container.Args[1]).To(ContainSubstring("nova-manage api_db sync"))
			Expect(container.Image).To(Equal(ContainerImage))
		})

		When("DB sync fails", func() {
			BeforeEach(func() {
				SimulateJobFailure(jobName)
			})

			// NOTE(gibi): lib-common only deletes the job if the job succeeds
			It("reports that DB sync is failed and the job is not deleted", func() {
				ExpectConditionWithDetails(
					novaAPIName,
					conditionGetterFunc(NovaAPIConditionGetter),
					condition.DBSyncReadyCondition,
					corev1.ConditionFalse,
					condition.ErrorReason,
					"DBsync job error occured Internal error occurred: Job Failed. Check job logs",
				)
				// This would fail the test case if the job does not exists
				GetJob(jobName)

				// We don't store the failed job's hash.
				novaAPI := GetNovaAPI(novaAPIName)
				Expect(novaAPI.Status.Hash).ShouldNot(HaveKey("dbsync"))

			})

			When("NovaAPI is deleted", func() {
				It("deletes the failed job", func() {
					ExpectConditionWithDetails(
						novaAPIName,
						conditionGetterFunc(NovaAPIConditionGetter),
						condition.DBSyncReadyCondition,
						corev1.ConditionFalse,
						condition.ErrorReason,
						"DBsync job error occured Internal error occurred: Job Failed. Check job logs",
					)

					DeleteNovaAPI(novaAPIName)

					Eventually(func() []batchv1.Job {
						return ListJobs(novaAPIName.Name).Items
					}, timeout, interval).Should(BeEmpty())
				})
			})
		})

		When("DB sync job finishes successfully", func() {
			BeforeEach(func() {
				SimulateJobSuccess(jobName)
			})

			It("reports that DB sync is ready and the job is deleted", func() {
				ExpectCondition(
					novaAPIName,
					conditionGetterFunc(NovaAPIConditionGetter),
					condition.DBSyncReadyCondition,
					corev1.ConditionTrue,
				)

				Expect(ListJobs(namespace).Items).To(BeEmpty())
			})

			It("stores the hash of the Job in the Status", func() {
				Eventually(func(g Gomega) {
					novaAPI := GetNovaAPI(novaAPIName)
					g.Expect(novaAPI.Status.Hash).Should(HaveKeyWithValue("dbsync", Not(BeEmpty())))
				}, timeout, interval).Should(Succeed())

			})

		})
	})

	When("NovAPI is configured to preserve jobs", func() {
		var jobName types.NamespacedName

		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaAPISecret(namespace, SecretName))

			novaAPIName = CreateNovaAPI(
				namespace,
				novav1.NovaAPISpec{
					Secret: SecretName,
					NovaServiceBase: novav1.NovaServiceBase{
						ContainerImage: ContainerImage,
					},
					Debug: novav1.Debug{PreserveJobs: true},
				},
			)
			DeferCleanup(DeleteNovaAPI, novaAPIName)

			ExpectCondition(
				novaAPIName,
				conditionGetterFunc(NovaAPIConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionFalse,
			)

			jobName = types.NamespacedName{
				Namespace: namespace,
				Name:      fmt.Sprintf("%s-api-db-sync", novaAPIName.Name)}
		})

		It("does not delete the DB sync job after it finished", func() {
			SimulateJobSuccess(jobName)

			ExpectCondition(
				novaAPIName,
				conditionGetterFunc(NovaAPIConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionTrue,
			)
			// This would fail the test case if the job does not exists
			GetJob(jobName)
		})

		It("does not delete the DB sync job after it failed", func() {
			SimulateJobFailure(jobName)

			ExpectConditionWithDetails(
				novaAPIName,
				conditionGetterFunc(NovaAPIConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"DBsync job error occured Internal error occurred: Job Failed. Check job logs",
			)
			// This would fail the test case if the job does not exists
			GetJob(jobName)
		})
	})
	When("NovAPI is created and the DB sync run successfully", func() {
		var jobName types.NamespacedName
		var deploymentName types.NamespacedName

		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaAPISecret(namespace, SecretName))

			novaAPIName = CreateNovaAPI(
				namespace,
				novav1.NovaAPISpec{
					Secret: SecretName,
					NovaServiceBase: novav1.NovaServiceBase{
						ContainerImage: ContainerImage,
						Replicas:       1,
					},
				},
			)
			DeferCleanup(DeleteNovaAPI, novaAPIName)

			ExpectCondition(
				novaAPIName,
				conditionGetterFunc(NovaAPIConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)

			jobName = types.NamespacedName{
				Namespace: namespace,
				Name:      fmt.Sprintf("%s-api-db-sync", novaAPIName.Name),
			}

			SimulateJobSuccess(jobName)

			ExpectCondition(
				novaAPIName,
				conditionGetterFunc(NovaAPIConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionTrue,
			)

			deploymentName = types.NamespacedName{
				Namespace: namespace,
				Name:      novaAPIName.Name,
			}
		})

		It("creates a Deployment for the nova-api service", func() {
			ExpectConditionWithDetails(
				novaAPIName,
				conditionGetterFunc(NovaAPIConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				condition.DeploymentReadyRunningMessage,
			)

			deployment := GetDeployment(deploymentName)
			Expect(int(*deployment.Spec.Replicas)).To(Equal(1))

			Expect(deployment.Spec.Template.Spec.Volumes).To(HaveLen(3))
			Expect(deployment.Spec.Template.Spec.InitContainers).To(HaveLen(1))
			initContainer := deployment.Spec.Template.Spec.InitContainers[0]
			Expect(initContainer.VolumeMounts).To(HaveLen(3))
			Expect(initContainer.Image).To(Equal(ContainerImage))

			Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))
			container := deployment.Spec.Template.Spec.Containers[0]
			Expect(container.VolumeMounts).To(HaveLen(2))
			Expect(container.Image).To(Equal(ContainerImage))

		})

		When("the Deployment has at least one Replica ready", func() {
			BeforeEach(func() {
				SkipInExistingCluster(
					"Deployment never finishes in a real env as dependencies like" +
						"ServiceAccount is missing",
				)
				ExpectConditionWithDetails(
					novaAPIName,
					conditionGetterFunc(NovaAPIConditionGetter),
					condition.DeploymentReadyCondition,
					corev1.ConditionFalse,
					condition.RequestedReason,
					condition.DeploymentReadyRunningMessage,
				)
				SimulateDeploymentReplicaReady(deploymentName)
			})

			It("reports that the depoyment is ready", func() {
				ExpectCondition(
					novaAPIName,
					conditionGetterFunc(NovaAPIConditionGetter),
					condition.DeploymentReadyCondition,
					corev1.ConditionTrue,
				)

				novaAPI := GetNovaAPI(novaAPIName)
				Expect(novaAPI.Status.ReadyCount).To(BeNumerically(">", 0))
			})

			It("isReady ", func() {
				ExpectCondition(
					novaAPIName,
					conditionGetterFunc(NovaAPIConditionGetter),
					condition.ReadyCondition,
					corev1.ConditionTrue,
				)
			})
		})
	})
})
