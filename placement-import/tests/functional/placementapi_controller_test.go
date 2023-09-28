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
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
)

var _ = Describe("PlacementAPI controller", func() {
	BeforeEach(func() {
		// lib-common uses OPERATOR_TEMPLATES env var to locate the "templates"
		// directory of the operator. We need to set them othervise lib-common
		// will fail to generate the ConfigMap as it does not find common.sh
		err := os.Setenv("OPERATOR_TEMPLATES", "../../templates")
		Expect(err).NotTo(HaveOccurred())
	})

	When("A PlacementAPI instance is created", func() {
		BeforeEach(func() {
			DeferCleanup(
				th.DeleteInstance,
				CreatePlacementAPI(names.PlacementAPIName, GetDefaultPlacementAPISpec()),
			)
		})

		It("should have the Spec fields defaulted", func() {
			Placement := GetPlacementAPI(names.PlacementAPIName)
			Expect(Placement.Spec.DatabaseInstance).Should(Equal("openstack"))
			Expect(Placement.Spec.DatabaseUser).Should(Equal("placement"))
			Expect(Placement.Spec.ServiceUser).Should(Equal("placement"))
			Expect(*(Placement.Spec.Replicas)).Should(Equal(int32(1)))
		})

		It("should have the Status fields initialized", func() {
			Placement := GetPlacementAPI(names.PlacementAPIName)
			Expect(Placement.Status.Hash).To(BeEmpty())
			Expect(Placement.Status.DatabaseHostname).To(Equal(""))
			Expect(Placement.Status.ReadyCount).To(Equal(int32(0)))
		})

		It("should have a finalizer", func() {
			// the reconciler loop adds the finalizer so we have to wait for
			// it to run
			Eventually(func() []string {
				return GetPlacementAPI(names.PlacementAPIName).Finalizers
			}, timeout, interval).Should(ContainElement("PlacementAPI"))
		})

		It("should not create a config map", func() {
			th.AssertConfigMapDoesNotExist(names.ConfigMapName)
		})

		It("should have input not ready and unknown Conditions initialized", func() {
			th.ExpectCondition(
				names.PlacementAPIName,
				ConditionGetterFunc(PlacementConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
			th.ExpectCondition(
				names.PlacementAPIName,
				ConditionGetterFunc(PlacementConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionFalse,
			)
			unknownConditions := []condition.Type{
				condition.DBReadyCondition,
				condition.DBSyncReadyCondition,
				condition.ExposeServiceReadyCondition,
				condition.ServiceConfigReadyCondition,
				condition.DeploymentReadyCondition,
				condition.KeystoneServiceReadyCondition,
				condition.KeystoneEndpointReadyCondition,
				condition.NetworkAttachmentsReadyCondition,
				condition.ServiceAccountReadyCondition,
				condition.RoleReadyCondition,
				condition.RoleBindingReadyCondition,
			}

			placement := GetPlacementAPI(names.PlacementAPIName)
			// +2 as InputReady and Ready is False asserted above
			Expect(placement.Status.Conditions).To(HaveLen(len(unknownConditions) + 2))

			for _, cond := range unknownConditions {
				th.ExpectCondition(
					names.PlacementAPIName,
					ConditionGetterFunc(PlacementConditionGetter),
					cond,
					corev1.ConditionUnknown,
				)
			}
		})
	})

	When("a secret is provided with missing fields", func() {
		BeforeEach(func() {
			DeferCleanup(
				th.DeleteInstance,
				CreatePlacementAPI(names.PlacementAPIName, GetDefaultPlacementAPISpec()),
			)
			DeferCleanup(
				k8sClient.Delete, ctx,
				th.CreateSecret(
					types.NamespacedName{Namespace: namespace, Name: SecretName},
					map[string][]byte{}),
			)
		})
		It("reports that input is not ready", func() {
			// FIXME(gibi): This is a bug as placement controller does not
			// check the content of the Secret so eventually a dbsync job is
			// created with incorrect config
			th.ExpectCondition(
				names.PlacementAPIName,
				ConditionGetterFunc(PlacementConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})

	When("the proper secret is provided", func() {
		BeforeEach(func() {
			DeferCleanup(
				th.DeleteInstance,
				CreatePlacementAPI(names.PlacementAPIName, GetDefaultPlacementAPISpec()),
			)
			DeferCleanup(
				k8sClient.Delete, ctx, CreatePlacementAPISecret(namespace, SecretName))
		})

		It("should have input ready", func() {
			th.ExpectCondition(
				names.PlacementAPIName,
				ConditionGetterFunc(PlacementConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				names.PlacementAPIName,
				ConditionGetterFunc(PlacementConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("should not create a config map", func() {
			th.AssertConfigMapDoesNotExist(names.ConfigMapName)
		})
	})

	When("keystoneAPI instance is available", func() {
		var keystoneAPI *keystonev1.KeystoneAPI

		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreatePlacementAPI(names.PlacementAPIName, GetDefaultPlacementAPISpec()))
			DeferCleanup(
				k8sClient.Delete, ctx, CreatePlacementAPISecret(namespace, SecretName))
			keystoneAPIName := keystone.CreateKeystoneAPI(namespace)
			keystoneAPI = keystone.GetKeystoneAPI(keystoneAPIName)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystoneAPIName)
		})

		It("should have config ready", func() {
			th.ExpectCondition(
				names.PlacementAPIName,
				ConditionGetterFunc(PlacementConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("should create a ConfigMap for placement.conf", func() {
			cm := th.GetConfigMap(names.ConfigMapName)

			Expect(cm.Data["placement.conf"]).Should(
				ContainSubstring("auth_url = %s", keystoneAPI.Status.APIEndpoints["internal"]))
			Expect(cm.Data["placement.conf"]).Should(
				ContainSubstring("www_authenticate_uri = %s", keystoneAPI.Status.APIEndpoints["public"]))
			Expect(cm.Data["placement.conf"]).Should(
				ContainSubstring("username = placement"))
		})

		It("creates service account, role and rolebindig", func() {
			th.ExpectCondition(
				names.PlacementAPIName,
				ConditionGetterFunc(PlacementConditionGetter),
				condition.ServiceAccountReadyCondition,
				corev1.ConditionTrue,
			)
			sa := th.GetServiceAccount(names.ServiceAccountName)

			th.ExpectCondition(
				names.PlacementAPIName,
				ConditionGetterFunc(PlacementConditionGetter),
				condition.RoleReadyCondition,
				corev1.ConditionTrue,
			)
			role := th.GetRole(names.RoleName)
			Expect(role.Rules).To(HaveLen(2))
			Expect(role.Rules[0].Resources).To(Equal([]string{"securitycontextconstraints"}))
			Expect(role.Rules[1].Resources).To(Equal([]string{"pods"}))

			th.ExpectCondition(
				names.PlacementAPIName,
				ConditionGetterFunc(PlacementConditionGetter),
				condition.RoleBindingReadyCondition,
				corev1.ConditionTrue,
			)
			binding := th.GetRoleBinding(names.RoleBindingName)
			Expect(binding.RoleRef.Name).To(Equal(role.Name))
			Expect(binding.Subjects).To(HaveLen(1))
			Expect(binding.Subjects[0].Name).To(Equal(sa.Name))
		})

		It("creates MariaDB database", func() {
			th.ExpectCondition(
				names.PlacementAPIName,
				ConditionGetterFunc(PlacementConditionGetter),
				condition.DBReadyCondition,
				corev1.ConditionFalse,
			)

			serviceSpec := corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 3306}}}
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(namespace, "openstack", serviceSpec),
			)
			db := mariadb.GetMariaDBDatabase(names.MariaDBDatabaseName)
			// FIXME(gibi): this should be hardcoded to "placement" as this is
			// the name of the DB schema to be created
			Expect(db.Spec.Name).To(Equal(names.PlacementAPIName.Name))
			Expect(db.Spec.Secret).To(Equal(SecretName))

			mariadb.SimulateMariaDBDatabaseCompleted(names.MariaDBDatabaseName)

			th.ExpectCondition(
				names.PlacementAPIName,
				ConditionGetterFunc(PlacementConditionGetter),
				condition.DBReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("creates keystone service", func() {
			th.ExpectCondition(
				names.PlacementAPIName,
				ConditionGetterFunc(PlacementConditionGetter),
				condition.KeystoneServiceReadyCondition,
				corev1.ConditionUnknown,
			)

			serviceSpec := corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 3306}}}
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(namespace, "openstack", serviceSpec),
			)
			mariadb.SimulateMariaDBDatabaseCompleted(names.MariaDBDatabaseName)

			keystone.SimulateKeystoneServiceReady(names.KeystoneServiceName)

			th.ExpectCondition(
				names.PlacementAPIName,
				ConditionGetterFunc(PlacementConditionGetter),
				condition.KeystoneServiceReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("creates keystone endpoint", func() {
			th.ExpectCondition(
				names.PlacementAPIName,
				ConditionGetterFunc(PlacementConditionGetter),
				condition.KeystoneEndpointReadyCondition,
				corev1.ConditionUnknown,
			)

			serviceSpec := corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 3306}}}
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(namespace, "openstack", serviceSpec),
			)
			mariadb.SimulateMariaDBDatabaseCompleted(names.MariaDBDatabaseName)

			keystone.SimulateKeystoneEndpointReady(names.KeystoneEndpointName)

			th.ExpectCondition(
				names.PlacementAPIName,
				ConditionGetterFunc(PlacementConditionGetter),
				condition.KeystoneEndpointReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("runs db sync", func() {
			serviceSpec := corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 3306}}}
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(namespace, "openstack", serviceSpec),
			)
			mariadb.SimulateMariaDBDatabaseCompleted(names.MariaDBDatabaseName)

			th.ExpectCondition(
				names.PlacementAPIName,
				ConditionGetterFunc(PlacementConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionFalse,
			)

			job := th.GetJob(names.DBSyncJobName)
			Expect(job.Spec.Template.Spec.Volumes).To(HaveLen(3))
			Expect(job.Spec.Template.Spec.InitContainers).To(HaveLen(1))
			Expect(job.Spec.Template.Spec.Containers).To(HaveLen(1))

			init := job.Spec.Template.Spec.InitContainers[0]
			Expect(init.VolumeMounts).To(HaveLen(3))
			Expect(init.Args[1]).To(ContainSubstring("init.sh"))
			Expect(init.Image).To(Equal("quay.io/podified-antelope-centos9/openstack-placement-api:current-podified"))
			env := &corev1.EnvVar{}
			Expect(init.Env).To(ContainElement(HaveField("Name", "DatabaseHost"), env))
			Expect(env.Value).To(Equal("hostname-for-openstack"))
			Expect(init.Env).To(ContainElement(HaveField("Name", "DatabaseUser"), env))
			Expect(env.Value).To(Equal("placement"))
			Expect(init.Env).To(ContainElement(HaveField("Name", "DatabaseName"), env))
			Expect(env.Value).To(Equal("placement"))
			Expect(init.Env).To(ContainElement(HaveField("Name", "DatabasePassword"), env))
			Expect(env.ValueFrom.SecretKeyRef.LocalObjectReference.Name).To(Equal(SecretName))
			Expect(env.ValueFrom.SecretKeyRef.Key).To(Equal("PlacementDatabasePassword"))
			Expect(init.Env).To(ContainElement(HaveField("Name", "PlacementPassword"), env))
			Expect(env.ValueFrom.SecretKeyRef.LocalObjectReference.Name).To(Equal(SecretName))
			Expect(env.ValueFrom.SecretKeyRef.Key).To(Equal("PlacementPassword"))

			container := job.Spec.Template.Spec.Containers[0]
			Expect(container.VolumeMounts).To(HaveLen(3))
			Expect(container.Args[1]).To(ContainSubstring("placement-manage db sync"))
			Expect(container.Image).To(Equal("quay.io/podified-antelope-centos9/openstack-placement-api:current-podified"))

			th.SimulateJobSuccess(names.DBSyncJobName)

			th.ExpectCondition(
				names.PlacementAPIName,
				ConditionGetterFunc(PlacementConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("creates deployment", func() {
			serviceSpec := corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 3306}}}
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(namespace, "openstack", serviceSpec),
			)
			mariadb.SimulateMariaDBDatabaseCompleted(names.MariaDBDatabaseName)
			th.SimulateJobSuccess(names.DBSyncJobName)

			th.ExpectCondition(
				names.PlacementAPIName,
				ConditionGetterFunc(PlacementConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionUnknown,
			)

			deployment := th.GetDeployment(names.DeploymentName)
			Expect(int(*deployment.Spec.Replicas)).To(Equal(1))
			Expect(deployment.Spec.Selector.MatchLabels).To(Equal(map[string]string{"service": "placement"}))
			Expect(deployment.Spec.Template.Spec.ServiceAccountName).To(Equal(names.ServiceAccountName.Name))
			th.SimulateDeploymentReplicaReady(names.DeploymentName)

			th.ExpectCondition(
				names.PlacementAPIName,
				ConditionGetterFunc(PlacementConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("exposes the service", func() {
			th.ExpectCondition(
				names.PlacementAPIName,
				ConditionGetterFunc(PlacementConditionGetter),
				condition.ExposeServiceReadyCondition,
				corev1.ConditionUnknown,
			)

			serviceSpec := corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 3306}}}
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(namespace, "openstack", serviceSpec),
			)
			mariadb.SimulateMariaDBDatabaseCompleted(names.MariaDBDatabaseName)
			th.SimulateJobSuccess(names.DBSyncJobName)
			th.SimulateDeploymentReplicaReady(names.DeploymentName)

			public := th.GetService(names.PublicServiceName)
			Expect(public.Labels["service"]).To(Equal("placement"))
			internal := th.GetService(names.InternalServiceName)
			Expect(internal.Labels["service"]).To(Equal("placement"))

			th.ExpectCondition(
				names.PlacementAPIName,
				ConditionGetterFunc(PlacementConditionGetter),
				condition.ExposeServiceReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("reports ready when successfully deployed", func() {
			th.ExpectCondition(
				names.PlacementAPIName,
				ConditionGetterFunc(PlacementConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)

			serviceSpec := corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 3306}}}
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(namespace, "openstack", serviceSpec),
			)
			mariadb.SimulateMariaDBDatabaseCompleted(names.MariaDBDatabaseName)
			keystone.SimulateKeystoneServiceReady(names.KeystoneServiceName)
			keystone.SimulateKeystoneEndpointReady(names.KeystoneEndpointName)
			th.SimulateJobSuccess(names.DBSyncJobName)
			th.SimulateDeploymentReplicaReady(names.DeploymentName)

			th.ExpectCondition(
				names.PlacementAPIName,
				ConditionGetterFunc(PlacementConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})

	When("A PlacementAPI is created with service override", func() {
		BeforeEach(func() {
			DeferCleanup(k8sClient.Delete, ctx, CreatePlacementAPISecret(namespace, SecretName))
			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(namespace))

			spec := GetDefaultPlacementAPISpec()
			serviceOverride := map[string]interface{}{}
			serviceOverride["internal"] = map[string]interface{}{
				"metadata": map[string]map[string]string{
					"annotations": {
						"dnsmasq.network.openstack.org/hostname": "placement-internal.openstack.svc",
						"metallb.universe.tf/address-pool":       "osp-internalapi",
						"metallb.universe.tf/allow-shared-ip":    "osp-internalapi",
						"metallb.universe.tf/loadBalancerIPs":    "internal-lb-ip-1,internal-lb-ip-2",
					},
					"labels": {
						"internal": "true",
						"service":  "placement",
					},
				},
				"spec": map[string]interface{}{
					"type": "LoadBalancer",
				},
			}

			spec["override"] = map[string]interface{}{
				"service": serviceOverride,
			}

			placementAPI := CreatePlacementAPI(names.PlacementAPIName, spec)
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					namespace,
					GetPlacementAPI(names.PlacementAPIName).Spec.DatabaseInstance,
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)

			mariadb.SimulateMariaDBDatabaseCompleted(names.MariaDBDatabaseName)
			th.SimulateJobSuccess(names.DBSyncJobName)
			th.SimulateDeploymentReplicaReady(names.DeploymentName)
			keystone.SimulateKeystoneServiceReady(names.KeystoneServiceName)
			keystone.SimulateKeystoneEndpointReady(names.KeystoneEndpointName)
			DeferCleanup(th.DeleteInstance, placementAPI)
		})

		It("creates KeystoneEndpoint", func() {
			keystoneEndpoint := keystone.GetKeystoneEndpoint(names.KeystoneEndpointName)
			endpoints := keystoneEndpoint.Spec.Endpoints
			Expect(endpoints).To(HaveKeyWithValue("public", "http://placement-public."+namespace+".svc:8778"))
			Expect(endpoints).To(HaveKeyWithValue("internal", "http://placement-internal."+namespace+".svc:8778"))

			th.ExpectCondition(
				names.PlacementAPIName,
				ConditionGetterFunc(PlacementConditionGetter),
				condition.KeystoneEndpointReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("creates LoadBalancer service", func() {
			// As the internal endpoint is configured in ExternalEndpoints it
			// gets a LoadBalancer Service with MetalLB annotations
			service := th.GetService(types.NamespacedName{Namespace: namespace, Name: "placement-internal"})
			Expect(service.Annotations).To(
				HaveKeyWithValue("dnsmasq.network.openstack.org/hostname", "placement-internal.openstack.svc"))
			Expect(service.Annotations).To(
				HaveKeyWithValue("metallb.universe.tf/address-pool", "osp-internalapi"))
			Expect(service.Annotations).To(
				HaveKeyWithValue("metallb.universe.tf/allow-shared-ip", "osp-internalapi"))
			Expect(service.Annotations).To(
				HaveKeyWithValue("metallb.universe.tf/loadBalancerIPs", "internal-lb-ip-1,internal-lb-ip-2"))

			th.ExpectCondition(
				names.PlacementAPIName,
				ConditionGetterFunc(PlacementConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})

	When("A PlacementAPI is created with service override endpointURL set", func() {
		BeforeEach(func() {
			DeferCleanup(k8sClient.Delete, ctx, CreatePlacementAPISecret(namespace, SecretName))
			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(namespace))

			spec := GetDefaultPlacementAPISpec()
			serviceOverride := map[string]interface{}{}
			serviceOverride["public"] = map[string]interface{}{
				"endpointURL": "http://placement-openstack.apps-crc.testing",
			}

			spec["override"] = map[string]interface{}{
				"service": serviceOverride,
			}

			placementAPI := CreatePlacementAPI(names.PlacementAPIName, spec)
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					namespace,
					GetPlacementAPI(names.PlacementAPIName).Spec.DatabaseInstance,
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)

			mariadb.SimulateMariaDBDatabaseCompleted(names.MariaDBDatabaseName)
			th.SimulateJobSuccess(names.DBSyncJobName)
			th.SimulateDeploymentReplicaReady(names.DeploymentName)
			keystone.SimulateKeystoneServiceReady(names.KeystoneServiceName)
			keystone.SimulateKeystoneEndpointReady(names.KeystoneEndpointName)
			DeferCleanup(th.DeleteInstance, placementAPI)
		})

		It("creates KeystoneEndpoint", func() {
			keystoneEndpoint := keystone.GetKeystoneEndpoint(names.KeystoneEndpointName)
			endpoints := keystoneEndpoint.Spec.Endpoints
			Expect(endpoints).To(HaveKeyWithValue("public", "http://placement-openstack.apps-crc.testing"))
			Expect(endpoints).To(HaveKeyWithValue("internal", "http://placement-internal."+namespace+".svc:8778"))

			th.ExpectCondition(
				names.PlacementAPIName,
				ConditionGetterFunc(PlacementConditionGetter),
				condition.KeystoneEndpointReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})

	Context("PlacementAPI is fully deployed", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreatePlacementAPI(names.PlacementAPIName, GetDefaultPlacementAPISpec()))
			DeferCleanup(
				k8sClient.Delete, ctx, CreatePlacementAPISecret(namespace, SecretName))
			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(namespace))

			serviceSpec := corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 3306}}}
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(namespace, "openstack", serviceSpec),
			)
			mariadb.SimulateMariaDBDatabaseCompleted(names.MariaDBDatabaseName)
			keystone.SimulateKeystoneServiceReady(names.KeystoneServiceName)
			keystone.SimulateKeystoneEndpointReady(names.KeystoneEndpointName)
			th.SimulateJobSuccess(names.DBSyncJobName)
			th.SimulateDeploymentReplicaReady(names.DeploymentName)

			th.ExpectCondition(
				names.PlacementAPIName,
				ConditionGetterFunc(PlacementConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("removes the finalizers when deleted", func() {
			placement := GetPlacementAPI(names.PlacementAPIName)
			Expect(placement.Finalizers).To(ContainElement("PlacementAPI"))
			keystoneService := keystone.GetKeystoneService(names.KeystoneServiceName)
			Expect(keystoneService.Finalizers).To(ContainElement("PlacementAPI"))
			keystoneEndpoint := keystone.GetKeystoneService(names.KeystoneEndpointName)
			Expect(keystoneEndpoint.Finalizers).To(ContainElement("PlacementAPI"))
			db := mariadb.GetMariaDBDatabase(names.MariaDBDatabaseName)
			Expect(db.Finalizers).To(ContainElement("PlacementAPI"))

			th.DeleteInstance(GetPlacementAPI(names.PlacementAPIName))

			keystoneService = keystone.GetKeystoneService(names.KeystoneServiceName)
			Expect(keystoneService.Finalizers).NotTo(ContainElement("PlacementAPI"))
			keystoneEndpoint = keystone.GetKeystoneService(names.KeystoneEndpointName)
			Expect(keystoneEndpoint.Finalizers).NotTo(ContainElement("PlacementAPI"))
			db = mariadb.GetMariaDBDatabase(names.MariaDBDatabaseName)
			Expect(db.Finalizers).NotTo(ContainElement("PlacementAPI"))
		})

		It("updates the deployment if configuration changes", func() {
			deployment := th.GetDeployment(names.DeploymentName)
			oldConfigHash := GetEnvVarValue(deployment.Spec.Template.Spec.Containers[0].Env, "CONFIG_HASH", "")
			Expect(oldConfigHash).NotTo(Equal(""))
			cm := th.GetConfigMap(names.ConfigMapName)
			Expect(cm.Data["custom.conf"]).ShouldNot(ContainSubstring("debug"))

			Eventually(func(g Gomega) {
				placement := GetPlacementAPI(names.PlacementAPIName)
				placement.Spec.CustomServiceConfig = "[DEFAULT]/ndebug = true"

				g.Expect(k8sClient.Update(ctx, placement)).Should(Succeed())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				deployment := th.GetDeployment(names.DeploymentName)
				newConfigHash := GetEnvVarValue(deployment.Spec.Template.Spec.Containers[0].Env, "CONFIG_HASH", "")
				g.Expect(newConfigHash).NotTo(Equal(""))
				g.Expect(newConfigHash).NotTo(Equal(oldConfigHash))

				cm := th.GetConfigMap(names.ConfigMapName)
				g.Expect(cm.Data["custom.conf"]).Should(ContainSubstring("debug = true"))
			}, timeout, interval).Should(Succeed())
		})

		It("updates the deployment if password changes", func() {
			deployment := th.GetDeployment(names.DeploymentName)
			oldConfigHash := GetEnvVarValue(deployment.Spec.Template.Spec.Containers[0].Env, "CONFIG_HASH", "")
			Expect(oldConfigHash).NotTo(Equal(""))

			th.UpdateSecret(
				types.NamespacedName{Namespace: namespace, Name: SecretName},
				"PlacementPassword", []byte("foobar"))

			logger.Info("Reconfigured")

			Eventually(func(g Gomega) {
				deployment := th.GetDeployment(names.DeploymentName)
				newConfigHash := GetEnvVarValue(deployment.Spec.Template.Spec.Containers[0].Env, "CONFIG_HASH", "")
				g.Expect(newConfigHash).NotTo(Equal(""))
				// FIXME(gibi): The placement-operator does not watch the input
				// secret so it does not detect that any input is changed.
				// Also the password values are not calculated into the input
				// hash as they are only applied in the init container
				// This should pass when this is fixed
				// g.Expect(newConfigHash).NotTo(Equal(oldConfigHash))
				g.Expect(newConfigHash).To(Equal(oldConfigHash))
				// TODO(gibi): once the password is in the generated config
				// assert it there
			}, timeout, interval).Should(Succeed())
		})

	})
})
