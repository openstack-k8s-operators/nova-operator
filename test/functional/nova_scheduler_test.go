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
	"net/http"

	. "github.com/onsi/ginkgo/v2" //revive:disable:dot-imports
	. "github.com/onsi/gomega"    //revive:disable:dot-imports

	//revive:disable-next-line:dot-imports
	. "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"

	networkv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	memcachedv1 "github.com/openstack-k8s-operators/infra-operator/apis/memcached/v1beta1"
	keystone_helper "github.com/openstack-k8s-operators/keystone-operator/api/test/helpers"
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
	api "github.com/openstack-k8s-operators/lib-common/modules/test/apis"
	mariadbv1 "github.com/openstack-k8s-operators/mariadb-operator/api/v1beta1"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

var _ = Describe("NovaScheduler controller", func() {
	BeforeEach(func() {
		mariadb.CreateMariaDBDatabase(novaNames.APIMariaDBDatabaseName.Namespace, novaNames.APIMariaDBDatabaseName.Name, mariadbv1.MariaDBDatabaseSpec{})
		DeferCleanup(k8sClient.Delete, ctx, mariadb.GetMariaDBDatabase(novaNames.APIMariaDBDatabaseName))

		apiMariaDBAccount, apiMariaDBSecret := mariadb.CreateMariaDBAccountAndSecret(novaNames.APIMariaDBDatabaseAccount, mariadbv1.MariaDBAccountSpec{})
		DeferCleanup(k8sClient.Delete, ctx, apiMariaDBAccount)
		DeferCleanup(k8sClient.Delete, ctx, apiMariaDBSecret)

		cell0Account, cell0Secret := mariadb.CreateMariaDBAccountAndSecret(cell0.MariaDBAccountName, mariadbv1.MariaDBAccountSpec{})
		DeferCleanup(k8sClient.Delete, ctx, cell0Account)
		DeferCleanup(k8sClient.Delete, ctx, cell0Secret)
	})

	BeforeEach(func() {
		// Uncomment this if you need the full output in the logs from gomega
		// matchers
		// format.MaxLength = 0
		memcachedSpec := memcachedv1.MemcachedSpec{
			MemcachedSpecCore: memcachedv1.MemcachedSpecCore{
				Replicas: ptr.To(int32(3)),
			},
		}
		DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(novaNames.NovaName.Namespace, MemcachedInstance, memcachedSpec))
		infra.SimulateMemcachedReady(novaNames.MemcachedNamespace)

		novaAPIServer := NewNovaAPIFixtureWithServer(logger)
		novaAPIServer.Setup()
		DeferCleanup(novaAPIServer.Cleanup)
		f := keystone_helper.NewKeystoneAPIFixtureWithServer(logger)
		text := ResponseHandleToken(f.Endpoint(), novaAPIServer.Endpoint())
		f.Setup(
			api.Handler{Pattern: "/", Func: f.HandleVersion},
			api.Handler{Pattern: "/v3/users", Func: f.HandleUsers},
			api.Handler{Pattern: "/v3/domains", Func: f.HandleDomains},
			api.Handler{Pattern: "/v3/auth/tokens", Func: func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case "POST":
					w.Header().Add("Content-Type", "application/json")
					w.WriteHeader(202)
					fmt.Fprint(w, text)
				}
			}})
		DeferCleanup(f.Cleanup)
		keystoneName := keystone.CreateKeystoneAPIWithFixture(novaNames.NovaName.Namespace, f)
		keystoneInstance := keystone.GetKeystoneAPI(keystoneName)
		keystoneInstance.Status = keystonev1.KeystoneAPIStatus{
			APIEndpoints: map[string]string{
				"public":   f.Endpoint(),
				"internal": f.Endpoint(),
			},
		}
		Expect(th.K8sClient.Status().Update(th.Ctx, keystoneInstance.DeepCopy())).Should(Succeed())

		DeferCleanup(keystone.DeleteKeystoneAPI, keystoneName)
		DeferCleanup(
			k8sClient.Delete, ctx, CreateNovaSecret(novaNames.NovaName.Namespace, SecretName))

		spec := GetDefaultNovaSchedulerSpec(novaNames)
		spec["customServiceConfig"] = "foo=bar"
		DeferCleanup(th.DeleteInstance, CreateNovaScheduler(novaNames.SchedulerName, spec))
	})

	When("a NovaScheduler CR is created pointing to a non existent Secret", func() {
		It("is not Ready", func() {
			th.ExpectCondition(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("has empty Status fields", func() {
			instance := GetNovaScheduler(novaNames.SchedulerName)
			// NOTE(gibi): Hash and Endpoints have `omitempty` tags so while
			// they are initialized to {} that value is omitted from the output
			// when sent to the client. So we see nils here.
			Expect(instance.Status.Hash).To(BeEmpty())
			Expect(instance.Status.ReadyCount).To(Equal(int32(0)))
		})

		It("is missing the secret", func() {
			th.ExpectCondition(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionFalse,
			)
		})

	})

	When("an unrelated Secret is created the CR state does not change", func() {
		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "not-relevant-secret",
					Namespace: novaNames.SchedulerName.Namespace,
				},
			}
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
			DeferCleanup(k8sClient.Delete, ctx, secret)
		})

		It("is not Ready", func() {
			th.ExpectCondition(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("is missing the secret", func() {
			th.ExpectCondition(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionFalse,
			)
		})

	})

	When("the Secret is created but some fields are missing", func() {
		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      novaNames.InternalTopLevelSecretName.Name,
					Namespace: novaNames.InternalTopLevelSecretName.Namespace,
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
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("reports that the inputs are not ready", func() {
			th.ExpectCondition(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionFalse,
			)
		})
	})

	When("the Secret is created with all the expected fields", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateInternalTopLevelSecret(novaNames))
		})

		It("reports that input is ready", func() {
			th.ExpectCondition(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("generated configs successfully", func() {
			// NOTE(gibi): NovaScheduler has no external dependency right now to
			// generate the configs.
			th.ExpectCondition(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)

			configDataMap := th.GetSecret(novaNames.SchedulerConfigDataName)
			Expect(configDataMap).ShouldNot(BeNil())
			Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
			configData := string(configDataMap.Data["01-nova.conf"])
			Expect(configData).To(ContainSubstring("transport_url=rabbit://api/fake"))
			Expect(configData).To(ContainSubstring("password = service-password"))
			Expect(configData).Should(
				ContainSubstring(fmt.Sprintf("memcache_servers=memcached-0.memcached.%s.svc:11211,memcached-1.memcached.%s.svc:11211,memcached-2.memcached.%s.svc:11211",
					novaNames.Namespace, novaNames.Namespace, novaNames.Namespace)))
			Expect(configData).Should(
				ContainSubstring(fmt.Sprintf("memcached_servers=inet:[memcached-0.memcached.%s.svc]:11211,inet:[memcached-1.memcached.%s.svc]:11211,inet:[memcached-2.memcached.%s.svc]:11211",
					novaNames.Namespace, novaNames.Namespace, novaNames.Namespace)))
			Expect(configData).Should(
				ContainSubstring("tls_enabled=false"))
			Expect(configData).Should(
				ContainSubstring("[upgrade_levels]\ncompute = auto"))
			Expect(configDataMap.Data).Should(HaveKey("02-nova-override.conf"))
			myCnf := configDataMap.Data["my.cnf"]
			Expect(myCnf).To(
				ContainSubstring("[client]\nssl=0"))
			extraConfigData := string(configDataMap.Data["02-nova-override.conf"])
			Expect(extraConfigData).To(Equal("foo=bar"))
		})

		It("stored the input hash in the Status", func() {
			Eventually(func(g Gomega) {
				novaScheduler := GetNovaScheduler(novaNames.SchedulerName)
				g.Expect(novaScheduler.Status.Hash).Should(HaveKeyWithValue("input", Not(BeEmpty())))
			}, timeout, interval).Should(Succeed())

		})
	})

	When("the NovaScheduler is created with a proper Secret", func() {

		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateInternalTopLevelSecret(novaNames))
		})

		It(" reports input ready", func() {
			th.ExpectCondition(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("creates a StatefulSet for the nova-scheduler service", func() {
			th.ExpectConditionWithDetails(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				condition.DeploymentReadyRunningMessage,
			)

			ss := th.GetStatefulSet(novaNames.SchedulerStatefulSetName)
			Expect(ss.Spec.Template.Spec.ServiceAccountName).To(Equal("nova-sa"))
			Expect(int(*ss.Spec.Replicas)).To(Equal(1))
			Expect(ss.Spec.Template.Spec.Volumes).To(HaveLen(1))
			Expect(ss.Spec.Template.Spec.Containers).To(HaveLen(1))

			container := ss.Spec.Template.Spec.Containers[0]
			Expect(container.VolumeMounts).To(HaveLen(2))
			Expect(container.Image).To(Equal(ContainerImage))
		})

		When("the StatefulSet has at least one Replica ready", func() {
			BeforeEach(func() {
				th.ExpectConditionWithDetails(
					novaNames.SchedulerName,
					ConditionGetterFunc(NovaSchedulerConditionGetter),
					condition.DeploymentReadyCondition,
					corev1.ConditionFalse,
					condition.RequestedReason,
					condition.DeploymentReadyRunningMessage,
				)
				th.SimulateStatefulSetReplicaReady(novaNames.SchedulerStatefulSetName)
			})

			It("reports that the StatefulSet is ready", func() {
				th.ExpectCondition(
					novaNames.SchedulerName,
					ConditionGetterFunc(NovaSchedulerConditionGetter),
					condition.DeploymentReadyCondition,
					corev1.ConditionTrue,
				)

				novaScheduler := GetNovaScheduler(novaNames.SchedulerName)
				Expect(novaScheduler.Status.ReadyCount).To(BeNumerically(">", 0))
			})
		})

		It("is Ready", func() {
			th.SimulateStatefulSetReplicaReady(novaNames.SchedulerStatefulSetName)
			th.ExpectCondition(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})
})

var _ = Describe("NovaScheduler controller", func() {
	BeforeEach(func() {
		mariadb.CreateMariaDBDatabase(novaNames.APIMariaDBDatabaseName.Namespace, novaNames.APIMariaDBDatabaseName.Name, mariadbv1.MariaDBDatabaseSpec{})
		DeferCleanup(k8sClient.Delete, ctx, mariadb.GetMariaDBDatabase(novaNames.APIMariaDBDatabaseName))

		apiMariaDBAccount, apiMariaDBSecret := mariadb.CreateMariaDBAccountAndSecret(
			novaNames.APIMariaDBDatabaseAccount, mariadbv1.MariaDBAccountSpec{})
		DeferCleanup(k8sClient.Delete, ctx, apiMariaDBAccount)
		DeferCleanup(k8sClient.Delete, ctx, apiMariaDBSecret)

		cell0Account, cell0Secret := mariadb.CreateMariaDBAccountAndSecret(
			cell0.MariaDBAccountName, mariadbv1.MariaDBAccountSpec{})
		DeferCleanup(k8sClient.Delete, ctx, cell0Account)
		DeferCleanup(k8sClient.Delete, ctx, cell0Secret)
		memcachedSpec := memcachedv1.MemcachedSpec{
			MemcachedSpecCore: memcachedv1.MemcachedSpecCore{
				Replicas: ptr.To(int32(3)),
			},
		}
		DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(novaNames.NovaName.Namespace, MemcachedInstance, memcachedSpec))
		infra.SimulateMemcachedReady(novaNames.MemcachedNamespace)

	})

	When("NovaScheduler is created with networkAttachments", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateInternalTopLevelSecret(novaNames))

			spec := GetDefaultNovaSchedulerSpec(novaNames)
			spec["networkAttachments"] = []string{"internalapi"}
			DeferCleanup(th.DeleteInstance, CreateNovaScheduler(novaNames.SchedulerName, spec))
		})

		It("reports that the definition is missing", func() {
			th.ExpectConditionWithDetails(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NetworkAttachment resources missing: internalapi",
			)
			th.ExpectCondition(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})
		It("reports that network attachment is missing", func() {
			nad := th.CreateNetworkAttachmentDefinition(novaNames.InternalAPINetworkNADName)
			DeferCleanup(th.DeleteInstance, nad)

			ss := th.GetStatefulSet(novaNames.SchedulerStatefulSetName)

			expectedAnnotation, err := json.Marshal(
				[]networkv1.NetworkSelectionElement{
					{
						Name:             "internalapi",
						Namespace:        novaNames.SchedulerName.Namespace,
						InterfaceRequest: "internalapi",
					}})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(ss.Spec.Template.ObjectMeta.Annotations).To(
				HaveKeyWithValue("k8s.v1.cni.cncf.io/networks", string(expectedAnnotation)),
			)

			// We don't add network attachment status annotations to the Pods
			// to simulate that the network attachments are missing.
			th.SimulateStatefulSetReplicaReadyWithPods(novaNames.SchedulerStatefulSetName, map[string][]string{})

			th.ExpectConditionWithDetails(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
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

			ss := th.GetStatefulSet(novaNames.SchedulerStatefulSetName)

			expectedAnnotation, err := json.Marshal(
				[]networkv1.NetworkSelectionElement{
					{
						Name:             "internalapi",
						Namespace:        novaNames.SchedulerName.Namespace,
						InterfaceRequest: "internalapi",
					}})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(ss.Spec.Template.ObjectMeta.Annotations).To(
				HaveKeyWithValue("k8s.v1.cni.cncf.io/networks", string(expectedAnnotation)),
			)

			// We simulate that there is no IP associated with the internalapi
			// network attachment
			th.SimulateStatefulSetReplicaReadyWithPods(
				novaNames.SchedulerStatefulSetName,
				map[string][]string{novaNames.SchedulerName.Namespace + "/internalapi": {}},
			)

			th.ExpectConditionWithDetails(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
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

			th.SimulateStatefulSetReplicaReadyWithPods(
				novaNames.SchedulerStatefulSetName,
				map[string][]string{novaNames.SchedulerName.Namespace + "/internalapi": {"10.0.0.1"}},
			)

			th.ExpectCondition(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionTrue,
			)

			Eventually(func(g Gomega) {
				novaScheduler := GetNovaScheduler(novaNames.SchedulerName)
				g.Expect(novaScheduler.Status.NetworkAttachments).To(
					Equal(map[string][]string{novaNames.SchedulerName.Namespace + "/internalapi": {"10.0.0.1"}}))
			}, timeout, interval).Should(Succeed())

			th.ExpectCondition(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})
	When("NovaScheduler is reconfigured", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateInternalTopLevelSecret(novaNames))
			DeferCleanup(
				th.DeleteInstance, CreateNovaScheduler(novaNames.SchedulerName, GetDefaultNovaSchedulerSpec(novaNames)))

			th.ExpectCondition(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)

			th.SimulateStatefulSetReplicaReady(novaNames.SchedulerStatefulSetName)
			th.ExpectCondition(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("applies new NetworkAttachments configuration", func() {
			Eventually(func(g Gomega) {
				novaScheduler := GetNovaScheduler(novaNames.SchedulerName)
				novaScheduler.Spec.NetworkAttachments = append(novaScheduler.Spec.NetworkAttachments, "internalapi")

				g.Expect(k8sClient.Update(ctx, novaScheduler)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			th.ExpectConditionWithDetails(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NetworkAttachment resources missing: internalapi",
			)

			th.ExpectConditionWithDetails(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NetworkAttachment resources missing: internalapi",
			)

			DeferCleanup(th.DeleteInstance, th.CreateNetworkAttachmentDefinition(novaNames.InternalAPINetworkNADName))

			th.ExpectConditionWithDetails(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occurred "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
			)

			th.ExpectConditionWithDetails(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occurred "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
			)

			th.SimulateStatefulSetReplicaReadyWithPods(
				novaNames.SchedulerStatefulSetName,
				map[string][]string{novaNames.SchedulerName.Namespace + "/internalapi": {"10.0.0.1"}},
			)

			th.ExpectCondition(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionTrue,
			)

			Eventually(func(g Gomega) {
				novaScheduler := GetNovaScheduler(novaNames.SchedulerName)
				g.Expect(novaScheduler.Status.NetworkAttachments).To(
					Equal(map[string][]string{novaNames.SchedulerName.Namespace + "/internalapi": {"10.0.0.1"}}))

			}, timeout, interval).Should(Succeed())

			th.ExpectCondition(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("applies new RegisteredCells input to its StatefulSet to trigger Pod restart", func() {
			originalConfigHash := GetEnvVarValue(
				th.GetStatefulSet(novaNames.SchedulerStatefulSetName).Spec.Template.Spec.Containers[0].Env, "CONFIG_HASH", "")

			// Simulate that a new cell is added and Nova controller registered it and
			// therefore a new cell is added to RegisteredCells
			Eventually(func(g Gomega) {
				novaAPI := GetNovaScheduler(novaNames.SchedulerName)
				novaAPI.Spec.RegisteredCells = map[string]string{"cell0": "cell0-config-hash"}
				g.Expect(k8sClient.Update(ctx, novaAPI)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			// Assert that the CONFIG_HASH of the StateFulSet is changed due to this reconfiguration
			Eventually(func(g Gomega) {
				currentConfigHash := GetEnvVarValue(
					th.GetStatefulSet(novaNames.SchedulerStatefulSetName).Spec.Template.Spec.Containers[0].Env, "CONFIG_HASH", "")
				g.Expect(originalConfigHash).NotTo(Equal(currentConfigHash))

			}, timeout, interval).Should(Succeed())
		})
	})

	When("NovaScheduler CR is created without container image defined", func() {
		BeforeEach(func() {
			spec := GetDefaultNovaSchedulerSpec(novaNames)
			spec["containerImage"] = ""
			scheduler := CreateNovaScheduler(novaNames.SchedulerName, spec)
			DeferCleanup(th.DeleteInstance, scheduler)
		})
		It("has the expected container image default", func() {
			novaSchedulerDefault := GetNovaScheduler(novaNames.SchedulerName)
			Expect(novaSchedulerDefault.Spec.ContainerImage).To(Equal(util.GetEnvVar("RELATED_IMAGE_NOVA_SCHEDULER_IMAGE_URL_DEFAULT", novav1.NovaSchedulerContainerImage)))
		})
	})
})

var _ = Describe("NovaScheduler controller cleaning", func() {
	BeforeEach(func() {
		mariadb.CreateMariaDBDatabase(novaNames.APIMariaDBDatabaseName.Namespace, novaNames.APIMariaDBDatabaseName.Name, mariadbv1.MariaDBDatabaseSpec{})
		DeferCleanup(k8sClient.Delete, ctx, mariadb.GetMariaDBDatabase(novaNames.APIMariaDBDatabaseName))

		apiMariaDBAccount, apiMariaDBSecret := mariadb.CreateMariaDBAccountAndSecret(novaNames.APIMariaDBDatabaseAccount, mariadbv1.MariaDBAccountSpec{})
		DeferCleanup(k8sClient.Delete, ctx, apiMariaDBAccount)
		DeferCleanup(k8sClient.Delete, ctx, apiMariaDBSecret)

		cell0Account, cell0Secret := mariadb.CreateMariaDBAccountAndSecret(cell0.MariaDBAccountName, mariadbv1.MariaDBAccountSpec{})
		DeferCleanup(k8sClient.Delete, ctx, cell0Account)
		DeferCleanup(k8sClient.Delete, ctx, cell0Secret)
	})

	var novaAPIFixture *NovaAPIFixture
	BeforeEach(func() {
		memcachedSpec := memcachedv1.MemcachedSpec{
			MemcachedSpecCore: memcachedv1.MemcachedSpecCore{
				Replicas: ptr.To(int32(3)),
			},
		}
		DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(novaNames.NovaName.Namespace, MemcachedInstance, memcachedSpec))
		infra.SimulateMemcachedReady(novaNames.MemcachedNamespace)

		DeferCleanup(
			k8sClient.Delete, ctx, CreateNovaSecret(novaNames.NovaName.Namespace, SecretName))
		novaAPIFixture = NewNovaAPIFixtureWithServer(logger)
		novaAPIFixture.Setup()
		f := keystone_helper.NewKeystoneAPIFixtureWithServer(logger)
		text := ResponseHandleToken(f.Endpoint(), novaAPIFixture.Endpoint())
		f.Setup(
			api.Handler{Pattern: "/", Func: f.HandleVersion},
			api.Handler{Pattern: "/v3/users", Func: f.HandleUsers},
			api.Handler{Pattern: "/v3/domains", Func: f.HandleDomains},
			api.Handler{Pattern: "/v3/auth/tokens", Func: func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case "POST":
					w.Header().Add("Content-Type", "application/json")
					w.WriteHeader(202)
					fmt.Fprint(w, text)
				}
			}})
		DeferCleanup(f.Cleanup)
		DeferCleanup(
			k8sClient.Delete, ctx, CreateInternalTopLevelSecret(novaNames))
		spec := GetDefaultNovaSchedulerSpec(novaNames)
		spec["keystoneAuthURL"] = f.Endpoint()
		DeferCleanup(
			th.DeleteInstance, CreateNovaScheduler(novaNames.SchedulerName, spec))
		DeferCleanup(novaAPIFixture.Cleanup)
	})
	When("NovaScheduler down service is removed from api", func() {
		It("during reconciling", func() {
			// We can assert the service cleanup behavior or the reconciler as the cleanup is triggered
			// unconditionally at the end of every successful reconciliation. So we don't actually need
			// to trigger a scale down to see the down services are deleted. The novaAPIFixture is
			// set up in a way that it always respond with a list of services in down state.
			th.SimulateStatefulSetReplicaReady(novaNames.SchedulerStatefulSetName)
			th.ExpectCondition(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
			Expect(novaAPIFixture.FindRequest("DELETE", "/compute/os-services/3", "")).To(BeTrue())
		})
	})
})

var _ = Describe("NovaScheduler controller", func() {
	BeforeEach(func() {
		mariadb.CreateMariaDBDatabase(novaNames.APIMariaDBDatabaseName.Namespace, novaNames.APIMariaDBDatabaseName.Name, mariadbv1.MariaDBDatabaseSpec{})
		DeferCleanup(k8sClient.Delete, ctx, mariadb.GetMariaDBDatabase(novaNames.APIMariaDBDatabaseName))

		mariadb.SimulateMariaDBTLSDatabaseCompleted(novaNames.APIMariaDBDatabaseName)

		apiMariaDBAccount, apiMariaDBSecret := mariadb.CreateMariaDBAccountAndSecret(novaNames.APIMariaDBDatabaseAccount, mariadbv1.MariaDBAccountSpec{})
		DeferCleanup(k8sClient.Delete, ctx, apiMariaDBAccount)
		DeferCleanup(k8sClient.Delete, ctx, apiMariaDBSecret)

		cell0Account, cell0Secret := mariadb.CreateMariaDBAccountAndSecret(cell0.MariaDBAccountName, mariadbv1.MariaDBAccountSpec{})
		DeferCleanup(k8sClient.Delete, ctx, cell0Account)
		DeferCleanup(k8sClient.Delete, ctx, cell0Secret)

		memcachedSpec := memcachedv1.MemcachedSpec{
			MemcachedSpecCore: memcachedv1.MemcachedSpecCore{
				Replicas: ptr.To(int32(3)),
			},
		}
		DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(novaNames.NovaName.Namespace, MemcachedInstance, memcachedSpec))
		infra.SimulateTLSMemcachedReady(novaNames.MemcachedNamespace)
	})
	When("NovaScheduler CR instance is deleted", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateInternalTopLevelSecret(novaNames))
			DeferCleanup(th.DeleteInstance, CreateNovaScheduler(novaNames.SchedulerName, GetDefaultNovaSchedulerSpec(novaNames)))
		})

		It("removes the finalizer from Memcached", func() {
			th.SimulateStatefulSetReplicaReady(novaNames.SchedulerName)
			th.ExpectCondition(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
			memcached := infra.GetMemcached(novaNames.MemcachedNamespace)
			Expect(memcached.Finalizers).To(ContainElement("NovaScheduler"))

			Eventually(func(g Gomega) {
				th.DeleteInstance(GetNovaScheduler(novaNames.SchedulerName))
				memcached := infra.GetMemcached(novaNames.MemcachedNamespace)
				g.Expect(memcached.Finalizers).NotTo(ContainElement("NovaScheduler"))
			}, timeout, interval).Should(Succeed())
		})
	})

	When("NovaScheduler is created with TLS CA cert secret", func() {
		BeforeEach(func() {
			spec := GetDefaultNovaSchedulerSpec(novaNames)
			spec["tls"] = map[string]interface{}{
				"caBundleSecretName": novaNames.CaBundleSecretName.Name,
			}

			DeferCleanup(
				k8sClient.Delete, ctx, CreateInternalTopLevelSecret(novaNames))
			DeferCleanup(th.DeleteInstance, CreateNovaScheduler(novaNames.SchedulerName, spec))
		})

		It("reports that the CA secret is missing", func() {
			th.ExpectConditionWithDetails(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				fmt.Sprintf("TLSInput error occured in TLS sources Secret %s/combined-ca-bundle not found", novaNames.Namespace),
			)
			th.ExpectCondition(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("creates a StatefulSet for nova-scheduler service with TLS CA cert attached", func() {
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCABundleSecret(novaNames.CaBundleSecretName))
			th.SimulateStatefulSetReplicaReady(novaNames.SchedulerStatefulSetName)

			th.ExpectCondition(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionTrue,
			)

			ss := th.GetStatefulSet(novaNames.SchedulerStatefulSetName)
			// Check the resulting deployment fields
			Expect(int(*ss.Spec.Replicas)).To(Equal(1))
			Expect(ss.Spec.Template.Spec.Volumes).To(HaveLen(2))
			Expect(ss.Spec.Template.Spec.Containers).To(HaveLen(1))

			// cert deployment volumes
			th.AssertVolumeExists(novaNames.CaBundleSecretName.Name, ss.Spec.Template.Spec.Volumes)

			// CA container certs
			apiContainer := ss.Spec.Template.Spec.Containers[0]
			th.AssertVolumeMountExists(novaNames.CaBundleSecretName.Name, "tls-ca-bundle.pem", apiContainer.VolumeMounts)

			configDataMap := th.GetSecret(novaNames.SchedulerConfigDataName)
			Expect(configDataMap).ShouldNot(BeNil())

			configData := string(configDataMap.Data["01-nova.conf"])
			Expect(configData).Should(
				ContainSubstring(fmt.Sprintf("memcache_servers=memcached-0.memcached.%s.svc:11211,memcached-1.memcached.%s.svc:11211,memcached-2.memcached.%s.svc:11211",
					novaNames.Namespace, novaNames.Namespace, novaNames.Namespace)))
			Expect(configData).Should(
				ContainSubstring(fmt.Sprintf("memcached_servers=inet:[memcached-0.memcached.%s.svc]:11211,inet:[memcached-1.memcached.%s.svc]:11211,inet:[memcached-2.memcached.%s.svc]:11211",
					novaNames.Namespace, novaNames.Namespace, novaNames.Namespace)))
			Expect(configData).Should(
				ContainSubstring("tls_enabled=true"))

			myCnf := configDataMap.Data["my.cnf"]
			Expect(myCnf).To(
				ContainSubstring("[client]\nssl-ca=/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem\nssl=1"))

			th.ExpectCondition(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("reconfigures the NovaScheduler pod when CA changes", func() {
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCABundleSecret(novaNames.CaBundleSecretName))
			th.SimulateStatefulSetReplicaReady(novaNames.SchedulerStatefulSetName)

			th.ExpectCondition(
				novaNames.SchedulerName,
				ConditionGetterFunc(NovaSchedulerConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionTrue,
			)

			ss := th.GetStatefulSet(novaNames.SchedulerStatefulSetName)

			// Check the resulting deployment fields
			Expect(int(*ss.Spec.Replicas)).To(Equal(1))
			Expect(ss.Spec.Template.Spec.Volumes).To(HaveLen(2))
			Expect(ss.Spec.Template.Spec.Containers).To(HaveLen(1))

			// Grab the current config hash
			originalHash := GetEnvVarValue(
				th.GetStatefulSet(novaNames.SchedulerStatefulSetName).Spec.Template.Spec.Containers[0].Env, "CONFIG_HASH", "")
			Expect(originalHash).NotTo(BeEmpty())

			// Change the content of the CA secret
			th.UpdateSecret(novaNames.CaBundleSecretName, "tls-ca-bundle.pem", []byte("DifferentCAData"))

			// Assert that the deployment is updated
			Eventually(func(g Gomega) {
				newHash := GetEnvVarValue(
					th.GetStatefulSet(novaNames.SchedulerStatefulSetName).Spec.Template.Spec.Containers[0].Env, "CONFIG_HASH", "")
				g.Expect(newHash).NotTo(BeEmpty())
				g.Expect(newHash).NotTo(Equal(originalHash))
			}, timeout, interval).Should(Succeed())
		})
	})
})
