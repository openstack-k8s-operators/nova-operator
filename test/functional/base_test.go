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
	"encoding/json"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	networkv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	routev1 "github.com/openshift/api/route/v1"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	aee "github.com/openstack-k8s-operators/openstack-ansibleee-operator/api/v1alpha1"
)

const (
	SecretName           = "test-secret"
	MessageBusSecretName = "rabbitmq-secret"
	ContainerImage       = "test://nova"

	timeout = 10 * time.Second
	// have maximum 100 retries before the timeout hits
	interval = timeout / 100
	// consistencyTimeout is the amount of time we use to repeatedly check
	// that a condition is still valid. This is intendet to be used in
	// asserts using `Consistently`.
	consistencyTimeout = timeout
)

func CreateUnstructured(rawObj map[string]interface{}) *unstructured.Unstructured {
	logger.Info("Creating", "raw", rawObj)
	unstructuredObj := &unstructured.Unstructured{Object: rawObj}
	_, err := controllerutil.CreateOrPatch(
		ctx, k8sClient, unstructuredObj, func() error { return nil })
	Expect(err).ShouldNot(HaveOccurred())
	return unstructuredObj
}

func GetDefaultNovaAPISpec() map[string]interface{} {
	return map[string]interface{}{
		"secret":                  SecretName,
		"apiDatabaseHostname":     "nova-api-db-hostname",
		"apiMessageBusSecretName": MessageBusSecretName,
		"cell0DatabaseHostname":   "nova-cell0-db-hostname",
		"keystoneAuthURL":         "keystone-auth-url",
		"containerImage":          ContainerImage,
	}
}

func CreateNovaAPI(namespace string, spec map[string]interface{}) client.Object {
	novaAPIName := uuid.New().String()

	raw := map[string]interface{}{
		"apiVersion": "nova.openstack.org/v1beta1",
		"kind":       "NovaAPI",
		"metadata": map[string]interface{}{
			"name":      novaAPIName,
			"namespace": namespace,
		},
		"spec": spec,
	}
	return CreateUnstructured(raw)

}

func GetNovaAPI(name types.NamespacedName) *novav1.NovaAPI {
	instance := &novav1.NovaAPI{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}

func NovaAPINotExists(name types.NamespacedName) {
	Consistently(func(g Gomega) {
		instance := &novav1.NovaAPI{}
		err := k8sClient.Get(ctx, name, instance)
		g.Expect(k8s_errors.IsNotFound(err)).To(BeTrue())
	}, consistencyTimeout, interval).Should(Succeed())
}

func NovaAPIConditionGetter(name types.NamespacedName) condition.Conditions {
	instance := GetNovaAPI(name)
	return instance.Status.Conditions
}

func NovaSchedulerConditionGetter(name types.NamespacedName) condition.Conditions {
	instance := GetNovaScheduler(name)
	return instance.Status.Conditions
}

func CreateSecret(name types.NamespacedName, data map[string][]byte) *corev1.Secret {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.Name,
			Namespace: name.Namespace,
		},
		Data: data,
	}
	Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
	return secret
}

// CreateSecret creates a secret that has all the information NovaAPI needs
func CreateNovaAPISecret(namespace string, name string) *corev1.Secret {
	return CreateSecret(
		types.NamespacedName{Namespace: namespace, Name: name},
		map[string][]byte{
			"NovaPassword":              []byte("12345678"),
			"NovaAPIDatabasePassword":   []byte("12345678"),
			"NovaCell0DatabasePassword": []byte("12345678"),
		},
	)
}

func GetDefaultNovaSpec() map[string]interface{} {
	return map[string]interface{}{
		"secret":        SecretName,
		"cellTemplates": map[string]interface{}{},
	}
}

func GetDefaultNovaCellTemplate() map[string]interface{} {
	return map[string]interface{}{
		"cellName":         "cell0",
		"cellDatabaseUser": "nova_cell0",
		"hasAPIAccess":     true,
	}
}

func CreateNova(name types.NamespacedName, spec map[string]interface{}) client.Object {
	raw := map[string]interface{}{
		"apiVersion": "nova.openstack.org/v1beta1",
		"kind":       "Nova",
		"metadata": map[string]interface{}{
			"name":      name.Name,
			"namespace": name.Namespace,
		},
		"spec": spec,
	}
	return CreateUnstructured(raw)
}

func CreateNovaWithoutCell0(name types.NamespacedName) client.Object {
	rawNova := map[string]interface{}{
		"apiVersion": "nova.openstack.org/v1beta1",
		"kind":       "Nova",
		"metadata": map[string]interface{}{
			"name":      name.Name,
			"namespace": name.Namespace,
		},
		"spec": map[string]interface{}{
			"secret":        SecretName,
			"cellTemplates": map[string]interface{}{},
		},
	}

	return CreateUnstructured(rawNova)
}

