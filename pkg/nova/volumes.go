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

package nova

import (
	corev1 "k8s.io/api/core/v1"
)

const (
	scriptVolume       = "scripts"
	configVolume       = "config-data"
	mergedConfigVolume = "config-data-merged"
	logVolume          = "logs"
)

// GetVolumes - returns the volumes used for the service deployment and for
// any jobs needs access for the full service configuration
func GetVolumes(scriptConfigMapName string, serviceConfigConfigMapName string) []corev1.Volume {
	var scriptMode int32 = 0740
	var configMode int32 = 0640

	return []corev1.Volume{
		{
			Name: scriptVolume,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					DefaultMode: &scriptMode,
					LocalObjectReference: corev1.LocalObjectReference{
						Name: scriptConfigMapName,
					},
				},
			},
		},
		{
			Name: configVolume,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					DefaultMode: &configMode,
					LocalObjectReference: corev1.LocalObjectReference{
						Name: serviceConfigConfigMapName,
					},
				},
			},
		},
		{
			Name: mergedConfigVolume,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{Medium: ""},
			},
		},
	}
}

// GetAllVolumeMounts - VolumeMounts providing access to both the raw input
// configuration and the volume of the merged configuration
func GetAllVolumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      scriptVolume,
			MountPath: "/usr/local/bin/container-scripts",
			ReadOnly:  true,
		},
		{
			Name:      configVolume,
			MountPath: "/var/lib/config-data/default",
			ReadOnly:  true,
		},
		{
			Name:      mergedConfigVolume,
			MountPath: "/var/lib/config-data/merged",
			ReadOnly:  false,
		},
	}
}

// GetServiceVolumeMounts - VolumeMounts to get access to the merged
// configuration
func GetServiceVolumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      scriptVolume,
			MountPath: "/usr/local/bin/container-scripts",
			ReadOnly:  true,
		},
		{
			Name:      mergedConfigVolume,
			MountPath: "/var/lib/config-data/merged",
			ReadOnly:  false,
		},
	}
}

// GetOpenstackVolumeMounts - VolumeMounts use to inject config and scripts
func GetOpenstackVolumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      configVolume,
			MountPath: "/var/lib/openstack/config",
			ReadOnly:  false,
		},
		{
			Name:      logVolume,
			MountPath: "/var/log/nova",
			ReadOnly:  false,
		},
	}
}

// GetOpenstackVolumes - returns the volumes used for the service deployment and for
// any jobs needs access for the full service configuration
func GetOpenstackVolumes(serviceConfigConfigMapName string) []corev1.Volume {
	var configMode int32 = 0640
	return []corev1.Volume{
		{
			Name: configVolume,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					DefaultMode: &configMode,
					LocalObjectReference: corev1.LocalObjectReference{
						Name: serviceConfigConfigMapName,
					},
				},
			},
		},
		{
			Name: logVolume,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{Medium: ""},
			},
		},
	}
}
