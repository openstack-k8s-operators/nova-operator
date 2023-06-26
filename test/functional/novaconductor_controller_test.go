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
	"encoding/json"
	"fmt"

	networkv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/openstack-k8s-operators/lib-common/modules/test/helpers"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
)

var _ = Describe("NovaConductor controller", func() {
	When("a NovaConductor CR is created pointing to a non existent Secret", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateNovaConductor(novaNames.ConductorName, GetDefaultNovaConductorSpec()))
		})

		It("is not Ready", func() {
			th.ExpectCondition(
				novaNames.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("has empty Status fields", func() {
			instance := GetNovaConductor(novaNames.ConductorName)
			// NOTE(gibi): Hash has `omitempty` tags so while
			// they are initialized to an empty map that value is omitted from
			// the output when sent to the client. So we see nils here.
			Expect(instance.Status.Hash).To(BeEmpty())
			Expect(instance.Status.ReadyCount).To(Equal(int32(0)))
		})

		It("is missing the secret", func() {
			th.ExpectCondition(
				novaNames.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionFalse,
			)
		})

		When("an unrelated Secret is created the CR state does not change", func() {
			BeforeEach(func() {
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "not-relevant-secret",
						Namespace: novaNames.ConductorName.Namespace,
					},
				}
				Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
				DeferCleanup(k8sClient.Delete, ctx, secret)
			})

			It("is not Ready", func() {
				th.ExpectCondition(
					novaNames.ConductorName,
					ConditionGetterFunc(NovaConductorConditionGetter),
					condition.ReadyCondition,
					corev1.ConditionFalse,
				)
			})

			It("is missing the secret", func() {
				th.ExpectCondition(
					novaNames.ConductorName,
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
						Namespace: novaNames.ConductorName.Namespace,
					},
					Data: map[string][]byte{
						"ServicePassword": []byte("12345678"),
					},
				}
				Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
				DeferCleanup(k8sClient.Delete, ctx, secret)
			})

			It("is not Ready", func() {
				th.ExpectCondition(
					novaNames.ConductorName,
					ConditionGetterFunc(NovaConductorConditionGetter),
					condition.ReadyCondition,
					corev1.ConditionFalse,
				)
			})

			It("reports that the inputs are not ready", func() {
				th.ExpectCondition(
					novaNames.ConductorName,
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
					CreateNovaConductorSecret(novaNames.ConductorName.Namespace, SecretName),
				)
				DeferCleanup(
					k8sClient.Delete, ctx,
					CreateNovaMessageBusSecret(novaNames.ConductorName.Namespace, MessageBusSecretName),
				)
			})

			It("reports that input is ready", func() {
				th.ExpectCondition(
					novaNames.ConductorName,
					ConditionGetterFunc(NovaConductorConditionGetter),
					condition.InputReadyCondition,
					corev1.ConditionTrue,
				)
			})
			It("generated configs successfully", func() {
				// NOTE(gibi): NovaConductor has no external dependency right now to
				// generate the configs.
				th.ExpectCondition(
					novaNames.ConductorName,
					ConditionGetterFunc(NovaConductorConditionGetter),
					condition.ServiceConfigReadyCondition,
					corev1.ConditionTrue,
				)

				configDataMap := th.GetSecret(
					types.NamespacedName{
						Namespace: novaNames.ConductorName.Namespace,
						Name:      fmt.Sprintf("%s-config-data", novaNames.ConductorName.Name),
					},
				)
				Expect(configDataMap.Data).Should(HaveKey("nova-blank.conf"))
				blankData := string(configDataMap.Data["nova-blank.conf"])
				Expect(blankData).To(Equal(""))

				Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
				configData := string(configDataMap.Data["01-nova.conf"])
				Expect(configData).Should(ContainSubstring("password = service-password"))
				Expect(configData).Should(ContainSubstring("transport_url=rabbit://rabbitmq-secret/fake"))
				Expect(configDataMap.Data).Should(HaveKey("02-nova-override.conf"))
				extraData := string(configDataMap.Data["02-nova-override.conf"])
				Expect(extraData).To(Equal("foo=bar"))

				scriptMap := th.GetSecret(
					types.NamespacedName{
						Namespace: novaNames.ConductorName.Namespace,
						Name:      fmt.Sprintf("%s-scripts", novaNames.ConductorName.Name),
					},
				)
				// Everything under templates/novaconductor are added automatically by
				// lib-common
				Expect(scriptMap.Data).Should(HaveKey("dbsync.sh"))
				scriptData := string(scriptMap.Data["dbsync.sh"])
				Expect(scriptData).Should(ContainSubstring("nova-manage db sync"))
				Expect(scriptData).Should(ContainSubstring("nova-manage api_db sync"))
			})

			It("stored the input hash in the Status", func() {
				Eventually(func(g Gomega) {
					novaConductor := GetNovaConductor(novaNames.ConductorName)
					g.Expect(novaConductor.Status.Hash).Should(HaveKeyWithValue("input", Not(BeEmpty())))
				}, timeout, interval).Should(Succeed())

			})

			When("the NovaConductor is deleted", func() {
				It("deletes the generated ConfigMaps", func() {
					th.ExpectCondition(
						novaNames.ConductorName,
						ConditionGetterFunc(NovaConductorConditionGetter),
						condition.ServiceConfigReadyCondition,
						corev1.ConditionTrue,
					)

					th.DeleteInstance(GetNovaConductor(novaNames.ConductorName))
					Eventually(func() []corev1.ConfigMap {
						return th.ListConfigMaps(novaNames.ConductorName.Name).Items
					}, timeout, interval).Should(BeEmpty())
				})
			})
		})
	})

	When("NovConductor is created with a proper Secret", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaConductorSecret(novaNames.ConductorName.Namespace, SecretName))
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaMessageBusSecret(novaNames.ConductorName.Namespace, MessageBusSecretName))

			spec := GetDefaultNovaConductorSpec()
			DeferCleanup(th.DeleteInstance, CreateNovaConductor(novaNames.ConductorName, spec))

			th.ExpectCondition(
				novaNames.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)
		})

		// NOTE(gibi): This could be racy when run against a real cluster
		// as the job might finish / fail automatically before this test can
		// assert the in progress state. Fortunately the real env is slow so
		// this actually passes.
		It("started the dbsync job and it reports waiting for that job to finish", func() {
			th.ExpectConditionWithDetails(
				novaNames.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				condition.DBSyncReadyRunningMessage,
			)
			job := th.GetJob(novaNames.ConductorDBSyncJobName)
			Expect(job.Spec.Template.Spec.Volumes).To(HaveLen(2))
			Expect(job.Spec.Template.Spec.InitContainers).To(HaveLen(0))
			Expect(job.Spec.Template.Spec.Containers).To(HaveLen(1))
			container := job.Spec.Template.Spec.Containers[0]
			Expect(container.VolumeMounts).To(HaveLen(2))
			Expect(container.Args[1]).To(ContainSubstring("dbsync.sh"))
			Expect(container.Image).To(Equal(ContainerImage))
		})

		When("DB sync fails", func() {
			BeforeEach(func() {
				th.SimulateJobFailure(novaNames.ConductorDBSyncJobName)
			})

			// NOTE(gibi): lib-common only deletes the job if the job succeeds
			It("reports that DB sync is failed and the job is not deleted", func() {
				th.ExpectConditionWithDetails(
					novaNames.ConductorName,
					ConditionGetterFunc(NovaConductorConditionGetter),
					condition.DBSyncReadyCondition,
					corev1.ConditionFalse,
					condition.ErrorReason,
					"DBsync job error occurred Internal error occurred: Job Failed. Check job logs",
				)
				// This would fail the test case if the job does not exists
				th.GetJob(novaNames.ConductorDBSyncJobName)

				// We don't store the failed job's hash.
				novaConductor := GetNovaConductor(novaNames.ConductorName)
				Expect(novaConductor.Status.Hash).ShouldNot(HaveKey("dbsync"))

			})

			When("NovaConductor is deleted", func() {
				It("deletes the failed job", func() {
					th.ExpectConditionWithDetails(
						novaNames.ConductorName,
						ConditionGetterFunc(NovaConductorConditionGetter),
						condition.DBSyncReadyCondition,
						corev1.ConditionFalse,
						condition.ErrorReason,
						"DBsync job error occurred Internal error occurred: Job Failed. Check job logs",
					)

					th.DeleteInstance(GetNovaConductor(novaNames.ConductorName))

					Eventually(func() []batchv1.Job {
						return th.ListJobs(novaNames.ConductorName.Name).Items
					}, timeout, interval).Should(BeEmpty())
				})
			})
		})

		When("DB sync job finishes successfully", func() {
			BeforeEach(func() {
				th.SimulateJobSuccess(novaNames.ConductorDBSyncJobName)
			})

			It("reports that DB sync is ready and the job is configured to be deleted", func() {
				th.ExpectCondition(
					novaNames.ConductorName,
					ConditionGetterFunc(NovaConductorConditionGetter),
					condition.DBSyncReadyCondition,
					corev1.ConditionTrue,
				)
				job := th.GetJob(novaNames.ConductorDBSyncJobName)
				Expect(job.Spec.TTLSecondsAfterFinished).NotTo(BeNil())
			})

			It("stores the hash of the Job in the Status", func() {
				Eventually(func(g Gomega) {
					novaConductor := GetNovaConductor(novaNames.ConductorName)
					g.Expect(novaConductor.Status.Hash).Should(HaveKeyWithValue("dbsync", Not(BeEmpty())))
				}, timeout, interval).Should(Succeed())

			})

			It("creates a StatefulSet for the nova-conductor service", func() {
				th.ExpectCondition(
					novaNames.ConductorName,
					ConditionGetterFunc(NovaConductorConditionGetter),
					condition.DeploymentReadyCondition,
					corev1.ConditionFalse,
				)
				ss := th.GetStatefulSet(novaNames.ConductorStatefulSetName)
				Expect(ss.Spec.Template.Spec.Containers).To(HaveLen(1))
				container := ss.Spec.Template.Spec.Containers[0]
				Expect(container.LivenessProbe.Exec.Command).To(
					Equal([]string{"/usr/bin/pgrep", "-r", "DRST", "nova-conductor"}))
				Expect(container.ReadinessProbe.Exec.Command).To(
					Equal([]string{"/usr/bin/pgrep", "-r", "DRST", "nova-conductor"}))

				th.SimulateStatefulSetReplicaReady(novaNames.ConductorStatefulSetName)
				th.ExpectCondition(
					novaNames.ConductorName,
					ConditionGetterFunc(NovaConductorConditionGetter),
					condition.DeploymentReadyCondition,
					corev1.ConditionTrue,
				)
				th.ExpectCondition(
					novaNames.ConductorName,
					ConditionGetterFunc(NovaConductorConditionGetter),
					condition.ReadyCondition,
					corev1.ConditionTrue,
				)
				conductor := GetNovaConductor(novaNames.ConductorName)
				Expect(conductor.Status.ReadyCount).To(BeNumerically(">", 0))
			})
		})
	})

	When("NovaConductor is configured to preserve jobs", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaConductorSecret(novaNames.ConductorName.Namespace, SecretName))
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaMessageBusSecret(novaNames.ConductorName.Namespace, MessageBusSecretName))

			spec := GetDefaultNovaConductorSpec()
			spec["debug"] = map[string]interface{}{
				"preserveJobs": true,
			}
			DeferCleanup(th.DeleteInstance, CreateNovaConductor(novaNames.ConductorName, spec))

			th.ExpectCondition(
				novaNames.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("does not configure DB sync job to be deleted after it finished", func() {
			th.SimulateJobSuccess(novaNames.ConductorDBSyncJobName)

			th.ExpectCondition(
				novaNames.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionTrue,
			)
			Expect(th.GetJob(novaNames.ConductorDBSyncJobName).Spec.TTLSecondsAfterFinished).To(BeNil())
		})

		It("does not configure DB sync job to be deleted after it failed", func() {
			th.SimulateJobFailure(novaNames.ConductorDBSyncJobName)

			th.ExpectConditionWithDetails(
				novaNames.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"DBsync job error occurred Internal error occurred: Job Failed. Check job logs",
			)
			Expect(th.GetJob(novaNames.ConductorDBSyncJobName).Spec.TTLSecondsAfterFinished).To(BeNil())
		})
	})

	When("PreserveJobs changed from true to false", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaConductorSecret(novaNames.ConductorName.Namespace, SecretName))
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaMessageBusSecret(novaNames.ConductorName.Namespace, MessageBusSecretName))

			spec := GetDefaultNovaConductorSpec()
			spec["debug"] = map[string]interface{}{
				"preserveJobs": true,
			}
			DeferCleanup(th.DeleteInstance, CreateNovaConductor(novaNames.ConductorName, spec))

			th.SimulateJobSuccess(novaNames.ConductorDBSyncJobName)
			th.ExpectCondition(
				novaNames.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionTrue,
			)

			Expect(th.GetJob(novaNames.ConductorDBSyncJobName).Spec.TTLSecondsAfterFinished).To(BeNil())

			// Update the NovaConductor to not preserve Jobs
			// Eventually is needed here to retry if the update returns conflict
			Eventually(func(g Gomega) {
				conductor := GetNovaConductor(novaNames.ConductorName)
				conductor.Spec.Debug.PreserveJobs = false
				g.Expect(k8sClient.Update(ctx, conductor)).Should(Succeed())
			}, timeout, interval).Should(Succeed())
		})

		It("marks the job to be deleted", func() {
			Eventually(func(g Gomega) {
				g.Expect(th.GetJob(novaNames.ConductorDBSyncJobName).Spec.TTLSecondsAfterFinished).NotTo(BeNil())
			}, timeout, interval).Should(Succeed())
		})
	})

})

