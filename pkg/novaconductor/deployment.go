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

package novaconductor

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
	instance *novav1.NovaConductor,
	configHash string,
	labels map[string]string,
	annotations map[string]string,
) *appsv1.StatefulSet {
	runAsUser := int64(0)

	initContainerDetails := ContainerInput{
		ContainerImage:       instance.Spec.ContainerImage,
		CellDatabaseHostname: instance.Spec.CellDatabaseHostname,
		CellDatabaseUser:     instance.Spec.CellDatabaseUser,
		CellDatabaseName:     "nova_" + instance.Spec.CellName,
		Secret:               instance.Spec.Secret,
		// NOTE(gibi): this is a hack until we implement proper secret handling
		// per cell
		CellDatabasePasswordSelector:        "NovaCell0DatabasePassword",
		KeystoneServiceUserPasswordSelector: instance.Spec.PasswordSelectors.Service,
		// NOTE(gibi): these might be empty if the conductor does not support
		// upcalls but that is OK
		APIDatabaseHostname:         instance.Spec.APIDatabaseHostname,
		APIDatabaseUser:             instance.Spec.APIDatabaseUser,
		APIDatabaseName:             nova.NovaAPIDatabaseName,
		APIDatabasePasswordSelector: instance.Spec.PasswordSelectors.APIDatabase,
		VolumeMounts:                nova.GetAllVolumeMounts(),
		CellMessageBusSecretName:    instance.Spec.CellMessageBusSecretName,
	}

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
				"/usr/bin/true",
			},
		}

		readinessProbe.Exec = &corev1.ExecAction{
			Command: []string{
				"/usr/bin/true",
			},
		}
	} else {
		args = append(args, nova.KollaServiceCommand)
		// TODO(gibi): replace this with a proper healthcheck once
		// https://review.opendev.org/q/topic:per-process-healthchecks merges.
		// NOTE(gibi): -r DRST means we consider nova-conductor processes
		// healthy if they are not in zombie state.
		livenessProbe.Exec = &corev1.ExecAction{
			Command: []string{
				"/usr/bin/pgrep", "-r", "DRST", "nova-conductor",
			},
		}

		readinessProbe.Exec = &corev1.ExecAction{
			Command: []string{
				"/usr/bin/pgrep", "-r", "DRST", "nova-conductor",
			},
		}
	}

	nodeSelector := map[string]string{}
	if instance.Spec.NodeSelector != nil {
		nodeSelector = instance.Spec.NodeSelector
	}

	envVars := map[string]env.Setter{}
	envVars["KOLLA_CONFIG_FILE"] = env.SetValue(MergedServiceConfigPath)
	envVars["KOLLA_CONFIG_STRATEGY"] = env.SetValue("COPY_ALWAYS")
	// NOTE(gibi): The stateafulset does not use this hash directly. We store it
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
					Annotations: annotations,
					Labels:      labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: nova.ServiceAccount,
					Volumes: nova.GetVolumes(
						nova.GetScriptConfigMapName(instance.Name),
						nova.GetServiceConfigConfigMapName(instance.Name),
					),
					InitContainers: initContainer(initContainerDetails),
					Containers: []corev1.Container{
						{
							Name: instance.Name + "-conductor",
							Command: []string{
								"/bin/bash",
							},
							Args:  args,
							Image: instance.Spec.ContainerImage,
							SecurityContext: &corev1.SecurityContext{
								RunAsUser: &runAsUser,
							},
							Env:            env,
							VolumeMounts:   nova.GetServiceVolumeMounts(),
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
