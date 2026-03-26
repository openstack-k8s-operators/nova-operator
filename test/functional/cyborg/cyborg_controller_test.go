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
	"errors"

	. "github.com/onsi/ginkgo/v2" //revive:disable:dot-imports
	. "github.com/onsi/gomega"    //revive:disable:dot-imports

	//revive:disable-next-line:dot-imports
	. "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"

	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	common_tls "github.com/openstack-k8s-operators/lib-common/modules/common/tls"
	mariadbv1 "github.com/openstack-k8s-operators/mariadb-operator/api/v1beta1"

	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	cyborgv1beta1 "github.com/openstack-k8s-operators/nova-operator/api/cyborg/v1beta1"
)

const (
	CyborgSecretName            = "osp-secret"
	CyborgContainerImage        = "test://cyborg-api"
	CyborgConductorImage        = "test://cyborg-conductor"
	CyborgAgentImage            = "test://cyborg-agent"
	CyborgPasswordSelectorValue = "CyborgPassword"
)

type CyborgNames struct {
	CyborgName          types.NamespacedName
	MariaDBServiceName  types.NamespacedName
	MariaDBDatabaseName types.NamespacedName
	MariaDBAccountName  types.NamespacedName
	TransportURLName    types.NamespacedName
	KeystoneServiceName types.NamespacedName
	DBSyncJobName       types.NamespacedName
	ConfigDataName      types.NamespacedName
	SubLevelSecretName  types.NamespacedName
	ServiceAccountName  types.NamespacedName
	RoleName            types.NamespacedName
	RoleBindingName     types.NamespacedName
}

func GetCyborgNames(cyborgName types.NamespacedName) CyborgNames {
	return CyborgNames{
		CyborgName: cyborgName,
		MariaDBServiceName: types.NamespacedName{
			Namespace: cyborgName.Namespace,
			Name:      "openstack",
		},
		MariaDBDatabaseName: types.NamespacedName{
			Namespace: cyborgName.Namespace,
			Name:      "cyborg",
		},
		MariaDBAccountName: types.NamespacedName{
			Namespace: cyborgName.Namespace,
			Name:      "cyborg",
		},
		TransportURLName: types.NamespacedName{
			Namespace: cyborgName.Namespace,
			Name:      cyborgName.Name + "-cyborg-transport",
		},
		KeystoneServiceName: types.NamespacedName{
			Namespace: cyborgName.Namespace,
			Name:      "cyborg",
		},
		DBSyncJobName: types.NamespacedName{
			Namespace: cyborgName.Namespace,
			Name:      cyborgName.Name + "-db-sync",
		},
		ConfigDataName: types.NamespacedName{
			Namespace: cyborgName.Namespace,
			Name:      cyborgName.Name + "-config-data",
		},
		SubLevelSecretName: types.NamespacedName{
			Namespace: cyborgName.Namespace,
			Name:      cyborgName.Name,
		},
		ServiceAccountName: types.NamespacedName{
			Namespace: cyborgName.Namespace,
			Name:      "cyborg-" + cyborgName.Name,
		},
		RoleName: types.NamespacedName{
			Namespace: cyborgName.Namespace,
			Name:      "cyborg-" + cyborgName.Name + "-role",
		},
		RoleBindingName: types.NamespacedName{
			Namespace: cyborgName.Namespace,
			Name:      "cyborg-" + cyborgName.Name + "-rolebinding",
		},
	}
}

