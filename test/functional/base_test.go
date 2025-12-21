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

	. "github.com/onsi/ginkgo/v2" //revive:disable:dot-imports
	. "github.com/onsi/gomega"    //revive:disable:dot-imports

	//revive:disable-next-line:dot-imports
	. "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"
	mariadbv1 "github.com/openstack-k8s-operators/mariadb-operator/api/v1beta1"

	"maps"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	SecretName     = "external-secret"
	ContainerImage = "test://nova"
	timeout        = 25 * time.Second
	// have maximum 100 retries before the timeout hits
	interval = timeout / 100
	// consistencyTimeout is the amount of time we use to repeatedly check
	// that a condition is still valid. This is intended to be used in
	// asserts using `Consistently`.
	consistencyTimeout = timeout
	ironicComputeName  = "ironic-compute"
	MemcachedInstance  = "memcached"
)

func GetDefaultNovaAPISpec(novaNames NovaNames) map[string]any {
	return map[string]any{
		"secret":                novaNames.InternalTopLevelSecretName.Name,
		"apiDatabaseHostname":   "nova-api-db-hostname",
		"cell0DatabaseHostname": "nova-cell0-db-hostname",
		"cell0DatabaseAccount":  cell0.MariaDBAccountName.Name,
		"keystoneAuthURL":       "keystone-internal-auth-url",
		"keystonePublicAuthURL": "keystone-public-auth-url",
		"containerImage":        ContainerImage,
		"serviceAccount":        "nova-sa",
		"registeredCells":       map[string]string{},
		"memcachedInstance":     MemcachedInstance,
		"apiDatabaseAccount":    novaNames.APIMariaDBDatabaseAccount.Name,
	}
}