func CreateNovaWithCell0(name types.NamespacedName) client.Object {
	rawNova := map[string]interface{}{
		"apiVersion": "nova.openstack.org/v1beta1",
		"kind":       "Nova",
		"metadata": map[string]interface{}{
			"name":      name.Name,
			"namespace": name.Namespace,
		},
		"spec": map[string]interface{}{
			"secret": SecretName,
			"cellTemplates": map[string]interface{}{
				"cell0": map[string]interface{}{
					"cellDatabaseUser": "nova_cell0",
					"hasAPIAccess":     true,
				},
			},
		},
	}

	return CreateUnstructured(rawNova)
}

func DeleteInstance(instance client.Object) {
	// We have to wait for the controller to fully delete the instance
	logger.Info("Deleting", "Name", instance.GetName(), "Namespace", instance.GetNamespace(), "Kind", instance.GetObjectKind().GroupVersionKind().Kind)
	Eventually(func(g Gomega) {
		name := types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
		err := k8sClient.Get(ctx, name, instance)
		// if it is already gone that is OK
		if k8s_errors.IsNotFound(err) {
			return
		}
		g.Expect(err).Should(BeNil())

		g.Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())

		err = k8sClient.Get(ctx, name, instance)
		g.Expect(k8s_errors.IsNotFound(err)).To(BeTrue())
	}, timeout, interval).Should(Succeed())
}