var _ = Describe("Cyborg controller", func() {
	When("a Cyborg CR is created with minimal spec", func() {
		BeforeEach(func() {
			DeferCleanup(
				th.DeleteInstance,
				CreateCyborg(cyborgNames.CyborgName, GetDefaultCyborgSpec()),
			)
		})

		It("initializes status conditions", func() {
			Eventually(func(g Gomega) {
				cyborg := GetCyborg(cyborgNames.CyborgName)
				g.Expect(cyborg.Status.Conditions).NotTo(BeNil())
				g.Expect(cyborg.Status.Conditions.Has(condition.ReadyCondition)).To(BeTrue())
				g.Expect(cyborg.Status.Conditions.Has(condition.DBReadyCondition)).To(BeTrue())
				g.Expect(cyborg.Status.Conditions.Has(cyborgv1beta1.CyborgRabbitMQTransportURLReadyCondition)).To(BeTrue())
				g.Expect(cyborg.Status.Conditions.Has(condition.InputReadyCondition)).To(BeTrue())
				g.Expect(cyborg.Status.Conditions.Has(condition.ServiceConfigReadyCondition)).To(BeTrue())
				g.Expect(cyborg.Status.Conditions.Has(condition.DBSyncReadyCondition)).To(BeTrue())
				g.Expect(cyborg.Status.Conditions.Has(condition.KeystoneServiceReadyCondition)).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		})

		It("creates the RBAC resources", func() {
			th.ExpectCondition(
				cyborgNames.CyborgName,
				ConditionGetterFunc(CyborgConditionGetter),
				condition.ServiceAccountReadyCondition,
				corev1.ConditionTrue,
			)
			sa := th.GetServiceAccount(cyborgNames.ServiceAccountName)
			Expect(sa).NotTo(BeNil())

			th.ExpectCondition(
				cyborgNames.CyborgName,
				ConditionGetterFunc(CyborgConditionGetter),
				condition.RoleReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				cyborgNames.CyborgName,
				ConditionGetterFunc(CyborgConditionGetter),
				condition.RoleBindingReadyCondition,
				corev1.ConditionTrue,
			)
			role := th.GetRole(cyborgNames.RoleName)
			Expect(role.Rules).To(HaveLen(2))
			binding := th.GetRoleBinding(cyborgNames.RoleBindingName)
			Expect(binding.RoleRef.Name).To(Equal(role.Name))
			Expect(binding.Subjects).To(HaveLen(1))
			Expect(binding.Subjects[0].Name).To(Equal(sa.Name))
		})

		It("sets the ObservedGeneration on the status", func() {
			Eventually(func(g Gomega) {
				cyborg := GetCyborg(cyborgNames.CyborgName)
				g.Expect(cyborg.Status.ObservedGeneration).To(Equal(cyborg.Generation))
			}, timeout, interval).Should(Succeed())
		})
	})

	When("a Cyborg CR is created with all required prerequisites", func() {
		BeforeEach(func() {
			serviceSpec := corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 3306}}}
			DeferCleanup(k8sClient.Delete, ctx, CreateCyborgSecret(cyborgNames.CyborgName.Namespace))
			DeferCleanup(k8sClient.Delete, ctx, CreateCyborgMessageBusSecret(cyborgNames))
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					cyborgNames.MariaDBServiceName.Namespace,
					cyborgNames.MariaDBServiceName.Name,
					serviceSpec,
				),
			)

			account, secret := mariadb.CreateMariaDBAccountAndSecret(
				cyborgNames.MariaDBAccountName, mariadbv1.MariaDBAccountSpec{})
			DeferCleanup(k8sClient.Delete, ctx, account)
			DeferCleanup(k8sClient.Delete, ctx, secret)

			DeferCleanup(
				th.DeleteInstance,
				CreateCyborg(cyborgNames.CyborgName, GetDefaultCyborgSpec()),
			)
		})

		It("creates a MariaDBDatabase and MariaDBAccount", func() {
			mariadb.SimulateMariaDBAccountCompleted(cyborgNames.MariaDBAccountName)
			mariadb.SimulateMariaDBDatabaseCompleted(cyborgNames.MariaDBDatabaseName)

			th.ExpectCondition(
				cyborgNames.CyborgName,
				ConditionGetterFunc(CyborgConditionGetter),
				condition.DBReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("creates a RabbitMQ TransportURL", func() {
			mariadb.SimulateMariaDBAccountCompleted(cyborgNames.MariaDBAccountName)
			mariadb.SimulateMariaDBDatabaseCompleted(cyborgNames.MariaDBDatabaseName)
			infra.SimulateTransportURLReady(cyborgNames.TransportURLName)

			th.ExpectCondition(
				cyborgNames.CyborgName,
				ConditionGetterFunc(CyborgConditionGetter),
				cyborgv1beta1.CyborgRabbitMQTransportURLReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("creates a sub-level secret with the required data", func() {
			mariadb.SimulateMariaDBAccountCompleted(cyborgNames.MariaDBAccountName)
			mariadb.SimulateMariaDBDatabaseCompleted(cyborgNames.MariaDBDatabaseName)
			infra.SimulateTransportURLReady(cyborgNames.TransportURLName)

			Eventually(func(g Gomega) {
				secret := th.GetSecret(cyborgNames.SubLevelSecretName)
				g.Expect(secret.Data).To(HaveKey(CyborgPasswordSelectorValue))
				g.Expect(secret.Data).To(HaveKey("transport_url"))
				g.Expect(secret.Data).To(HaveKey("database_account"))
				g.Expect(secret.Data).To(HaveKey("database_username"))
				g.Expect(secret.Data).To(HaveKey("database_password"))
				g.Expect(secret.Data).To(HaveKey("database_hostname"))
			}, timeout, interval).Should(Succeed())
		})

		It("creates a config data secret for dbsync", func() {
			mariadb.SimulateMariaDBAccountCompleted(cyborgNames.MariaDBAccountName)
			mariadb.SimulateMariaDBDatabaseCompleted(cyborgNames.MariaDBDatabaseName)
			infra.SimulateTransportURLReady(cyborgNames.TransportURLName)
			keystone.SimulateKeystoneServiceReady(cyborgNames.KeystoneServiceName)

			Eventually(func(g Gomega) {
				configSecret := th.GetSecret(cyborgNames.ConfigDataName)
				g.Expect(configSecret.Data).To(HaveKey("00-default.conf"))
				g.Expect(configSecret.Data).To(HaveKey("my.cnf"))

				defaultConf := string(configSecret.Data["00-default.conf"])
				g.Expect(defaultConf).To(ContainSubstring("[database]"))
				g.Expect(defaultConf).To(ContainSubstring("connection = mysql+pymysql://"))
			}, timeout, interval).Should(Succeed())
		})

		It("reaches Ready when all dependencies are resolved", func() {
			mariadb.SimulateMariaDBAccountCompleted(cyborgNames.MariaDBAccountName)
			mariadb.SimulateMariaDBDatabaseCompleted(cyborgNames.MariaDBDatabaseName)

			th.ExpectCondition(
				cyborgNames.CyborgName,
				ConditionGetterFunc(CyborgConditionGetter),
				condition.DBReadyCondition,
				corev1.ConditionTrue,
			)

			infra.SimulateTransportURLReady(cyborgNames.TransportURLName)

			th.ExpectCondition(
				cyborgNames.CyborgName,
				ConditionGetterFunc(CyborgConditionGetter),
				cyborgv1beta1.CyborgRabbitMQTransportURLReadyCondition,
				corev1.ConditionTrue,
			)

			keystone.SimulateKeystoneServiceReady(cyborgNames.KeystoneServiceName)

			th.ExpectCondition(
				cyborgNames.CyborgName,
				ConditionGetterFunc(CyborgConditionGetter),
				condition.KeystoneServiceReadyCondition,
				corev1.ConditionTrue,
			)

			th.ExpectCondition(
				cyborgNames.CyborgName,
				ConditionGetterFunc(CyborgConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionFalse,
			)

			th.SimulateJobSuccess(cyborgNames.DBSyncJobName)

			th.ExpectCondition(
				cyborgNames.CyborgName,
				ConditionGetterFunc(CyborgConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})

	When("Cyborg CR is created with TLS and ApplicationCredentials", func() {
		const (
			apiTLSSecretName   = "cyborg-api-tls"             //nolint:gosec
			caBundleSecretName = "cyborg-test-ca-bundle"      //nolint:gosec
			appCredSecretName  = "cyborg-app-cred-secret"     //nolint:gosec
			appCredID          = "test-cyborg-appcred-id"     //nolint:gosec
			appCredSecretValue = "test-cyborg-appcred-secret" //nolint:gosec
		)

		BeforeEach(func() {
			serviceSpec := corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 3306}}}
			DeferCleanup(k8sClient.Delete, ctx, CreateCyborgSecret(cyborgNames.CyborgName.Namespace))
			DeferCleanup(k8sClient.Delete, ctx, CreateCyborgMessageBusSecret(cyborgNames))
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					cyborgNames.MariaDBServiceName.Namespace,
					cyborgNames.MariaDBServiceName.Name,
					serviceSpec,
				),
			)

			account, dbSecret := mariadb.CreateMariaDBAccountAndSecret(
				cyborgNames.MariaDBAccountName, mariadbv1.MariaDBAccountSpec{})
			DeferCleanup(k8sClient.Delete, ctx, account)
			DeferCleanup(k8sClient.Delete, ctx, dbSecret)

			apiTLSSecret := th.CreateSecret(
				types.NamespacedName{Namespace: cyborgNames.CyborgName.Namespace, Name: apiTLSSecretName},
				map[string][]byte{
					common_tls.CertKey:    []byte("dummy-tls-cert"),
					common_tls.PrivateKey: []byte("dummy-tls-key"),
				},
			)
			DeferCleanup(k8sClient.Delete, ctx, apiTLSSecret)

			caBundleSecret := th.CreateSecret(
				types.NamespacedName{Namespace: cyborgNames.CyborgName.Namespace, Name: caBundleSecretName},
				map[string][]byte{
					common_tls.CABundleKey: []byte("dummy-ca-bundle"),
				},
			)
			DeferCleanup(k8sClient.Delete, ctx, caBundleSecret)

			appCredSecret := th.CreateSecret(
				types.NamespacedName{Namespace: cyborgNames.CyborgName.Namespace, Name: appCredSecretName},
				map[string][]byte{
					keystonev1.ACIDSecretKey:     []byte(appCredID),
					keystonev1.ACSecretSecretKey: []byte(appCredSecretValue),
				},
			)
			DeferCleanup(k8sClient.Delete, ctx, appCredSecret)

			DeferCleanup(
				th.DeleteInstance,
				CreateCyborg(
					cyborgNames.CyborgName,
					GetCyborgSpecWithTLSAndAppCred(apiTLSSecretName, caBundleSecretName, appCredSecretName),
				),
			)
		})

		It("creates dbsync job, TLS-aware config secret, and application credential data in the sub-level secret", func() {
			mariadb.SimulateMariaDBAccountCompleted(cyborgNames.MariaDBAccountName)
			mariadb.SimulateMariaDBTLSDatabaseCompleted(cyborgNames.MariaDBDatabaseName)
			infra.SimulateTransportURLReady(cyborgNames.TransportURLName)
			keystone.SimulateKeystoneServiceReady(cyborgNames.KeystoneServiceName)

			Eventually(func(_ Gomega) {
				_ = th.GetJob(cyborgNames.DBSyncJobName)
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				configSecret := th.GetSecret(cyborgNames.ConfigDataName)
				myCnf := string(configSecret.Data["my.cnf"])
				g.Expect(myCnf).To(ContainSubstring("ssl-ca="))
				g.Expect(myCnf).To(ContainSubstring("ssl=1"))
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				subSecret := th.GetSecret(cyborgNames.SubLevelSecretName)
				g.Expect(subSecret.Data).To(HaveKey("ACID"))
				g.Expect(subSecret.Data).To(HaveKey("ACSecret"))
				g.Expect(string(subSecret.Data["ACID"])).To(Equal(appCredID))
				g.Expect(string(subSecret.Data["ACSecret"])).To(Equal(appCredSecretValue))
			}, timeout, interval).Should(Succeed())
		})
	})

	When("Cyborg CR is deleted", func() {
		It("cleans up finalizers", func() {
			serviceSpec := corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 3306}}}
			DeferCleanup(k8sClient.Delete, ctx, CreateCyborgSecret(cyborgNames.CyborgName.Namespace))
			DeferCleanup(k8sClient.Delete, ctx, CreateCyborgMessageBusSecret(cyborgNames))
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					cyborgNames.MariaDBServiceName.Namespace,
					cyborgNames.MariaDBServiceName.Name,
					serviceSpec,
				),
			)

			account, secret := mariadb.CreateMariaDBAccountAndSecret(
				cyborgNames.MariaDBAccountName, mariadbv1.MariaDBAccountSpec{})
			DeferCleanup(k8sClient.Delete, ctx, account)
			DeferCleanup(k8sClient.Delete, ctx, secret)

			cyborg := CreateCyborg(cyborgNames.CyborgName, GetDefaultCyborgSpec())

			mariadb.SimulateMariaDBAccountCompleted(cyborgNames.MariaDBAccountName)
			mariadb.SimulateMariaDBDatabaseCompleted(cyborgNames.MariaDBDatabaseName)
			infra.SimulateTransportURLReady(cyborgNames.TransportURLName)
			keystone.SimulateKeystoneServiceReady(cyborgNames.KeystoneServiceName)
			th.SimulateJobSuccess(cyborgNames.DBSyncJobName)

			th.ExpectCondition(
				cyborgNames.CyborgName,
				ConditionGetterFunc(CyborgConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionTrue,
			)

			th.DeleteInstance(cyborg)

			Eventually(func(g Gomega) {
				instance := &cyborgv1beta1.Cyborg{}
				err := k8sClient.Get(ctx, cyborgNames.CyborgName, instance)
				g.Expect(err).To(HaveOccurred())
			}, timeout, interval).Should(Succeed())
		})
	})
})

