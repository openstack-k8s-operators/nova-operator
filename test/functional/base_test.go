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
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	aee "github.com/openstack-k8s-operators/openstack-ansibleee-operator/api/v1alpha1"
)

const (
	SecretName           = "test-secret"
	MessageBusSecretName = "rabbitmq-secret"
	ContainerImage       = "test://nova"
	timeout              = 10 * time.Second
	// have maximum 100 retries before the timeout hits
	interval = timeout / 100
	// consistencyTimeout is the amount of time we use to repeatedly check
	// that a condition is still valid. This is intended to be used in
	// asserts using `Consistently`.
	consistencyTimeout = timeout
)

func GetDefaultNovaAPISpec() map[string]interface{} {
	return map[string]interface{}{
		"secret":                  SecretName,
		"apiDatabaseHostname":     "nova-api-db-hostname",
		"apiMessageBusSecretName": MessageBusSecretName,
		"cell0DatabaseHostname":   "nova-cell0-db-hostname",
		"keystoneAuthURL":         "keystone-auth-url",
		"containerImage":          ContainerImage,
		"serviceAccount":          "nova",
		"registeredCells":         map[string]string{},
	}
}

func CreateNovaAPI(name types.NamespacedName, spec map[string]interface{}) client.Object {
	raw := map[string]interface{}{
		"apiVersion": "nova.openstack.org/v1beta1",
		"kind":       "NovaAPI",
		"metadata": map[string]interface{}{
			"name":      name.Name,
			"namespace": name.Namespace,
		},
		"spec": spec,
	}
	return th.CreateUnstructured(raw)

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

// th.CreateSecret creates a secret that has all the information NovaAPI needs
func CreateNovaAPISecret(namespace string, name string) *corev1.Secret {
	return th.CreateSecret(
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
	return th.CreateUnstructured(raw)
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

	return th.CreateUnstructured(rawNova)
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

	return th.CreateUnstructured(rawNova)
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
		"serviceAccount":           "nova",
	}
}

func CreateNovaConductor(name types.NamespacedName, spec map[string]interface{}) client.Object {
	raw := map[string]interface{}{
		"apiVersion": "nova.openstack.org/v1beta1",
		"kind":       "NovaConductor",
		"metadata": map[string]interface{}{
			"name":      name.Name,
			"namespace": name.Namespace,
		},
		"spec": spec,
	}
	return th.CreateUnstructured(raw)
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
	return th.CreateSecret(
		types.NamespacedName{Namespace: namespace, Name: name},
		map[string][]byte{
			"NovaCell0DatabasePassword": []byte("12345678"),
			"NovaPassword":              []byte("12345678"),
		},
	)
}

func CreateNovaMessageBusSecret(namespace string, name string) *corev1.Secret {
	s := th.CreateSecret(
		types.NamespacedName{Namespace: namespace, Name: name},
		map[string][]byte{
			"transport_url": []byte(fmt.Sprintf("rabbit://%s/fake", name)),
		},
	)
	logger.Info("Secret created", "name", name)
	return s
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
		"serviceAccount":            "nova",
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
	return th.CreateUnstructured(raw)
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
	return th.CreateSecret(
		types.NamespacedName{Namespace: namespace, Name: name},
		map[string][]byte{
			"NovaPassword":              []byte("12345678"),
			"NovaAPIDatabasePassword":   []byte("12345678"),
			"NovaCell0DatabasePassword": []byte("12345678"),
			"MetadataSecret":            []byte("12345678"),
		},
	)
}

func GetDefaultNovaSchedulerSpec() map[string]interface{} {
	return map[string]interface{}{
		"secret":                  SecretName,
		"apiDatabaseHostname":     "nova-api-db-hostname",
		"apiMessageBusSecretName": MessageBusSecretName,
		"cell0DatabaseHostname":   "nova-cell0-db-hostname",
		"keystoneAuthURL":         "keystone-auth-url",
		"containerImage":          ContainerImage,
		"serviceAccount":          "nova",
		"registeredCells":         map[string]string{},
	}
}

func CreateNovaScheduler(name types.NamespacedName, spec map[string]interface{}) client.Object {
	raw := map[string]interface{}{
		"apiVersion": "nova.openstack.org/v1beta1",
		"kind":       "NovaScheduler",
		"metadata": map[string]interface{}{
			"name":      name.Name,
			"namespace": name.Namespace,
		},
		"spec": spec,
	}
	return th.CreateUnstructured(raw)
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
	return th.CreateUnstructured(raw)
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

func CreateNovaExternalComputeInventoryConfigMap(name types.NamespacedName) client.Object {

	return th.CreateConfigMap(
		name, map[string]interface{}{"inventory": "an ansible inventory"})
}

func CreateNovaExternalComputeSSHSecret(name types.NamespacedName) *corev1.Secret {
	return th.CreateSecret(
		name,
		map[string][]byte{
			"ssh-privatekey": []byte("a private key"),
		},
	)
}

func GetAEE(name types.NamespacedName) *aee.OpenStackAnsibleEE {
	instance := &aee.OpenStackAnsibleEE{}

	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}

func SimulateAEESucceeded(name types.NamespacedName) {
	Eventually(func(g Gomega) {
		ansibleEE := GetAEE(name)
		ansibleEE.Status.JobStatus = "Succeeded"
		g.Expect(k8sClient.Status().Update(ctx, ansibleEE)).To(Succeed())

	}, timeout, interval).Should(Succeed())

	logger.Info("Simulated AEE success", "on", name)
}

type CellNames struct {
	CellName                      types.NamespacedName
	MariaDBDatabaseName           types.NamespacedName
	CellConductorName             types.NamespacedName
	CellDBSyncJobName             types.NamespacedName
	ConductorStatefulSetName      types.NamespacedName
	TransportURLName              types.NamespacedName
	CellMappingJobName            types.NamespacedName
	MetadataStatefulSetName       types.NamespacedName
	NoVNCProxyNameStatefulSetName types.NamespacedName

}

func GetCellNames(novaName types.NamespacedName, cell string) CellNames {
	cellName := types.NamespacedName{
		Namespace: novaName.Namespace,
		Name:      novaName.Name + "-" + cell,
	}
	cellConductor := types.NamespacedName{
		Namespace: novaName.Namespace,
		Name:      cellName.Name + "-conductor",
	}
	c := CellNames{
		CellName: cellName,
		MariaDBDatabaseName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      "nova-" + cell,
		},
		CellConductorName: cellConductor,
		CellDBSyncJobName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      cellConductor.Name + "-db-sync",
		},
		ConductorStatefulSetName: cellConductor,
		TransportURLName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      cellName.Name + "-transport",
		},
		CellMappingJobName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      cellName.Name + "-cell-mapping",
		},
		MetadataStatefulSetName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      cellName.Name + "-metadata",
		},
		NoVNCProxyNameStatefulSetName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      cellName.Name + "-novncproxy",
		},
	}

	if cell == "cell0" {
		c.TransportURLName = types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      novaName.Name + "-api-transport",
		}
	}

	return c
}

