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
	env "github.com/openstack-k8s-operators/lib-common/modules/common/env"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/nova-operator/pkg/nova"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func HostDiscoveryJob(
	instance *novav1.NovaCell,
	novacompute *novav1.NovaCompute,
	configName string,
	scriptName string,
	inputHash string,
	labels map[string]string,
) *batchv1.Job {
	runAsUser := int64(0)

	args := []string{
		"-c",
		"/usr/local/bin/kolla_set_configs && /var/lib/openstack/bin/host_discover.sh",
	}

	envVars := map[string]env.Setter{}
	envVars["KOLLA_CONFIG_STRATEGY"] = env.SetValue("COPY_ALWAYS")
	envVars["KOLLA_BOOTSTRAP"] = env.SetValue("true")
	envVars["INPUT_HASH"] = env.SetValue(inputHash)

	env := env.MergeEnvs([]corev1.EnvVar{}, envVars)

	jobName := instance.Name + "-host-discover"

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: instance.Namespace,
			Labels:    labels,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy:      corev1.RestartPolicyOnFailure,
					ServiceAccountName: instance.Spec.ServiceAccount,
					Volumes: []corev1.Volume{
						nova.GetConfigVolume(configName),
						nova.GetScriptVolume(scriptName),
					},
					Containers: []corev1.Container{
						{
							Name: "nova-manage",
							Command: []string{
								"/bin/bash",
							},
							Args:  args,
							Image: novacompute.Spec.ContainerImage,
							SecurityContext: &corev1.SecurityContext{
								RunAsUser: &runAsUser,
							},
							Env: env,
							VolumeMounts: []corev1.VolumeMount{
								nova.GetConfigVolumeMount(),
								nova.GetScriptVolumeMount(),
								nova.GetKollaConfigVolumeMount("nova-manage"),
							},
						},
					},
				},
			},
		},
	}

	return job
}