func CreateNovaAPI(name types.NamespacedName, spec map[string]any) client.Object {
	raw := map[string]any{
		"apiVersion": "nova.openstack.org/v1beta1",
		"kind":       "NovaAPI",
		"metadata": map[string]any{
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

func GetDefaultNovaSpec() map[string]any {
	return map[string]any{
		"secret":                SecretName,
		"cellTemplates":         map[string]any{},
		"apiMessageBusInstance": cell0.TransportURLName.Name,
		"apiDatabaseAccount":    novaNames.APIMariaDBDatabaseAccount.Name,
	}
}

func GetDefaultNovaCellTemplate() map[string]any {
	return map[string]any{
		"cellDatabaseAccount": cell0.MariaDBAccountName.Name,
		"hasAPIAccess":        true,
		"apiDatabaseAccount":  novaNames.APIMariaDBDatabaseAccount.Name,
	}
}

func CreateNova(name types.NamespacedName, spec map[string]any) client.Object {
	raw := map[string]any{
		"apiVersion": "nova.openstack.org/v1beta1",
		"kind":       "Nova",
		"metadata": map[string]any{
			"name":      name.Name,
			"namespace": name.Namespace,
		},
		"spec": spec,
	}
	return th.CreateUnstructured(raw)
}

func CreateNovaWithCell0(name types.NamespacedName) client.Object {
	rawNova := map[string]any{
		"apiVersion": "nova.openstack.org/v1beta1",
		"kind":       "Nova",
		"metadata": map[string]any{
			"name":      name.Name,
			"namespace": name.Namespace,
		},
		"spec": map[string]any{
			"secret":             SecretName,
			"apiDatabaseAccount": novaNames.APIMariaDBDatabaseAccount.Name,
			"cellTemplates": map[string]any{
				"cell0": map[string]any{
					"cellDatabaseAccount": cell0.MariaDBAccountName.Name,
					"apiDatabaseAccount":  novaNames.APIMariaDBDatabaseAccount.Name,
					"hasAPIAccess":        true,
					"dbPurge": map[string]any{
						"schedule": "1 0 * * *",
					},
				},
			},
			"apiMessageBusInstance": cell0.TransportURLName.Name,
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

func GetDefaultNovaConductorSpec(cell CellNames) map[string]any {
	return map[string]any{
		"cellName":            cell.CellName,
		"secret":              cell.InternalCellSecretName.Name,
		"apiDatabaseAccount":  novaNames.APIMariaDBDatabaseAccount.Name,
		"containerImage":      ContainerImage,
		"keystoneAuthURL":     "keystone-auth-url",
		"serviceAccount":      "nova-sa",
		"customServiceConfig": "foo=bar",
		"memcachedInstance":   MemcachedInstance,
		"cellDatabaseAccount": cell.MariaDBAccountName.Name,
	}
}

func CreateNovaConductor(name types.NamespacedName, spec map[string]any) client.Object {
	raw := map[string]any{
		"apiVersion": "nova.openstack.org/v1beta1",
		"kind":       "NovaConductor",
		"metadata": map[string]any{
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

func CreateNovaMessageBusSecret(cell CellNames) *corev1.Secret {
	s := th.CreateSecret(
		types.NamespacedName{
			Namespace: cell.CellCRName.Namespace,
			Name:      fmt.Sprintf("%s-secret", cell.TransportURLName.Name)},
		map[string][]byte{
			"transport_url": fmt.Appendf(nil, "rabbit://%s/fake", cell.CellName),
		},
	)
	logger.Info("Secret created", "name", s.Name)
	return s
}

func CreateNotificationTransportURLSecret(notificationsBus NotificationsBusNames) *corev1.Secret {
	s := th.CreateSecret(
		types.NamespacedName{
			Namespace: novaNames.NovaName.Namespace,
			Name:      fmt.Sprintf("%s-secret", notificationsBus.BusName)},
		map[string][]byte{
			"transport_url": fmt.Appendf(nil, "rabbit://%s/fake", notificationsBus.TransportURLName.Name),
		},
	)
	logger.Info("Secret created", "name", s.Name)
	return s
}

func GetDefaultNovaCellSpec(cell CellNames) map[string]any {
	return map[string]any{
		"cellName":             cell.CellName,
		"secret":               cell.InternalCellSecretName.Name,
		"apiDatabaseAccount":   novaNames.APIMariaDBDatabaseAccount.Name,
		"cellDatabaseHostname": "cell-database-hostname",
		"keystoneAuthURL":      "keystone-auth-url",
		"serviceAccount":       "nova",
		"memcachedInstance":    MemcachedInstance,
		"cellDatabaseAccount":  cell.MariaDBAccountName.Name,
	}
}

func CreateNovaCell(name types.NamespacedName, spec map[string]any) client.Object {

	raw := map[string]any{
		"apiVersion": "nova.openstack.org/v1beta1",
		"kind":       "NovaCell",
		"metadata": map[string]any{
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
			"NovaPassword":   []byte("service-password"),
			"MetadataSecret": []byte("metadata-secret"),
		},
	)
}

func GetDefaultNovaSchedulerSpec(novaNames NovaNames) map[string]any {
	return map[string]any{
		"secret":                novaNames.InternalTopLevelSecretName.Name,
		"apiDatabaseAccount":    novaNames.APIMariaDBDatabaseAccount.Name,
		"apiDatabaseHostname":   "nova-api-db-hostname",
		"cell0DatabaseHostname": "nova-cell0-db-hostname",
		"cell0DatabaseAccount":  cell0.MariaDBAccountName.Name,
		"keystoneAuthURL":       "keystone-auth-url",
		"containerImage":        ContainerImage,
		"serviceAccount":        "nova-sa",
		"registeredCells":       map[string]string{},
		"memcachedInstance":     MemcachedInstance,
	}
}

func CreateNovaScheduler(name types.NamespacedName, spec map[string]any) client.Object {
	raw := map[string]any{
		"apiVersion": "nova.openstack.org/v1beta1",
		"kind":       "NovaScheduler",
		"metadata": map[string]any{
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

type CellNames struct {
	CellName                         string
	CellCRName                       types.NamespacedName
	MariaDBDatabaseName              types.NamespacedName
	MariaDBAccountName               types.NamespacedName
	APIDatabaseAccountName           types.NamespacedName
	ConductorName                    types.NamespacedName
	DBSyncJobName                    types.NamespacedName
	ConductorConfigDataName          types.NamespacedName
	ConductorScriptDataName          types.NamespacedName
	ConductorStatefulSetName         types.NamespacedName
	TransportURLName                 types.NamespacedName
	CellMappingJobName               types.NamespacedName
	CellDeleteJobName                types.NamespacedName
	MetadataName                     types.NamespacedName
	MetadataStatefulSetName          types.NamespacedName
	MetadataConfigDataName           types.NamespacedName
	MetadataNeutronConfigDataName    types.NamespacedName
	NoVNCProxyName                   types.NamespacedName
	NoVNCProxyStatefulSetName        types.NamespacedName
	CellNoVNCProxyNameConfigDataName types.NamespacedName
	InternalCellSecretName           types.NamespacedName
	InternalAPINetworkNADName        types.NamespacedName
	ComputeConfigSecretName          types.NamespacedName
	NovaComputeName                  types.NamespacedName
	NovaComputeStatefulSetName       types.NamespacedName
	NovaComputeConfigDataName        types.NamespacedName
	HostDiscoveryJobName             types.NamespacedName
	DBPurgeCronJobName               types.NamespacedName
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
	metadataName := types.NamespacedName{
		Namespace: novaName.Namespace,
		Name:      cellName.Name + "-metadata",
	}
	novncproxyName := types.NamespacedName{
		Namespace: novaName.Namespace,
		Name:      cellName.Name + "-novncproxy",
	}
	novaCompute := types.NamespacedName{
		Namespace: novaName.Namespace,
		Name:      cellName.Name + "-" + ironicComputeName + "-compute",
	}

	c := CellNames{
		CellName:   cell,
		CellCRName: cellName,
		MariaDBDatabaseName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      "nova-" + cell,
		},
		MariaDBAccountName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      "test-nova-" + cell + "-account",
		},
		APIDatabaseAccountName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      "test-nova-api-account",
		},
		ConductorName: cellConductor,
		DBSyncJobName: types.NamespacedName{
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
		CellDeleteJobName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      cellName.Name + "-cell-delete",
		},
		ConductorConfigDataName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      cellConductor.Name + "-config-data",
		},
		ConductorScriptDataName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      cellConductor.Name + "-scripts",
		},
		MetadataName:            metadataName,
		MetadataStatefulSetName: metadataName,
		MetadataConfigDataName: types.NamespacedName{
			Namespace: metadataName.Namespace,
			Name:      metadataName.Name + "-config-data",
		},
		MetadataNeutronConfigDataName: types.NamespacedName{
			Namespace: metadataName.Namespace,
			Name:      metadataName.Name + "-neutron-config",
		},
		NoVNCProxyName:            novncproxyName,
		NoVNCProxyStatefulSetName: novncproxyName,
		CellNoVNCProxyNameConfigDataName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      cellName.Name + "-novncproxy" + "-config-data",
		},
		NovaComputeName:            novaCompute,
		NovaComputeStatefulSetName: novaCompute,
		NovaComputeConfigDataName: types.NamespacedName{
			Namespace: novaCompute.Namespace,
			Name:      cellName.Name + "-" + ironicComputeName + "-compute" + "-config-data",
		},
		HostDiscoveryJobName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      cellName.Name + "-host-discover",
		},
		InternalCellSecretName: cellName,
		InternalAPINetworkNADName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      "internalapi",
		},
		ComputeConfigSecretName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      cellName.Name + "-compute-config",
		},
		DBPurgeCronJobName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      cellName.Name + "-db-purge",
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

type NotificationsBusNames struct {
	BusName          string
	TransportURLName types.NamespacedName
}

func GetNotificationsBusNames(novaName types.NamespacedName) NotificationsBusNames {
	return NotificationsBusNames{
		BusName: "rabbitmq-broadcaster",
		TransportURLName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      novaName.Name + "-notification-transport"},
	}
}

type NovaNames struct {
	Namespace                      string
	NovaName                       types.NamespacedName
	InternalNovaServiceName        types.NamespacedName
	PublicNovaServiceName          types.NamespacedName
	AdminNovaServiceName           types.NamespacedName
	KeystoneServiceName            types.NamespacedName
	APIName                        types.NamespacedName
	APIMariaDBDatabaseName         types.NamespacedName
	APIMariaDBDatabaseAccount      types.NamespacedName
	APIStatefulSetName             types.NamespacedName
	APIKeystoneEndpointName        types.NamespacedName
	APIConfigDataName              types.NamespacedName
	InternalCertSecretName         types.NamespacedName
	PublicCertSecretName           types.NamespacedName
	MTLSSecretName                 types.NamespacedName
	CaBundleSecretName             types.NamespacedName
	VNCProxyVencryptCertSecretName types.NamespacedName
	// refers internal API network for all Nova services (not just nova API)
	InternalAPINetworkNADName       types.NamespacedName
	SchedulerName                   types.NamespacedName
	SchedulerStatefulSetName        types.NamespacedName
	SchedulerConfigDataName         types.NamespacedName
	MetadataName                    types.NamespacedName
	MetadataStatefulSetName         types.NamespacedName
	MetadataNeutronConfigDataName   types.NamespacedName
	ServiceAccountName              types.NamespacedName
	RoleName                        types.NamespacedName
	RoleBindingName                 types.NamespacedName
	MetadataConfigDataName          types.NamespacedName
	InternalNovaMetadataServiceName types.NamespacedName
	InternalTopLevelSecretName      types.NamespacedName
	MemcachedNamespace              types.NamespacedName
	Cells                           map[string]CellNames
	NovaTopologies                  []types.NamespacedName
	KeystoneAPIName                 types.NamespacedName
}

func GetNovaNames(novaName types.NamespacedName, cellNames []string) NovaNames {
	// NOTE(bogdando): use random UUIDs instead of static "nova" part of names.
	// These **must** replicate existing Nova*/Dataplane controllers suffixing/prefixing logic.
	// While dynamic UUIDs also provide enhanced testing coverage for "synthetic" cases,
	// which could not be caught for normal names with static "nova" prefixes.
	novaAPI := types.NamespacedName{
		Namespace: novaName.Namespace,
		Name:      fmt.Sprintf("%s-api", novaName.Name),
	}
	novaScheduler := types.NamespacedName{
		Namespace: novaName.Namespace,
		Name:      fmt.Sprintf("%s-scheduler", novaName.Name),
	}
	novaMetadata := types.NamespacedName{
		Namespace: novaName.Namespace,
		Name:      fmt.Sprintf("%s-metadata", novaName.Name),
	}

	cells := map[string]CellNames{}
	for _, cellName := range cellNames {
		cells[cellName] = GetCellNames(novaName, cellName)
	}

	return NovaNames{
		Namespace: novaName.Namespace,
		NovaName:  novaName,
		InternalNovaServiceName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      "nova-internal",
		},
		PublicNovaServiceName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      "nova-public",
		},
		KeystoneServiceName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      "nova", // static value hardcoded in controller code
		},
		APIName: novaAPI,
		APIMariaDBDatabaseName: types.NamespacedName{
			Namespace: novaAPI.Namespace,
			Name:      "nova-api", // a static DB name for nova
		},
		APIMariaDBDatabaseAccount: types.NamespacedName{
			Namespace: novaAPI.Namespace,
			Name:      "test-nova-api-account",
		},
		APIStatefulSetName: novaAPI,
		APIKeystoneEndpointName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      "nova", // a static keystone endpoint name for nova
		},
		APIConfigDataName: types.NamespacedName{
			Namespace: novaAPI.Namespace,
			Name:      novaAPI.Name + "-config-data",
		},
		InternalCertSecretName: types.NamespacedName{
			Namespace: novaAPI.Namespace,
			Name:      "internal-tls-certs"},
		PublicCertSecretName: types.NamespacedName{
			Namespace: novaAPI.Namespace,
			Name:      "public-tls-certs"},
		MTLSSecretName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      "cert-memcached-mtls"},
		CaBundleSecretName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      "combined-ca-bundle"},
		VNCProxyVencryptCertSecretName: types.NamespacedName{
			Namespace: novaAPI.Namespace,
			Name:      "vencrypt-tls-certs"},
		InternalAPINetworkNADName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      "internalapi",
		},
		SchedulerName:            novaScheduler,
		SchedulerStatefulSetName: novaScheduler,
		SchedulerConfigDataName: types.NamespacedName{
			Namespace: novaScheduler.Namespace,
			Name:      novaScheduler.Name + "-config-data",
		},
		MetadataName:            novaMetadata,
		MetadataStatefulSetName: novaMetadata,
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
		MetadataConfigDataName: types.NamespacedName{
			Namespace: novaMetadata.Namespace,
			Name:      novaMetadata.Name + "-config-data",
		},
		MetadataNeutronConfigDataName: types.NamespacedName{
			Namespace: novaMetadata.Namespace,
			Name:      novaMetadata.Name + "-neutron-config",
		},
		InternalNovaMetadataServiceName: types.NamespacedName{
			Namespace: novaMetadata.Namespace,
			Name:      "nova-metadata-internal",
		},
		InternalTopLevelSecretName: novaName,
		MemcachedNamespace: types.NamespacedName{
			Name:      MemcachedInstance,
			Namespace: novaName.Namespace,
		},
		Cells: cells,
		NovaTopologies: []types.NamespacedName{
			{
				Namespace: novaName.Namespace,
				Name:      "nova",
			},
			{
				Namespace: novaName.Namespace,
				Name:      "novaapi",
			},
			{
				Namespace: novaName.Namespace,
				Name:      "novascheduler",
			},
			{
				Namespace: novaName.Namespace,
				Name:      "novametadata",
			},
			{
				Namespace: novaName.Namespace,
				Name:      "nova-cell0",
			},
			{
				Namespace: novaName.Namespace,
				Name:      "nova-cell1",
			},
		},
	}
}

