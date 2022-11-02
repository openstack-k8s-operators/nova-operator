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

package novaapi

import (
	common "github.com/openstack-k8s-operators/lib-common/modules/common"
	affinity "github.com/openstack-k8s-operators/lib-common/modules/common/affinity"
	env "github.com/openstack-k8s-operators/lib-common/modules/common/env"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/nova-operator/pkg/nova"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// StatefulSet - returns the StatefulSet definition for the nova-api service
func StatefulSet(
	instance *novav1.NovaAPI,
	configHash string,
	labels map[string]string,
) *appsv1.StatefulSet {
	runAsUser := int64(0)

	livenessProbe := &corev1.Probe{
		// TODO might need tuning
		TimeoutSeconds:      5,
		PeriodSeconds:       3,
		InitialDelaySeconds: 3,
	}
	readinessProbe := &corev1.Probe{
		// TODO might need tuning
		TimeoutSeconds:      5,
		PeriodSeconds:       5,
		InitialDelaySeconds: 5,
	}

	args := []string{"-c"}
	if instance.Spec.Debug.StopService {
		args = append(args, common.DebugCommand)
		livenessProbe.Exec = &corev1.ExecAction{
			Command: []string{
				"/bin/true",
			},
		}

		readinessProbe.Exec = &corev1.ExecAction{
			Command: []string{
				"/bin/true",
			},
		}
	} else {
		args = append(args, nova.KollaServiceCommand)
		// TODO(gibi): user the proper http-get probes once the service is
		// exposed
		livenessProbe.Exec = &corev1.ExecAction{
			Command: []string{
				"/bin/true",
			},
		}

		readinessProbe.Exec = &corev1.ExecAction{
			Command: []string{
				"/bin/true",
			},
		}
		// //
		// // https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/
		// //
		// livenessProbe.HTTPGet = &corev1.HTTPGetAction{
		// 	Port: intstr.IntOrString{Type: intstr.Int, IntVal: int32(APIServicePort)},
		// }
		// readinessProbe.HTTPGet = &corev1.HTTPGetAction{
		// 	Port: intstr.IntOrString{Type: intstr.Int, IntVal: int32(APIServicePort)},
		// }
	}

	nodeSelector := map[string]string{}
	if instance.Spec.NodeSelector != nil {
		nodeSelector = instance.Spec.NodeSelector
	}

	envVars := map[string]env.Setter{}
	envVars["KOLLA_CONFIG_FILE"] = env.SetValue(MergedServiceConfigPath)
	envVars["KOLLA_CONFIG_STRATEGY"] = env.SetValue("COPY_ALWAYS")
	// NOTE(gibi): The statefulset does not use this hash directly. We store it
	// in the environment to trigger a Pod restart if any input of the
	// statefulset has changed. The k8s will trigger a restart automatically if
	// the env changes.
	envVars["CONFIG_HASH"] = env.SetValue(configHash)
	env := env.MergeEnvs([]corev1.EnvVar{}, envVars)

	statefulset := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
		},
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Replicas: &instance.Spec.Replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: nova.ServiceAccount,
					Volumes: nova.GetVolumes(
						nova.GetScriptConfigMapName(instance.Name),
						nova.GetServiceConfigConfigMapName(instance.Name),
					),
					Containers: []corev1.Container{
						{
							Name: instance.Name + "-api",
							Command: []string{
								"/bin/bash",
							},
							Args:  args,
							Image: instance.Spec.ContainerImage,
							SecurityContext: &corev1.SecurityContext{
								RunAsUser: &runAsUser,
							},
							Env:            env,
							VolumeMounts:   nova.GetOpenstackVolumeMounts(),
							Resources:      instance.Spec.Resources,
							ReadinessProbe: readinessProbe,
							LivenessProbe:  livenessProbe,
						},
					},
					NodeSelector: nodeSelector,
					// If possible two pods of the same service should not
					// run on the same worker node. If this is not possible
					// the get still created on the same worker node.
					Affinity: affinity.DistributePods(
						common.AppSelector,
						[]string{
							instance.Name,
						},
						corev1.LabelHostname,
					),
				},
			},
		},
	}

	return statefulset
}