type NovaNames struct {
	Namespace                       string
	NovaName                        types.NamespacedName
	InternalNovaServiceName         types.NamespacedName
	PublicNovaServiceName           types.NamespacedName
	AdminNovaServiceName            types.NamespacedName
	InternalNovaRouteName           types.NamespacedName
	PublicNovaRouteName             types.NamespacedName
	AdminNovaRouteName              types.NamespacedName
	ComputeName                     types.NamespacedName
	KeystoneServiceName             types.NamespacedName
	APIName                         types.NamespacedName
	APIMariaDBDatabaseName          types.NamespacedName
	APIDeploymentName               types.NamespacedName
	APIKeystoneEndpointName         types.NamespacedName
	APIStatefulSetName              types.NamespacedName
	APIConfigDataName               types.NamespacedName
	InternalAPINetworkNADName       types.NamespacedName // refers internalapi network, not Nova API
	SchedulerName                   types.NamespacedName
	SchedulerStatefulSetName        types.NamespacedName
	SchedulerConfigDataName         types.NamespacedName
	ConductorName                   types.NamespacedName
	ConductorDBSyncJobName          types.NamespacedName
	ConductorStatefulSetName        types.NamespacedName
	ConductorConfigDataName         types.NamespacedName
	ConductorScriptDataName         types.NamespacedName
	MetadataName                    types.NamespacedName
	MetadataStatefulSetName         types.NamespacedName
	NoVNCProxyName                  types.NamespacedName
	NoVNCProxyNameStatefulSetName   types.NamespacedName
	ServiceAccountName              types.NamespacedName
	RoleName                        types.NamespacedName
	RoleBindingName                 types.NamespacedName
	MetadataConfigDataName          types.NamespacedName
	InternalNovaMetadataServiceName types.NamespacedName
	InternalNovaMetadataRouteName   types.NamespacedName
	Cells                           map[string]CellNames
}

