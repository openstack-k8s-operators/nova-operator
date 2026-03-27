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
package cyborg_test

import (
	"time"

	. "github.com/onsi/gomega" //revive:disable:dot-imports

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	cyborgv1beta1 "github.com/openstack-k8s-operators/nova-operator/api/cyborg/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	SecretName     = "external-secret"
	ContainerImage = "test://nova"
	timeout        = 45 * time.Second
	// have maximum 100 retries before the timeout hits
	interval = timeout / 100
)

func GetCronJob(name types.NamespacedName) *batchv1.CronJob {
	cron := &batchv1.CronJob{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, cron)).Should(Succeed())
	}, timeout, interval).Should(Succeed())

	return cron
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

func GetDefaultCyborgSpec() map[string]any {
	return map[string]any{
		"apiContainerImageURL":       CyborgContainerImage,
		"conductorContainerImageURL": CyborgConductorImage,
		"agentContainerImageURL":     CyborgAgentImage,
	}
}

func GetCyborgSpecWithTLSAndAppCred(apiTLSSecretName, caBundleSecretName, appCredSecretName string) map[string]any {
	spec := GetDefaultCyborgSpec()
	spec["auth"] = map[string]any{
		"applicationCredentialSecret": appCredSecretName,
	}
	spec["apiServiceTemplate"] = map[string]any{
		"tls": map[string]any{
			"api": map[string]any{
				"public": map[string]any{
					"secretName": apiTLSSecretName,
				},
			},
			"caBundleSecretName": caBundleSecretName,
		},
	}
	return spec
}

func CreateCyborg(name types.NamespacedName, spec map[string]any) client.Object {
	raw := map[string]any{
		"apiVersion": "cyborg.openstack.org/v1beta1",
		"kind":       "Cyborg",
		"metadata": map[string]any{
			"name":      name.Name,
			"namespace": name.Namespace,
		},
		"spec": spec,
	}
	return th.CreateUnstructured(raw)
}

func GetCyborg(name types.NamespacedName) *cyborgv1beta1.Cyborg {
	instance := &cyborgv1beta1.Cyborg{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}

func CyborgConditionGetter(name types.NamespacedName) condition.Conditions {
	instance := GetCyborg(name)
	return instance.Status.Conditions
}

func CreateCyborgSecret(namespace string) *corev1.Secret {
	return th.CreateSecret(
		types.NamespacedName{Namespace: namespace, Name: CyborgSecretName},
		map[string][]byte{
			CyborgPasswordSelectorValue: []byte("cyborg-service-password"),
		},
	)
}

func CreateCyborgMessageBusSecret(names CyborgNames) *corev1.Secret {
	return th.CreateSecret(
		types.NamespacedName{
			Namespace: names.TransportURLName.Namespace,
			Name:      "rabbitmq-secret",
		},
		map[string][]byte{
			"transport_url": []byte("rabbit://cyborg/fake"),
		},
	)
}

func GetCyborgConductor(name types.NamespacedName) *cyborgv1beta1.CyborgConductor {
	instance := &cyborgv1beta1.CyborgConductor{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}

func CyborgConductorConditionGetter(name types.NamespacedName) condition.Conditions {
	instance := GetCyborgConductor(name)
	return instance.Status.Conditions
}

func CreateKeystoneAPIForCyborg(namespace string) types.NamespacedName {
	keystoneAPIName := keystone.CreateKeystoneAPI(namespace)
	keystoneAPI := keystone.GetKeystoneAPI(keystoneAPIName)
	keystoneAPI.Spec.Region = "regionOne"
	Expect(k8sClient.Update(ctx, keystoneAPI)).To(Succeed())
	Eventually(func(g Gomega) {
		ks := keystone.GetKeystoneAPI(keystoneAPIName)
		ks.Status.Region = "regionOne"
		g.Expect(k8sClient.Status().Update(ctx, ks)).To(Succeed())
	}, timeout, interval).Should(Succeed())
	return keystoneAPIName
}

func SimulateCyborgPrerequisitesReady(names CyborgNames) {
	mariadb.SimulateMariaDBAccountCompleted(names.MariaDBAccountName)
	mariadb.SimulateMariaDBDatabaseCompleted(names.MariaDBDatabaseName)
	infra.SimulateTransportURLReady(names.TransportURLName)
	keystone.SimulateKeystoneServiceReady(names.KeystoneServiceName)
	th.SimulateJobSuccess(names.DBSyncJobName)
}

func CreateCyborgTopology(namespace, name string) *topologyv1.Topology {
	topology := &topologyv1.Topology{}
	topology.Name = name
	topology.Namespace = namespace
	topology.Spec.TopologySpreadConstraints = &[]corev1.TopologySpreadConstraint{
		{
			MaxSkew:           1,
			TopologyKey:       "kubernetes.io/hostname",
			WhenUnsatisfiable: corev1.ScheduleAnyway,
		},
	}
	Expect(k8sClient.Create(ctx, topology)).To(Succeed())
	return topology
}
