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
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	placementv1 "github.com/openstack-k8s-operators/placement-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/placement-operator/pkg/placement"
)

type Names struct {
	Namespace              string
	PlacementAPIName       types.NamespacedName
	ConfigMapName          types.NamespacedName
	DBSyncJobName          types.NamespacedName
	MariaDBDatabaseName    types.NamespacedName
	DeploymentName         types.NamespacedName
	PublicServiceName      types.NamespacedName
	InternalServiceName    types.NamespacedName
	KeystoneServiceName    types.NamespacedName
	KeystoneEndpointName   types.NamespacedName
	ServiceAccountName     types.NamespacedName
	RoleName               types.NamespacedName
	RoleBindingName        types.NamespacedName
	CaBundleSecretName     types.NamespacedName
	InternalCertSecretName types.NamespacedName
	PublicCertSecretName   types.NamespacedName
}

func CreateNames(placementAPIName types.NamespacedName) Names {
	return Names{
		Namespace:        placementAPIName.Namespace,
		PlacementAPIName: placementAPIName,
		ConfigMapName: types.NamespacedName{
			Namespace: placementAPIName.Namespace,
			Name:      placementAPIName.Name + "-config-data"},
		DBSyncJobName: types.NamespacedName{
			Namespace: placementAPIName.Namespace,
			Name:      placementAPIName.Name + "-db-sync"},
		MariaDBDatabaseName: types.NamespacedName{
			Namespace: placementAPIName.Namespace,
			Name:      placement.DatabaseName},
		DeploymentName: types.NamespacedName{
			Namespace: placementAPIName.Namespace,
			Name:      placementAPIName.Name},
		PublicServiceName: types.NamespacedName{
			Namespace: placementAPIName.Namespace,
			Name:      "placement-public"},
		InternalServiceName: types.NamespacedName{
			Namespace: placementAPIName.Namespace,
			Name:      "placement-internal"},
		KeystoneServiceName: types.NamespacedName{
			Namespace: placementAPIName.Namespace,
			Name:      "placement"},
		KeystoneEndpointName: types.NamespacedName{
			Namespace: placementAPIName.Namespace,
			Name:      "placement"},
		ServiceAccountName: types.NamespacedName{
			Namespace: placementAPIName.Namespace,
			Name:      "placement-" + placementAPIName.Name},
		RoleName: types.NamespacedName{
			Namespace: placementAPIName.Namespace,
			Name:      "placement-" + placementAPIName.Name + "-role"},
		RoleBindingName: types.NamespacedName{
			Namespace: placementAPIName.Namespace,
			Name:      "placement-" + placementAPIName.Name + "-rolebinding"},
		CaBundleSecretName: types.NamespacedName{
			Namespace: placementAPIName.Namespace,
			Name:      CABundleSecretName},
		InternalCertSecretName: types.NamespacedName{
			Namespace: placementAPIName.Namespace,
			Name:      InternalCertSecretName},
		PublicCertSecretName: types.NamespacedName{
			Namespace: placementAPIName.Namespace,
			Name:      PublicCertSecretName},
	}
}

func GetDefaultPlacementAPISpec() map[string]interface{} {
	return map[string]interface{}{
		"databaseInstance": "openstack",
		"secret":           SecretName,
	}
}

func GetTLSPlacementAPISpec(names Names) map[string]interface{} {
	return map[string]interface{}{
		"databaseInstance": "openstack",
		"replicas":         1,
		"secret":           SecretName,
		"tls": map[string]interface{}{
			"api": map[string]interface{}{
				"internal": map[string]interface{}{
					"secretName": names.InternalCertSecretName.Name,
				},
				"public": map[string]interface{}{
					"secretName": names.PublicCertSecretName.Name,
				},
			},
			"caBundleSecretName": names.CaBundleSecretName.Name,
		},
	}
}

func CreatePlacementAPI(name types.NamespacedName, spec map[string]interface{}) client.Object {

	raw := map[string]interface{}{
		"apiVersion": "placement.openstack.org/v1beta1",
		"kind":       "PlacementAPI",
		"metadata": map[string]interface{}{
			"name":      name.Name,
			"namespace": name.Namespace,
		},
		"spec": spec,
	}
	return th.CreateUnstructured(raw)
}

func GetPlacementAPI(name types.NamespacedName) *placementv1.PlacementAPI {
	instance := &placementv1.PlacementAPI{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}

func CreatePlacementAPISecret(namespace string, name string) *corev1.Secret {
	return th.CreateSecret(
		types.NamespacedName{Namespace: namespace, Name: name},
		map[string][]byte{
			"PlacementPassword":         []byte("12345678"),
			"PlacementDatabasePassword": []byte("12345678"),
		},
	)
}

func PlacementConditionGetter(name types.NamespacedName) condition.Conditions {
	instance := GetPlacementAPI(name)
	return instance.Status.Conditions
}