func CreateNovaMetadata(name types.NamespacedName, spec map[string]any) client.Object {
	raw := map[string]any{
		"apiVersion": "nova.openstack.org/v1beta1",
		"kind":       "NovaMetadata",
		"metadata": map[string]any{
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

func CreateInternalTopLevelSecret(novaNames NovaNames) *corev1.Secret {
	return th.CreateSecret(
		novaNames.InternalTopLevelSecretName,
		map[string][]byte{
			"ServicePassword":            []byte("service-password"),
			"MetadataSecret":             []byte("metadata-secret"),
			"transport_url":              []byte("rabbit://api/fake"),
			"notification_transport_url": []byte("rabbit://notifications/fake"),
		},
	)
}

func GetDefaultNovaMetadataSpec(secretName types.NamespacedName) map[string]any {
	return map[string]any{
		"secret":               secretName.Name,
		"apiDatabaseAccount":   novaNames.APIMariaDBDatabaseAccount.Name,
		"apiDatabaseHostname":  "nova-api-db-hostname",
		"cellDatabaseHostname": "nova-cell-db-hostname",
		"containerImage":       ContainerImage,
		"keystoneAuthURL":      "keystone-auth-url",
		"serviceAccount":       "nova-sa",
		"memcachedInstance":    MemcachedInstance,
		"cellDatabaseAccount":  cell0.MariaDBAccountName.Name,
	}
}

func AssertMetadataDoesNotExist(name types.NamespacedName) {
	instance := &novav1.NovaMetadata{}
	Eventually(func(g Gomega) {
		err := k8sClient.Get(ctx, name, instance)
		g.Expect(k8s_errors.IsNotFound(err)).To(BeTrue())
	}, timeout, interval).Should(Succeed())
}

func CreateNovaNoVNCProxy(name types.NamespacedName, spec map[string]any) client.Object {
	raw := map[string]any{
		"apiVersion": "nova.openstack.org/v1beta1",
		"kind":       "NovaNoVNCProxy",
		"metadata": map[string]any{
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

func GetDefaultNovaNoVNCProxySpec(cell CellNames) map[string]any {
	return map[string]any{
		"secret":               cell.InternalCellSecretName.Name,
		"apiDatabaseAccount":   novaNames.APIMariaDBDatabaseAccount.Name,
		"cellDatabaseHostname": "nova-cell-db-hostname",
		"containerImage":       ContainerImage,
		"keystoneAuthURL":      "keystone-auth-url",
		"serviceAccount":       "nova-sa",
		"cellName":             cell.CellName,
		"memcachedInstance":    MemcachedInstance,
		"cellDatabaseAccount":  cell.MariaDBAccountName.Name,
	}
}

func CreateCellInternalSecret(cell CellNames, additionalValues map[string][]byte) *corev1.Secret {

	secretMap := map[string][]byte{
		"ServicePassword":            []byte("service-password"),
		"transport_url":              fmt.Appendf(nil, "rabbit://%s/fake", cell.CellName),
		"notification_transport_url": []byte("rabbit://notifications/fake"),
	}
	// (ksambor) this can be replaced with maps.Copy directly from maps
	// not experimental package when we move to go 1.21
	maps.Copy(secretMap, additionalValues)
	return th.CreateSecret(
		cell.InternalCellSecretName,
		secretMap,
	)
}

func CreateMetadataCellInternalSecret(cell CellNames) *corev1.Secret {
	metadataSecret := map[string][]byte{
		"MetadataSecret": []byte("metadata-secret"),
	}
	return CreateCellInternalSecret(cell, metadataSecret)
}

func CreateDefaultCellInternalSecret(cell CellNames) *corev1.Secret {
	return CreateCellInternalSecret(cell, map[string][]byte{})
}

func AssertNoVNCProxyDoesNotExist(name types.NamespacedName) {
	instance := &novav1.NovaNoVNCProxy{}
	Eventually(func(g Gomega) {
		err := k8sClient.Get(ctx, name, instance)
		g.Expect(k8s_errors.IsNotFound(err)).To(BeTrue())
	}, timeout, interval).Should(Succeed())
}

func CreateNovaCompute(name types.NamespacedName, spec map[string]any) client.Object {
	raw := map[string]any{
		"apiVersion": "nova.openstack.org/v1beta1",
		"kind":       "NovaCompute",
		"metadata": map[string]any{
			"name":      name.Name,
			"namespace": name.Namespace,
		},
		"spec": spec,
	}
	return th.CreateUnstructured(raw)
}

func GetNovaCompute(name types.NamespacedName) *novav1.NovaCompute {
	instance := &novav1.NovaCompute{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}

func NovaComputeConditionGetter(name types.NamespacedName) condition.Conditions {
	instance := GetNovaCompute(name)
	return instance.Status.Conditions
}

func GetDefaultNovaComputeTemplate() map[string]any {
	return map[string]any{
		"computeDriver": novav1.IronicDriver,
		"name":          ironicComputeName,
	}
}

func GetDefaultNovaComputeSpec(cell CellNames) map[string]any {
	return map[string]any{
		"secret":               cell.InternalCellSecretName.Name,
		"apiDatabaseAccount":   novaNames.APIMariaDBDatabaseAccount.Name,
		"computeName":          "compute1",
		"cellDatabaseHostname": "nova-cell-db-hostname",
		"containerImage":       ContainerImage,
		"keystoneAuthURL":      "keystone-auth-url",
		"serviceAccount":       "nova",
		"cellName":             cell.CellName,
		"computeDriver":        novav1.IronicDriver,
		"cellDatabaseAccount":  cell.MariaDBAccountName.Name,
	}
}

func AssertComputeDoesNotExist(name types.NamespacedName) {
	instance := &novav1.NovaCompute{}
	Eventually(func(g Gomega) {
		err := k8sClient.Get(ctx, name, instance)
		g.Expect(k8s_errors.IsNotFound(err)).To(BeTrue())
	}, timeout, interval).Should(Succeed())
}

func GetCronJob(name types.NamespacedName) *batchv1.CronJob {
	cron := &batchv1.CronJob{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, cron)).Should(Succeed())
	}, timeout, interval).Should(Succeed())

	return cron
}

func SimulateReadyOfNovaTopServices() {
	keystone.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)
	th.SimulateJobSuccess(cell0.DBSyncJobName)
	th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
	th.SimulateJobSuccess(cell0.CellMappingJobName)

	Eventually(func(g Gomega) {
		th.SimulateStatefulSetReplicaReady(cell0.ConductorStatefulSetName)
		cell := GetNovaCell(cell0.CellCRName)
		g.Expect(cell.Status.Conditions.Get(condition.ReadyCondition).Status).To(
			Equal(corev1.ConditionTrue))
	}, timeout, interval).Should(Succeed())

	Eventually(func(g Gomega) {
		th.SimulateStatefulSetReplicaReady(novaNames.APIStatefulSetName)
		instance := &keystonev1.KeystoneEndpoint{}
		g.Expect(th.K8sClient.Get(th.Ctx, novaNames.APIKeystoneEndpointName, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())

	keystone.SimulateKeystoneEndpointReady(novaNames.APIKeystoneEndpointName)

	th.SimulateStatefulSetReplicaReady(novaNames.SchedulerStatefulSetName)
	th.SimulateStatefulSetReplicaReady(novaNames.MetadataStatefulSetName)
	th.SimulateStatefulSetReplicaReady(novaNames.APIStatefulSetName)
	Eventually(func(g Gomega) {
		nova := GetNova(novaNames.NovaName)
		g.Expect(nova.Status.APIServiceReadyCount).To(Equal(int32(1)))
		g.Expect(nova.Status.SchedulerServiceReadyCount).To(Equal(int32(1)))
		g.Expect(nova.Status.MetadataServiceReadyCount).To(Equal(int32(1)))
	}, timeout, interval).Should(Succeed())
}

// GetSampleTopologySpec - An opinionated Topology Spec sample used to
// test Nova components. It returns both the user input representation
// in the form of map[string]string, and the Golang expected representation
// used in the test asserts.
func GetSampleTopologySpec(label string) (map[string]any, []corev1.TopologySpreadConstraint) {
	// Build the topology Spec yaml representation
	topologySpec := map[string]any{
		"topologySpreadConstraints": []map[string]any{
			{
				"maxSkew":           1,
				"topologyKey":       corev1.LabelHostname,
				"whenUnsatisfiable": "ScheduleAnyway",
				"labelSelector": map[string]any{
					"matchLabels": map[string]any{
						"service": label,
					},
				},
			},
		},
	}
	// Build the topologyObj representation
	topologySpecObj := []corev1.TopologySpreadConstraint{
		{
			MaxSkew:           1,
			TopologyKey:       corev1.LabelHostname,
			WhenUnsatisfiable: corev1.ScheduleAnyway,
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"service": label,
				},
			},
		},
	}
	return topologySpec, topologySpecObj
}

func AssertNotHaveNotificationTransportURL(configData string) {

	Expect(configData).ToNot(
		ContainSubstring("[notifications]"))

	Expect(configData).To(
		ContainSubstring("[oslo_messaging_notifications]\ndriver = noop"))

}

func AssertHaveNotificationTransportURL(notificationsTransportURLName string, configData string) {

	expectedConf1 := "[notifications]\nnotify_on_state_change = vm_and_task_state\nnotification_format=both"

	expectedConf2 := fmt.Sprintf(
		"[oslo_messaging_notifications]\ntransport_url = rabbit://%s/fake\ndriver = messagingv2",
		notificationsTransportURLName)

	Expect(configData).To(
		ContainSubstring(expectedConf1))

	Expect(configData).To(
		ContainSubstring(expectedConf2))

}

func CreateNovaWithNCellsAndEnsureReady(cellNumber int, novaNames *NovaNames) {
	if cellNumber < 1 {
		panic("At least 1 cell, cell0 must required for Nova CR")
	}

	serviceSpec := corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 3306}}}
	cellTemplates := make(map[string]any)

	DeferCleanup(k8sClient.Delete, ctx, CreateNovaSecret(novaNames.NovaName.Namespace, SecretName))
	DeferCleanup(
		mariadb.DeleteDBService,
		mariadb.CreateDBService(novaNames.APIMariaDBDatabaseName.Namespace, novaNames.APIMariaDBDatabaseName.Name, serviceSpec))

	apiAccount, apiSecret := mariadb.CreateMariaDBAccountAndSecret(
		novaNames.APIMariaDBDatabaseAccount, mariadbv1.MariaDBAccountSpec{})
	DeferCleanup(k8sClient.Delete, ctx, apiAccount)
	DeferCleanup(k8sClient.Delete, ctx, apiSecret)

	for i := range cellNumber {
		cellName := fmt.Sprintf("cell%d", i)
		cell := novaNames.Cells[cellName]

		// rabbit and galera
		DeferCleanup(k8sClient.Delete, ctx, CreateNovaMessageBusSecret(cell))
		DeferCleanup(mariadb.DeleteDBService, mariadb.CreateDBService(cell.MariaDBDatabaseName.Namespace, cell.MariaDBDatabaseName.Name, serviceSpec))

		// account & Secret
		account, secret := mariadb.CreateMariaDBAccountAndSecret(cell.MariaDBAccountName, mariadbv1.MariaDBAccountSpec{})

		// Defer cleanup based on cell only for default cell (cell0), others might be deleted in caller test
		if i == 0 {
			DeferCleanup(k8sClient.Delete, ctx, account)
			DeferCleanup(k8sClient.Delete, ctx, secret)
		} else {
			logger.Info(fmt.Sprintf("Not creating defer cleanup for %s ...", cellName), " -- ", secret)
		}

		// Build cell template
		template := GetDefaultNovaCellTemplate()
		template["cellDatabaseInstance"] = cell.MariaDBDatabaseName.Name
		template["cellDatabaseAccount"] = account.Name
		if i != 0 {
			// cell0
			template["cellMessageBusInstance"] = cell.TransportURLName.Name
		}

		if i == 1 {
			// cell1
			template["novaComputeTemplates"] = map[string]any{
				ironicComputeName: GetDefaultNovaComputeTemplate(),
			}
		}
		if i != 0 && i != 1 {
			// cell2 ..
			template["hasAPIAccess"] = false
		}

		cellTemplates[cellName] = template
	}

	// Create Nova spec
	spec := GetDefaultNovaSpec()
	spec["cellTemplates"] = cellTemplates
	spec["apiDatabaseInstance"] = novaNames.APIMariaDBDatabaseName.Name
	spec["apiMessageBusInstance"] = novaNames.Cells["cell0"].TransportURLName.Name

	// Deploy Nova and simulate its dependencies
	DeferCleanup(th.DeleteInstance, CreateNova(novaNames.NovaName, spec))
	// DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(novaNames.NovaName.Namespace))
	novaNames.KeystoneAPIName = keystone.CreateKeystoneAPI(novaNames.NovaName.Namespace)
	DeferCleanup(keystone.DeleteKeystoneAPI, novaNames.KeystoneAPIName)

	// Set region on KeystoneAPI to ensure GetRegion() returns a value
	Eventually(func(g Gomega) {
		keystoneAPI := keystone.GetKeystoneAPI(novaNames.KeystoneAPIName)
		keystoneAPI.Spec.Region = "regionOne"
		g.Expect(k8sClient.Update(ctx, keystoneAPI)).To(Succeed())
		keystoneAPI.Status.Region = "regionOne"
		g.Expect(k8sClient.Status().Update(ctx, keystoneAPI)).To(Succeed())
	}, timeout, interval).Should(Succeed())

	memcachedSpec := infra.GetDefaultMemcachedSpec()
	DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(novaNames.NovaName.Namespace, MemcachedInstance, memcachedSpec))

	infra.SimulateMemcachedReady(novaNames.MemcachedNamespace)
	keystone.SimulateKeystoneServiceReady(novaNames.KeystoneServiceName)

	mariadb.SimulateMariaDBDatabaseCompleted(novaNames.APIMariaDBDatabaseName)
	mariadb.SimulateMariaDBAccountCompleted(novaNames.APIMariaDBDatabaseAccount)

	for i := range cellNumber {
		cell := novaNames.Cells[fmt.Sprintf("cell%d", i)]
		mariadb.SimulateMariaDBDatabaseCompleted(cell.MariaDBDatabaseName)
		mariadb.SimulateMariaDBAccountCompleted(cell.MariaDBAccountName)
		infra.SimulateTransportURLReady(cell.TransportURLName)

		if i != 0 {
			// cell0
			th.SimulateStatefulSetReplicaReady(cell.NoVNCProxyStatefulSetName)
		}

		th.SimulateJobSuccess(cell.DBSyncJobName)
		th.SimulateStatefulSetReplicaReady(cell.ConductorStatefulSetName)

		if i == 1 {
			// only cell1 will have computes
			th.SimulateStatefulSetReplicaReady(cell.NovaComputeStatefulSetName)
		}

		th.SimulateJobSuccess(cell.CellMappingJobName)

		if i == 1 {
			// run cell1 computes host discovery
			th.SimulateJobSuccess(cell1.HostDiscoveryJobName)
		}

	}

	// Final Ready condition check
	th.ExpectCondition(
		novaNames.NovaName,
		ConditionGetterFunc(NovaConditionGetter),
		novav1.NovaAllCellsReadyCondition,
		corev1.ConditionTrue,
	)
	SimulateReadyOfNovaTopServices()
	th.ExpectCondition(
		novaNames.NovaName,
		ConditionGetterFunc(NovaConditionGetter),
		condition.ReadyCondition,
		corev1.ConditionTrue,
	)
}