func GetNovaNames(novaName types.NamespacedName, cellNames []string) NovaNames {
	// NOTE(bogdando): use random UUIDs instead of static "nova" part of names.
	// These **must** replicate existing Nova*/Dataplane controllers suffixing/prefixing logic.
	// While dynamic UUIDs also provide enhanced testing coverage for "synthetic" cases,
	// which could not be caught for normal names with static "nova" prefixes.
	computeExt := types.NamespacedName{
		Namespace: novaName.Namespace,
		Name:      uuid.New().String(),
	}
	novaAPI := types.NamespacedName{
		Namespace: novaName.Namespace,
		Name:      fmt.Sprintf("%s-api", novaName.Name),
	}
	novaScheduler := types.NamespacedName{
		Namespace: novaName.Namespace,
		Name:      fmt.Sprintf("%s-scheduler", novaName.Name),
	}
	novaConductor := types.NamespacedName{
		Namespace: novaName.Namespace,
		Name:      fmt.Sprintf("%s-conductor", novaName.Name),
	}
	novaMetadata := types.NamespacedName{
		Namespace: novaName.Namespace,
		Name:      fmt.Sprintf("%s-metadata", novaName.Name),
	}
	novaNoVNCProxy := types.NamespacedName{
		Namespace: novaName.Namespace,
		Name:      fmt.Sprintf("%s-novncproxy", novaName.Name),
	}
	cells := map[string]CellNames{}
	for _, cellName := range cellNames {
		cells[cellName] = GetCellNames(novaName, cellName)
	}

	return NovaNames{
		Namespace: novaName.Namespace,
		NovaName:  novaName,
		InternalNovaServiceName: types.NamespacedName{ // TODO replace for nova-internal
			Namespace: novaName.Namespace,
			Name:      novaName.Name + "-internal",
		},
		PublicNovaServiceName: types.NamespacedName{ // TODO replace for nova-public
			Namespace: novaName.Namespace,
			Name:      novaName.Name + "-public",
		},
		AdminNovaServiceName: types.NamespacedName{ // TODO replace for nova-admin
			Namespace: novaName.Namespace,
			Name:      novaName.Name + "-admin",
		},
		InternalNovaRouteName: types.NamespacedName{ // TODO replace for nova-internal
			Namespace: novaName.Namespace,
			Name:      novaName.Name + "-internal",
		},
		PublicNovaRouteName: types.NamespacedName{ // TODO replace for nova-public
			Namespace: novaName.Namespace,
			Name:      novaName.Name + "-public",
		},
		AdminNovaRouteName: types.NamespacedName{ // TODO replace for nova-admin
			Namespace: novaName.Namespace,
			Name:      novaName.Name + "-admin",
		},
		ComputeName: computeExt,
		KeystoneServiceName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      "nova", // static value hardcoded in controller code
		},
		APIName: novaAPI,
		APIMariaDBDatabaseName: types.NamespacedName{
			Namespace: novaAPI.Namespace,
			Name:      "nova-api", // a static DB name for nova
		},
		APIDeploymentName: novaAPI,
		APIKeystoneEndpointName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      "nova", // a static keystone endpoint name for nova
		},
		APIStatefulSetName: novaAPI,
		APIConfigDataName: types.NamespacedName{ // TODO replace configDataMap for API
			Namespace: novaAPI.Namespace,
			Name:      novaAPI.Name + "-config-data",
		},
		InternalAPINetworkNADName: types.NamespacedName{ // TODO replace for internalAPINADName
			Namespace: novaAPI.Namespace,
			Name:      "internalapi",
		},
		SchedulerName:            novaScheduler,
		SchedulerStatefulSetName: novaScheduler,
		SchedulerConfigDataName: types.NamespacedName{ // TODO replace configDataMap for Sched
			Namespace: novaScheduler.Namespace,
			Name:      novaScheduler.Name + "-config-data",
		},
		ConductorName: novaConductor,
		ConductorDBSyncJobName: types.NamespacedName{
			Namespace: novaConductor.Namespace,
			Name:      novaConductor.Name + "-db-sync",
		},
		ConductorStatefulSetName: novaConductor,
		ConductorConfigDataName: types.NamespacedName{ // TODO replace configDataMap for Cond
			Namespace: novaConductor.Namespace,
			Name:      novaConductor.Name + "-config-data",
		},
		ConductorScriptDataName: types.NamespacedName{ // TODO replace scriptMap for Cond
			Namespace: novaConductor.Namespace,
			Name:      novaConductor.Name + "-script",
		},
		MetadataName:                  novaMetadata,
		MetadataStatefulSetName:       novaMetadata,
		NoVNCProxyName:                novaNoVNCProxy,
		NoVNCProxyNameStatefulSetName: novaNoVNCProxy,
		ServiceAccountName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      "nova-" + novaName.Name,
		},
		RoleName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      "nova-" + novaName.Name + "-role",
		},
		RoleBindingName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      "nova-" + novaName.Name + "-rolebinding",
		},
		MetadataConfigDataName: types.NamespacedName{ // TODO replace configDataMap for Sched
			Namespace: novaMetadata.Namespace,
			Name:      novaMetadata.Name + "-config-data",
		},
		InternalNovaMetadataServiceName: types.NamespacedName{ // TODO replace for nova-metadata-internal
			Namespace: novaMetadata.Namespace,
			Name:      novaMetadata.Name + "-internal",
		},
		InternalNovaMetadataRouteName: types.NamespacedName{ // TODO replace for nova-metadata-internal
			Namespace: novaMetadata.Namespace,
			Name:      novaMetadata.Name + "-internal",
		},

		Cells: cells,
	}
}