var _ = Describe("Cyborg defaults", func() {
	It("sets all expected kubebuilder and webhook defaults", func() {
		DeferCleanup(
			th.DeleteInstance,
			CreateCyborg(cyborgNames.CyborgName, GetDefaultCyborgSpec()),
		)

		cyborg := GetCyborg(cyborgNames.CyborgName)

		// kubebuilder defaults for CyborgSpecCore pointer fields
		Expect(cyborg.Spec.KeystoneInstance).NotTo(BeNil())
		Expect(*cyborg.Spec.KeystoneInstance).To(Equal("keystone"))
		Expect(cyborg.Spec.DatabaseInstance).NotTo(BeNil())
		Expect(*cyborg.Spec.DatabaseInstance).To(Equal("openstack"))
		Expect(cyborg.Spec.ServiceUser).NotTo(BeNil())
		Expect(*cyborg.Spec.ServiceUser).To(Equal("cyborg"))
		Expect(cyborg.Spec.PasswordSelectors).NotTo(BeNil())
		Expect(cyborg.Spec.PasswordSelectors.Service).To(Equal("CyborgPassword"))
		Expect(cyborg.Spec.DatabaseAccount).NotTo(BeNil())
		Expect(*cyborg.Spec.DatabaseAccount).To(Equal("cyborg"))
		Expect(cyborg.Spec.APITimeout).NotTo(BeNil())
		Expect(*cyborg.Spec.APITimeout).To(Equal(60))
		Expect(cyborg.Spec.Secret).NotTo(BeNil())
		Expect(*cyborg.Spec.Secret).To(Equal("osp-secret"))

		// kubebuilder defaults for non-pointer fields
		Expect(cyborg.Spec.PreserveJobs).To(BeFalse())

		// kubebuilder defaults for sub-template replicas
		Expect(cyborg.Spec.APIServiceTemplate.Replicas).NotTo(BeNil())
		Expect(*cyborg.Spec.APIServiceTemplate.Replicas).To(Equal(int32(1)))
		Expect(cyborg.Spec.ConductorServiceTemplate.Replicas).NotTo(BeNil())
		Expect(*cyborg.Spec.ConductorServiceTemplate.Replicas).To(Equal(int32(1)))

		// webhook default for messagingBus.Cluster
		Expect(cyborg.Spec.MessagingBus.Cluster).To(Equal("rabbitmq"))
	})
})

