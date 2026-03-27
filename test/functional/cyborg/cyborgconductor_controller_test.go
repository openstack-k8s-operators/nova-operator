/*
Copyright 2026.

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
package cyborg_test

import (
	. "github.com/onsi/ginkgo/v2" //revive:disable:dot-imports
	. "github.com/onsi/gomega"    //revive:disable:dot-imports

	//revive:disable-next-line:dot-imports
	. "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"
	cyborgv1beta1 "github.com/openstack-k8s-operators/nova-operator/api/cyborg/v1beta1"
)

const (
	ConductorTestImage = "test://cyborg-conductor"
	ConductorTestSA    = "cyborg-test-sa"
)

type CyborgConductorNames struct {
	ConductorName    types.NamespacedName
	ConfigSecretName types.NamespacedName
	StatefulSetName  types.NamespacedName
	ConfigDataName   types.NamespacedName
}

func GetCyborgConductorNames(namespace string, name string) CyborgConductorNames {
	return CyborgConductorNames{
		ConductorName: types.NamespacedName{
			Namespace: namespace,
			Name:      name,
		},
		ConfigSecretName: types.NamespacedName{
			Namespace: namespace,
			Name:      name + "-input-secret",
		},
		StatefulSetName: types.NamespacedName{
			Namespace: namespace,
			Name:      name,
		},
		ConfigDataName: types.NamespacedName{
			Namespace: namespace,
			Name:      name + "-config-data",
		},
	}
}

func CreateCyborgConductorConfigSecret(name types.NamespacedName) *corev1.Secret {
	return th.CreateSecret(name, map[string][]byte{
		"transport_url":     []byte("rabbit://user:pass@rabbitmq:5672/"),
		"database_account":  []byte("cyborg"),
		"database_username": []byte("cyborg_user"),
		"database_password": []byte("cyborg_pass"),
		"database_hostname": []byte("openstack.openstack.svc"),
		"ServiceUser":       []byte("cyborg"),
		"ServicePassword":   []byte("service-pass"),
		"KeystoneAuthURL":   []byte("https://keystone-internal.openstack.svc:5000"),
		"Region":            []byte("regionOne"),
	})
}

func CreateCyborgConductorConfigSecretWithAppCred(name types.NamespacedName, acid, acSecret string) *corev1.Secret {
	return th.CreateSecret(name, map[string][]byte{
		"transport_url":     []byte("rabbit://user:pass@rabbitmq:5672/?ssl=1"),
		"database_account":  []byte("cyborg"),
		"database_username": []byte("cyborg_user"),
		"database_password": []byte("cyborg_pass"),
		"database_hostname": []byte("openstack.openstack.svc"),
		"ServiceUser":       []byte("cyborg"),
		"ServicePassword":   []byte("service-pass"),
		"KeystoneAuthURL":   []byte("https://keystone-internal.openstack.svc:5000"),
		"Region":            []byte("regionOne"),
		"ACID":              []byte(acid),
		"ACSecret":          []byte(acSecret),
	})
}

func CreateCyborgConductorCR(name types.NamespacedName, spec cyborgv1beta1.CyborgConductorSpec) client.Object {
	conductor := &cyborgv1beta1.CyborgConductor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.Name,
			Namespace: name.Namespace,
		},
		Spec: spec,
	}
	Expect(k8sClient.Create(ctx, conductor)).To(Succeed())
	return conductor
}

var _ = Describe("CyborgConductor controller", func() {
	var conductorNames CyborgConductorNames

	BeforeEach(func() {
		conductorNames = GetCyborgConductorNames(
			cyborgNames.CyborgName.Namespace,
			"cyborg-conductor-test",
		)
	})

	When("CyborgConductor is created with a missing config secret", func() {
		It("sets InputReady to False while waiting for the secret", func() {
			DeferCleanup(th.DeleteInstance, CreateCyborgConductorCR(
				conductorNames.ConductorName,
				cyborgv1beta1.CyborgConductorSpec{
					CyborgConductorTemplate: cyborgv1beta1.CyborgConductorTemplate{
						Replicas: ptr.To(int32(1)),
					},
					ConfigSecret:   "nonexistent-secret",
					ContainerImage: ConductorTestImage,
					ServiceAccount: ConductorTestSA,
				},
			))

			th.ExpectCondition(
				conductorNames.ConductorName,
				ConditionGetterFunc(CyborgConductorConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionFalse,
			)
		})
	})

	When("CyborgConductor is created with default config", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx,
				CreateCyborgConductorConfigSecret(conductorNames.ConfigSecretName),
			)

			DeferCleanup(th.DeleteInstance, CreateCyborgConductorCR(
				conductorNames.ConductorName,
				cyborgv1beta1.CyborgConductorSpec{
					CyborgConductorTemplate: cyborgv1beta1.CyborgConductorTemplate{
						Replicas: ptr.To(int32(1)),
					},
					ConfigSecret:   conductorNames.ConfigSecretName.Name,
					ContainerImage: ConductorTestImage,
					ServiceAccount: ConductorTestSA,
				},
			))
		})

		It("initializes all expected status conditions", func() {
			Eventually(func(g Gomega) {
				conductor := GetCyborgConductor(conductorNames.ConductorName)
				g.Expect(conductor.Status.Conditions).NotTo(BeNil())
				g.Expect(conductor.Status.Conditions.Has(condition.ReadyCondition)).To(BeTrue())
				g.Expect(conductor.Status.Conditions.Has(condition.InputReadyCondition)).To(BeTrue())
				g.Expect(conductor.Status.Conditions.Has(condition.ServiceConfigReadyCondition)).To(BeTrue())
				g.Expect(conductor.Status.Conditions.Has(condition.DeploymentReadyCondition)).To(BeTrue())
				// No topology => no TopologyReadyCondition
				g.Expect(conductor.Status.Conditions.Has(condition.TopologyReadyCondition)).To(BeFalse())
			}, timeout, interval).Should(Succeed())
		})

		It("marks InputReady as True", func() {
			th.ExpectCondition(
				conductorNames.ConductorName,
				ConditionGetterFunc(CyborgConductorConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("generates a config secret with all expected keys and config sections", func() {
			Eventually(func(g Gomega) {
				configSecret := th.GetSecret(conductorNames.ConfigDataName)
				g.Expect(configSecret.Data).To(HaveKey("00-default.conf"))
				g.Expect(configSecret.Data).To(HaveKey("my.cnf"))
				g.Expect(configSecret.Data).To(HaveKey("cyborg-conductor-config.json"))

				defaultConf := string(configSecret.Data["00-default.conf"])
				g.Expect(defaultConf).To(ContainSubstring("[database]"))
				g.Expect(defaultConf).To(ContainSubstring("connection = mysql+pymysql://cyborg_user:cyborg_pass@openstack.openstack.svc/cyborg"))
				g.Expect(defaultConf).To(ContainSubstring("[oslo_messaging_rabbit]"))
				g.Expect(defaultConf).To(ContainSubstring("transport_url"))
				g.Expect(defaultConf).To(ContainSubstring("[keystone_authtoken]"))
				g.Expect(defaultConf).To(ContainSubstring("[placement]"))
				g.Expect(defaultConf).To(ContainSubstring("[nova]"))
				g.Expect(defaultConf).To(ContainSubstring("[agent]"))
				g.Expect(defaultConf).To(ContainSubstring("auth_type = password"))
				g.Expect(defaultConf).To(ContainSubstring("username = cyborg"))
				g.Expect(defaultConf).To(ContainSubstring("region_name = regionOne"))

				// No TLS => my.cnf should be minimal
				myCnf := string(configSecret.Data["my.cnf"])
				g.Expect(myCnf).To(Equal("[client]\n"))
			}, timeout, interval).Should(Succeed())

			th.ExpectCondition(
				conductorNames.ConductorName,
				ConditionGetterFunc(CyborgConductorConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("creates a StatefulSet with the correct spec", func() {
			Eventually(func(g Gomega) {
				ss := th.GetStatefulSet(conductorNames.StatefulSetName)
				g.Expect(ss.Spec.Replicas).NotTo(BeNil())
				g.Expect(*ss.Spec.Replicas).To(Equal(int32(1)))
				g.Expect(ss.Spec.Template.Spec.ServiceAccountName).To(Equal(ConductorTestSA))
				g.Expect(ss.Spec.Template.Spec.Containers).To(HaveLen(1))

				container := ss.Spec.Template.Spec.Containers[0]
				g.Expect(container.Name).To(Equal("cyborg-conductor"))
				g.Expect(container.Image).To(Equal(ConductorTestImage))

				hasConfigHash := false
				hasKollaStrategy := false
				for _, envVar := range container.Env {
					if envVar.Name == "CONFIG_HASH" {
						hasConfigHash = true
						g.Expect(envVar.Value).NotTo(BeEmpty())
					}
					if envVar.Name == "KOLLA_CONFIG_STRATEGY" {
						hasKollaStrategy = true
						g.Expect(envVar.Value).To(Equal("COPY_ALWAYS"))
					}
				}
				g.Expect(hasConfigHash).To(BeTrue())
				g.Expect(hasKollaStrategy).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		})

		It("sets the hash in the status", func() {
			Eventually(func(g Gomega) {
				conductor := GetCyborgConductor(conductorNames.ConductorName)
				g.Expect(conductor.Status.Hash).NotTo(BeNil())
				g.Expect(conductor.Status.Hash).To(HaveKey("input"))
			}, timeout, interval).Should(Succeed())
		})

		It("reaches Ready when the StatefulSet replicas are ready", func() {
			th.SimulateStatefulSetReplicaReady(conductorNames.StatefulSetName)

			th.ExpectCondition(
				conductorNames.ConductorName,
				ConditionGetterFunc(CyborgConductorConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				conductorNames.ConductorName,
				ConditionGetterFunc(CyborgConductorConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)

			conductor := GetCyborgConductor(conductorNames.ConductorName)
			Expect(conductor.Status.ObservedGeneration).To(Equal(conductor.Generation))
		})
	})

	When("CyborgConductor is created with custom resources", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx,
				CreateCyborgConductorConfigSecret(conductorNames.ConfigSecretName),
			)

			DeferCleanup(th.DeleteInstance, CreateCyborgConductorCR(
				conductorNames.ConductorName,
				cyborgv1beta1.CyborgConductorSpec{
					CyborgConductorTemplate: cyborgv1beta1.CyborgConductorTemplate{
						Replicas: ptr.To(int32(2)),
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("256Mi"),
								corev1.ResourceCPU:    resource.MustParse("500m"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("512Mi"),
								corev1.ResourceCPU:    resource.MustParse("1"),
							},
						},
					},
					ConfigSecret:   conductorNames.ConfigSecretName.Name,
					ContainerImage: ConductorTestImage,
					ServiceAccount: ConductorTestSA,
				},
			))
		})

		It("applies resources and replicas to the StatefulSet", func() {
			Eventually(func(g Gomega) {
				ss := th.GetStatefulSet(conductorNames.StatefulSetName)
				g.Expect(*ss.Spec.Replicas).To(Equal(int32(2)))

				container := ss.Spec.Template.Spec.Containers[0]
				g.Expect(container.Resources.Requests[corev1.ResourceMemory]).To(Equal(resource.MustParse("256Mi")))
				g.Expect(container.Resources.Requests[corev1.ResourceCPU]).To(Equal(resource.MustParse("500m")))
				g.Expect(container.Resources.Limits[corev1.ResourceMemory]).To(Equal(resource.MustParse("512Mi")))
				g.Expect(container.Resources.Limits[corev1.ResourceCPU]).To(Equal(resource.MustParse("1")))
			}, timeout, interval).Should(Succeed())
		})
	})

	When("CyborgConductor is created with a TopologyRef", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx,
				CreateCyborgConductorConfigSecret(conductorNames.ConfigSecretName),
			)

			topologyObj := CreateCyborgTopology(conductorNames.ConductorName.Namespace, "conductor-test-topo")
			DeferCleanup(k8sClient.Delete, ctx, topologyObj)

			DeferCleanup(th.DeleteInstance, CreateCyborgConductorCR(
				conductorNames.ConductorName,
				cyborgv1beta1.CyborgConductorSpec{
					CyborgConductorTemplate: cyborgv1beta1.CyborgConductorTemplate{
						Replicas: ptr.To(int32(1)),
						TopologyRef: &topologyv1.TopoRef{
							Name: "conductor-test-topo",
						},
					},
					ConfigSecret:   conductorNames.ConfigSecretName.Name,
					ContainerImage: ConductorTestImage,
					ServiceAccount: ConductorTestSA,
				},
			))
		})

		It("initializes TopologyReadyCondition", func() {
			Eventually(func(g Gomega) {
				conductor := GetCyborgConductor(conductorNames.ConductorName)
				g.Expect(conductor.Status.Conditions.Has(condition.TopologyReadyCondition)).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		})

		It("sets TopologyReady to True once the Topology is resolved", func() {
			th.ExpectCondition(
				conductorNames.ConductorName,
				ConditionGetterFunc(CyborgConductorConditionGetter),
				condition.TopologyReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("creates a StatefulSet and reaches Ready", func() {
			th.SimulateStatefulSetReplicaReady(conductorNames.StatefulSetName)

			th.ExpectCondition(
				conductorNames.ConductorName,
				ConditionGetterFunc(CyborgConductorConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})

	When("CyborgConductor is created with TLS CA bundle", func() {
		const caBundleSecretName = "conductor-test-ca-bundle"

		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx,
				CreateCyborgConductorConfigSecret(conductorNames.ConfigSecretName),
			)

			caBundleSecret := th.CreateSecret(
				types.NamespacedName{
					Namespace: conductorNames.ConductorName.Namespace,
					Name:      caBundleSecretName,
				},
				map[string][]byte{tls.CABundleKey: []byte("dummy-ca-bundle")},
			)
			DeferCleanup(k8sClient.Delete, ctx, caBundleSecret)

			DeferCleanup(th.DeleteInstance, CreateCyborgConductorCR(
				conductorNames.ConductorName,
				cyborgv1beta1.CyborgConductorSpec{
					CyborgConductorTemplate: cyborgv1beta1.CyborgConductorTemplate{
						Replicas: ptr.To(int32(1)),
					},
					ConfigSecret:   conductorNames.ConfigSecretName.Name,
					ContainerImage: ConductorTestImage,
					ServiceAccount: ConductorTestSA,
					TLS:            tls.Ca{CaBundleSecretName: caBundleSecretName},
				},
			))
		})

		It("generates my.cnf with TLS settings", func() {
			Eventually(func(g Gomega) {
				configSecret := th.GetSecret(conductorNames.ConfigDataName)
				myCnf := string(configSecret.Data["my.cnf"])
				g.Expect(myCnf).To(ContainSubstring("ssl-ca="))
				g.Expect(myCnf).To(ContainSubstring("ssl=1"))
			}, timeout, interval).Should(Succeed())
		})

		It("includes cafile in the generated default config", func() {
			Eventually(func(g Gomega) {
				configSecret := th.GetSecret(conductorNames.ConfigDataName)
				defaultConf := string(configSecret.Data["00-default.conf"])
				g.Expect(defaultConf).To(ContainSubstring("cafile = " + tls.DownstreamTLSCABundlePath))
			}, timeout, interval).Should(Succeed())
		})

		It("mounts the CA bundle volume in the StatefulSet", func() {
			Eventually(func(g Gomega) {
				ss := th.GetStatefulSet(conductorNames.StatefulSetName)

				hasCAVolume := false
				for _, v := range ss.Spec.Template.Spec.Volumes {
					if v.Secret != nil && v.Secret.SecretName == caBundleSecretName {
						hasCAVolume = true
					}
				}
				g.Expect(hasCAVolume).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		})
	})

	When("CyborgConductor is created with application credentials", func() {
		const (
			appCredID     = "test-acid-123"     //nolint:gosec
			appCredSecret = "test-acsecret-456" //nolint:gosec
		)

		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx,
				CreateCyborgConductorConfigSecretWithAppCred(
					conductorNames.ConfigSecretName, appCredID, appCredSecret),
			)

			DeferCleanup(th.DeleteInstance, CreateCyborgConductorCR(
				conductorNames.ConductorName,
				cyborgv1beta1.CyborgConductorSpec{
					CyborgConductorTemplate: cyborgv1beta1.CyborgConductorTemplate{
						Replicas: ptr.To(int32(1)),
					},
					ConfigSecret:   conductorNames.ConfigSecretName.Name,
					ContainerImage: ConductorTestImage,
					ServiceAccount: ConductorTestSA,
				},
			))
		})

		It("generates config with v3applicationcredential auth instead of password", func() {
			Eventually(func(g Gomega) {
				configSecret := th.GetSecret(conductorNames.ConfigDataName)
				defaultConf := string(configSecret.Data["00-default.conf"])
				g.Expect(defaultConf).To(ContainSubstring("auth_type = v3applicationcredential"))
				g.Expect(defaultConf).To(ContainSubstring("application_credential_id = " + appCredID))
				g.Expect(defaultConf).To(ContainSubstring("application_credential_secret = " + appCredSecret))
				g.Expect(defaultConf).NotTo(ContainSubstring("auth_type = password"))
			}, timeout, interval).Should(Succeed())
		})
	})

	When("CyborgConductor is created with custom service config", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx,
				CreateCyborgConductorConfigSecret(conductorNames.ConfigSecretName),
			)

			DeferCleanup(th.DeleteInstance, CreateCyborgConductorCR(
				conductorNames.ConductorName,
				cyborgv1beta1.CyborgConductorSpec{
					CyborgConductorTemplate: cyborgv1beta1.CyborgConductorTemplate{
						Replicas:            ptr.To(int32(1)),
						CustomServiceConfig: "[DEFAULT]\nmy_custom_key = my_custom_value\n",
					},
					ConfigSecret:   conductorNames.ConfigSecretName.Name,
					ContainerImage: ConductorTestImage,
					ServiceAccount: ConductorTestSA,
				},
			))
		})

		It("includes the custom config in the generated config secret", func() {
			Eventually(func(g Gomega) {
				configSecret := th.GetSecret(conductorNames.ConfigDataName)
				g.Expect(configSecret.Data).To(HaveKey("01-service-custom.conf"))
				customConf := string(configSecret.Data["01-service-custom.conf"])
				g.Expect(customConf).To(ContainSubstring("my_custom_key = my_custom_value"))
			}, timeout, interval).Should(Succeed())
		})
	})

	When("CyborgConductor is created with a NodeSelector", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx,
				CreateCyborgConductorConfigSecret(conductorNames.ConfigSecretName),
			)

			DeferCleanup(th.DeleteInstance, CreateCyborgConductorCR(
				conductorNames.ConductorName,
				cyborgv1beta1.CyborgConductorSpec{
					CyborgConductorTemplate: cyborgv1beta1.CyborgConductorTemplate{
						Replicas: ptr.To(int32(1)),
						NodeSelector: &map[string]string{
							"disktype": "ssd",
						},
					},
					ConfigSecret:   conductorNames.ConfigSecretName.Name,
					ContainerImage: ConductorTestImage,
					ServiceAccount: ConductorTestSA,
				},
			))
		})

		It("applies the nodeSelector to the StatefulSet pod template", func() {
			Eventually(func(g Gomega) {
				ss := th.GetStatefulSet(conductorNames.StatefulSetName)
				g.Expect(ss.Spec.Template.Spec.NodeSelector).To(HaveKeyWithValue("disktype", "ssd"))
			}, timeout, interval).Should(Succeed())
		})
	})

	When("the config secret is updated", func() {
		It("updates the CONFIG_HASH in the StatefulSet", func() {
			DeferCleanup(
				k8sClient.Delete, ctx,
				CreateCyborgConductorConfigSecret(conductorNames.ConfigSecretName),
			)

			DeferCleanup(th.DeleteInstance, CreateCyborgConductorCR(
				conductorNames.ConductorName,
				cyborgv1beta1.CyborgConductorSpec{
					CyborgConductorTemplate: cyborgv1beta1.CyborgConductorTemplate{
						Replicas: ptr.To(int32(1)),
					},
					ConfigSecret:   conductorNames.ConfigSecretName.Name,
					ContainerImage: ConductorTestImage,
					ServiceAccount: ConductorTestSA,
				},
			))

			var originalHash string
			Eventually(func(g Gomega) {
				ss := th.GetStatefulSet(conductorNames.StatefulSetName)
				for _, envVar := range ss.Spec.Template.Spec.Containers[0].Env {
					if envVar.Name == "CONFIG_HASH" {
						originalHash = envVar.Value
					}
				}
				g.Expect(originalHash).NotTo(BeEmpty())
			}, timeout, interval).Should(Succeed())

			// Update the secret with new data
			Eventually(func(g Gomega) {
				secret := &corev1.Secret{}
				g.Expect(k8sClient.Get(ctx, conductorNames.ConfigSecretName, secret)).To(Succeed())
				secret.Data["ServicePassword"] = []byte("new-password")
				g.Expect(k8sClient.Update(ctx, secret)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				ss := th.GetStatefulSet(conductorNames.StatefulSetName)
				for _, envVar := range ss.Spec.Template.Spec.Containers[0].Env {
					if envVar.Name == "CONFIG_HASH" {
						g.Expect(envVar.Value).NotTo(Equal(originalHash))
					}
				}
			}, timeout, interval).Should(Succeed())
		})
	})
})
