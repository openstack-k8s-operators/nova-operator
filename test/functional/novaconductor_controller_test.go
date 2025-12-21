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

	. "github.com/onsi/ginkgo/v2" //revive:disable:dot-imports
	. "github.com/onsi/gomega"    //revive:disable:dot-imports
	"github.com/onsi/gomega/format"
	"k8s.io/apimachinery/pkg/types"

	//revive:disable-next-line:dot-imports
	. "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"

	networkv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
	mariadbv1 "github.com/openstack-k8s-operators/mariadb-operator/api/v1beta1"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

var _ = Describe("NovaConductor controller", func() {
	BeforeEach(func() {
		mariadb.CreateMariaDBDatabase(cell0.MariaDBDatabaseName.Namespace, cell0.MariaDBDatabaseName.Name, mariadbv1.MariaDBDatabaseSpec{})
		DeferCleanup(k8sClient.Delete, ctx, mariadb.GetMariaDBDatabase(cell0.MariaDBDatabaseName))

		apiMariaDBAccount, apiMariaDBSecret := mariadb.CreateMariaDBAccountAndSecret(
			novaNames.APIMariaDBDatabaseAccount, mariadbv1.MariaDBAccountSpec{})
		DeferCleanup(k8sClient.Delete, ctx, apiMariaDBAccount)
		DeferCleanup(k8sClient.Delete, ctx, apiMariaDBSecret)

		cell0Account, cell0Secret := mariadb.CreateMariaDBAccountAndSecret(
			cell0.MariaDBAccountName, mariadbv1.MariaDBAccountSpec{})
		DeferCleanup(k8sClient.Delete, ctx, cell0Account)
		DeferCleanup(k8sClient.Delete, ctx, cell0Secret)

		memcachedSpec := infra.GetDefaultMemcachedSpec()
		DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(novaNames.NovaName.Namespace, MemcachedInstance, memcachedSpec))
		infra.SimulateMemcachedReady(novaNames.MemcachedNamespace)

	})

	When("a NovaConductor CR is created pointing to a non existent Secret", func() {
		BeforeEach(func() {
			DeferCleanup(
				th.DeleteInstance, CreateNovaConductor(cell0.ConductorName, GetDefaultNovaConductorSpec(cell0)))
		})

		It("is not Ready", func() {
			th.ExpectCondition(
				cell0.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("has empty Status fields", func() {
			instance := GetNovaConductor(cell0.ConductorName)
			// NOTE(gibi): Hash has `omitempty` tags so while
			// they are initialized to an empty map that value is omitted from
			// the output when sent to the client. So we see nils here.
			Expect(instance.Status.Hash).To(BeEmpty())
			Expect(instance.Status.ReadyCount).To(Equal(int32(0)))
		})

		It("defaults Spec fields", func() {
			instance := GetNovaConductor(cell0.ConductorName)
			Expect(instance.Spec.DBPurge.Schedule).To(Equal(ptr.To("0 0 * * *")))
			Expect(instance.Spec.DBPurge.ArchiveAge).To(Equal(ptr.To(30)))
			Expect(instance.Spec.DBPurge.PurgeAge).To(Equal(ptr.To(90)))
		})

		It("is missing the secret", func() {
			th.ExpectCondition(
				cell0.ConductorName,
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
						Namespace: cell0.ConductorName.Namespace,
					},
				}
				Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
				DeferCleanup(k8sClient.Delete, ctx, secret)
			})

			It("is not Ready", func() {
				th.ExpectCondition(
					cell0.ConductorName,
					ConditionGetterFunc(NovaConductorConditionGetter),
					condition.ReadyCondition,
					corev1.ConditionFalse,
				)
			})

			It("is missing the secret", func() {
				th.ExpectCondition(
					cell0.ConductorName,
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
						Name:      cell0.InternalCellSecretName.Name,
						Namespace: cell0.InternalCellSecretName.Namespace,
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
					cell0.ConductorName,
					ConditionGetterFunc(NovaConductorConditionGetter),
					condition.ReadyCondition,
					corev1.ConditionFalse,
				)
			})

			It("reports that the inputs are not ready", func() {
				th.ExpectCondition(
					cell0.ConductorName,
					ConditionGetterFunc(NovaConductorConditionGetter),
					condition.InputReadyCondition,
					corev1.ConditionFalse,
				)
			})
		})

		When("the Secret is created but notification fields is missing", func() {
			BeforeEach(func() {
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      cell0.InternalCellSecretName.Name,
						Namespace: cell0.InternalCellSecretName.Namespace,
					},
					Data: map[string][]byte{
						"ServicePassword": []byte("12345678"),
						"transport_url":   []byte("rabbit://cell0/fake"),
						// notification_transport_url is missing
					},
				}
				Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
				DeferCleanup(k8sClient.Delete, ctx, secret)
			})

			It("is not Ready", func() {
				th.ExpectCondition(
					cell0.ConductorName,
					ConditionGetterFunc(NovaConductorConditionGetter),
					condition.ReadyCondition,
					corev1.ConditionFalse,
				)
			})

			It("reports that the inputs are not ready", func() {
				th.ExpectConditionWithDetails(
					cell0.ConductorName,
					ConditionGetterFunc(NovaConductorConditionGetter),
					condition.InputReadyCondition,
					corev1.ConditionFalse,
					condition.ErrorReason,
					fmt.Sprintf("Input data error occurred field not found in Secret: 'notification_transport_url' not found in secret/%s", cell0.InternalCellSecretName.Name),
				)
			})
		})

		When("the Secret is created with all the expected fields", func() {
			BeforeEach(func() {
				DeferCleanup(
					k8sClient.Delete,
					ctx,
					CreateDefaultCellInternalSecret(cell0),
				)
			})

			It("reports that input is ready", func() {
				th.ExpectCondition(
					cell0.ConductorName,
					ConditionGetterFunc(NovaConductorConditionGetter),
					condition.InputReadyCondition,
					corev1.ConditionTrue,
				)
			})
			It("generated configs successfully", func() {
				// NOTE(gibi): NovaConductor has no external dependency right now to
				// generate the configs.
				th.ExpectCondition(
					cell0.ConductorName,
					ConditionGetterFunc(NovaConductorConditionGetter),
					condition.ServiceConfigReadyCondition,
					corev1.ConditionTrue,
				)

				configDataMap := th.GetSecret(cell0.ConductorConfigDataName)
				Expect(configDataMap.Data).Should(HaveKey("nova-blank.conf"))
				blankData := string(configDataMap.Data["nova-blank.conf"])
				Expect(blankData).To(Equal(""))

				Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
				configData := string(configDataMap.Data["01-nova.conf"])
				AssertHaveNotificationTransportURL("notifications", configData)

				Expect(configData).Should(ContainSubstring("password = service-password"))
				Expect(configData).Should(ContainSubstring("transport_url=rabbit://cell0/fake"))
				Expect(configData).Should(
					ContainSubstring("[upgrade_levels]\ncompute = auto"))
				Expect(configData).Should(
					ContainSubstring("backend = dogpile.cache.memcached"))
				Expect(configData).ShouldNot(
					ContainSubstring(fmt.Sprintf("memcache_servers=memcached-0.memcached.%s.svc:11211,memcached-1.memcached.%s.svc:11211,memcached-2.memcached.%s.svc:11211",
						novaNames.Namespace, novaNames.Namespace, novaNames.Namespace)))
				Expect(configData).Should(
					ContainSubstring(fmt.Sprintf("memcached_servers=memcached-0.memcached.%s.svc:11211,memcached-1.memcached.%s.svc:11211,memcached-2.memcached.%s.svc:11211",
						novaNames.Namespace, novaNames.Namespace, novaNames.Namespace)))
				Expect(configData).Should(
					ContainSubstring("tls_enabled=false"))
				Expect(configDataMap.Data).Should(HaveKey("02-nova-override.conf"))
				myCnf := configDataMap.Data["my.cnf"]
				Expect(myCnf).To(
					ContainSubstring("[client]\nssl=0"))
				extraData := string(configDataMap.Data["02-nova-override.conf"])
				Expect(extraData).To(Equal("foo=bar"))

				scriptMap := th.GetSecret(cell0.ConductorScriptDataName)
				// Everything under templates/novaconductor are added automatically by
				// lib-common
				Expect(scriptMap.Data).Should(HaveKey("dbsync.sh"))
				scriptData := string(scriptMap.Data["dbsync.sh"])
				Expect(scriptData).Should(ContainSubstring("nova-manage db sync"))
				Expect(scriptData).Should(ContainSubstring("nova-manage api_db sync"))
				Expect(scriptMap.Data).Should(HaveKey("dbpurge.sh"))
				scriptData = string(scriptMap.Data["dbpurge.sh"])
				Expect(scriptData).Should(ContainSubstring("nova-manage db archive_deleted_rows"))
				Expect(scriptData).Should(ContainSubstring("nova-manage db purge"))
			})

			It("stored the input hash in the Status", func() {
				Eventually(func(g Gomega) {
					novaConductor := GetNovaConductor(cell0.ConductorName)
					g.Expect(novaConductor.Status.Hash).Should(HaveKeyWithValue("input", Not(BeEmpty())))
				}, timeout, interval).Should(Succeed())

			})
		})
	})

	When("NovConductor is created with a proper Secret", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateDefaultCellInternalSecret(cell0))

			spec := GetDefaultNovaConductorSpec(cell0)
			newSelector := map[string]string{"foo": "bar"}
			spec["nodeSelector"] = &newSelector
			DeferCleanup(th.DeleteInstance, CreateNovaConductor(cell0.ConductorName, spec))

			th.ExpectCondition(
				cell0.ConductorName,
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
				cell0.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				condition.DBSyncReadyRunningMessage,
			)
			job := th.GetJob(cell0.DBSyncJobName)
			Expect(job.Spec.Template.Spec.ServiceAccountName).To(Equal("nova-sa"))
			Expect(job.Spec.Template.Spec.Volumes).To(HaveLen(2))
			Expect(job.Spec.Template.Spec.InitContainers).To(BeEmpty())
			Expect(job.Spec.Template.Spec.Containers).To(HaveLen(1))
			container := job.Spec.Template.Spec.Containers[0]
			Expect(container.VolumeMounts).To(HaveLen(3))
			Expect(container.Image).To(Equal(ContainerImage))
		})

		When("DB sync fails", func() {
			BeforeEach(func() {
				th.SimulateJobFailure(cell0.DBSyncJobName)
			})

			// NOTE(gibi): lib-common only deletes the job if the job succeeds
			It("reports that DB sync is failed and the job is not deleted", func() {
				th.ExpectConditionWithDetails(
					cell0.ConductorName,
					ConditionGetterFunc(NovaConductorConditionGetter),
					condition.DBSyncReadyCondition,
					corev1.ConditionFalse,
					condition.ErrorReason,
					"DBsync job error occurred Internal error occurred: Job Attempt #1 Failed. Check job logs",
				)
				// This would fail the test case if the job does not exists
				th.GetJob(cell0.DBSyncJobName)

				// We don't store the failed job's hash.
				novaConductor := GetNovaConductor(cell0.ConductorName)
				Expect(novaConductor.Status.Hash).ShouldNot(HaveKey("dbsync"))

			})
		})

		When("DB sync job finishes successfully", func() {
			BeforeEach(func() {
				th.SimulateJobSuccess(cell0.DBSyncJobName)
			})

			It("reports that DB sync is ready and the job is configured to be deleted", func() {
				th.ExpectCondition(
					cell0.ConductorName,
					ConditionGetterFunc(NovaConductorConditionGetter),
					condition.DBSyncReadyCondition,
					corev1.ConditionTrue,
				)
				job := th.GetJob(cell0.DBSyncJobName)
				Expect(job.Spec.TTLSecondsAfterFinished).NotTo(BeNil())
			})

			It("stores the hash of the Job in the Status", func() {
				Eventually(func(g Gomega) {
					novaConductor := GetNovaConductor(cell0.ConductorName)
					g.Expect(novaConductor.Status.Hash).Should(HaveKeyWithValue("dbsync", Not(BeEmpty())))
				}, timeout, interval).Should(Succeed())

			})

			It("creates a StatefulSet for the nova-conductor service", func() {
				th.ExpectCondition(
					cell0.ConductorName,
					ConditionGetterFunc(NovaConductorConditionGetter),
					condition.DeploymentReadyCondition,
					corev1.ConditionFalse,
				)
				ss := th.GetStatefulSet(cell0.ConductorStatefulSetName)
				Expect(ss.Spec.Template.Spec.ServiceAccountName).To(Equal("nova-sa"))
				Expect(ss.Spec.Template.Spec.Containers).To(HaveLen(1))
				container := ss.Spec.Template.Spec.Containers[0]
				Expect(container.LivenessProbe.Exec.Command).To(
					Equal([]string{"/usr/bin/pgrep", "-r", "DRST", "nova-conductor"}))
				Expect(container.ReadinessProbe.Exec.Command).To(
					Equal([]string{"/usr/bin/pgrep", "-r", "DRST", "nova-conductor"}))

				th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
				th.ExpectCondition(
					cell0.ConductorName,
					ConditionGetterFunc(NovaConductorConditionGetter),
					condition.DeploymentReadyCondition,
					corev1.ConditionTrue,
				)
				th.ExpectCondition(
					cell0.ConductorName,
					ConditionGetterFunc(NovaConductorConditionGetter),
					condition.ReadyCondition,
					corev1.ConditionTrue,
				)
				conductor := GetNovaConductor(cell0.ConductorName)
				Expect(conductor.Status.ReadyCount).To(BeNumerically(">", 0))
			})

			It("creates the DB purge CronJob", func() {
				th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)

				conductor := GetNovaConductor(cell0.ConductorName)
				cron := GetCronJob(cell0.DBPurgeCronJobName)

				Expect(cron.Spec.Schedule).To(Equal(*conductor.Spec.DBPurge.Schedule))
				jobEnv := cron.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Env
				Expect(GetEnvVarValue(jobEnv, "ARCHIVE_AGE", "")).To(
					Equal(fmt.Sprintf("%d", *conductor.Spec.DBPurge.ArchiveAge)))
				Expect(GetEnvVarValue(jobEnv, "PURGE_AGE", "")).To(
					Equal(fmt.Sprintf("%d", *conductor.Spec.DBPurge.PurgeAge)))
				service := cron.Spec.JobTemplate.Labels["service"]
				Expect(service).To(Equal("nova-conductor"))
				nodeSelector := cron.Spec.JobTemplate.Spec.Template.Spec.NodeSelector
				Expect(nodeSelector).NotTo(BeNil())
				Expect(nodeSelector).To(Equal(map[string]string{"foo": "bar"}))

				th.ExpectCondition(
					cell0.ConductorName,
					ConditionGetterFunc(NovaConductorConditionGetter),
					condition.CronJobReadyCondition,
					corev1.ConditionTrue,
				)
			})
		})
	})

	When("NovaConductor is configured to preserve jobs", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateDefaultCellInternalSecret(cell0))

			spec := GetDefaultNovaConductorSpec(cell0)
			spec["preserveJobs"] = true
			DeferCleanup(th.DeleteInstance, CreateNovaConductor(cell0.ConductorName, spec))

			th.ExpectCondition(
				cell0.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("does not configure DB sync job to be deleted after it finished", func() {
			th.SimulateJobSuccess(cell0.DBSyncJobName)

			th.ExpectCondition(
				cell0.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionTrue,
			)
			Expect(th.GetJob(cell0.DBSyncJobName).Spec.TTLSecondsAfterFinished).To(BeNil())
		})

		It("does not configure DB sync job to be deleted after it failed", func() {
			th.SimulateJobFailure(cell0.DBSyncJobName)

			th.ExpectConditionWithDetails(
				cell0.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"DBsync job error occurred Internal error occurred: Job Attempt #1 Failed. Check job logs",
			)
			Expect(th.GetJob(cell0.DBSyncJobName).Spec.TTLSecondsAfterFinished).To(BeNil())
		})
	})

	When("PreserveJobs changed from true to false", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateDefaultCellInternalSecret(cell0))

			spec := GetDefaultNovaConductorSpec(cell0)
			spec["preserveJobs"] = true
			DeferCleanup(th.DeleteInstance, CreateNovaConductor(cell0.ConductorName, spec))

			th.SimulateJobSuccess(cell0.DBSyncJobName)
			th.ExpectCondition(
				cell0.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionTrue,
			)

			Expect(th.GetJob(cell0.DBSyncJobName).Spec.TTLSecondsAfterFinished).To(BeNil())

			// Update the NovaConductor to not preserve Jobs
			// Eventually is needed here to retry if the update returns conflict
			Eventually(func(g Gomega) {
				conductor := GetNovaConductor(cell0.ConductorName)
				conductor.Spec.PreserveJobs = false
				g.Expect(k8sClient.Update(ctx, conductor)).Should(Succeed())
			}, timeout, interval).Should(Succeed())
		})

		It("marks the job to be deleted", func() {
			Eventually(func(g Gomega) {
				g.Expect(th.GetJob(cell0.DBSyncJobName).Spec.TTLSecondsAfterFinished).NotTo(BeNil())
			}, timeout, interval).Should(Succeed())
		})
	})

})

