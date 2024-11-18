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

package novacompute

import (
	common "github.com/openstack-k8s-operators/lib-common/modules/common"
	affinity "github.com/openstack-k8s-operators/lib-common/modules/common/affinity"
	env "github.com/openstack-k8s-operators/lib-common/modules/common/env"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/nova-operator/pkg/nova"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

// StatefulSet - returns the StatefulSet definition for the nova-compute service
func StatefulSet(
	instance *novav1.NovaCompute,
	configHash string,
	labels map[string]string,
	annotations map[string]string,
) *appsv1.StatefulSet {
	// After the first successful startupProbe, livenessProbe takes over
	livenessProbe := &corev1.Probe{
		// TODO might need tuning
		TimeoutSeconds: 10,
		PeriodSeconds:  10,
	}
	readinessProbe := &corev1.Probe{
		// TODO might need tuning
		TimeoutSeconds: 5,
		PeriodSeconds:  5,
	}

	args := []string{"-c", nova.KollaServiceCommand}
	livenessProbe.Exec = &corev1.ExecAction{
		Command: []string{
			"/usr/bin/pgrep", "-r", "DRST", "nova-compute",
		},
	}

	readinessProbe.Exec = &corev1.ExecAction{
		Command: []string{
			"/usr/bin/pgrep", "-r", "DRST", "nova-compute",
		},
	}

	envVars := map[string]env.Setter{}
	envVars["KOLLA_CONFIG_STRATEGY"] = env.SetValue("COPY_ALWAYS")
	// NOTE(gibi): The statefulset does not use this hash directly. We store it
	// in the environment to trigger a Pod restart if any input of the
	// statefulset has changed. The k8s will trigger a restart automatically if
	// the env changes.
	envVars["CONFIG_HASH"] = env.SetValue(configHash)
	env := env.MergeEnvs([]corev1.EnvVar{}, envVars)

	// create Volume and VolumeMounts
	volumes := []corev1.Volume{
		nova.GetConfigVolume(nova.GetServiceConfigSecretName(instance.Name)),
	}
	volumeMounts := []corev1.VolumeMount{
		nova.GetConfigVolumeMount(),
		nova.GetKollaConfigVolumeMount("nova-compute"),
	}

	// add CA cert if defined
	if instance.Spec.TLS.CaBundleSecretName != "" {
		volumes = append(volumes, instance.Spec.TLS.CreateVolume())
		volumeMounts = append(volumeMounts, instance.Spec.TLS.CreateVolumeMounts(nil)...)
	}

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
					Volumes:            volumes,
					Containers: []corev1.Container{
						{
							Name: instance.Name + "-compute",
							Command: []string{
								"/bin/bash",
							},
							Args:  args,
							Image: instance.Spec.ContainerImage,
							SecurityContext: &corev1.SecurityContext{
								RunAsUser: ptr.To(nova.NovaUserID),
							},
							Env:            env,
							VolumeMounts:   volumeMounts,
							Resources:      instance.Spec.Resources,
							ReadinessProbe: readinessProbe,
							LivenessProbe:  livenessProbe,
						},
					},
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

	if instance.Spec.NodeSelector != nil {
		statefulset.Spec.Template.Spec.NodeSelector = *instance.Spec.NodeSelector
	}

	return statefulset
}
