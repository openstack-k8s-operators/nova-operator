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

/// GetOpenstackVolumeMountsGeneric - VolumeMounts use to inject config and/or scripts

func getOpenstackVolumeMountsGeneric(with_scripts bool) []corev1.VolumeMount {
	mounts := []corev1.VolumeMount{
		{
			Name:      configVolume,
			MountPath: "/var/lib/openstack/config",
			ReadOnly:  true,
		},
		{
			Name:      logVolume,
			MountPath: "/var/log/nova",
			ReadOnly:  false,
		},
	}
	if with_scripts {
		mounts = append(mounts, corev1.VolumeMount{Name: scriptVolume, MountPath: "/var/lib/openstack/bin", ReadOnly: true})
	}
	return mounts
}

// GetOpenstackVolumeMounts - VolumeMounts use to inject config and scripts
func GetOpenstackVolumeMounts() []corev1.VolumeMount {
	return getOpenstackVolumeMountsGeneric(false)
}

// GetOpenstackVolumeMountsWithScripts - VolumeMounts use to inject config and scripts
func GetOpenstackVolumeMountsWithScripts() []corev1.VolumeMount {
	return getOpenstackVolumeMountsGeneric(true)
}

func getVolumesGeneric(scriptConfigMapName string, serviceConfigConfigMapName string) []corev1.Volume {
	var configMode int32 = 0640
	var scriptMode int32 = 0740
	vols := []corev1.Volume{
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
	if len(scriptConfigMapName) > 0 {
		vols = append(vols,
			corev1.Volume{
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
		)
	}
	return vols
}

// GetOpenstackVolumesWithScripts - returns the volumes used for the service deployment and for
// any jobs needs access for the full service configuration and scripts
func GetOpenstackVolumesWithScripts(scriptConfigMapName string, serviceConfigConfigMapName string) []corev1.Volume {
	return getVolumesGeneric(scriptConfigMapName, serviceConfigConfigMapName)
}

// GetOpenstackVolumes - returns the volumes used for the service deployment and for
// any jobs needs access for the full service configuration
func GetOpenstackVolumes(serviceConfigConfigMapName string) []corev1.Volume {
	return getVolumesGeneric("", serviceConfigConfigMapName)
}