func GetNova(name types.NamespacedName) *novav1.Nova {
	instance := &novav1.Nova{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}

func NovaConditionGetter(name types.NamespacedName) condition.Conditions {
	instance := GetNova(name)
	return instance.Status.Conditions
}

func GetDefaultNovaConductorSpec() map[string]interface{} {
	return map[string]interface{}{
		"cellName":                 "cell0",
		"secret":                   SecretName,
		"cellMessageBusSecretName": MessageBusSecretName,
		"containerImage":           ContainerImage,
		"keystoneAuthURL":          "keystone-auth-url",
	}
}

func CreateNovaConductor(namespace string, spec map[string]interface{}) client.Object {
	novaAPIName := uuid.New().String()

	raw := map[string]interface{}{
		"apiVersion": "nova.openstack.org/v1beta1",
		"kind":       "NovaConductor",
		"metadata": map[string]interface{}{
			"name":      novaAPIName,
			"namespace": namespace,
		},
		"spec": spec,
	}
	return CreateUnstructured(raw)
}

func GetNovaConductor(name types.NamespacedName) *novav1.NovaConductor {
	instance := &novav1.NovaConductor{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}

func NovaConductorConditionGetter(name types.NamespacedName) condition.Conditions {
	instance := GetNovaConductor(name)
	return instance.Status.Conditions
}

// CreateNovaConductorSecret creates a secret that has all the information
// NovaConductor needs
func CreateNovaConductorSecret(namespace string, name string) *corev1.Secret {
	return CreateSecret(
		types.NamespacedName{Namespace: namespace, Name: name},
		map[string][]byte{
			"NovaCell0DatabasePassword": []byte("12345678"),
		},
	)
}

func CreateNovaMessageBusSecret(namespace string, name string) *corev1.Secret {
	return CreateSecret(
		types.NamespacedName{Namespace: namespace, Name: name},
		map[string][]byte{
			"transport_url": []byte("rabbit://fake"),
		},
	)
}

func GetDefaultNovaCellSpec() map[string]interface{} {
	return map[string]interface{}{
		"cellName":                  "cell0",
		"secret":                    SecretName,
		"cellDatabaseHostname":      "cell-database-hostname",
		"cellMessageBusSecretName":  MessageBusSecretName,
		"keystoneAuthURL":           "keystone-auth-url",
		"conductorServiceTemplate":  map[string]interface{}{},
		"noVNCProxyServiceTemplate": map[string]interface{}{},
	}
}

func CreateNovaCell(name types.NamespacedName, spec map[string]interface{}) client.Object {

	raw := map[string]interface{}{
		"apiVersion": "nova.openstack.org/v1beta1",
		"kind":       "NovaCell",
		"metadata": map[string]interface{}{
			"name":      name.Name,
			"namespace": name.Namespace,
		},
		"spec": spec,
	}
	return CreateUnstructured(raw)
}

func GetNovaCell(name types.NamespacedName) *novav1.NovaCell {
	instance := &novav1.NovaCell{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}

func NovaCellNotExists(name types.NamespacedName) {
	Consistently(func(g Gomega) {
		instance := &novav1.NovaCell{}
		err := k8sClient.Get(ctx, name, instance)
		g.Expect(k8s_errors.IsNotFound(err)).To(BeTrue())
	}, consistencyTimeout, interval).Should(Succeed())
}

func NovaCellConditionGetter(name types.NamespacedName) condition.Conditions {
	instance := GetNovaCell(name)
	return instance.Status.Conditions
}

func CreateNovaSecret(namespace string, name string) *corev1.Secret {
	return CreateSecret(
		types.NamespacedName{Namespace: namespace, Name: name},
		map[string][]byte{
			"NovaPassword":              []byte("12345678"),
			"NovaAPIDatabasePassword":   []byte("12345678"),
			"NovaCell0DatabasePassword": []byte("12345678"),
		},
	)
}

func GetService(name types.NamespacedName) *corev1.Service {
	instance := &corev1.Service{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}

func AssertRouteExists(name types.NamespacedName) *routev1.Route {
	instance := &routev1.Route{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}

func AssertRouteNotExists(name types.NamespacedName) *routev1.Route {
	instance := &routev1.Route{}
	Consistently(func(g Gomega) {
		err := k8sClient.Get(ctx, name, instance)
		g.Expect(k8s_errors.IsNotFound(err)).To(BeTrue())
	}, consistencyTimeout, interval).Should(Succeed())
	return instance
}

func GetDefaultNovaSchedulerSpec() map[string]interface{} {
	return map[string]interface{}{
		"secret":                  SecretName,
		"apiDatabaseHostname":     "nova-api-db-hostname",
		"apiMessageBusSecretName": MessageBusSecretName,
		"cell0DatabaseHostname":   "nova-cell0-db-hostname",
		"keystoneAuthURL":         "keystone-auth-url",
		"containerImage":          ContainerImage,
	}
}

func CreateNovaScheduler(namespace string, spec map[string]interface{}) client.Object {
	name := uuid.New().String()

	raw := map[string]interface{}{
		"apiVersion": "nova.openstack.org/v1beta1",
		"kind":       "NovaScheduler",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
		"spec": spec,
	}
	return CreateUnstructured(raw)
}

func GetNovaScheduler(name types.NamespacedName) *novav1.NovaScheduler {
	instance := &novav1.NovaScheduler{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}

func NovaSchedulerNotExists(name types.NamespacedName) {
	Consistently(func(g Gomega) {
		instance := &novav1.NovaScheduler{}
		err := k8sClient.Get(ctx, name, instance)
		g.Expect(k8s_errors.IsNotFound(err)).To(BeTrue())
	}, consistencyTimeout, interval).Should(Succeed())
}

func CreateNetworkAttachmentDefinition(name types.NamespacedName) client.Object {
	instance := &networkv1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.Name,
			Namespace: name.Namespace,
		},
		Spec: networkv1.NetworkAttachmentDefinitionSpec{
			Config: "",
		},
	}
	Expect(k8sClient.Create(ctx, instance)).Should(Succeed())
	return instance
}

func GetDefaultNovaExternalComputeSpec(novaName string, computeName string) map[string]interface{} {
	return map[string]interface{}{
		"novaInstance":           novaName,
		"inventoryConfigMapName": computeName + "-inventory-configmap",
		"sshKeySecretName":       computeName + "-ssh-key-secret",
	}
}

func CreateNovaExternalCompute(name types.NamespacedName, spec map[string]interface{}) client.Object {
	raw := map[string]interface{}{
		"apiVersion": "nova.openstack.org/v1beta1",
		"kind":       "NovaExternalCompute",
		"metadata": map[string]interface{}{
			"name":      name.Name,
			"namespace": name.Namespace,
		},
		"spec": spec,
	}
	return CreateUnstructured(raw)
}


func SimulateStatefulSetReplicaReadyWithPods(name types.NamespacedName, networkIPs map[string][]string) {
	ss := th.GetStatefulSet(name)
	for i := 0; i < int(*ss.Spec.Replicas); i++ {
		pod := &corev1.Pod{
			ObjectMeta: ss.Spec.Template.ObjectMeta,
			Spec:       ss.Spec.Template.Spec,
		}
		pod.ObjectMeta.Namespace = name.Namespace
		pod.ObjectMeta.GenerateName = name.Name

		var netStatus []networkv1.NetworkStatus
		for network, IPs := range networkIPs {
			netStatus = append(
				netStatus,
				networkv1.NetworkStatus{
					Name: network,
					IPs:  IPs,
				},
			)
		}
		netStatusAnnotation, err := json.Marshal(netStatus)
		Expect(err).NotTo(HaveOccurred())
		pod.Annotations[networkv1.NetworkStatusAnnot] = string(netStatusAnnotation)

		Expect(k8sClient.Create(ctx, pod)).Should(Succeed())
	}

	Eventually(func(g Gomega) {
		ss := th.GetStatefulSet(name)
		ss.Status.Replicas = 1
		ss.Status.ReadyReplicas = 1
		g.Expect(k8sClient.Status().Update(ctx, ss)).To(Succeed())

	}, timeout, interval).Should(Succeed())

	logger.Info("Simulated statefulset success", "on", name)
}

func GetNovaExternalCompute(name types.NamespacedName) *novav1.NovaExternalCompute {
	instance := &novav1.NovaExternalCompute{}

	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}

func NovaExternalComputeConditionGetter(name types.NamespacedName) condition.Conditions {
	instance := GetNovaExternalCompute(name)
	return instance.Status.Conditions
}

func CreateEmptySecret(name types.NamespacedName) *corev1.Secret {
	return CreateSecret(name, map[string][]byte{})
}

func CreateEmptyConfigMap(name types.NamespacedName) *corev1.ConfigMap {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.Name,
			Namespace: name.Namespace,
		},
		Data: map[string]string{},
	}
	Expect(k8sClient.Create(ctx, configMap)).Should(Succeed())
	return configMap
}

func CreateNovaExternalComputeInventoryConfigMap(name types.NamespacedName) *corev1.ConfigMap {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.Name,
			Namespace: name.Namespace,
		},
		Data: map[string]string{
			"inventory": "an ansible inventory",
		},
	}
	Expect(k8sClient.Create(ctx, configMap)).Should(Succeed())
	return configMap
}

