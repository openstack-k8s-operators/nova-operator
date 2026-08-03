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
	"fmt"

	"github.com/openstack-k8s-operators/lib-common/modules/common/volume"
	corev1 "k8s.io/api/core/v1"
)

const (
	scriptVolume = "scripts"
	// ConfigVolume is the name of the volume for nova config data
	ConfigVolume = "config-data"
	logVolume    = "logs"
)

var (
	configMode int32 = 0440
	// scriptMode grants execute to the FSGroup-matched group, not just the
	// (root-owned, per kubelet) file owner -- scripts are exec'd directly
	// from this mount now that kolla no longer copies them elsewhere with
	// its own chmod first.
	scriptMode int32 = 0750
)

// GetConfVolumeMounts returns the final-path SubPath mounts for the config
// snippets shared by every Nova service: nova.conf, nova.conf.d/01-nova.conf,
// nova.conf.d/02-nova-override.conf (only when the service has a
// CustomServiceConfig set, since that key is only added to the config Secret
// in that case), and /etc/my.cnf.
func GetConfVolumeMounts(hasCustomServiceConfig bool, mountMyCnf ...bool) []corev1.VolumeMount {
	vm := []corev1.VolumeMount{
		{
			Name:      ConfigVolume,
			MountPath: "/etc/nova/nova.conf",
			SubPath:   "nova-blank.conf",
			ReadOnly:  true,
		},
		{
			Name:      ConfigVolume,
			MountPath: "/etc/nova/nova.conf.d/01-nova.conf",
			SubPath:   "01-nova.conf",
			ReadOnly:  true,
		},
	}
	if hasCustomServiceConfig {
		vm = append(vm, corev1.VolumeMount{
			Name:      ConfigVolume,
			MountPath: "/etc/nova/nova.conf.d/02-nova-override.conf",
			SubPath:   "02-nova-override.conf",
			ReadOnly:  true,
		})
	}
	if len(mountMyCnf) == 0 || mountMyCnf[0] {
		vm = append(vm, corev1.VolumeMount{
			Name:      ConfigVolume,
			MountPath: "/etc/my.cnf",
			SubPath:   "my.cnf",
			ReadOnly:  true,
		})
	}
	return vm
}

// GetConfigOverwriteVolumeMounts returns SubPath volume mounts that place
// each defaultConfigOverwrite key as an individual file under basePath
// (e.g. /etc/nova/policy.yaml, /etc/nova/api-paste.ini). The overwrite data
// lives in the same config Secret (merged in via CustomData).
func GetConfigOverwriteVolumeMounts(overwriteKeys []string, basePath string) []corev1.VolumeMount {
	mounts := make([]corev1.VolumeMount, 0, len(overwriteKeys))
	for _, key := range overwriteKeys {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      ConfigVolume,
			MountPath: fmt.Sprintf("%s/%s", basePath, key),
			SubPath:   key,
			ReadOnly:  true,
		})
	}
	return mounts
}

// GetConfigVolume returns a volume for Nova configuration files from a secret
func GetConfigVolume(secretName string) corev1.Volume {
	return corev1.Volume{
		Name: ConfigVolume,
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
	return volume.WritableDirVolume(logVolume)
}

// GetScriptVolumeMount returns a volume mount for Nova script files
func GetScriptVolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      scriptVolume,
		MountPath: "/usr/local/bin/container-scripts",
		ReadOnly:  true,
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
