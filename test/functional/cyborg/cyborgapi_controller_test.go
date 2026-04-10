/*
Copyright 2024.

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
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"
	cyborgv1beta1 "github.com/openstack-k8s-operators/nova-operator/api/cyborg/v1beta1"
)

const (
	APITestImage = "test://cyborg-api"
	APITestSA    = "cyborg-test-api-sa"
)

type CyborgAPINames struct {
	APIName                types.NamespacedName
	ConfigSecretName       types.NamespacedName
	StatefulSetName        types.NamespacedName
	ConfigDataName         types.NamespacedName
	KeystoneEndpointName   types.NamespacedName
	PublicServiceName      types.NamespacedName
	InternalServiceName    types.NamespacedName
	CaBundleSecretName     types.NamespacedName
	InternalCertSecretName types.NamespacedName
	PublicCertSecretName   types.NamespacedName
}

func GetCyborgAPINames(namespace string, name string) CyborgAPINames {
	return CyborgAPINames{
		APIName: types.NamespacedName{
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
		KeystoneEndpointName: types.NamespacedName{
			Namespace: namespace,
			Name:      "cyborg",
		},
		PublicServiceName: types.NamespacedName{
			Namespace: namespace,
			Name:      "cyborg-public",
		},
		InternalServiceName: types.NamespacedName{
			Namespace: namespace,
			Name:      "cyborg-internal",
		},
		CaBundleSecretName: types.NamespacedName{
			Namespace: namespace,
			Name:      "combined-ca-bundle",
		},
		InternalCertSecretName: types.NamespacedName{
			Namespace: namespace,
			Name:      "cert-cyborg-internal-svc",
		},
		PublicCertSecretName: types.NamespacedName{
			Namespace: namespace,
			Name:      "cert-cyborg-public-svc",
		},
	}
}

func CreateCyborgAPIConfigSecret(name types.NamespacedName) *corev1.Secret {
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

func CreateCyborgAPIConfigSecretWithAppCred(name types.NamespacedName, acid, acSecret string) *corev1.Secret {
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

func CreateCyborgAPICR(name types.NamespacedName, spec cyborgv1beta1.CyborgAPISpec) client.Object {
	api := &cyborgv1beta1.CyborgAPI{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.Name,
			Namespace: name.Namespace,
		},
		Spec: spec,
	}
	Expect(k8sClient.Create(ctx, api)).To(Succeed())
	return api
}

var _ = Describe("CyborgAPI controller", func() {
	var apiNames CyborgAPINames

	BeforeEach(func() {
		apiNames = GetCyborgAPINames(
			cyborgNames.CyborgName.Namespace,
			"cyborg-api-test",
		)
	})

	When("CyborgAPI is created with a missing config secret", func() {
		It("sets InputReady to False while waiting for the secret", func() {
			DeferCleanup(th.DeleteInstance, CreateCyborgAPICR(
				apiNames.APIName,
				cyborgv1beta1.CyborgAPISpec{
					CyborgAPITemplate: cyborgv1beta1.CyborgAPITemplate{
						Replicas: ptr.To(int32(1)),
					},
					ConfigSecret:   "nonexistent-secret",
					ContainerImage: APITestImage,
					ServiceAccount: APITestSA,
					APITimeout:     ptr.To(60),
				},
			))

			th.ExpectCondition(
				apiNames.APIName,
				ConditionGetterFunc(CyborgAPIConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionFalse,
			)
		})
	})

	When("CyborgAPI is created with default config", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx,
				CreateCyborgAPIConfigSecret(apiNames.ConfigSecretName),
			)

			DeferCleanup(th.DeleteInstance, CreateCyborgAPICR(
				apiNames.APIName,
				cyborgv1beta1.CyborgAPISpec{
					CyborgAPITemplate: cyborgv1beta1.CyborgAPITemplate{
						Replicas: ptr.To(int32(1)),
					},
					ConfigSecret:   apiNames.ConfigSecretName.Name,
					ContainerImage: APITestImage,
					ServiceAccount: APITestSA,
					APITimeout:     ptr.To(60),
				},
			))
		})

		It("initializes all expected status conditions", func() {
			Eventually(func(g Gomega) {
				api := GetCyborgAPI(apiNames.APIName)
				g.Expect(api.Status.Conditions).NotTo(BeNil())
				g.Expect(api.Status.Conditions.Has(condition.ReadyCondition)).To(BeTrue())
				g.Expect(api.Status.Conditions.Has(condition.InputReadyCondition)).To(BeTrue())
				g.Expect(api.Status.Conditions.Has(condition.TLSInputReadyCondition)).To(BeTrue())
				g.Expect(api.Status.Conditions.Has(condition.ServiceConfigReadyCondition)).To(BeTrue())
				g.Expect(api.Status.Conditions.Has(condition.DeploymentReadyCondition)).To(BeTrue())
				g.Expect(api.Status.Conditions.Has(condition.CreateServiceReadyCondition)).To(BeTrue())
				g.Expect(api.Status.Conditions.Has(condition.KeystoneEndpointReadyCondition)).To(BeTrue())
				// No topology => no TopologyReadyCondition
				g.Expect(api.Status.Conditions.Has(condition.TopologyReadyCondition)).To(BeFalse())
			}, timeout, interval).Should(Succeed())
		})

		It("marks InputReady as True", func() {
			th.ExpectCondition(
				apiNames.APIName,
				ConditionGetterFunc(CyborgAPIConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("generates a config secret with all expected keys and config sections", func() {
			Eventually(func(g Gomega) {
				configSecret := th.GetSecret(apiNames.ConfigDataName)
				g.Expect(configSecret.Data).To(HaveKey("00-default.conf"))
				g.Expect(configSecret.Data).To(HaveKey("my.cnf"))
				g.Expect(configSecret.Data).To(HaveKey("cyborg-api-config.json"))
				g.Expect(configSecret.Data).To(HaveKey("httpd.conf"))
				g.Expect(configSecret.Data).To(HaveKey("ssl.conf"))
				g.Expect(configSecret.Data).To(HaveKey("10-cyborg-wsgi-main.conf"))

				wsgiConf := string(configSecret.Data["10-cyborg-wsgi-main.conf"])
				g.Expect(wsgiConf).To(ContainSubstring("TimeOut 60"))
				g.Expect(wsgiConf).To(ContainSubstring("WSGIScriptAlias / \"/var/lib/kolla/venv/lib/python3.12/site-packages/cyborg/wsgi/api.py\""))

				defaultConf := string(configSecret.Data["00-default.conf"])
				g.Expect(defaultConf).To(ContainSubstring("[database]"))
				g.Expect(defaultConf).To(ContainSubstring("connection = mysql+pymysql://cyborg_user:cyborg_pass@openstack.openstack.svc/cyborg"))
				g.Expect(defaultConf).To(ContainSubstring("[oslo_messaging_rabbit]"))
				g.Expect(defaultConf).To(ContainSubstring("transport_url"))
				g.Expect(defaultConf).To(ContainSubstring("[keystone_authtoken]"))
				g.Expect(defaultConf).To(ContainSubstring("[placement]"))
				g.Expect(defaultConf).To(ContainSubstring("[nova]"))
				g.Expect(defaultConf).To(ContainSubstring("auth_type = password"))
				g.Expect(defaultConf).To(ContainSubstring("username = cyborg"))
				g.Expect(defaultConf).To(ContainSubstring("region_name = regionOne"))
				g.Expect(defaultConf).To(ContainSubstring("log_file = /var/log/cyborg/cyborg-api-test.log"))

				// No TLS => my.cnf should be minimal
				myCnf := string(configSecret.Data["my.cnf"])
				g.Expect(myCnf).To(Equal("[client]\n"))
			}, timeout, interval).Should(Succeed())

			th.ExpectCondition(
				apiNames.APIName,
				ConditionGetterFunc(CyborgAPIConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("creates a StatefulSet with the correct spec", func() {
			Eventually(func(g Gomega) {
				ss := th.GetStatefulSet(apiNames.StatefulSetName)
				g.Expect(ss.Spec.Replicas).NotTo(BeNil())
				g.Expect(*ss.Spec.Replicas).To(Equal(int32(1)))
				g.Expect(ss.Spec.Template.Spec.ServiceAccountName).To(Equal(APITestSA))
				g.Expect(ss.Spec.Template.Spec.Containers).To(HaveLen(2))

				// Log sidecar container (index 0)
				logContainer := ss.Spec.Template.Spec.Containers[0]
				g.Expect(logContainer.Name).To(Equal("cyborg-api-log"))
				g.Expect(logContainer.Image).To(Equal(APITestImage))
				g.Expect(logContainer.Command).To(Equal([]string{"/usr/bin/dumb-init"}))
				g.Expect(logContainer.Args).To(ContainElement("/var/log/cyborg/cyborg-api-test.log"))
				hasLogVolumeMount := false
				for _, vm := range logContainer.VolumeMounts {
					if vm.MountPath == "/var/log/cyborg" {
						hasLogVolumeMount = true
					}
				}
				g.Expect(hasLogVolumeMount).To(BeTrue())

				// Main API container (index 1)
				container := ss.Spec.Template.Spec.Containers[1]
				g.Expect(container.Name).To(Equal("cyborg-api"))
				g.Expect(container.Image).To(Equal(APITestImage))

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

				// Log volume (EmptyDir) must be present
				hasLogVolume := false
				for _, v := range ss.Spec.Template.Spec.Volumes {
					if v.Name == "logs" && v.EmptyDir != nil {
						hasLogVolume = true
					}
				}
				g.Expect(hasLogVolume).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		})

		It("sets the hash in the status", func() {
			Eventually(func(g Gomega) {
				api := GetCyborgAPI(apiNames.APIName)
				g.Expect(api.Status.Hash).NotTo(BeNil())
				g.Expect(api.Status.Hash).To(HaveKey("input"))
			}, timeout, interval).Should(Succeed())
		})

		It("does not expose services until the StatefulSet is ready", func() {
			// Services should not exist before deployment is ready
			Eventually(func(g Gomega) {
				// Config secret should be created (StatefulSet exists)
				_ = th.GetStatefulSet(apiNames.StatefulSetName)
				// But services should not exist yet
				svcList := &corev1.ServiceList{}
				g.Expect(k8sClient.List(ctx, svcList, client.InNamespace(apiNames.APIName.Namespace))).To(Succeed())
				names := []string{}
				for _, svc := range svcList.Items {
					names = append(names, svc.Name)
				}
				g.Expect(names).NotTo(ContainElement("cyborg-public"))
				g.Expect(names).NotTo(ContainElement("cyborg-internal"))
			}, timeout, interval).Should(Succeed())
		})

		It("exposes internal and public services after deployment is ready", func() {
			th.SimulateStatefulSetReplicaReady(apiNames.StatefulSetName)

			th.ExpectCondition(
				apiNames.APIName,
				ConditionGetterFunc(CyborgAPIConditionGetter),
				condition.CreateServiceReadyCondition,
				corev1.ConditionTrue,
			)

			_ = th.GetService(apiNames.PublicServiceName)
			_ = th.GetService(apiNames.InternalServiceName)
		})

		It("creates the KeystoneEndpoint with correct URLs after deployment is ready", func() {
			th.SimulateStatefulSetReplicaReady(apiNames.StatefulSetName)
			keystone.SimulateKeystoneEndpointReady(apiNames.KeystoneEndpointName)

			ksEndpoint := keystone.GetKeystoneEndpoint(apiNames.KeystoneEndpointName)
			endpoints := ksEndpoint.Spec.Endpoints
			Expect(endpoints).To(HaveKeyWithValue("public",
				"http://cyborg-public."+apiNames.APIName.Namespace+".svc:6666/v2"))
			Expect(endpoints).To(HaveKeyWithValue("internal",
				"http://cyborg-internal."+apiNames.APIName.Namespace+".svc:6666/v2"))

			th.ExpectCondition(
				apiNames.APIName,
				ConditionGetterFunc(CyborgAPIConditionGetter),
				condition.KeystoneEndpointReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("reaches Ready when StatefulSet and KeystoneEndpoint are ready", func() {
			// DeploymentReady is False while the StatefulSet replicas are not yet ready
			th.ExpectCondition(
				apiNames.APIName,
				ConditionGetterFunc(CyborgAPIConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionFalse,
			)

			th.SimulateStatefulSetReplicaReady(apiNames.StatefulSetName)
			keystone.SimulateKeystoneEndpointReady(apiNames.KeystoneEndpointName)

			th.ExpectCondition(
				apiNames.APIName,
				ConditionGetterFunc(CyborgAPIConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				apiNames.APIName,
				ConditionGetterFunc(CyborgAPIConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)

			api := GetCyborgAPI(apiNames.APIName)
			Expect(api.Status.ObservedGeneration).To(Equal(api.Generation))
		})
	})

	When("CyborgAPI is created with custom resources", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx,
				CreateCyborgAPIConfigSecret(apiNames.ConfigSecretName),
			)

			DeferCleanup(th.DeleteInstance, CreateCyborgAPICR(
				apiNames.APIName,
				cyborgv1beta1.CyborgAPISpec{
					CyborgAPITemplate: cyborgv1beta1.CyborgAPITemplate{
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
					ConfigSecret:   apiNames.ConfigSecretName.Name,
					ContainerImage: APITestImage,
					ServiceAccount: APITestSA,
					APITimeout:     ptr.To(60),
				},
			))
		})

		It("applies resources and replicas to the StatefulSet", func() {
			Eventually(func(g Gomega) {
				ss := th.GetStatefulSet(apiNames.StatefulSetName)
				g.Expect(*ss.Spec.Replicas).To(Equal(int32(2)))

				// API container is at index 1 (log sidecar is at index 0)
				container := ss.Spec.Template.Spec.Containers[1]
				g.Expect(container.Resources.Requests[corev1.ResourceMemory]).To(Equal(resource.MustParse("256Mi")))
				g.Expect(container.Resources.Requests[corev1.ResourceCPU]).To(Equal(resource.MustParse("500m")))
				g.Expect(container.Resources.Limits[corev1.ResourceMemory]).To(Equal(resource.MustParse("512Mi")))
				g.Expect(container.Resources.Limits[corev1.ResourceCPU]).To(Equal(resource.MustParse("1")))
			}, timeout, interval).Should(Succeed())
		})
	})

	When("CyborgAPI is created with a TopologyRef", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx,
				CreateCyborgAPIConfigSecret(apiNames.ConfigSecretName),
			)

			topologyObj := CreateCyborgTopology(apiNames.APIName.Namespace, "api-test-topo")
			DeferCleanup(k8sClient.Delete, ctx, topologyObj)

			DeferCleanup(th.DeleteInstance, CreateCyborgAPICR(
				apiNames.APIName,
				cyborgv1beta1.CyborgAPISpec{
					CyborgAPITemplate: cyborgv1beta1.CyborgAPITemplate{
						Replicas: ptr.To(int32(1)),
						TopologyRef: &topologyv1.TopoRef{
							Name: "api-test-topo",
						},
					},
					ConfigSecret:   apiNames.ConfigSecretName.Name,
					ContainerImage: APITestImage,
					ServiceAccount: APITestSA,
					APITimeout:     ptr.To(60),
				},
			))
		})

		It("initializes TopologyReadyCondition", func() {
			Eventually(func(g Gomega) {
				api := GetCyborgAPI(apiNames.APIName)
				g.Expect(api.Status.Conditions.Has(condition.TopologyReadyCondition)).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		})

		It("sets TopologyReady to True once the Topology is resolved", func() {
			th.ExpectCondition(
				apiNames.APIName,
				ConditionGetterFunc(CyborgAPIConditionGetter),
				condition.TopologyReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("creates a StatefulSet and reaches Ready", func() {
			th.SimulateStatefulSetReplicaReady(apiNames.StatefulSetName)
			keystone.SimulateKeystoneEndpointReady(apiNames.KeystoneEndpointName)

			th.ExpectCondition(
				apiNames.APIName,
				ConditionGetterFunc(CyborgAPIConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})

	When("CyborgAPI is created with TLS CA bundle", func() {
		const caBundleSecretName = "api-test-ca-bundle" // #nosec G101

		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx,
				CreateCyborgAPIConfigSecret(apiNames.ConfigSecretName),
			)

			caBundleSecret := th.CreateSecret(
				types.NamespacedName{
					Namespace: apiNames.APIName.Namespace,
					Name:      caBundleSecretName,
				},
				map[string][]byte{tls.CABundleKey: []byte("dummy-ca-bundle")},
			)
			DeferCleanup(k8sClient.Delete, ctx, caBundleSecret)

			DeferCleanup(th.DeleteInstance, CreateCyborgAPICR(
				apiNames.APIName,
				cyborgv1beta1.CyborgAPISpec{
					CyborgAPITemplate: cyborgv1beta1.CyborgAPITemplate{
						Replicas: ptr.To(int32(1)),
						TLS:      tls.API{Ca: tls.Ca{CaBundleSecretName: caBundleSecretName}},
					},
					ConfigSecret:   apiNames.ConfigSecretName.Name,
					ContainerImage: APITestImage,
					ServiceAccount: APITestSA,
					APITimeout:     ptr.To(60),
				},
			))
		})

		It("generates my.cnf with TLS settings", func() {
			Eventually(func(g Gomega) {
				configSecret := th.GetSecret(apiNames.ConfigDataName)
				myCnf := string(configSecret.Data["my.cnf"])
				g.Expect(myCnf).To(ContainSubstring("ssl-ca="))
				g.Expect(myCnf).To(ContainSubstring("ssl=1"))
			}, timeout, interval).Should(Succeed())
		})

		It("includes cafile in the generated default config", func() {
			Eventually(func(g Gomega) {
				configSecret := th.GetSecret(apiNames.ConfigDataName)
				defaultConf := string(configSecret.Data["00-default.conf"])
				g.Expect(defaultConf).To(ContainSubstring("cafile = " + tls.DownstreamTLSCABundlePath))
			}, timeout, interval).Should(Succeed())
		})

		It("mounts the CA bundle volume in the StatefulSet", func() {
			Eventually(func(g Gomega) {
				ss := th.GetStatefulSet(apiNames.StatefulSetName)

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

	When("CyborgAPI is created with application credentials", func() {
		const (
			appCredID     = "test-api-acid-123"     //nolint:gosec
			appCredSecret = "test-api-acsecret-456" //nolint:gosec
		)

		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx,
				CreateCyborgAPIConfigSecretWithAppCred(
					apiNames.ConfigSecretName, appCredID, appCredSecret),
			)

			DeferCleanup(th.DeleteInstance, CreateCyborgAPICR(
				apiNames.APIName,
				cyborgv1beta1.CyborgAPISpec{
					CyborgAPITemplate: cyborgv1beta1.CyborgAPITemplate{
						Replicas: ptr.To(int32(1)),
					},
					ConfigSecret:   apiNames.ConfigSecretName.Name,
					ContainerImage: APITestImage,
					ServiceAccount: APITestSA,
					APITimeout:     ptr.To(60),
				},
			))
		})

		It("generates config with v3applicationcredential auth instead of password", func() {
			Eventually(func(g Gomega) {
				configSecret := th.GetSecret(apiNames.ConfigDataName)
				defaultConf := string(configSecret.Data["00-default.conf"])
				g.Expect(defaultConf).To(ContainSubstring("auth_type = v3applicationcredential"))
				g.Expect(defaultConf).To(ContainSubstring("application_credential_id = " + appCredID))
				g.Expect(defaultConf).To(ContainSubstring("application_credential_secret = " + appCredSecret))
				g.Expect(defaultConf).NotTo(ContainSubstring("auth_type = password"))
			}, timeout, interval).Should(Succeed())
		})
	})

	When("CyborgAPI is created with quorum queues enabled", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx,
				th.CreateSecret(apiNames.ConfigSecretName, map[string][]byte{
					"transport_url":     []byte("rabbit://user:pass@rabbitmq:5672/"),
					"database_account":  []byte("cyborg"),
					"database_username": []byte("cyborg_user"),
					"database_password": []byte("cyborg_pass"),
					"database_hostname": []byte("openstack.openstack.svc"),
					"ServiceUser":       []byte("cyborg"),
					"ServicePassword":   []byte("service-pass"),
					"KeystoneAuthURL":   []byte("https://keystone-internal.openstack.svc:5000"),
					"Region":            []byte("regionOne"),
					"quorumqueues":      []byte("true"),
				}),
			)

			DeferCleanup(th.DeleteInstance, CreateCyborgAPICR(
				apiNames.APIName,
				cyborgv1beta1.CyborgAPISpec{
					CyborgAPITemplate: cyborgv1beta1.CyborgAPITemplate{
						Replicas: ptr.To(int32(1)),
					},
					ConfigSecret:   apiNames.ConfigSecretName.Name,
					ContainerImage: APITestImage,
					ServiceAccount: APITestSA,
					APITimeout:     ptr.To(60),
				},
			))
		})

		It("generates config with quorum queue settings", func() {
			Eventually(func(g Gomega) {
				configSecret := th.GetSecret(apiNames.ConfigDataName)
				defaultConf := string(configSecret.Data["00-default.conf"])
				g.Expect(defaultConf).To(ContainSubstring("rabbit_quorum_queue=true"))
				g.Expect(defaultConf).To(ContainSubstring("rabbit_transient_quorum_queue=true"))
			}, timeout, interval).Should(Succeed())
		})
	})

	When("CyborgAPI is created with only a CA bundle (no API certs)", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx,
				CreateCyborgAPIConfigSecret(apiNames.ConfigSecretName),
			)

			DeferCleanup(k8sClient.Delete, ctx, th.CreateCABundleSecret(apiNames.CaBundleSecretName))

			DeferCleanup(th.DeleteInstance, CreateCyborgAPICR(
				apiNames.APIName,
				cyborgv1beta1.CyborgAPISpec{
					CyborgAPITemplate: cyborgv1beta1.CyborgAPITemplate{
						Replicas: ptr.To(int32(1)),
						TLS:      tls.API{Ca: tls.Ca{CaBundleSecretName: apiNames.CaBundleSecretName.Name}},
					},
					ConfigSecret:   apiNames.ConfigSecretName.Name,
					ContainerImage: APITestImage,
					ServiceAccount: APITestSA,
					APITimeout:     ptr.To(60),
				},
			))
		})

		It("sets TLSInputReady to True without API cert secrets", func() {
			th.ExpectCondition(
				apiNames.APIName,
				ConditionGetterFunc(CyborgAPIConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("generates my.cnf with TLS settings and cafile in default config", func() {
			Eventually(func(g Gomega) {
				configSecret := th.GetSecret(apiNames.ConfigDataName)
				g.Expect(string(configSecret.Data["my.cnf"])).To(ContainSubstring("ssl-ca="))
				g.Expect(string(configSecret.Data["00-default.conf"])).To(ContainSubstring(
					"cafile = " + tls.DownstreamTLSCABundlePath))
			}, timeout, interval).Should(Succeed())
		})
	})

	When("CyborgAPI is created with custom service config", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx,
				CreateCyborgAPIConfigSecret(apiNames.ConfigSecretName),
			)

			DeferCleanup(th.DeleteInstance, CreateCyborgAPICR(
				apiNames.APIName,
				cyborgv1beta1.CyborgAPISpec{
					CyborgAPITemplate: cyborgv1beta1.CyborgAPITemplate{
						Replicas:            ptr.To(int32(1)),
						CustomServiceConfig: "[DEFAULT]\nmy_api_key = my_api_value\n",
					},
					ConfigSecret:   apiNames.ConfigSecretName.Name,
					ContainerImage: APITestImage,
					ServiceAccount: APITestSA,
					APITimeout:     ptr.To(60),
				},
			))
		})

		It("includes the custom config in the generated config secret", func() {
			Eventually(func(g Gomega) {
				configSecret := th.GetSecret(apiNames.ConfigDataName)
				g.Expect(configSecret.Data).To(HaveKey("01-service-custom.conf"))
				customConf := string(configSecret.Data["01-service-custom.conf"])
				g.Expect(customConf).To(ContainSubstring("my_api_key = my_api_value"))
			}, timeout, interval).Should(Succeed())
		})
	})

	When("CyborgAPI is created with a NodeSelector", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx,
				CreateCyborgAPIConfigSecret(apiNames.ConfigSecretName),
			)

			DeferCleanup(th.DeleteInstance, CreateCyborgAPICR(
				apiNames.APIName,
				cyborgv1beta1.CyborgAPISpec{
					CyborgAPITemplate: cyborgv1beta1.CyborgAPITemplate{
						Replicas: ptr.To(int32(1)),
						NodeSelector: &map[string]string{
							"disktype": "ssd",
						},
					},
					ConfigSecret:   apiNames.ConfigSecretName.Name,
					ContainerImage: APITestImage,
					ServiceAccount: APITestSA,
					APITimeout:     ptr.To(60),
				},
			))
		})

		It("applies the nodeSelector to the StatefulSet pod template", func() {
			Eventually(func(g Gomega) {
				ss := th.GetStatefulSet(apiNames.StatefulSetName)
				g.Expect(ss.Spec.Template.Spec.NodeSelector).To(HaveKeyWithValue("disktype", "ssd"))
			}, timeout, interval).Should(Succeed())
		})
	})

	When("the config secret is updated", func() {
		It("updates the CONFIG_HASH in the StatefulSet", func() {
			DeferCleanup(
				k8sClient.Delete, ctx,
				CreateCyborgAPIConfigSecret(apiNames.ConfigSecretName),
			)

			DeferCleanup(th.DeleteInstance, CreateCyborgAPICR(
				apiNames.APIName,
				cyborgv1beta1.CyborgAPISpec{
					CyborgAPITemplate: cyborgv1beta1.CyborgAPITemplate{
						Replicas: ptr.To(int32(1)),
					},
					ConfigSecret:   apiNames.ConfigSecretName.Name,
					ContainerImage: APITestImage,
					ServiceAccount: APITestSA,
					APITimeout:     ptr.To(60),
				},
			))

			var originalHash string
			Eventually(func(g Gomega) {
				ss := th.GetStatefulSet(apiNames.StatefulSetName)
				for _, envVar := range ss.Spec.Template.Spec.Containers[1].Env {
					if envVar.Name == "CONFIG_HASH" {
						originalHash = envVar.Value
					}
				}
				g.Expect(originalHash).NotTo(BeEmpty())
			}, timeout, interval).Should(Succeed())

			// Update the secret with new data
			Eventually(func(g Gomega) {
				secret := &corev1.Secret{}
				g.Expect(k8sClient.Get(ctx, apiNames.ConfigSecretName, secret)).To(Succeed())
				secret.Data["ServicePassword"] = []byte("new-api-password")
				g.Expect(k8sClient.Update(ctx, secret)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				ss := th.GetStatefulSet(apiNames.StatefulSetName)
				for _, envVar := range ss.Spec.Template.Spec.Containers[1].Env {
					if envVar.Name == "CONFIG_HASH" {
						g.Expect(envVar.Value).NotTo(Equal(originalHash))
					}
				}
			}, timeout, interval).Should(Succeed())
		})
	})

	When("CyborgAPI is created with a missing CA bundle secret", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx,
				CreateCyborgAPIConfigSecret(apiNames.ConfigSecretName),
			)

			DeferCleanup(th.DeleteInstance, CreateCyborgAPICR(
				apiNames.APIName,
				cyborgv1beta1.CyborgAPISpec{
					CyborgAPITemplate: cyborgv1beta1.CyborgAPITemplate{
						Replicas: ptr.To(int32(1)),
						TLS: tls.API{
							Ca: tls.Ca{CaBundleSecretName: apiNames.CaBundleSecretName.Name},
						},
					},
					ConfigSecret:   apiNames.ConfigSecretName.Name,
					ContainerImage: APITestImage,
					ServiceAccount: APITestSA,
					APITimeout:     ptr.To(60),
				},
			))
		})

		It("sets TLSInputReady to False while waiting for the CA bundle secret", func() {
			th.ExpectConditionWithDetails(
				apiNames.APIName,
				ConditionGetterFunc(CyborgAPIConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"TLSInput is missing: "+apiNames.CaBundleSecretName.Name,
			)
		})
	})

	When("CyborgAPI is created with an invalid CA bundle secret", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx,
				CreateCyborgAPIConfigSecret(apiNames.ConfigSecretName),
			)

			// Secret exists but is missing the required tls-ca-bundle.pem key
			DeferCleanup(k8sClient.Delete, ctx, th.CreateSecret(
				apiNames.CaBundleSecretName,
				map[string][]byte{"wrong-key": []byte("some-data")},
			))

			DeferCleanup(th.DeleteInstance, CreateCyborgAPICR(
				apiNames.APIName,
				cyborgv1beta1.CyborgAPISpec{
					CyborgAPITemplate: cyborgv1beta1.CyborgAPITemplate{
						Replicas: ptr.To(int32(1)),
						TLS: tls.API{
							Ca: tls.Ca{CaBundleSecretName: apiNames.CaBundleSecretName.Name},
						},
					},
					ConfigSecret:   apiNames.ConfigSecretName.Name,
					ContainerImage: APITestImage,
					ServiceAccount: APITestSA,
					APITimeout:     ptr.To(60),
				},
			))
		})

		It("sets TLSInputReady to False with a missing key error", func() {
			th.ExpectConditionWithDetails(
				apiNames.APIName,
				ConditionGetterFunc(CyborgAPIConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"TLSInput error occured in TLS sources field not found in Secret: "+ //nolint:misspell // codespell:ignore occured
					"field tls-ca-bundle.pem not found in Secret "+
					apiNames.CaBundleSecretName.Namespace+"/"+apiNames.CaBundleSecretName.Name,
			)
		})
	})

	When("CyborgAPI is created with TLS certs but an invalid cert secret", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx,
				CreateCyborgAPIConfigSecret(apiNames.ConfigSecretName),
			)

			DeferCleanup(k8sClient.Delete, ctx, th.CreateCABundleSecret(apiNames.CaBundleSecretName))
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(apiNames.PublicCertSecretName))
			// Internal cert secret exists but is missing the required tls.key field
			DeferCleanup(k8sClient.Delete, ctx, th.CreateSecret(
				apiNames.InternalCertSecretName,
				map[string][]byte{"wrong-key": []byte("some-data")},
			))

			DeferCleanup(th.DeleteInstance, CreateCyborgAPICR(
				apiNames.APIName,
				cyborgv1beta1.CyborgAPISpec{
					CyborgAPITemplate: cyborgv1beta1.CyborgAPITemplate{
						Replicas: ptr.To(int32(1)),
						TLS: tls.API{
							Ca: tls.Ca{CaBundleSecretName: apiNames.CaBundleSecretName.Name},
							API: tls.APIService{
								Internal: tls.GenericService{SecretName: &apiNames.InternalCertSecretName.Name},
								Public:   tls.GenericService{SecretName: &apiNames.PublicCertSecretName.Name},
							},
						},
					},
					ConfigSecret:   apiNames.ConfigSecretName.Name,
					ContainerImage: APITestImage,
					ServiceAccount: APITestSA,
					APITimeout:     ptr.To(60),
				},
			))
		})

		It("sets TLSInputReady to False with a missing key error", func() {
			th.ExpectConditionWithDetails(
				apiNames.APIName,
				ConditionGetterFunc(CyborgAPIConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"TLSInput error occured in TLS sources field not found in Secret: "+ //nolint:misspell // codespell:ignore occured
					"field tls.key not found in Secret "+
					apiNames.InternalCertSecretName.Namespace+"/"+apiNames.InternalCertSecretName.Name,
			)
		})
	})

	When("CyborgAPI is created with valid TLS certs", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx,
				CreateCyborgAPIConfigSecret(apiNames.ConfigSecretName),
			)

			DeferCleanup(k8sClient.Delete, ctx, th.CreateCABundleSecret(apiNames.CaBundleSecretName))
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(apiNames.InternalCertSecretName))
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(apiNames.PublicCertSecretName))

			DeferCleanup(th.DeleteInstance, CreateCyborgAPICR(
				apiNames.APIName,
				cyborgv1beta1.CyborgAPISpec{
					CyborgAPITemplate: cyborgv1beta1.CyborgAPITemplate{
						Replicas: ptr.To(int32(1)),
						TLS: tls.API{
							Ca: tls.Ca{CaBundleSecretName: apiNames.CaBundleSecretName.Name},
							API: tls.APIService{
								Internal: tls.GenericService{SecretName: &apiNames.InternalCertSecretName.Name},
								Public:   tls.GenericService{SecretName: &apiNames.PublicCertSecretName.Name},
							},
						},
					},
					ConfigSecret:   apiNames.ConfigSecretName.Name,
					ContainerImage: APITestImage,
					ServiceAccount: APITestSA,
					APITimeout:     ptr.To(60),
				},
			))
		})

		It("sets TLSInputReady to True", func() {
			th.ExpectCondition(
				apiNames.APIName,
				ConditionGetterFunc(CyborgAPIConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("creates https endpoints in the KeystoneEndpoint", func() {
			th.SimulateStatefulSetReplicaReady(apiNames.StatefulSetName)
			keystone.SimulateKeystoneEndpointReady(apiNames.KeystoneEndpointName)

			ksEndpoint := keystone.GetKeystoneEndpoint(apiNames.KeystoneEndpointName)
			endpoints := ksEndpoint.Spec.Endpoints
			Expect(endpoints).To(HaveKeyWithValue("public",
				"https://cyborg-public."+apiNames.APIName.Namespace+".svc:6666/v2"))
			Expect(endpoints).To(HaveKeyWithValue("internal",
				"https://cyborg-internal."+apiNames.APIName.Namespace+".svc:6666/v2"))
		})
	})

	When("CyborgAPI CR is deleted", func() {
		It("removes the finalizer from the KeystoneEndpoint", func() {
			DeferCleanup(
				k8sClient.Delete, ctx,
				CreateCyborgAPIConfigSecret(apiNames.ConfigSecretName),
			)

			api := CreateCyborgAPICR(
				apiNames.APIName,
				cyborgv1beta1.CyborgAPISpec{
					CyborgAPITemplate: cyborgv1beta1.CyborgAPITemplate{
						Replicas: ptr.To(int32(1)),
					},
					ConfigSecret:   apiNames.ConfigSecretName.Name,
					ContainerImage: APITestImage,
					ServiceAccount: APITestSA,
					APITimeout:     ptr.To(60),
				},
			)

			// Drive to fully ready
			th.SimulateStatefulSetReplicaReady(apiNames.StatefulSetName)
			keystone.SimulateKeystoneEndpointReady(apiNames.KeystoneEndpointName)

			th.ExpectCondition(
				apiNames.APIName,
				ConditionGetterFunc(CyborgAPIConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)

			// The KeystoneEndpoint should carry our finalizer
			ksEndpoint := keystone.GetKeystoneEndpoint(apiNames.KeystoneEndpointName)
			Expect(ksEndpoint.Finalizers).To(ContainElement("openstack.org/cyborgapi"))

			// Delete the CyborgAPI and confirm the finalizer is removed
			th.DeleteInstance(api)

			Eventually(func(g Gomega) {
				ksEndpoint := keystone.GetKeystoneEndpoint(apiNames.KeystoneEndpointName)
				g.Expect(ksEndpoint.Finalizers).NotTo(ContainElement("openstack.org/cyborgapi"))
			}, timeout, interval).Should(Succeed())
		})
	})
})