var _ = Describe("Cyborg webhook validation", func() {
	It("rejects Cyborg with wrong service override endpoint type in apiServiceTemplate", func() {
		spec := GetDefaultCyborgSpec()
		spec["apiServiceTemplate"] = map[string]any{
			"override": map[string]any{
				"service": map[string]any{
					"internal": map[string]any{},
					"wrooong":  map[string]any{},
				},
			},
		}
		raw := map[string]any{
			"apiVersion": "cyborg.openstack.org/v1beta1",
			"kind":       "Cyborg",
			"metadata": map[string]any{
				"name":      cyborgNames.CyborgName.Name,
				"namespace": cyborgNames.CyborgName.Namespace,
			},
			"spec": spec,
		}

		unstructuredObj := &unstructured.Unstructured{Object: raw}
		_, err := controllerutil.CreateOrPatch(
			ctx, k8sClient, unstructuredObj, func() error { return nil })

		Expect(err).To(HaveOccurred())
		var statusError *k8s_errors.StatusError
		Expect(errors.As(err, &statusError)).To(BeTrue())
		Expect(statusError.ErrStatus.Details.Kind).To(Equal("Cyborg"))
		Expect(statusError.ErrStatus.Message).To(
			ContainSubstring(
				"invalid: spec.apiServiceTemplate.override.service[wrooong]: " +
					"Invalid value: \"wrooong\": invalid endpoint type: wrooong",
			),
		)
	})

	It("accepts Cyborg with a correct spec", func() {
		raw := map[string]any{
			"apiVersion": "cyborg.openstack.org/v1beta1",
			"kind":       "Cyborg",
			"metadata": map[string]any{
				"name":      cyborgNames.CyborgName.Name,
				"namespace": cyborgNames.CyborgName.Namespace,
			},
			"spec": GetDefaultCyborgSpec(),
		}

		unstructuredObj := &unstructured.Unstructured{Object: raw}
		_, err := controllerutil.CreateOrPatch(
			ctx, k8sClient, unstructuredObj, func() error { return nil })

		Expect(err).Should(Succeed())
		DeferCleanup(th.DeleteInstance, unstructuredObj)
	})
})
