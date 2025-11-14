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
	scriptVolume = "scripts"
	configVolume = "config-data"
	logVolume    = "logs"
)

var (
	configMode int32 = 0640
	scriptMode int32 = 0740
)

// GetConfigVolumeMount returns a volume mount for Nova configuration files
func GetConfigVolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      configVolume,
		MountPath: "/var/lib/openstack/config",
		ReadOnly:  false,
	}
}

// GetKollaConfigVolumeMount returns a volume mount for Kolla configuration files
func GetKollaConfigVolumeMount(serviceName string) corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      configVolume,
		MountPath: "/var/lib/kolla/config_files/config.json",
		SubPath:   serviceName + "-config.json",
		ReadOnly:  false,
	}
}

// GetConfigVolume returns a volume for Nova configuration files from a secret
func GetConfigVolume(secretName string) corev1.Volume {
	return corev1.Volume{
		Name: configVolume,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				DefaultMode: &configMode,
				SecretName:  secretName,
			},
		},
	}
}

// GetLogVolumeMount returns a volume mount for Nova log files
func GetLogVolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      logVolume,
		MountPath: "/var/log/nova",
		ReadOnly:  false,
	}
}

// GetLogVolume returns an empty directory volume for Nova log files
func GetLogVolume() corev1.Volume {
	return corev1.Volume{
		Name: logVolume,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{Medium: ""},
		},
	}
}

// GetScriptVolumeMount returns a volume mount for Nova script files
func GetScriptVolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      scriptVolume,
		MountPath: "/var/lib/openstack/bin",
		ReadOnly:  false,
	}
}

// GetScriptVolume returns a volume for Nova script files from a secret
func GetScriptVolume(secretName string) corev1.Volume {
	return corev1.Volume{
		Name: scriptVolume,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				DefaultMode: &scriptMode,
				SecretName:  secretName,
			},
		},
	}
}
