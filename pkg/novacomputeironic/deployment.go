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

package novacomputeironic

import (
	common "github.com/openstack-k8s-operators/lib-common/modules/common"
	affinity "github.com/openstack-k8s-operators/lib-common/modules/common/affinity"
	env "github.com/openstack-k8s-operators/lib-common/modules/common/env"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/nova-operator/pkg/nova"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// StatefulSet - returns the StatefulSet definition for the nova-compute-ironic service
func StatefulSet(
	instance *novav1.NovaComputeIronic,
	configHash string,
	labels map[string]string,
	annotations map[string]string,
) *appsv1.StatefulSet {
	runAsUser := int64(0)

	// This allows the pod to start up slowly. The pod will only be killed
	// if it does not succeed a probe in 60 seconds.
	startupProbe := &corev1.Probe{
		FailureThreshold: 6,
		PeriodSeconds:    10,
	}
	// After the first successful startupProbe, livenessProbe takes over
	livenessProbe := &corev1.Probe{
		// TODO might need tuning
		TimeoutSeconds: 30,
		PeriodSeconds:  30,
	}
	readinessProbe := &corev1.Probe{
		// TODO might need tuning
		TimeoutSeconds: 30,
		PeriodSeconds:  30,
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
		startupProbe.Exec = &corev1.ExecAction{
			Command: []string{
				"/bin/true",
			},
		}
	} else {
		args = append(args, nova.KollaServiceCommand)
		livenessProbe.HTTPGet = &corev1.HTTPGetAction{
			Port: intstr.IntOrString{Type: intstr.Int, IntVal: int32(APIServicePort)},
		}
		readinessProbe.HTTPGet = &corev1.HTTPGetAction{
			Port: intstr.IntOrString{Type: intstr.Int, IntVal: int32(APIServicePort)},
		}
		startupProbe.HTTPGet = &corev1.HTTPGetAction{
			Port: intstr.IntOrString{Type: intstr.Int, IntVal: int32(APIServicePort)},
		}
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
			PodManagementPolicy: appsv1.ParallelPodManagement,
			Replicas:            instance.Spec.Replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: annotations,
					Labels:      labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: instance.Spec.ServiceAccount,
					Volumes: []corev1.Volume{
						nova.GetConfigVolume(nova.GetServiceConfigSecretName(instance.Name)),
						nova.GetLogVolume(),
					},
					Containers: []corev1.Container{
						// the first container in a pod is the default selected
						// by oc log so define the log stream container first.
						{
							Name: instance.Name + "-log",
							Command: []string{
								"/bin/bash",
							},
							Args:  []string{"-c", "tail -n+1 -F /var/log/nova/nova-compute-ironic.log"},
							Image: instance.Spec.ContainerImage,
							SecurityContext: &corev1.SecurityContext{
								RunAsUser: &runAsUser,
							},
							Env:            env,
							VolumeMounts:   []corev1.VolumeMount{nova.GetLogVolumeMount()},
							Resources:      instance.Spec.Resources,
							StartupProbe:   startupProbe,
							ReadinessProbe: readinessProbe,
							LivenessProbe:  livenessProbe,
						},
						{
							Name: instance.Name + "-computeironic",
							Command: []string{
								"/bin/bash",
							},
							Args:  args,
							Image: instance.Spec.ContainerImage,
							SecurityContext: &corev1.SecurityContext{
								RunAsUser: &runAsUser,
							},
							Env: env,
							VolumeMounts: []corev1.VolumeMount{
								nova.GetConfigVolumeMount(),
								nova.GetLogVolumeMount(),
							},
							Resources:      instance.Spec.Resources,
							StartupProbe:   startupProbe,
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
