/*
Copyright 2023.

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

	//revive:disable-next-line:dot-imports
	. "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"

	networkv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	memcachedv1 "github.com/openstack-k8s-operators/infra-operator/apis/memcached/v1beta1"
	mariadbv1 "github.com/openstack-k8s-operators/mariadb-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
)

var _ = Describe("NovaMetadata controller", func() {
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

		mariadb.CreateMariaDBDatabase(novaNames.APIMariaDBDatabaseName.Namespace, novaNames.APIMariaDBDatabaseName.Name, mariadbv1.MariaDBDatabaseSpec{})
		memcachedSpec := memcachedv1.MemcachedSpec{
			MemcachedSpecCore: memcachedv1.MemcachedSpecCore{
				Replicas: ptr.To(int32(3)),
			},
		}
		DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(novaNames.NovaName.Namespace, MemcachedInstance, memcachedSpec))
		infra.SimulateMemcachedReady(novaNames.MemcachedNamespace)
		DeferCleanup(k8sClient.Delete, ctx, mariadb.GetMariaDBDatabase(novaNames.APIMariaDBDatabaseName))

	})

	When("with standard spec without network interface", func() {
		BeforeEach(func() {
			spec := GetDefaultNovaMetadataSpec(novaNames.InternalTopLevelSecretName)
			spec["customServiceConfig"] = "foo=bar"
			spec["defaultConfigOverwrite"] = map[string]interface{}{
				"api-paste.ini": "pipeline = cors metaapp",
			}

			DeferCleanup(th.DeleteInstance, CreateNovaMetadata(novaNames.MetadataName, spec))
		})
		When("a NovaMetadata CR is created pointing to a non existent Secret", func() {

			It("is not Ready", func() {
				th.ExpectCondition(
					novaNames.MetadataName,
					ConditionGetterFunc(NovaMetadataConditionGetter),
					condition.ReadyCondition, corev1.ConditionFalse,
				)
			})

			It("has empty Status fields", func() {
				instance := GetNovaMetadata(novaNames.MetadataName)
				// NOTE(gibi): Hash has `omitempty` tags so while
				// they are initialized to an empty map that value is omitted from
				// the output when sent to the client. So we see nils here.
				Expect(instance.Status.Hash).To(BeEmpty())
				Expect(instance.Status.ReadyCount).To(Equal(int32(0)))
			})
			It("is missing the secret", func() {
				th.ExpectCondition(
					novaNames.MetadataName,
					ConditionGetterFunc(NovaMetadataConditionGetter),
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
						Namespace: novaNames.MetadataName.Namespace,
					},
				}
				Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
				DeferCleanup(k8sClient.Delete, ctx, secret)
			})

			It("is not Ready", func() {
				th.ExpectCondition(
					novaNames.MetadataName,
					ConditionGetterFunc(NovaMetadataConditionGetter),
					condition.ReadyCondition,
					corev1.ConditionFalse,
				)
			})

			It("is missing the secret", func() {
				th.ExpectCondition(
					novaNames.MetadataName,
					ConditionGetterFunc(NovaMetadataConditionGetter),
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
					novaNames.MetadataName,
					ConditionGetterFunc(NovaMetadataConditionGetter),
					condition.ReadyCondition,
					corev1.ConditionFalse,
				)
			})

			It("reports that the inputs are not ready", func() {
				th.ExpectCondition(
					novaNames.MetadataName,
					ConditionGetterFunc(NovaMetadataConditionGetter),
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
					CreateInternalTopLevelSecret(novaNames),
				)
			})

			It("reports that input is ready", func() {
				th.ExpectCondition(
					novaNames.MetadataName,
					ConditionGetterFunc(NovaMetadataConditionGetter),
					condition.InputReadyCondition,
					corev1.ConditionTrue,
				)
			})
			It("generated configs successfully", func() {
				th.ExpectCondition(
					novaNames.MetadataName,
					ConditionGetterFunc(NovaMetadataConditionGetter),
					condition.ServiceConfigReadyCondition,
					corev1.ConditionTrue,
				)

				configDataMap := th.GetSecret(novaNames.MetadataConfigDataName)
				Expect(configDataMap).ShouldNot(BeNil())
				Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
				configData := string(configDataMap.Data["01-nova.conf"])
				Expect(configData).Should(ContainSubstring("transport_url=rabbit://api/fake"))
				Expect(configData).Should(ContainSubstring("password = service-password"))
				Expect(configData).Should(ContainSubstring("metadata_proxy_shared_secret = metadata-secret"))
				Expect(configData).Should(ContainSubstring("local_metadata_per_cell = false"))
				Expect(configData).Should(ContainSubstring("enabled_apis=metadata"))
				Expect(configData).Should(ContainSubstring("metadata_workers=1"))

				apiAccount := mariadb.GetMariaDBAccount(novaNames.APIMariaDBDatabaseAccount)
				apiSecret := th.GetSecret(types.NamespacedName{Name: apiAccount.Spec.Secret, Namespace: novaNames.APIMariaDBDatabaseAccount.Namespace})

				Expect(configData).To(
					ContainSubstring(
						fmt.Sprintf("connection = mysql+pymysql://%s:%s@nova-api-db-hostname/nova_api?read_default_file=/etc/my.cnf",
							apiAccount.Spec.UserName, apiSecret.Data[mariadbv1.DatabasePasswordSelector])),
				)

				Expect(configData).Should(
					ContainSubstring("[upgrade_levels]\ncompute = auto"))
				Expect(configData).Should(
					ContainSubstring("backend = dogpile.cache.memcached"))
				Expect(configData).Should(
					ContainSubstring(fmt.Sprintf("memcache_servers=memcached-0.memcached.%s.svc:11211,memcached-1.memcached.%s.svc:11211,memcached-2.memcached.%s.svc:11211",
						novaNames.Namespace, novaNames.Namespace, novaNames.Namespace)))
				Expect(configData).Should(
					ContainSubstring(fmt.Sprintf("memcached_servers=inet:[memcached-0.memcached.%s.svc]:11211,inet:[memcached-1.memcached.%s.svc]:11211,inet:[memcached-2.memcached.%s.svc]:11211",
						novaNames.Namespace, novaNames.Namespace, novaNames.Namespace)))
				Expect(configData).Should(
					ContainSubstring("tls_enabled=false"))
				Expect(configDataMap.Data).Should(HaveKey("02-nova-override.conf"))
				myCnf := configDataMap.Data["my.cnf"]
				Expect(myCnf).To(
					ContainSubstring("[client]\nssl=0"))
				extraData := string(configDataMap.Data["02-nova-override.conf"])
				Expect(extraData).To(Equal("foo=bar"))

				Expect(configDataMap.Data).Should(HaveKey("api-paste.ini"))
				pasteData := string(configDataMap.Data["api-paste.ini"])
				Expect(pasteData).To(Equal("pipeline = cors metaapp"))
			})

			It("stored the input hash in the Status", func() {
				Eventually(func(g Gomega) {
					novaMetadata := GetNovaMetadata(novaNames.MetadataName)
					g.Expect(novaMetadata.Status.Hash).Should(HaveKeyWithValue("input", Not(BeEmpty())))
				}, timeout, interval).Should(Succeed())

			})
		})

		When("NovaMetadata is created with a proper Secret", func() {
			BeforeEach(func() {
				DeferCleanup(
					k8sClient.Delete, ctx, CreateInternalTopLevelSecret(novaNames))
			})

			It(" reports input ready", func() {
				th.ExpectCondition(
					novaNames.MetadataName,
					ConditionGetterFunc(NovaMetadataConditionGetter),
					condition.InputReadyCondition,
					corev1.ConditionTrue,
				)
			})

			It("creates a StatefulSet for the nova-metadata service", func() {
				th.ExpectConditionWithDetails(
					novaNames.MetadataName,
					ConditionGetterFunc(NovaMetadataConditionGetter),
					condition.DeploymentReadyCondition,
					corev1.ConditionFalse,
					condition.RequestedReason,
					condition.DeploymentReadyRunningMessage,
				)

				ss := th.GetStatefulSet(novaNames.MetadataStatefulSetName)
				Expect(ss.Spec.Template.Spec.ServiceAccountName).To(Equal("nova-sa"))
				Expect(int(*ss.Spec.Replicas)).To(Equal(1))
				Expect(ss.Spec.Template.Spec.Volumes).To(HaveLen(2))
				Expect(ss.Spec.Template.Spec.Containers).To(HaveLen(2))
				Expect(ss.Spec.Selector.MatchLabels).To(Equal(map[string]string{"service": "nova-metadata"}))

				container := ss.Spec.Template.Spec.Containers[0]
				Expect(container.VolumeMounts).To(HaveLen(1))
				Expect(container.Image).To(Equal(ContainerImage))

				container = ss.Spec.Template.Spec.Containers[1]
				Expect(container.VolumeMounts).To(HaveLen(3))
				Expect(container.Image).To(Equal(ContainerImage))

				Expect(container.LivenessProbe.HTTPGet.Port.IntVal).To(Equal(int32(8775)))
				Expect(container.ReadinessProbe.HTTPGet.Port.IntVal).To(Equal(int32(8775)))

			})

			When("the StatefulSet has at least one Replica ready", func() {
				BeforeEach(func() {
					th.ExpectConditionWithDetails(
						novaNames.MetadataName,
						ConditionGetterFunc(NovaMetadataConditionGetter),
						condition.DeploymentReadyCondition,
						corev1.ConditionFalse,
						condition.RequestedReason,
						condition.DeploymentReadyRunningMessage,
					)
					th.SimulateStatefulSetReplicaReady(novaNames.MetadataStatefulSetName)
				})

				It("reports that the StatefulSet is ready", func() {
					th.GetStatefulSet(novaNames.MetadataStatefulSetName)
					th.ExpectCondition(
						novaNames.MetadataName,
						ConditionGetterFunc(NovaMetadataConditionGetter),
						condition.DeploymentReadyCondition,
						corev1.ConditionTrue,
					)

					novaMetadata := GetNovaMetadata(novaNames.MetadataName)
					Expect(novaMetadata.Status.ReadyCount).To(BeNumerically(">", 0))
				})
			})

			It("exposes the service", func() {
				th.SimulateStatefulSetReplicaReady(novaNames.MetadataStatefulSetName)
				th.ExpectCondition(
					novaNames.MetadataName,
					ConditionGetterFunc(NovaMetadataConditionGetter),
					condition.ExposeServiceReadyCondition,
					corev1.ConditionTrue,
				)
				service := th.GetService(novaNames.InternalNovaMetadataServiceName)
				Expect(service.Labels["service"]).To(Equal("nova-metadata"))
				Expect(service.Labels).NotTo(HaveKey("cell"))
			})

			It("generates compute config", func() {
				th.SimulateStatefulSetReplicaReady(novaNames.MetadataStatefulSetName)

				th.ExpectCondition(
					novaNames.MetadataName,
					ConditionGetterFunc(NovaMetadataConditionGetter),
					novav1.NovaComputeServiceConfigReady,
					corev1.ConditionTrue,
				)

				computeConfigData := th.GetSecret(novaNames.MetadataNeutronConfigDataName)
				Expect(computeConfigData).ShouldNot(BeNil())
				Expect(computeConfigData.Data).To(HaveLen(1))
				Expect(computeConfigData.Data).Should(HaveKey("05-nova-metadata.conf"))
				configData := string(computeConfigData.Data["05-nova-metadata.conf"])
				Expect(configData).To(
					ContainSubstring(
						fmt.Sprintf(
							"nova_metadata_host = nova-metadata-internal.%s.svc",
							novaNames.MetadataName.Namespace,
						),
					),
				)
				Expect(configData).To(ContainSubstring("nova_metadata_port = 8775"))
				Expect(configData).To(ContainSubstring("nova_metadata_protocol = http"))
				Expect(configData).To(ContainSubstring("metadata_proxy_shared_secret = metadata-secret"))

				metadata := GetNovaMetadata(novaNames.MetadataName)
				Expect(metadata.Status.Hash[novaNames.MetadataNeutronConfigDataName.Name]).NotTo(BeEmpty())
			})

			It("is Ready", func() {
				th.SimulateStatefulSetReplicaReady(novaNames.MetadataStatefulSetName)

				th.ExpectCondition(
					novaNames.MetadataName,
					ConditionGetterFunc(NovaMetadataConditionGetter),
					condition.ReadyCondition,
					corev1.ConditionTrue,
				)
			})
		})
	})
})

var _ = Describe("NovaMetadata controller", func() {
	BeforeEach(func() {
		mariadb.CreateMariaDBDatabase(novaNames.APIMariaDBDatabaseName.Namespace, novaNames.APIMariaDBDatabaseName.Name, mariadbv1.MariaDBDatabaseSpec{})
		DeferCleanup(k8sClient.Delete, ctx, mariadb.GetMariaDBDatabase(novaNames.APIMariaDBDatabaseName))

		mariadb.CreateMariaDBDatabase(cell1.MariaDBDatabaseName.Namespace, cell1.MariaDBDatabaseName.Name, mariadbv1.MariaDBDatabaseSpec{})
		DeferCleanup(k8sClient.Delete, ctx, mariadb.GetMariaDBDatabase(cell1.MariaDBDatabaseName))

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

		memcachedSpec := memcachedv1.MemcachedSpec{
			MemcachedSpecCore: memcachedv1.MemcachedSpecCore{
				Replicas: ptr.To(int32(3)),
			},
		}
		DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(novaNames.NovaName.Namespace, MemcachedInstance, memcachedSpec))
		infra.SimulateMemcachedReady(novaNames.MemcachedNamespace)

	})

	When("configured with cell name", func() {
		BeforeEach(func() {
			spec := GetDefaultNovaMetadataSpec(cell1.InternalCellSecretName)
			spec["cellName"] = cell1.CellName
			metadata := CreateNovaMetadata(cell1.MetadataName, spec)
			DeferCleanup(th.DeleteInstance, metadata)
			DeferCleanup(
				k8sClient.Delete, ctx, CreateMetadataCellInternalSecret(cell1))
		})
		It("generated config with correct local_metadata_per_cell", func() {
			th.ExpectCondition(
				cell1.MetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)

			configDataMap := th.GetSecret(cell1.MetadataConfigDataName)
			Expect(configDataMap).ShouldNot(BeNil())
			Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
			configData := string(configDataMap.Data["01-nova.conf"])
			Expect(configData).Should(
				ContainSubstring("transport_url=rabbit://cell1/fake"))
			Expect(configData).Should(
				ContainSubstring("metadata_proxy_shared_secret = metadata-secret"))
			Expect(configData).Should(
				ContainSubstring("password = service-password"))
			Expect(configData).Should(
				ContainSubstring("local_metadata_per_cell = true"))
			th.ExpectCondition(
				cell1.MetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.ExposeServiceReadyCondition,
				corev1.ConditionTrue,
			)
			service := th.GetService(types.NamespacedName{Namespace: cell1.MetadataName.Namespace, Name: "nova-metadata-cell1-internal"})
			Expect(service.Labels["service"]).To(Equal("nova-metadata"))
			Expect(service.Labels["cell"]).To(Equal("cell1"))

			ss := th.GetStatefulSet(cell1.MetadataStatefulSetName)
			Expect(ss.Spec.Selector.MatchLabels).To(
				Equal(map[string]string{
					"service": "nova-metadata",
					"cell":    "cell1",
				}))
		})

		It("generates compute config", func() {
			th.SimulateStatefulSetReplicaReady(cell1.MetadataStatefulSetName)

			th.ExpectCondition(
				cell1.MetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				novav1.NovaComputeServiceConfigReady,
				corev1.ConditionTrue,
			)

			computeConfigData := th.GetSecret(cell1.MetadataNeutronConfigDataName)
			Expect(computeConfigData).ShouldNot(BeNil())
			Expect(computeConfigData.Data).Should(HaveKey("05-nova-metadata.conf"))
			configData := string(computeConfigData.Data["05-nova-metadata.conf"])
			Expect(configData).To(
				ContainSubstring(
					fmt.Sprintf(
						"nova_metadata_host = nova-metadata-cell1-internal.%s.svc",
						cell1.MetadataName.Namespace,
					),
				),
			)
			Expect(configData).To(ContainSubstring("nova_metadata_port = 8775"))
			Expect(configData).To(ContainSubstring("nova_metadata_protocol = http"))
			Expect(configData).To(ContainSubstring("metadata_proxy_shared_secret = metadata-secret"))

			metadata := GetNovaMetadata(cell1.MetadataName)
			Expect(metadata.Status.Hash[cell1.MetadataNeutronConfigDataName.Name]).NotTo(BeEmpty())
		})

	})

	When("NovaMetadata is created with networkAttachments", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateInternalTopLevelSecret(novaNames))

			spec := GetDefaultNovaMetadataSpec(novaNames.InternalTopLevelSecretName)
			spec["networkAttachments"] = []string{"internalapi"}
			DeferCleanup(th.DeleteInstance, CreateNovaMetadata(novaNames.MetadataName, spec))
		})

		It("reports that the definition is missing", func() {
			th.ExpectConditionWithDetails(
				novaNames.MetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NetworkAttachment resources missing: internalapi",
			)
			th.ExpectCondition(
				novaNames.MetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})
		It("reports that network attachment is missing", func() {
			internalMetadataName := types.NamespacedName{Namespace: novaNames.MetadataName.Namespace, Name: "internalapi"}
			nad := th.CreateNetworkAttachmentDefinition(internalMetadataName)
			DeferCleanup(th.DeleteInstance, nad)

			ss := th.GetStatefulSet(novaNames.MetadataStatefulSetName)

			expectedAnnotation, err := json.Marshal(
				[]networkv1.NetworkSelectionElement{
					{
						Name:             "internalapi",
						Namespace:        novaNames.MetadataName.Namespace,
						InterfaceRequest: "internalapi",
					}})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(ss.Spec.Template.ObjectMeta.Annotations).To(
				HaveKeyWithValue("k8s.v1.cni.cncf.io/networks", string(expectedAnnotation)),
			)

			// We don't add network attachment status annotations to the Pods
			// to simulate that the network attachments are missing.
			th.SimulateStatefulSetReplicaReadyWithPods(novaNames.MetadataStatefulSetName, map[string][]string{})

			th.ExpectConditionWithDetails(
				novaNames.MetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occurred "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
			)
		})
		It("reports that an IP is missing", func() {
			internalMetadataName := types.NamespacedName{Namespace: novaNames.MetadataName.Namespace, Name: "internalapi"}
			nad := th.CreateNetworkAttachmentDefinition(internalMetadataName)
			DeferCleanup(th.DeleteInstance, nad)

			ss := th.GetStatefulSet(novaNames.MetadataStatefulSetName)

			expectedAnnotation, err := json.Marshal(
				[]networkv1.NetworkSelectionElement{
					{
						Name:             "internalapi",
						Namespace:        novaNames.MetadataName.Namespace,
						InterfaceRequest: "internalapi",
					}})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(ss.Spec.Template.ObjectMeta.Annotations).To(
				HaveKeyWithValue("k8s.v1.cni.cncf.io/networks", string(expectedAnnotation)),
			)

			// We simulate that there is no IP associated with the internalapi
			// network attachment
			th.SimulateStatefulSetReplicaReadyWithPods(
				novaNames.MetadataStatefulSetName,
				map[string][]string{novaNames.MetadataName.Namespace + "/internalapi": {}},
			)

			th.ExpectConditionWithDetails(
				novaNames.MetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occurred "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
			)
		})
		It("reports NetworkAttachmentsReady if the Pods got the proper annotations", func() {
			internalMetadataName := types.NamespacedName{Namespace: novaNames.MetadataName.Namespace, Name: "internalapi"}
			nad := th.CreateNetworkAttachmentDefinition(internalMetadataName)
			DeferCleanup(th.DeleteInstance, nad)

			th.SimulateStatefulSetReplicaReadyWithPods(
				novaNames.MetadataStatefulSetName,
				map[string][]string{novaNames.MetadataName.Namespace + "/internalapi": {"10.0.0.1"}},
			)

			th.ExpectCondition(
				novaNames.MetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionTrue,
			)

			Eventually(func(g Gomega) {
				instance := GetNovaMetadata(novaNames.MetadataName)
				g.Expect(instance.Status.NetworkAttachments).To(
					Equal(map[string][]string{novaNames.MetadataName.Namespace + "/internalapi": {"10.0.0.1"}}))

			}, timeout, interval).Should(Succeed())

			th.ExpectCondition(
				novaNames.MetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})
	When("NovaMetadata is created with service override", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateInternalTopLevelSecret(novaNames))

			spec := GetDefaultNovaMetadataSpec(novaNames.InternalTopLevelSecretName)
			serviceOverride := map[string]interface{}{
				"metadata": map[string]map[string]string{
					"annotations": {
						"metallb.universe.tf/address-pool":    "osp-internalapi",
						"metallb.universe.tf/allow-shared-ip": "osp-internalapi",
						"metallb.universe.tf/loadBalancerIPs": "internal-lb-ip-1,internal-lb-ip-2",
					},
					"labels": {
						"internal": "true",
						"service":  "nova",
					},
				},
				"spec": map[string]interface{}{
					"type": "LoadBalancer",
				},
			}

			spec["override"] = map[string]interface{}{
				"service": serviceOverride,
			}

			metadata := CreateNovaMetadata(novaNames.MetadataName, spec)
			DeferCleanup(th.DeleteInstance, metadata)
		})

		It("creates LoadBalancer service", func() {
			th.SimulateStatefulSetReplicaReady(novaNames.MetadataStatefulSetName)

			// As the internal endpoint is configured via service override to
			// be a LoadBalancer Service with MetalLB annotations
			service := th.GetService(novaNames.InternalNovaMetadataServiceName)
			Expect(service.Annotations).To(
				HaveKeyWithValue("dnsmasq.network.openstack.org/hostname", fmt.Sprintf("nova-metadata-internal.%s.svc", novaNames.Namespace)))
			Expect(service.Annotations).To(
				HaveKeyWithValue("metallb.universe.tf/address-pool", "osp-internalapi"))
			Expect(service.Annotations).To(
				HaveKeyWithValue("metallb.universe.tf/allow-shared-ip", "osp-internalapi"))
			Expect(service.Annotations).To(
				HaveKeyWithValue("metallb.universe.tf/loadBalancerIPs", "internal-lb-ip-1,internal-lb-ip-2"))
			Expect(service.Spec.Type).To(Equal(corev1.ServiceTypeLoadBalancer))

			th.ExpectCondition(
				novaNames.MetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})
	When("NovaMetadata is reconfigured", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateInternalTopLevelSecret(novaNames))

			metadata := CreateNovaMetadata(
				novaNames.MetadataName, GetDefaultNovaMetadataSpec(novaNames.InternalTopLevelSecretName))
			DeferCleanup(th.DeleteInstance, metadata)

			th.ExpectCondition(
				novaNames.MetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)

			th.SimulateStatefulSetReplicaReady(novaNames.MetadataStatefulSetName)
			th.ExpectCondition(
				novaNames.MetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("applies new NetworkAttachments configuration", func() {
			Eventually(func(g Gomega) {
				novaMetadata := GetNovaMetadata(novaNames.MetadataName)
				novaMetadata.Spec.NetworkAttachments = append(novaMetadata.Spec.NetworkAttachments, "internalapi")

				g.Expect(k8sClient.Update(ctx, novaMetadata)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			th.ExpectConditionWithDetails(
				novaNames.MetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NetworkAttachment resources missing: internalapi",
			)
			th.ExpectConditionWithDetails(
				novaNames.MetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NetworkAttachment resources missing: internalapi",
			)

			DeferCleanup(th.DeleteInstance, th.CreateNetworkAttachmentDefinition(novaNames.InternalAPINetworkNADName))

			th.ExpectConditionWithDetails(
				novaNames.MetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occurred "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
			)
			th.ExpectConditionWithDetails(
				novaNames.MetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occurred "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
			)

			th.SimulateStatefulSetReplicaReadyWithPods(
				novaNames.MetadataStatefulSetName,
				map[string][]string{novaNames.MetadataName.Namespace + "/internalapi": {"10.0.0.1"}},
			)

			th.ExpectCondition(
				novaNames.MetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionTrue,
			)

			Eventually(func(g Gomega) {
				novaMetadata := GetNovaMetadata(novaNames.MetadataName)
				g.Expect(novaMetadata.Status.NetworkAttachments).To(
					Equal(map[string][]string{novaNames.MetadataName.Namespace + "/internalapi": {"10.0.0.1"}}))

			}, timeout, interval).Should(Succeed())

			th.ExpectCondition(
				novaNames.MetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("applies new RegisteredCells input to its StatefulSet to trigger Pod restart", func() {
			originalConfigHash := GetEnvVarValue(
				th.GetStatefulSet(novaNames.MetadataStatefulSetName).Spec.Template.Spec.Containers[0].Env, "CONFIG_HASH", "")

			// Simulate that a new cell is added and Nova controller registered it and
			// therefore a new cell is added to RegisteredCells
			Eventually(func(g Gomega) {
				novaMetadata := GetNovaMetadata(novaNames.MetadataName)
				novaMetadata.Spec.RegisteredCells = map[string]string{"cell0": "cell0-config-hash"}
				g.Expect(k8sClient.Update(ctx, novaMetadata)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			// Assert that the CONFIG_HASH of the StateFulSet is changed due to this reconfiguration
			Eventually(func(g Gomega) {
				currentConfigHash := GetEnvVarValue(
					th.GetStatefulSet(novaNames.MetadataStatefulSetName).Spec.Template.Spec.Containers[0].Env, "CONFIG_HASH", "")
				g.Expect(originalConfigHash).NotTo(Equal(currentConfigHash))

			}, timeout, interval).Should(Succeed())
		})
	})

	When("starts zero replicas", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateInternalTopLevelSecret(novaNames))

			spec := GetDefaultNovaMetadataSpec(novaNames.InternalTopLevelSecretName)
			spec["replicas"] = 0
			metadata := CreateNovaMetadata(novaNames.MetadataName, spec)
			DeferCleanup(th.DeleteInstance, metadata)
		})
		It("and deployment is Ready", func() {
			ss := th.GetStatefulSet(novaNames.MetadataStatefulSetName)
			Expect(int(*ss.Spec.Replicas)).To(Equal(0))
			th.SimulateStatefulSetReplicaReady(novaNames.MetadataStatefulSetName)
			th.ExpectCondition(
				novaNames.MetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionTrue,
			)

		})
	})

	When("NovaMetadata CR instance is deleted", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateInternalTopLevelSecret(novaNames))
			DeferCleanup(th.DeleteInstance, CreateNovaMetadata(novaNames.MetadataName, GetDefaultNovaMetadataSpec(novaNames.InternalTopLevelSecretName)))
		})

		It("removes the finalizer from Memcached", func() {
			th.SimulateStatefulSetReplicaReady(novaNames.MetadataStatefulSetName)
			th.ExpectCondition(
				novaNames.MetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
			memcached := infra.GetMemcached(novaNames.MemcachedNamespace)
			Expect(memcached.Finalizers).To(ContainElement("NovaMetadata"))

			Eventually(func(g Gomega) {
				th.DeleteInstance(GetNovaMetadata(novaNames.MetadataName))
				memcached := infra.GetMemcached(novaNames.MemcachedNamespace)
				g.Expect(memcached.Finalizers).NotTo(ContainElement("NovaMetadata"))
			}, timeout, interval).Should(Succeed())
		})
	})

	When("NovaMetadata CR is created without container image defined", func() {
		BeforeEach(func() {
			spec := GetDefaultNovaMetadataSpec(novaNames.InternalTopLevelSecretName)
			spec["containerImage"] = ""
			metadata := CreateNovaMetadata(novaNames.MetadataName, spec)
			DeferCleanup(th.DeleteInstance, metadata)
		})
		It("has the expected container image default", func() {
			novaMetadataDefault := GetNovaMetadata(novaNames.MetadataName)
			Expect(novaMetadataDefault.Spec.ContainerImage).To(Equal(util.GetEnvVar("RELATED_IMAGE_NOVA_API_IMAGE_URL_DEFAULT", novav1.NovaMetadataContainerImage)))
		})
	})
})

var _ = Describe("NovaMetadata controller", func() {
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

		mariadb.CreateMariaDBDatabase(novaNames.APIMariaDBDatabaseName.Namespace, novaNames.APIMariaDBDatabaseName.Name, mariadbv1.MariaDBDatabaseSpec{})
		DeferCleanup(k8sClient.Delete, ctx, mariadb.GetMariaDBDatabase(novaNames.APIMariaDBDatabaseName))

		mariadb.SimulateMariaDBTLSDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
		mariadb.SimulateMariaDBAccountCompleted(novaNames.APIMariaDBDatabaseAccount)
		memcachedSpec := memcachedv1.MemcachedSpec{
			MemcachedSpecCore: memcachedv1.MemcachedSpecCore{
				Replicas: ptr.To(int32(3)),
			},
		}
		DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(novaNames.NovaName.Namespace, MemcachedInstance, memcachedSpec))
		infra.SimulateTLSMemcachedReady(novaNames.MemcachedNamespace)

	})

	When("NovaMetadata is created with TLS CA cert secret", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateInternalTopLevelSecret(novaNames))

			spec := GetDefaultNovaMetadataSpec(novaNames.InternalTopLevelSecretName)
			spec["tls"] = map[string]interface{}{
				"secretName":         ptr.To(novaNames.InternalCertSecretName.Name),
				"caBundleSecretName": novaNames.CaBundleSecretName.Name,
			}

			DeferCleanup(th.DeleteInstance, CreateNovaMetadata(novaNames.MetadataName, spec))
		})

		It("reports that the CA secret is missing", func() {
			th.ExpectConditionWithDetails(
				novaNames.MetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				fmt.Sprintf("TLSInput error occured in TLS sources Secret %s/combined-ca-bundle not found", novaNames.Namespace),
			)
			th.ExpectCondition(
				novaNames.MetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("reports that the service cert secret is missing", func() {
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCABundleSecret(novaNames.CaBundleSecretName))

			th.ExpectConditionWithDetails(
				novaNames.MetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				fmt.Sprintf("TLSInput error occured in TLS sources Secret %s/internal-tls-certs not found", novaNames.Namespace),
			)
			th.ExpectCondition(
				novaNames.MetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("creates a StatefulSet for nova-metadata service with TLS CA cert attached", func() {
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCABundleSecret(novaNames.CaBundleSecretName))
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(novaNames.InternalCertSecretName))
			th.SimulateStatefulSetReplicaReady(novaNames.MetadataStatefulSetName)

			th.ExpectCondition(
				novaNames.MetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionTrue,
			)

			ss := th.GetStatefulSet(novaNames.MetadataStatefulSetName)

			// Check the resulting deployment fields
			Expect(int(*ss.Spec.Replicas)).To(Equal(1))
			Expect(ss.Spec.Template.Spec.Volumes).To(HaveLen(4))
			Expect(ss.Spec.Template.Spec.Containers).To(HaveLen(2))

			// cert deployment volumes
			th.AssertVolumeExists(novaNames.CaBundleSecretName.Name, ss.Spec.Template.Spec.Volumes)
			th.AssertVolumeExists("nova-metadata-tls-certs", ss.Spec.Template.Spec.Volumes)

			// CA container certs
			apiContainer := ss.Spec.Template.Spec.Containers[1]
			th.AssertVolumeMountExists(novaNames.CaBundleSecretName.Name, "tls-ca-bundle.pem", apiContainer.VolumeMounts)
			th.AssertVolumeMountExists("nova-metadata-tls-certs", "tls.key", apiContainer.VolumeMounts)
			th.AssertVolumeMountExists("nova-metadata-tls-certs", "tls.crt", apiContainer.VolumeMounts)

			th.ExpectCondition(
				novaNames.MetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)

			configDataMap := th.GetSecret(novaNames.MetadataConfigDataName)
			Expect(configDataMap).ShouldNot(BeNil())
			Expect(configDataMap.Data).Should(HaveKey("httpd.conf"))
			Expect(configDataMap.Data).Should(HaveKey("ssl.conf"))
			configData := string(configDataMap.Data["httpd.conf"])
			Expect(configData).Should(ContainSubstring("SSLEngine on"))
			Expect(configData).Should(ContainSubstring("SSLCertificateFile      \"/etc/pki/tls/certs/nova-metadata.crt\""))
			Expect(configData).Should(ContainSubstring("SSLCertificateKeyFile   \"/etc/pki/tls/private/nova-metadata.key\""))

			configData = string(configDataMap.Data["01-nova.conf"])
			Expect(configData).Should(
				ContainSubstring("backend = dogpile.cache.pymemcache"))
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

			computeConfigData := th.GetSecret(novaNames.MetadataNeutronConfigDataName)
			Expect(computeConfigData).ShouldNot(BeNil())
			Expect(computeConfigData.Data).To(HaveLen(1))
			Expect(computeConfigData.Data).Should(HaveKey("05-nova-metadata.conf"))
			configData = string(computeConfigData.Data["05-nova-metadata.conf"])
			Expect(configData).Should(ContainSubstring("nova_metadata_protocol = https"))
		})

		It("reconfigures the Metadata pod when CA changes", func() {
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCABundleSecret(novaNames.CaBundleSecretName))
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(novaNames.InternalCertSecretName))
			th.SimulateStatefulSetReplicaReady(novaNames.MetadataStatefulSetName)

			th.ExpectCondition(
				novaNames.MetadataName,
				ConditionGetterFunc(NovaMetadataConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionTrue,
			)

			ss := th.GetStatefulSet(novaNames.MetadataStatefulSetName)

			// Check the resulting deployment fields
			Expect(int(*ss.Spec.Replicas)).To(Equal(1))
			Expect(ss.Spec.Template.Spec.Volumes).To(HaveLen(4))
			Expect(ss.Spec.Template.Spec.Containers).To(HaveLen(2))

			// Grab the current config hash
			originalHash := GetEnvVarValue(
				th.GetStatefulSet(novaNames.MetadataStatefulSetName).Spec.Template.Spec.Containers[0].Env, "CONFIG_HASH", "")
			Expect(originalHash).NotTo(BeEmpty())

			// Change the content of the CA secret
			th.UpdateSecret(novaNames.CaBundleSecretName, "tls-ca-bundle.pem", []byte("DifferentCAData"))

			// Assert that the deployment is updated
			Eventually(func(g Gomega) {
				newHash := GetEnvVarValue(
					th.GetStatefulSet(novaNames.MetadataStatefulSetName).Spec.Template.Spec.Containers[0].Env, "CONFIG_HASH", "")
				g.Expect(newHash).NotTo(BeEmpty())
				g.Expect(newHash).NotTo(Equal(originalHash))
			}, timeout, interval).Should(Succeed())
		})
	})
})