func CreateNovaMetadata(name types.NamespacedName, spec map[string]interface{}) client.Object {
	raw := map[string]interface{}{
		"apiVersion": "nova.openstack.org/v1beta1",
		"kind":       "NovaMetadata",
		"metadata": map[string]interface{}{
			"name":      name.Name,
			"namespace": name.Namespace,
		},
		"spec": spec,
	}
	return th.CreateUnstructured(raw)
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
			"MetadataSecret":            []byte("12345678"),
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
		"keystoneAuthURL":         "keystone-auth-url",
		"serviceAccount":          "nova",
	}
}

func CreateNovaNoVNCProxy(name types.NamespacedName, spec map[string]interface{}) client.Object {
	raw := map[string]interface{}{
		"apiVersion": "nova.openstack.org/v1beta1",
		"kind":       "NovaNoVNCProxy",
		"metadata": map[string]interface{}{
			"name":      name.Name,
			"namespace": name.Namespace,
		},
		"spec": spec,
	}
	return th.CreateUnstructured(raw)
}

func NoVNCProxyConditionGetter(name types.NamespacedName) condition.Conditions {
	instance := GetNovaNoVNCProxy(name)
	return instance.Status.Conditions
}

func GetNovaNoVNCProxy(name types.NamespacedName) *novav1.NovaNoVNCProxy {
	instance := &novav1.NovaNoVNCProxy{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}

func GetDefaultNovaNoVNCProxySpec() map[string]interface{} {
	return map[string]interface{}{
		"secret":               SecretName,
		"cellDatabaseHostname": "nova-cell-db-hostname",
		"containerImage":       ContainerImage,
		"keystoneAuthURL":      "keystone-auth-url",
		"serviceAccount":       "nova",
		"cellName":             "cell1",
	}
}

func CreateNovaNoVNCProxySecret(namespace string, name string) *corev1.Secret {
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