var _ = Describe("NovaConductor controller", func() {
	BeforeEach(func() {
		DeferCleanup(
			k8sClient.Delete, ctx, CreateNovaMessageBusSecret(novaNames.ConductorName.Namespace, MessageBusSecretName))
	})

	When("NovaConductor is created with networkAttachments", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaConductorSecret(novaNames.ConductorName.Namespace, SecretName))

			spec := GetDefaultNovaConductorSpec()
			spec["networkAttachments"] = []string{"internalapi"}
			DeferCleanup(th.DeleteInstance, CreateNovaConductor(novaNames.ConductorName, spec))
		})

		It("reports that the definition is missing", func() {
			th.ExpectConditionWithDetails(
				novaNames.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NetworkAttachment resources missing: internalapi",
			)
			th.ExpectCondition(
				novaNames.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})
		It("reports that network attachment is missing", func() {
			internalAPINADName := types.NamespacedName{Namespace: novaNames.ConductorName.Namespace, Name: "internalapi"}
			nad := th.CreateNetworkAttachmentDefinition(internalAPINADName)
			DeferCleanup(th.DeleteInstance, nad)
			th.SimulateJobSuccess(novaNames.ConductorDBSyncJobName)

			ss := th.GetStatefulSet(novaNames.ConductorStatefulSetName)

			expectedAnnotation, err := json.Marshal(
				[]networkv1.NetworkSelectionElement{
					{
						Name:             "internalapi",
						Namespace:        novaNames.ConductorName.Namespace,
						InterfaceRequest: "internalapi",
					}})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(ss.Spec.Template.ObjectMeta.Annotations).To(
				HaveKeyWithValue("k8s.v1.cni.cncf.io/networks", string(expectedAnnotation)),
			)

			// We don't add network attachment status annotations to the Pods
			// to simulate that the network attachments are missing.
			th.SimulateStatefulSetReplicaReadyWithPods(novaNames.ConductorStatefulSetName, map[string][]string{})

			th.ExpectConditionWithDetails(
				novaNames.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occurred "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
			)
		})
		It("reports that an IP is missing", func() {
			internalAPINADName := types.NamespacedName{Namespace: novaNames.ConductorName.Namespace, Name: "internalapi"}
			nad := th.CreateNetworkAttachmentDefinition(internalAPINADName)
			DeferCleanup(th.DeleteInstance, nad)
			th.SimulateJobSuccess(novaNames.ConductorDBSyncJobName)

			ss := th.GetStatefulSet(novaNames.ConductorStatefulSetName)

			expectedAnnotation, err := json.Marshal(
				[]networkv1.NetworkSelectionElement{
					{
						Name:             "internalapi",
						Namespace:        novaNames.ConductorName.Namespace,
						InterfaceRequest: "internalapi",
					}})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(ss.Spec.Template.ObjectMeta.Annotations).To(
				HaveKeyWithValue("k8s.v1.cni.cncf.io/networks", string(expectedAnnotation)),
			)

			// We simulate that there is no IP associated with the internalapi
			// network attachment
			th.SimulateStatefulSetReplicaReadyWithPods(
				novaNames.ConductorStatefulSetName,
				map[string][]string{novaNames.ConductorName.Namespace + "/internalapi": {}},
			)

			th.ExpectConditionWithDetails(
				novaNames.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occurred "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
			)
		})
		It("reports NetworkAttachmentsReady if the Pods got the proper annotations", func() {
			internalAPINADName := types.NamespacedName{Namespace: novaNames.ConductorName.Namespace, Name: "internalapi"}
			nad := th.CreateNetworkAttachmentDefinition(internalAPINADName)
			DeferCleanup(th.DeleteInstance, nad)
			th.SimulateJobSuccess(novaNames.ConductorDBSyncJobName)

			th.SimulateStatefulSetReplicaReadyWithPods(
				novaNames.ConductorStatefulSetName,
				map[string][]string{novaNames.ConductorName.Namespace + "/internalapi": {"10.0.0.1"}},
			)

			th.ExpectCondition(
				novaNames.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionTrue,
			)

			Eventually(func(g Gomega) {
				novaConductor := GetNovaConductor(novaNames.ConductorName)
				g.Expect(novaConductor.Status.NetworkAttachments).To(
					Equal(map[string][]string{novaNames.ConductorName.Namespace + "/internalapi": {"10.0.0.1"}}))
			}, timeout, interval).Should(Succeed())

			th.ExpectCondition(
				novaNames.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})
	When("NovaConductor is reconfigured", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaConductorSecret(novaNames.ConductorName.Namespace, SecretName))
			DeferCleanup(th.DeleteInstance, CreateNovaConductor(novaNames.ConductorName, GetDefaultNovaConductorSpec()))

			th.ExpectCondition(
				novaNames.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)

			th.SimulateJobSuccess(novaNames.ConductorDBSyncJobName)
			th.SimulateStatefulSetReplicaReady(novaNames.ConductorStatefulSetName)
			th.ExpectCondition(
				novaNames.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("applies new NetworkAttachments configuration", func() {
			Eventually(func(g Gomega) {
				novaConductor := GetNovaConductor(novaNames.ConductorName)
				novaConductor.Spec.NetworkAttachments = append(novaConductor.Spec.NetworkAttachments, "internalapi")

				err := k8sClient.Update(ctx, novaConductor)
				g.Expect(err == nil || k8s_errors.IsConflict(err)).To(BeTrue())
			}, timeout, interval).Should(Succeed())

			th.ExpectConditionWithDetails(
				novaNames.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NetworkAttachment resources missing: internalapi",
			)

			th.ExpectConditionWithDetails(
				novaNames.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NetworkAttachment resources missing: internalapi",
			)

			internalAPINADName := types.NamespacedName{Namespace: novaNames.ConductorName.Namespace, Name: "internalapi"}
			DeferCleanup(th.DeleteInstance, th.CreateNetworkAttachmentDefinition(internalAPINADName))

			th.ExpectConditionWithDetails(
				novaNames.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occurred "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
			)

			th.ExpectConditionWithDetails(
				novaNames.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occurred "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
			)

			th.SimulateStatefulSetReplicaReadyWithPods(
				novaNames.ConductorStatefulSetName,
				map[string][]string{novaNames.ConductorName.Namespace + "/internalapi": {"10.0.0.1"}},
			)

			th.ExpectCondition(
				novaNames.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionTrue,
			)

			Eventually(func(g Gomega) {
				novaConductor := GetNovaConductor(novaNames.ConductorName)
				g.Expect(novaConductor.Status.NetworkAttachments).To(
					Equal(map[string][]string{novaNames.ConductorName.Namespace + "/internalapi": {"10.0.0.1"}}))

			}, timeout, interval).Should(Succeed())

			th.ExpectCondition(
				novaNames.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})

	When("NovaConductor CR is created without container image defined", func() {
		BeforeEach(func() {
			spec := GetDefaultNovaConductorSpec()
			spec["containerImage"] = ""
			conductor := CreateNovaConductor(novaNames.ConductorName, spec)
			DeferCleanup(th.DeleteInstance, conductor)
		})
		It("has the expected container image default", func() {
			novaConductorDefault := GetNovaConductor(novaNames.ConductorName)
			Expect(novaConductorDefault.Spec.ContainerImage).To(Equal(util.GetEnvVar("NOVA_CONDUCTOR_IMAGE_URL_DEFAULT", novav1.NovaConductorContainerImage)))
		})
	})
})
