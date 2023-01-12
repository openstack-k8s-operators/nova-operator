/*
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

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
)

const CellMessageBusSecretName = "rabbitmq-transport-url-nova-api-transport"

var _ = Describe("NovaConductor controller", func() {
	var namespace string
	var novaConductorName types.NamespacedName

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

	})

	When("a NovaConductor CR is created pointing to a non existent Secret", func() {
		BeforeEach(func() {
			novaConductorName = CreateNovaConductor(namespace, GetDefaultNovaConductorSpec())
			DeferCleanup(DeleteNovaConductor, novaConductorName)
		})

		It("is not Ready", func() {
			th.ExpectCondition(
				novaConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.ReadyCondition, corev1.ConditionUnknown,
			)
		})

		It("has empty Status fields", func() {
			instance := GetNovaConductor(novaConductorName)
			// NOTE(gibi): Hash has `omitempty` tags so while
			// they are initialized to an empty map that value is omited from
			// the output when sent to the client. So we see nils here.
			Expect(instance.Status.Hash).To(BeEmpty())
			Expect(instance.Status.ReadyCount).To(Equal(int32(0)))
		})

		It("is missing the secret", func() {
			th.ExpectCondition(
				novaConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
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
				th.ExpectCondition(
					novaConductorName,
					ConditionGetterFunc(NovaConductorConditionGetter),
					condition.ReadyCondition,
					corev1.ConditionUnknown,
				)
			})

			It("is missing the secret", func() {
				th.ExpectCondition(
					novaConductorName,
					ConditionGetterFunc(NovaConductorConditionGetter),
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
				th.ExpectCondition(
					novaConductorName,
					ConditionGetterFunc(NovaConductorConditionGetter),
					condition.ReadyCondition,
					corev1.ConditionUnknown,
				)
			})

			It("reports that the inputes are not ready", func() {
				th.ExpectCondition(
					novaConductorName,
					ConditionGetterFunc(NovaConductorConditionGetter),
					condition.InputReadyCondition,
					corev1.ConditionFalse,
				)
			})
		})

		When("the Secret is created with all the expected fields", func() {
			BeforeEach(func() {
				DeferCleanup(
					k8sClient.Delete,
					ctx,
					CreateNovaConductorSecret(namespace, SecretName),
				)
			})

			It("reports that input is ready", func() {
				th.ExpectCondition(
					novaConductorName,
					ConditionGetterFunc(NovaConductorConditionGetter),
					condition.InputReadyCondition,
					corev1.ConditionTrue,
				)
			})
			It("generated configs successfully", func() {
				// NOTE(gibi): NovaConductor has no external dependency right now to
				// generate the configs.
				th.ExpectCondition(
					novaConductorName,
					ConditionGetterFunc(NovaConductorConditionGetter),
					condition.ServiceConfigReadyCondition,
					corev1.ConditionTrue,
				)

				configDataMap := th.GetConfigMap(
					types.NamespacedName{
						Namespace: namespace,
						Name:      fmt.Sprintf("%s-config-data", novaConductorName.Name),
					},
				)
				Expect(configDataMap.Data).Should(HaveKeyWithValue("custom.conf", ""))

				scriptMap := th.GetConfigMap(
					types.NamespacedName{
						Namespace: namespace,
						Name:      fmt.Sprintf("%s-scripts", novaConductorName.Name),
					},
				)
				// This is explicitly added to the map by the controller
				Expect(scriptMap.Data).Should(HaveKeyWithValue(
					"common.sh", ContainSubstring("function merge_config_dir")))
				// Everything under templates/novaconductor are added automatically by
				// lib-common
				Expect(scriptMap.Data).Should(HaveKeyWithValue(
					"init.sh", ContainSubstring("database connection mysql+pymysql")))
				Expect(scriptMap.Data).Should(HaveKeyWithValue(
					"dbsync.sh", ContainSubstring("nova-manage db sync")))
				Expect(scriptMap.Data).Should(HaveKeyWithValue(
					"dbsync.sh", ContainSubstring("nova-manage api_db sync")))
				Expect(scriptMap.Data).Should(HaveKeyWithValue(
					"dbsync.sh", ContainSubstring("nova-manage cell_v2 map_cell0")))
			})

			It("stored the input hash in the Status", func() {
				Eventually(func(g Gomega) {
					novaConductor := GetNovaConductor(novaConductorName)
					g.Expect(novaConductor.Status.Hash).Should(HaveKeyWithValue("input", Not(BeEmpty())))
				}, timeout, interval).Should(Succeed())

			})

			When("the NovaConductor is deleted", func() {
				It("deletes the generated ConfigMaps", func() {
					th.ExpectCondition(
						novaConductorName,
						ConditionGetterFunc(NovaConductorConditionGetter),
						condition.ServiceConfigReadyCondition,
						corev1.ConditionTrue,
					)

					DeleteNovaConductor(novaConductorName)

					Eventually(func() []corev1.ConfigMap {
						return th.ListConfigMaps(novaConductorName.Name).Items
					}, timeout, interval).Should(BeEmpty())
				})
			})
		})
	})

	When("NovConductor is created with a proper Secret", func() {
		var jobName types.NamespacedName
		var statefulSetName types.NamespacedName

		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaConductorSecret(namespace, SecretName))
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaMessageBusSecret(namespace, CellMessageBusSecretName))

			spec := GetDefaultNovaConductorSpec()
			spec["cellMessageBusSecretName"] = CellMessageBusSecretName
			novaConductorName = CreateNovaConductor(namespace, spec)
			DeferCleanup(DeleteNovaConductor, novaConductorName)

			th.ExpectCondition(
				novaConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)

			jobName = types.NamespacedName{
				Namespace: namespace,
				Name:      novaConductorName.Name + "-db-sync",
			}
			statefulSetName = types.NamespacedName{
				Namespace: namespace,
				Name:      novaConductorName.Name,
			}

		})

		// NOTE(gibi): This could be racy when run against a real cluster
		// as the job might finish / fail automatically before this test can
		// assert the in progress state. Fortunately the real env is slow so
		// this actually passes.
		It("started the dbsync job and it reports waiting for that job to finish", func() {
			th.ExpectConditionWithDetails(
				novaConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				condition.DBSyncReadyRunningMessage,
			)
			job := th.GetJob(jobName)
			// TODO(gibi): We could verify a lot of fields but should we?
			Expect(job.Spec.Template.Spec.Volumes).To(HaveLen(3))
			Expect(job.Spec.Template.Spec.InitContainers).To(HaveLen(1))
			initContainer := job.Spec.Template.Spec.InitContainers[0]
			Expect(initContainer.VolumeMounts).To(HaveLen(3))
			Expect(initContainer.Image).To(Equal(ContainerImage))
			Expect(initContainer.Env).To(ContainElement(
				corev1.EnvVar{
					Name: "TransportURL",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: CellMessageBusSecretName,
							},
							Key: "transport_url",
						},
					},
				},
			))

			Expect(job.Spec.Template.Spec.Containers).To(HaveLen(1))
			container := job.Spec.Template.Spec.Containers[0]
			Expect(container.VolumeMounts).To(HaveLen(2))
			Expect(container.Args[1]).To(ContainSubstring("dbsync.sh"))
			Expect(container.Image).To(Equal(ContainerImage))
		})

		When("DB sync fails", func() {
			BeforeEach(func() {
				th.SimulateJobFailure(jobName)
			})

			// NOTE(gibi): lib-common only deletes the job if the job succeeds
			It("reports that DB sync is failed and the job is not deleted", func() {
				th.ExpectConditionWithDetails(
					novaConductorName,
					ConditionGetterFunc(NovaConductorConditionGetter),
					condition.DBSyncReadyCondition,
					corev1.ConditionFalse,
					condition.ErrorReason,
					"DBsync job error occured Internal error occurred: Job Failed. Check job logs",
				)
				// This would fail the test case if the job does not exists
				th.GetJob(jobName)

				// We don't store the failed job's hash.
				novaConductor := GetNovaConductor(novaConductorName)
				Expect(novaConductor.Status.Hash).ShouldNot(HaveKey("dbsync"))

			})

			When("NovaConductor is deleted", func() {
				It("deletes the failed job", func() {
					th.ExpectConditionWithDetails(
						novaConductorName,
						ConditionGetterFunc(NovaConductorConditionGetter),
						condition.DBSyncReadyCondition,
						corev1.ConditionFalse,
						condition.ErrorReason,
						"DBsync job error occured Internal error occurred: Job Failed. Check job logs",
					)

					DeleteNovaConductor(novaConductorName)

					Eventually(func() []batchv1.Job {
						return th.ListJobs(novaConductorName.Name).Items
					}, timeout, interval).Should(BeEmpty())
				})
			})
		})

		When("DB sync job finishes successfully", func() {
			BeforeEach(func() {
				th.SimulateJobSuccess(jobName)
			})

			It("reports that DB sync is ready and the job is configured to be deleted", func() {
				th.ExpectCondition(
					novaConductorName,
					ConditionGetterFunc(NovaConductorConditionGetter),
					condition.DBSyncReadyCondition,
					corev1.ConditionTrue,
				)
				job := th.GetJob(jobName)
				Expect(job.Spec.TTLSecondsAfterFinished).NotTo(BeNil())
			})

			It("stores the hash of the Job in the Status", func() {
				Eventually(func(g Gomega) {
					novaConductor := GetNovaConductor(novaConductorName)
					g.Expect(novaConductor.Status.Hash).Should(HaveKeyWithValue("dbsync", Not(BeEmpty())))
				}, timeout, interval).Should(Succeed())

			})

			It("creates a StatefulSet for the nova-conductor service", func() {
				th.ExpectCondition(
					novaConductorName,
					ConditionGetterFunc(NovaConductorConditionGetter),
					condition.DeploymentReadyCondition,
					corev1.ConditionFalse,
				)
				ss := th.GetStatefulSet(statefulSetName)
				Expect(ss.Spec.Template.Spec.Containers).To(HaveLen(1))
				container := ss.Spec.Template.Spec.Containers[0]
				Expect(container.LivenessProbe.Exec.Command).To(
					Equal([]string{"/usr/bin/pgrep", "-r", "DRST", "nova-conductor"}))
				Expect(container.ReadinessProbe.Exec.Command).To(
					Equal([]string{"/usr/bin/pgrep", "-r", "DRST", "nova-conductor"}))

				th.SimulateStatefulSetReplicaReady(statefulSetName)
				th.ExpectCondition(
					novaConductorName,
					ConditionGetterFunc(NovaConductorConditionGetter),
					condition.DeploymentReadyCondition,
					corev1.ConditionTrue,
				)
				th.ExpectCondition(
					novaConductorName,
					ConditionGetterFunc(NovaConductorConditionGetter),
					condition.ReadyCondition,
					corev1.ConditionTrue,
				)
				conductor := GetNovaConductor(novaConductorName)
				Expect(conductor.Status.ReadyCount).To(BeNumerically(">", 0))
			})
		})
	})

	When("NovaConductor is configured to preserve jobs", func() {
		var jobName types.NamespacedName

		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaConductorSecret(namespace, SecretName))
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaMessageBusSecret(namespace, CellMessageBusSecretName))

			spec := GetDefaultNovaConductorSpec()
			spec["debug"] = map[string]interface{}{
				"preserveJobs": true,
			}
			novaConductorName = CreateNovaConductor(namespace, spec)
			DeferCleanup(DeleteNovaConductor, novaConductorName)

			th.ExpectCondition(
				novaConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionFalse,
			)

			jobName = types.NamespacedName{
				Namespace: namespace,
				Name:      novaConductorName.Name + "-db-sync",
			}
		})

		It("does not configure DB sync job to be deleted after it finished", func() {
			th.SimulateJobSuccess(jobName)

			th.ExpectCondition(
				novaConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionTrue,
			)
			Expect(th.GetJob(jobName).Spec.TTLSecondsAfterFinished).To(BeNil())
		})

		It("does not configure DB sync job to be deleted after it failed", func() {
			th.SimulateJobFailure(jobName)

			th.ExpectConditionWithDetails(
				novaConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"DBsync job error occured Internal error occurred: Job Failed. Check job logs",
			)
			Expect(th.GetJob(jobName).Spec.TTLSecondsAfterFinished).To(BeNil())
		})
	})

	When("PreserveJobs changed from true to false", func() {
		var jobName types.NamespacedName

		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaConductorSecret(namespace, SecretName))
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaMessageBusSecret(namespace, CellMessageBusSecretName))

			spec := GetDefaultNovaConductorSpec()
			spec["debug"] = map[string]interface{}{
				"preserveJobs": true,
			}
			novaConductorName = CreateNovaConductor(namespace, spec)
			DeferCleanup(DeleteNovaConductor, novaConductorName)

			jobName = types.NamespacedName{
				Namespace: namespace,
				Name:      novaConductorName.Name + "-db-sync",
			}

			th.SimulateJobSuccess(jobName)
			th.ExpectCondition(
				novaConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionTrue,
			)

			Expect(th.GetJob(jobName).Spec.TTLSecondsAfterFinished).To(BeNil())

			// Update the NovaConductor to not preserve Jobs
			// Eventually is needed here to retry if the update returns conflict
			Eventually(func(g Gomega) {
				conductor := GetNovaConductor(novaConductorName)
				conductor.Spec.Debug.PreserveJobs = false
				g.Expect(k8sClient.Update(ctx, conductor)).Should(Succeed())
			}, timeout, interval).Should(Succeed())
		})

		It("marks the job to be deleted", func() {
			Eventually(func(g Gomega) {
				g.Expect(th.GetJob(jobName).Spec.TTLSecondsAfterFinished).NotTo(BeNil())
			}, timeout, interval).Should(Succeed())
		})
	})
})