var _ = Describe("NovaConductor controller", func() {
	BeforeEach(func() {

		mariadb.CreateMariaDBDatabase(cell0.MariaDBDatabaseName.Namespace, cell0.MariaDBDatabaseName.Name, mariadbv1.MariaDBDatabaseSpec{})
		DeferCleanup(k8sClient.Delete, ctx, mariadb.GetMariaDBDatabase(cell0.MariaDBDatabaseName))

		apiMariaDBAccount, apiMariaDBSecret := mariadb.CreateMariaDBAccountAndSecret(
			novaNames.APIMariaDBDatabaseAccount, mariadbv1.MariaDBAccountSpec{})
		DeferCleanup(k8sClient.Delete, ctx, apiMariaDBAccount)
		DeferCleanup(k8sClient.Delete, ctx, apiMariaDBSecret)

		cell0Account, cell0Secret := mariadb.CreateMariaDBAccountAndSecret(
			cell0.MariaDBAccountName, mariadbv1.MariaDBAccountSpec{})
		DeferCleanup(k8sClient.Delete, ctx, cell0Account)
		DeferCleanup(k8sClient.Delete, ctx, cell0Secret)

		memcachedSpec := infra.GetDefaultMemcachedSpec()
		DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(novaNames.NovaName.Namespace, MemcachedInstance, memcachedSpec))
		infra.SimulateMemcachedReady(novaNames.MemcachedNamespace)
	})

	When("NovaConductor is created with networkAttachments", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateDefaultCellInternalSecret(cell0))

			spec := GetDefaultNovaConductorSpec(cell0)
			spec["networkAttachments"] = []string{"internalapi"}
			DeferCleanup(th.DeleteInstance, CreateNovaConductor(cell0.ConductorName, spec))
		})

		It("reports that the definition is missing", func() {
			th.ExpectConditionWithDetails(
				cell0.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachment resources missing: internalapi",
			)
			th.ExpectCondition(
				cell0.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})
		It("reports that network attachment is missing", func() {
			nad := th.CreateNetworkAttachmentDefinition(novaNames.InternalAPINetworkNADName)
			DeferCleanup(th.DeleteInstance, nad)
			th.SimulateJobSuccess(cell0.DBSyncJobName)

			ss := th.GetStatefulSet(cell0.ConductorStatefulSetName)

			expectedAnnotation, err := json.Marshal(
				[]networkv1.NetworkSelectionElement{
					{
						Name:             "internalapi",
						Namespace:        cell0.ConductorName.Namespace,
						InterfaceRequest: "internalapi",
					}})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(ss.Spec.Template.ObjectMeta.Annotations).To(
				HaveKeyWithValue("k8s.v1.cni.cncf.io/networks", string(expectedAnnotation)),
			)

			// We don't add network attachment status annotations to the Pods
			// to simulate that the network attachments are missing.
			th.SimulateStatefulSetReplicaReadyWithPods(cell0.ConductorStatefulSetName, map[string][]string{})

			th.ExpectConditionWithDetails(
				cell0.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occurred "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
			)
		})
		It("reports that an IP is missing", func() {
			nad := th.CreateNetworkAttachmentDefinition(novaNames.InternalAPINetworkNADName)
			DeferCleanup(th.DeleteInstance, nad)
			th.SimulateJobSuccess(cell0.DBSyncJobName)

			ss := th.GetStatefulSet(cell0.ConductorStatefulSetName)

			expectedAnnotation, err := json.Marshal(
				[]networkv1.NetworkSelectionElement{
					{
						Name:             "internalapi",
						Namespace:        cell0.ConductorName.Namespace,
						InterfaceRequest: "internalapi",
					}})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(ss.Spec.Template.ObjectMeta.Annotations).To(
				HaveKeyWithValue("k8s.v1.cni.cncf.io/networks", string(expectedAnnotation)),
			)

			// We simulate that there is no IP associated with the internalapi
			// network attachment
			th.SimulateStatefulSetReplicaReadyWithPods(
				cell0.ConductorStatefulSetName,
				map[string][]string{cell0.ConductorName.Namespace + "/internalapi": {}},
			)

			th.ExpectConditionWithDetails(
				cell0.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occurred "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
			)
		})
		It("reports NetworkAttachmentsReady if the Pods got the proper annotations", func() {
			nad := th.CreateNetworkAttachmentDefinition(novaNames.InternalAPINetworkNADName)
			DeferCleanup(th.DeleteInstance, nad)
			th.SimulateJobSuccess(cell0.DBSyncJobName)

			th.SimulateStatefulSetReplicaReadyWithPods(
				cell0.ConductorStatefulSetName,
				map[string][]string{cell0.ConductorName.Namespace + "/internalapi": {"10.0.0.1"}},
			)

			th.ExpectCondition(
				cell0.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionTrue,
			)

			Eventually(func(g Gomega) {
				novaConductor := GetNovaConductor(cell0.ConductorName)
				g.Expect(novaConductor.Status.NetworkAttachments).To(
					Equal(map[string][]string{cell0.ConductorName.Namespace + "/internalapi": {"10.0.0.1"}}))
			}, timeout, interval).Should(Succeed())

			th.ExpectCondition(
				cell0.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})
	When("NovaConductor is reconfigured", func() {
		neutronEndpoint := types.NamespacedName{}
		cinderEndpoint := types.NamespacedName{}

		BeforeEach(func() {
			neutronEndpoint = types.NamespacedName{Name: "neutron", Namespace: novaNames.Namespace}
			DeferCleanup(keystone.DeleteKeystoneEndpoint, keystone.CreateKeystoneEndpoint(neutronEndpoint))
			keystone.SimulateKeystoneEndpointReady(neutronEndpoint)
			cinderEndpoint = types.NamespacedName{Name: "cinder", Namespace: novaNames.Namespace}
			DeferCleanup(keystone.DeleteKeystoneEndpoint, keystone.CreateKeystoneEndpoint(cinderEndpoint))
			keystone.SimulateKeystoneEndpointReady(cinderEndpoint)

			DeferCleanup(
				k8sClient.Delete, ctx, CreateDefaultCellInternalSecret(cell0))
			DeferCleanup(th.DeleteInstance, CreateNovaConductor(cell0.ConductorName, GetDefaultNovaConductorSpec(cell0)))

			th.ExpectCondition(
				cell0.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)

			th.SimulateJobSuccess(cell0.DBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
			th.ExpectCondition(
				cell0.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("applies new NetworkAttachments configuration", func() {
			Eventually(func(g Gomega) {
				novaConductor := GetNovaConductor(cell0.ConductorName)
				novaConductor.Spec.NetworkAttachments = append(novaConductor.Spec.NetworkAttachments, "internalapi")

				g.Expect(k8sClient.Update(ctx, novaConductor)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			th.ExpectConditionWithDetails(
				cell0.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachment resources missing: internalapi",
			)

			th.ExpectConditionWithDetails(
				cell0.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachment resources missing: internalapi",
			)

			DeferCleanup(th.DeleteInstance, th.CreateNetworkAttachmentDefinition(novaNames.InternalAPINetworkNADName))

			th.ExpectConditionWithDetails(
				cell0.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occurred "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
			)

			th.ExpectConditionWithDetails(
				cell0.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occurred "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
			)

			th.SimulateStatefulSetReplicaReadyWithPods(
				cell0.ConductorStatefulSetName,
				map[string][]string{cell0.ConductorName.Namespace + "/internalapi": {"10.0.0.1"}},
			)

			th.ExpectCondition(
				cell0.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionTrue,
			)

			Eventually(func(g Gomega) {
				novaConductor := GetNovaConductor(cell0.ConductorName)
				g.Expect(novaConductor.Status.NetworkAttachments).To(
					Equal(map[string][]string{cell0.ConductorName.Namespace + "/internalapi": {"10.0.0.1"}}))

			}, timeout, interval).Should(Succeed())

			th.ExpectCondition(
				cell0.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("updates the deployment if neutron internal endpoint changes", func() {
			originalConfigHash := GetEnvVarValue(
				th.GetStatefulSet(cell0.ConductorName).Spec.Template.Spec.Containers[0].Env, "CONFIG_HASH", "")
			Expect(originalConfigHash).NotTo(Equal(""))

			keystone.UpdateKeystoneEndpoint(neutronEndpoint, "internal", "https://neutron-internal")
			logger.Info("Reconfigured")

			// Assert that the CONFIG_HASH of the StateFulSet is changed due to this reconfiguration
			Eventually(func(g Gomega) {
				currentConfigHash := GetEnvVarValue(
					th.GetStatefulSet(cell0.ConductorName).Spec.Template.Spec.Containers[0].Env, "CONFIG_HASH", "")
				g.Expect(originalConfigHash).NotTo(Equal(currentConfigHash))

			}, timeout, interval).Should(Succeed())
		})

		It("updates the deployment if neutron internal endpoint gets deleted", func() {
			originalConfigHash := GetEnvVarValue(
				th.GetStatefulSet(cell0.ConductorName).Spec.Template.Spec.Containers[0].Env, "CONFIG_HASH", "")
			Expect(originalConfigHash).NotTo(Equal(""))

			keystone.DeleteKeystoneEndpoint(neutronEndpoint)
			logger.Info("Reconfigured")

			// Assert that the CONFIG_HASH of the StateFulSet is changed due to this reconfiguration
			Eventually(func(g Gomega) {
				currentConfigHash := GetEnvVarValue(
					th.GetStatefulSet(cell0.ConductorName).Spec.Template.Spec.Containers[0].Env, "CONFIG_HASH", "")
				g.Expect(originalConfigHash).NotTo(Equal(currentConfigHash))

			}, timeout, interval).Should(Succeed())
		})
	})

	When("NovaConductor CR is created without container image defined", func() {
		BeforeEach(func() {
			spec := GetDefaultNovaConductorSpec(cell0)
			spec["containerImage"] = ""
			conductor := CreateNovaConductor(cell0.ConductorName, spec)
			DeferCleanup(th.DeleteInstance, conductor)
		})
		It("has the expected container image default", func() {
			novaConductorDefault := GetNovaConductor(cell0.ConductorName)
			Expect(novaConductorDefault.Spec.ContainerImage).To(Equal(util.GetEnvVar("RELATED_IMAGE_NOVA_CONDUCTOR_IMAGE_URL_DEFAULT", novav1.NovaConductorContainerImage)))
		})
	})

	When("NovaConductor is created with a wrong topologyRef", func() {
		BeforeEach(func() {
			spec := GetDefaultNovaConductorSpec(cell0)
			spec["topologyRef"] = map[string]any{"name": "foo"}
			conductor := CreateNovaConductor(cell0.ConductorName, spec)
			DeferCleanup(th.DeleteInstance, conductor)
		})
		It("points to a non existing topology CR", func() {
			th.ExpectCondition(
				cell0.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.TopologyReadyCondition,
				corev1.ConditionUnknown,
			)
			th.ExpectCondition(
				cell0.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})
	})

	When("NovaConductor is created with topology", func() {
		var topologyRefCnd topologyv1.TopoRef
		var topologyRefAlt topologyv1.TopoRef
		var expectedTopologySpec []corev1.TopologySpreadConstraint
		BeforeEach(func() {
			var topologySpec map[string]any
			// Build the topology Spec
			topologySpec, expectedTopologySpec = GetSampleTopologySpec(cell0.ConductorName.Name)
			// Create Test Topologies
			_, topologyRefAlt = infra.CreateTopology(novaNames.NovaTopologies[0], topologySpec)
			_, topologyRefCnd = infra.CreateTopology(novaNames.NovaTopologies[4], topologySpec)

			DeferCleanup(
				k8sClient.Delete, ctx, CreateDefaultCellInternalSecret(cell0))

			spec := GetDefaultNovaConductorSpec(cell0)
			spec["topologyRef"] = map[string]any{"name": topologyRefCnd.Name}

			DeferCleanup(th.DeleteInstance, CreateNovaConductor(cell0.ConductorName, spec))
		})
		It("sets lastAppliedTopology field in NovaConductor topology .Status", func() {
			th.SimulateJobSuccess(cell0.DBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
			cnd := GetNovaConductor(cell0.ConductorName)

			Expect(cnd.Status.LastAppliedTopology).ToNot(BeNil())
			Expect(cnd.Status.LastAppliedTopology).To(Equal(&topologyRefCnd))

			th.ExpectCondition(
				cell0.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.TopologyReadyCondition,
				corev1.ConditionTrue,
			)

			Eventually(func(g Gomega) {
				ss := th.GetStatefulSet(cell0.ConductorName)
				podTemplate := ss.Spec.Template.Spec
				g.Expect(podTemplate.TopologySpreadConstraints).ToNot(BeNil())
				// No default Pod Antiaffinity is applied
				g.Expect(podTemplate.Affinity).To(BeNil())
			}, timeout, interval).Should(Succeed())

			// Check finalizer
			Eventually(func(g Gomega) {
				tp := infra.GetTopology(types.NamespacedName{
					Name:      topologyRefCnd.Name,
					Namespace: topologyRefCnd.Namespace,
				})
				finalizers := tp.GetFinalizers()
				g.Expect(finalizers).To(ContainElement(
					fmt.Sprintf("openstack.org/novaconductor-%s", cell0.ConductorName.Name)))
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				tpAlt := infra.GetTopology(types.NamespacedName{
					Name:      topologyRefAlt.Name,
					Namespace: topologyRefAlt.Namespace,
				})
				finalizers := tpAlt.GetFinalizers()
				g.Expect(finalizers).ToNot(ContainElement(
					fmt.Sprintf("openstack.org/novaconductor-%s", cell0.ConductorName.Name)))
			}, timeout, interval).Should(Succeed())
		})
		It("updates lastAppliedTopology in NovaConductor .Status", func() {
			Eventually(func(g Gomega) {
				cnd := GetNovaConductor(cell0.ConductorName)
				cnd.Spec.TopologyRef.Name = topologyRefAlt.Name
				g.Expect(k8sClient.Update(ctx, cnd)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			th.SimulateJobSuccess(cell0.DBSyncJobName)
			Eventually(func(g Gomega) {
				cnd := GetNovaConductor(cell0.ConductorName)
				g.Expect(cnd.Status.LastAppliedTopology).ToNot(BeNil())
				g.Expect(cnd.Status.LastAppliedTopology).To(Equal(&topologyRefAlt))
			}, timeout, interval).Should(Succeed())

			th.ExpectCondition(
				cell0.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.TopologyReadyCondition,
				corev1.ConditionTrue,
			)

			Eventually(func(g Gomega) {
				ss := th.GetStatefulSet(cell0.ConductorName)
				podTemplate := ss.Spec.Template.Spec
				g.Expect(podTemplate.TopologySpreadConstraints).ToNot(BeNil())
				// No default Pod Antiaffinity is applied
				g.Expect(podTemplate.Affinity).To(BeNil())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				ss := th.GetStatefulSet(cell0.ConductorName)
				podTemplate := ss.Spec.Template.Spec
				g.Expect(podTemplate.TopologySpreadConstraints).To(Equal(expectedTopologySpec))
			}, timeout, interval).Should(Succeed())

			// Check finalizer is set to topologyRefAlt and is not set to
			// topologyRef
			Eventually(func(g Gomega) {
				tp := infra.GetTopology(types.NamespacedName{
					Name:      topologyRefCnd.Name,
					Namespace: topologyRefCnd.Namespace,
				})
				finalizers := tp.GetFinalizers()
				g.Expect(finalizers).ToNot(ContainElement(
					fmt.Sprintf("openstack.org/novaconductor-%s", cell0.ConductorName.Name)))
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				tpAlt := infra.GetTopology(types.NamespacedName{
					Name:      topologyRefAlt.Name,
					Namespace: topologyRefAlt.Namespace,
				})
				finalizers := tpAlt.GetFinalizers()
				g.Expect(finalizers).To(ContainElement(
					fmt.Sprintf("openstack.org/novaconductor-%s", cell0.ConductorName.Name)))
			}, timeout, interval).Should(Succeed())
		})
		It("removes topologyRef from NovaConductor spec", func() {
			Eventually(func(g Gomega) {
				cnd := GetNovaConductor(cell0.ConductorName)
				cnd.Spec.TopologyRef = nil
				g.Expect(k8sClient.Update(ctx, cnd)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			th.SimulateJobSuccess(cell0.DBSyncJobName)
			Eventually(func(g Gomega) {
				cnd := GetNovaConductor(cell0.ConductorName)
				g.Expect(cnd.Status.LastAppliedTopology).Should(BeNil())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				ss := th.GetStatefulSet(cell0.ConductorName)
				podTemplate := ss.Spec.Template.Spec
				g.Expect(podTemplate.TopologySpreadConstraints).To(BeNil())
				// Default Pod AntiAffinity is applied
				g.Expect(podTemplate.Affinity).ToNot(BeNil())
			}, timeout, interval).Should(Succeed())

			// Check finalizer is not present anymore
			Eventually(func(g Gomega) {
				tp := infra.GetTopology(types.NamespacedName{
					Name:      topologyRefCnd.Name,
					Namespace: topologyRefCnd.Namespace,
				})
				finalizers := tp.GetFinalizers()
				g.Expect(finalizers).ToNot(ContainElement(
					fmt.Sprintf("openstack.org/novaconductor-%s", cell0.ConductorName.Name)))
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				tpAlt := infra.GetTopology(types.NamespacedName{
					Name:      topologyRefAlt.Name,
					Namespace: topologyRefAlt.Namespace,
				})
				finalizers := tpAlt.GetFinalizers()
				g.Expect(finalizers).ToNot(ContainElement(
					fmt.Sprintf("openstack.org/novaconductor-%s", cell0.ConductorName.Name)))
			}, timeout, interval).Should(Succeed())
		})
	})
})

var _ = Describe("NovaConductor controller cleaning", func() {
	BeforeEach(func() {
		apiMariaDBAccount, apiMariaDBSecret := mariadb.CreateMariaDBAccountAndSecret(
			novaNames.APIMariaDBDatabaseAccount, mariadbv1.MariaDBAccountSpec{})
		DeferCleanup(k8sClient.Delete, ctx, apiMariaDBAccount)
		DeferCleanup(k8sClient.Delete, ctx, apiMariaDBSecret)

		cell0Account, cell0Secret := mariadb.CreateMariaDBAccountAndSecret(
			cell0.MariaDBAccountName, mariadbv1.MariaDBAccountSpec{})
		DeferCleanup(k8sClient.Delete, ctx, cell0Account)
		DeferCleanup(k8sClient.Delete, ctx, cell0Secret)

	})

	var novaAPIServer *NovaAPIFixture
	BeforeEach(func() {
		mariadb.CreateMariaDBDatabase(cell0.MariaDBDatabaseName.Namespace, cell0.MariaDBDatabaseName.Name, mariadbv1.MariaDBDatabaseSpec{})
		DeferCleanup(k8sClient.Delete, ctx, mariadb.GetMariaDBDatabase(cell0.MariaDBDatabaseName))

		memcachedSpec := infra.GetDefaultMemcachedSpec()
		DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(novaNames.NovaName.Namespace, MemcachedInstance, memcachedSpec))
		infra.SimulateMemcachedReady(novaNames.MemcachedNamespace)

		spec := GetDefaultNovaConductorSpec(cell0)
		// wire up the api simulator by using its endpoint as the keystone
		// URL used by the conductor controller.
		keystoneFixture, f := SetupAPIFixtures(logger)
		novaAPIServer = f
		spec["keystoneAuthURL"] = keystoneFixture.Endpoint()
		// Explicitly set region to match the test fixture's service catalog
		spec["region"] = "regionOne"
		DeferCleanup(th.DeleteInstance, CreateNovaConductor(cell0.ConductorName, spec))
		DeferCleanup(
			k8sClient.Delete, ctx, CreateDefaultCellInternalSecret(cell0))
		th.SimulateJobSuccess(cell0.DBSyncJobName)
		th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
	})
	When("NovaConductor down service is removed from api", func() {
		It("during reconciling", func() {
			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
			th.ExpectCondition(
				cell0.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionTrue,
			)
			Expect(novaAPIServer.HasRequest("GET", "/compute/os-services/", "binary=nova-conductor")).To(BeTrue())
			Expect(novaAPIServer.HasRequest("DELETE", "/compute/os-services/5", "")).NotTo(BeTrue())
			// ID(6, 7)should not delete as these are from cell1 and not from cell0
			Expect(novaAPIServer.HasRequest("DELETE", "/compute/os-services/6", "")).NotTo(BeTrue())
			Expect(novaAPIServer.HasRequest("DELETE", "/compute/os-services/7", "")).NotTo(BeTrue())
			Expect(novaAPIServer.HasRequest("DELETE", "/compute/os-services/8", "")).To(BeTrue())
			req := novaAPIServer.FindRequest("DELETE", "/compute/os-services/8", "")
			Expect(req.Header.Get("X-OpenStack-Nova-API-Version")).To(Equal("2.95"))
		})
	})
})

var _ = Describe("NovaConductor controller", func() {
	BeforeEach(func() {
		apiMariaDBAccount, apiMariaDBSecret := mariadb.CreateMariaDBAccountAndSecret(
			novaNames.APIMariaDBDatabaseAccount, mariadbv1.MariaDBAccountSpec{})
		DeferCleanup(k8sClient.Delete, ctx, apiMariaDBAccount)
		DeferCleanup(k8sClient.Delete, ctx, apiMariaDBSecret)

		cell0Account, cell0Secret := mariadb.CreateMariaDBAccountAndSecret(
			cell0.MariaDBAccountName, mariadbv1.MariaDBAccountSpec{})
		DeferCleanup(k8sClient.Delete, ctx, cell0Account)
		DeferCleanup(k8sClient.Delete, ctx, cell0Secret)

		mariadb.CreateMariaDBDatabase(cell0.MariaDBDatabaseName.Namespace, cell0.MariaDBDatabaseName.Name, mariadbv1.MariaDBDatabaseSpec{})
		mariadb.SimulateMariaDBTLSDatabaseCompleted(cell0.MariaDBDatabaseName)
		memcachedSpec := infra.GetDefaultMemcachedSpec()
		DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(novaNames.NovaName.Namespace, MemcachedInstance, memcachedSpec))
		infra.SimulateTLSMemcachedReady(novaNames.MemcachedNamespace)
	})

	When("NovaConductor CR instance is deleted", func() {
		BeforeEach(func() {

			DeferCleanup(
				k8sClient.Delete, ctx, CreateDefaultCellInternalSecret(cell0))
			DeferCleanup(th.DeleteInstance, CreateNovaConductor(cell0.ConductorName, GetDefaultNovaConductorSpec(cell0)))
		})

		It("removes the finalizer from Memcached", func() {
			th.SimulateJobSuccess(cell0.DBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
			th.ExpectCondition(
				cell0.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
			memcached := infra.GetMemcached(novaNames.MemcachedNamespace)
			Expect(memcached.Finalizers).To(ContainElement("openstack.org/novaconductor"))

			Eventually(func(g Gomega) {
				th.DeleteInstance(GetNovaConductor(cell0.ConductorName))
				memcached := infra.GetMemcached(novaNames.MemcachedNamespace)
				g.Expect(memcached.Finalizers).NotTo(ContainElement("openstack.org/novaconductor"))
			}, timeout, interval).Should(Succeed())
		})
	})

	When("NovaConductor is created with TLS CA cert secret", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateDefaultCellInternalSecret(cell0))

			spec := GetDefaultNovaConductorSpec(cell0)
			spec["tls"] = map[string]any{
				"caBundleSecretName": novaNames.CaBundleSecretName.Name,
			}
			DeferCleanup(th.DeleteInstance, CreateNovaConductor(cell0.ConductorName, spec))
		})

		It("reports that the CA secret is missing", func() {
			th.ExpectConditionWithDetails(
				cell0.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				fmt.Sprintf("TLSInput is missing: %s", novaNames.CaBundleSecretName.Name),
			)
			th.ExpectCondition(
				cell0.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("creates a StatefulSet for nova-conductor service with TLS CA cert attached", func() {
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCABundleSecret(novaNames.CaBundleSecretName))
			th.SimulateJobSuccess(cell0.DBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)

			th.ExpectCondition(
				cell0.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionTrue,
			)

			ss := th.GetStatefulSet(cell0.ConductorStatefulSetName)

			// Check the resulting deployment fields
			Expect(int(*ss.Spec.Replicas)).To(Equal(1))
			Expect(ss.Spec.Template.Spec.Volumes).To(HaveLen(2))
			Expect(ss.Spec.Template.Spec.Containers).To(HaveLen(1))

			// cert deployment volumes
			th.AssertVolumeExists(novaNames.CaBundleSecretName.Name, ss.Spec.Template.Spec.Volumes)

			// CA container certs
			apiContainer := ss.Spec.Template.Spec.Containers[0]
			th.AssertVolumeMountExists(novaNames.CaBundleSecretName.Name, "tls-ca-bundle.pem", apiContainer.VolumeMounts)

			th.ExpectCondition(
				cell0.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)

			configDataMap := th.GetSecret(cell0.ConductorConfigDataName)
			Expect(configDataMap).ShouldNot(BeNil())

			configData := string(configDataMap.Data["01-nova.conf"])
			Expect(configData).Should(
				ContainSubstring("backend = oslo_cache.memcache_pool"))
			Expect(configData).Should(
				ContainSubstring(fmt.Sprintf("memcache_servers=memcached-0.memcached.%s.svc:11211,memcached-1.memcached.%s.svc:11211,memcached-2.memcached.%s.svc:11211",
					novaNames.Namespace, novaNames.Namespace, novaNames.Namespace)))
			Expect(configData).Should(
				ContainSubstring(fmt.Sprintf("memcached_servers=memcached-0.memcached.%s.svc:11211,memcached-1.memcached.%s.svc:11211,memcached-2.memcached.%s.svc:11211",
					novaNames.Namespace, novaNames.Namespace, novaNames.Namespace)))
			Expect(configData).Should(
				ContainSubstring("tls_enabled=true"))

			myCnf := configDataMap.Data["my.cnf"]
			Expect(myCnf).To(
				ContainSubstring("[client]\nssl-ca=/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem\nssl=1"))
		})

		It("reconfigures the NovaConductor pod when CA changes", func() {
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCABundleSecret(novaNames.CaBundleSecretName))
			th.SimulateJobSuccess(cell0.DBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)

			ss := th.GetStatefulSet(cell0.ConductorStatefulSetName)

			// Check the resulting deployment fields
			Expect(int(*ss.Spec.Replicas)).To(Equal(1))
			Expect(ss.Spec.Template.Spec.Volumes).To(HaveLen(2))
			Expect(ss.Spec.Template.Spec.Containers).To(HaveLen(1))

			// Grab the current config hash
			originalHash := GetEnvVarValue(
				th.GetStatefulSet(cell0.ConductorStatefulSetName).Spec.Template.Spec.Containers[0].Env, "CONFIG_HASH", "")
			Expect(originalHash).NotTo(BeEmpty())

			// Change the content of the CA secret
			th.UpdateSecret(novaNames.CaBundleSecretName, "tls-ca-bundle.pem", []byte("DifferentCAData"))

			// Assert that the deployment is updated
			Eventually(func(g Gomega) {
				newHash := GetEnvVarValue(
					th.GetStatefulSet(cell0.ConductorStatefulSetName).Spec.Template.Spec.Containers[0].Env, "CONFIG_HASH", "")
				g.Expect(newHash).NotTo(BeEmpty())
				g.Expect(newHash).NotTo(Equal(originalHash))
			}, timeout, interval).Should(Succeed())
		})
	})
})

var _ = Describe("NovaConductor controller", func() {
	BeforeEach(func() {
		apiMariaDBAccount, apiMariaDBSecret := mariadb.CreateMariaDBAccountAndSecret(
			novaNames.APIMariaDBDatabaseAccount, mariadbv1.MariaDBAccountSpec{})
		DeferCleanup(k8sClient.Delete, ctx, apiMariaDBAccount)
		DeferCleanup(k8sClient.Delete, ctx, apiMariaDBSecret)

		cell0Account, cell0Secret := mariadb.CreateMariaDBAccountAndSecret(
			cell0.MariaDBAccountName, mariadbv1.MariaDBAccountSpec{})
		DeferCleanup(k8sClient.Delete, ctx, cell0Account)
		DeferCleanup(k8sClient.Delete, ctx, cell0Secret)

		mariadb.CreateMariaDBDatabase(cell0.MariaDBDatabaseName.Namespace, cell0.MariaDBDatabaseName.Name, mariadbv1.MariaDBDatabaseSpec{})
		mariadb.SimulateMariaDBTLSDatabaseCompleted(cell0.MariaDBDatabaseName)
		memcachedSpec := infra.GetDefaultMemcachedSpec()
		// Create Memcached with MTLS auth
		DeferCleanup(infra.DeleteMemcached, infra.CreateMTLSMemcached(novaNames.NovaName.Namespace, MemcachedInstance, memcachedSpec))
		infra.SimulateMTLSMemcachedReady(novaNames.MemcachedNamespace)
	})

	When("NovaConductor is configured for MTLS memcached auth", func() {
		BeforeEach(func() {
			spec := GetDefaultNovaConductorSpec(cell0)
			spec["tls"] = map[string]any{
				"caBundleSecretName": novaNames.CaBundleSecretName.Name,
			}

			DeferCleanup(
				k8sClient.Delete, ctx, CreateDefaultCellInternalSecret(cell0))
			DeferCleanup(th.DeleteInstance, CreateNovaConductor(cell0.ConductorName, spec))
		})

		It("creates a StatefulSet for nova-conductor service with MTLS certs attached", func() {
			format.MaxLength = 0
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCABundleSecret(novaNames.CaBundleSecretName))

			th.SimulateJobSuccess(cell0.DBSyncJobName)
			th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)

			th.ExpectCondition(
				cell0.ConductorName,
				ConditionGetterFunc(NovaConductorConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionTrue,
			)

			ss := th.GetStatefulSet(cell0.ConductorStatefulSetName)
			// Check the resulting statefulset fields
			Expect(int(*ss.Spec.Replicas)).To(Equal(1))

			// MTLS additional volume
			Expect(ss.Spec.Template.Spec.Volumes).To(HaveLen(3))
			Expect(ss.Spec.Template.Spec.Containers).To(HaveLen(1))

			// MTLS additional volume
			th.AssertVolumeExists(novaNames.MTLSSecretName.Name, ss.Spec.Template.Spec.Volumes)

			// conductor container certs
			conductorContainer := ss.Spec.Template.Spec.Containers[0]

			// MTLS additional volumemounts
			th.AssertVolumeMountExists(novaNames.MTLSSecretName.Name, "tls.key", conductorContainer.VolumeMounts)
			th.AssertVolumeMountExists(novaNames.MTLSSecretName.Name, "tls.crt", conductorContainer.VolumeMounts)
			th.AssertVolumeMountExists(novaNames.MTLSSecretName.Name, "ca.crt", conductorContainer.VolumeMounts)

			configDataMap := th.GetSecret(cell0.ConductorConfigDataName)
			Expect(configDataMap).ShouldNot(BeNil())

			configData := string(configDataMap.Data["01-nova.conf"])

			// MTLS - [cache]
			Expect(configData).Should(
				ContainSubstring("tls_certfile=/etc/pki/tls/certs/mtls.crt"))
			Expect(configData).Should(
				ContainSubstring("tls_keyfile=/etc/pki/tls/private/mtls.key"))
			Expect(configData).Should(
				ContainSubstring("tls_cafile=/etc/pki/tls/certs/mtls-ca.crt"))
			// MTLS - [keystone_authtoken]
			Expect(configData).Should(
				ContainSubstring("memcache_tls_certfile = /etc/pki/tls/certs/mtls.crt"))
			Expect(configData).Should(
				ContainSubstring("memcache_tls_keyfile = /etc/pki/tls/private/mtls.key"))
			Expect(configData).Should(
				ContainSubstring("memcache_tls_cafile = /etc/pki/tls/certs/mtls-ca.crt"))
			Expect(configData).Should(
				ContainSubstring("memcache_tls_enabled = true"))
		})

	})
})
