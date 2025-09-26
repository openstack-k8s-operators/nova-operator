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
	"fmt"
	. "github.com/onsi/gomega" //revive:disable:dot-imports

	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	placementv1 "github.com/openstack-k8s-operators/placement-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/placement-operator/pkg/placement"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Names struct {
	Namespace              string
	PlacementAPIName       types.NamespacedName
	ConfigMapName          types.NamespacedName
	DBSyncJobName          types.NamespacedName
	MariaDBDatabaseName    types.NamespacedName
	MariaDBAccount         types.NamespacedName
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
	PlacementAPITopologies []types.NamespacedName
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
		MariaDBAccount: types.NamespacedName{
			Namespace: placementAPIName.Namespace,
			Name:      AccountName},
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
		PlacementAPITopologies: []types.NamespacedName{
			{
				Namespace: namespace,
				Name:      fmt.Sprintf("%s-topology", placementAPIName.Name),
			},
			{
				Namespace: namespace,
				Name:      fmt.Sprintf("%s-topology-alt", placementAPIName.Name),
			},
		},
	}
}

func GetDefaultPlacementAPISpec() map[string]any {
	return map[string]any{
		"databaseInstance": "openstack",
		"secret":           SecretName,
		"databaseAccount":  AccountName,
	}
}

func GetTLSPlacementAPISpec(names Names) map[string]any {
	return map[string]any{
		"databaseInstance": "openstack",
		"replicas":         1,
		"secret":           SecretName,
		"databaseAccount":  AccountName,
		"tls": map[string]any{
			"api": map[string]any{
				"internal": map[string]any{
					"secretName": names.InternalCertSecretName.Name,
				},
				"public": map[string]any{
					"secretName": names.PublicCertSecretName.Name,
				},
			},
			"caBundleSecretName": names.CaBundleSecretName.Name,
		},
	}
}

func CreatePlacementAPI(name types.NamespacedName, spec map[string]any) client.Object {

	raw := map[string]any{
		"apiVersion": "placement.openstack.org/v1beta1",
		"kind":       "PlacementAPI",
		"metadata": map[string]any{
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

// GetSampleTopologySpec - A sample (and opinionated) Topology Spec used to
// test PlacementAPI
// Note this is just an example that should not be used in production for
// multiple reasons:
// 1. It uses ScheduleAnyway as strategy, which is something we might
// want to avoid by default
// 2. Usually a topologySpreadConstraints is used to take care about
// multi AZ, which is not applicable in this context
func GetSampleTopologySpec(label string) (map[string]any, []corev1.TopologySpreadConstraint) {
	// Build the topology Spec
	topologySpec := map[string]any{
		"topologySpreadConstraints": []map[string]any{
			{
				"maxSkew":           1,
				"topologyKey":       corev1.LabelHostname,
				"whenUnsatisfiable": "ScheduleAnyway",
				"labelSelector": map[string]any{
					"matchLabels": map[string]any{
						"service":  placement.ServiceName,
						"topology": label,
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
					"service":  placement.ServiceName,
					"topology": label,
				},
			},
		},
	}
	return topologySpec, topologySpecObj
}