func CreateNovaExternalComputeSSHSecret(name types.NamespacedName) *corev1.Secret {
	return CreateSecret(
		name,
		map[string][]byte{
			"ssh-privatekey": []byte("a private key"),
		},
	)
}

func DeleteSecret(name types.NamespacedName) {
	Eventually(func(g Gomega) {
		secret := &corev1.Secret{}
		err := k8sClient.Get(ctx, name, secret)
		// if it is already gone that is OK
		if k8s_errors.IsNotFound(err) {
			return
		}
		g.Expect(err).Should(BeNil())

		g.Expect(k8sClient.Delete(ctx, secret)).Should(Succeed())

		err = k8sClient.Get(ctx, name, secret)
		g.Expect(k8s_errors.IsNotFound(err)).To(BeTrue())
	}, timeout, interval).Should(Succeed())
}

func DeleteConfigMap(name types.NamespacedName) {
	Eventually(func(g Gomega) {
		configMap := &corev1.ConfigMap{}
		err := k8sClient.Get(ctx, name, configMap)
		// if it is already gone that is OK
		if k8s_errors.IsNotFound(err) {
			return
		}
		g.Expect(err).Should(BeNil())

		g.Expect(k8sClient.Delete(ctx, configMap)).Should(Succeed())

		err = k8sClient.Get(ctx, name, configMap)
		g.Expect(k8s_errors.IsNotFound(err)).To(BeTrue())
	}, timeout, interval).Should(Succeed())
}


func GetAEE(name types.NamespacedName) *aee.OpenStackAnsibleEE {
	instance := &aee.OpenStackAnsibleEE{}

	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}

func SimulateAEESucceded(name types.NamespacedName) {
	Eventually(func(g Gomega) {
		ansibleEE := GetAEE(name)
		ansibleEE.Status.JobStatus = "Succeeded"
		g.Expect(k8sClient.Status().Update(ctx, ansibleEE)).To(Succeed())

	}, timeout, interval).Should(Succeed())

	logger.Info("Simulated AEE success", "on", name)
}

func CreateNovaMetadata(namespace string, spec map[string]interface{}) client.Object {
	novaMetadataName := uuid.New().String()

	raw := map[string]interface{}{
		"apiVersion": "nova.openstack.org/v1beta1",
		"kind":       "NovaMetadata",
		"metadata": map[string]interface{}{
			"name":      novaMetadataName,
			"namespace": namespace,
		},
		"spec": spec,
	}
	return CreateUnstructured(raw)
}

func GetNovaMetadata(name types.NamespacedName) *novav1.NovaMetadata {
	instance := &novav1.NovaMetadata{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}

func NovaMetadataConditionGetter(name types.NamespacedName) condition.Conditions {
	instance := GetNovaMetadata(name)
	return instance.Status.Conditions
}

func CreateNovaMetadataSecret(namespace string, name string) *corev1.Secret {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"NovaPassword":              []byte("12345678"),
			"NovaAPIDatabasePassword":   []byte("12345678"),
			"NovaCell0DatabasePassword": []byte("12345678"),
		},
	}
	Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
	return secret
}

func GetDefaultNovaMetadataSpec() map[string]interface{} {
	return map[string]interface{}{
		"secret":                  SecretName,
		"apiDatabaseHostname":     "nova-api-db-hostname",
		"apiMessageBusSecretName": MessageBusSecretName,
		"cellDatabaseHostname":    "nova-cell-db-hostname",
		"containerImage":          ContainerImage,
	}
}