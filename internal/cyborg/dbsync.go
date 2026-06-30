/*
Copyright 2026.

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

package cyborg

import (
	cyborgv1beta1 "github.com/openstack-k8s-operators/nova-operator/api/cyborg/v1beta1"

	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DbSyncJob creates the Job definition for running the Cyborg database migration
func DbSyncJob(
	instance *cyborgv1beta1.Cyborg,
	labels map[string]string,
	annotations map[string]string,
) *batchv1.Job {
	runAsUser := int64(0)
	completions := int32(1)
	parallelism := int32(1)
	var config0644AccessMode int32 = 0644

	dbSyncVolume := []corev1.Volume{
		{
			Name: "db-sync-config-data",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					DefaultMode: &config0644AccessMode,
					SecretName:  instance.Name + "-config-data",
					Items: []corev1.KeyToPath{
						{
							Key:  DefaultsConfigFileName,
							Path: DefaultsConfigFileName,
						},
					},
				},
			},
		},
	}

	dbSyncMounts := []corev1.VolumeMount{
		{
			Name:      "db-sync-config-data",
			MountPath: "/etc/cyborg/cyborg.conf.d",
			ReadOnly:  true,
		},
		{
			Name:      ConfigVolume,
			MountPath: "/var/lib/kolla/config_files/config.json",
			SubPath:   "cyborg-dbsync-config.json",
			ReadOnly:  true,
		},
	}

	volumes := []corev1.Volume{
		{
			Name: ConfigVolume,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					DefaultMode: &config0644AccessMode,
					SecretName:  instance.Name + "-config-data",
				},
			},
		},
	}
	volumes = append(volumes, dbSyncVolume...)

	volumeMounts := []corev1.VolumeMount{
		{
			Name:      ConfigVolume,
			MountPath: "/var/lib/config-data/default",
			ReadOnly:  true,
		},
		{
			Name:      ConfigVolume,
			MountPath: "/etc/my.cnf",
			SubPath:   "my.cnf",
			ReadOnly:  true,
		},
	}
	volumeMounts = append(volumeMounts, dbSyncMounts...)

	if instance.Spec.APIServiceTemplate.TLS.CaBundleSecretName != "" {
		volumes = append(volumes, instance.Spec.APIServiceTemplate.TLS.CreateVolume())
		volumeMounts = append(volumeMounts, instance.Spec.APIServiceTemplate.TLS.CreateVolumeMounts(nil)...)
	}

	args := []string{"-c", DBSyncCommand}

	envVars := make(map[string]env.Setter)
	envVars["KOLLA_CONFIG_STRATEGY"] = env.SetValue("COPY_ALWAYS")
	envVars["KOLLA_BOOTSTRAP"] = env.SetValue("TRUE")

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:        instance.Name + "-db-sync",
			Namespace:   instance.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: batchv1.JobSpec{
			Completions: &completions,
			Parallelism: &parallelism,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: annotations,
				},
				Spec: corev1.PodSpec{
					RestartPolicy:      corev1.RestartPolicyOnFailure,
					ServiceAccountName: "cyborg-" + instance.Name,
					Containers: []corev1.Container{
						{
							Name: "cyborg-db-sync",
							Command: []string{
								"/bin/bash",
							},
							Args:  args,
							Image: instance.Spec.ConductorContainerImageURL,
							SecurityContext: &corev1.SecurityContext{
								RunAsUser: &runAsUser,
							},
							Env:          env.MergeEnvs([]corev1.EnvVar{}, envVars),
							VolumeMounts: volumeMounts,
						},
					},
					Volumes: volumes,
				},
			},
		},
	}

	return job
}
