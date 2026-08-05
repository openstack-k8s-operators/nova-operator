/*

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
	corev1 "k8s.io/api/core/v1"
)

var configMode int32 = 0440

// GetConfigVolume returns a volume for Cyborg configuration files from a secret
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

// GetConfVolumeMounts returns the final-path SubPath mounts for the config
// snippets shared by cyborg-api/cyborg-conductor: cyborg.conf.d/00-default.conf,
// cyborg.conf.d/01-service-custom.conf (only when the service has a
// CustomServiceConfig set, since that key is only added to the config Secret
// in that case), and /etc/my.cnf. Neither service needs a primary
// /etc/cyborg/cyborg.conf file -- both discover config purely from
// --config-dir/oslo.config's default config-dir search, matching what
// cyborg-conductor's own kolla config.json command already did
// (--config-dir /etc/cyborg/cyborg.conf.d) and cyborg-api's WSGI app relies
// on implicitly (its config.json never copied a primary cyborg.conf either).
func GetConfVolumeMounts(hasCustomServiceConfig bool) []corev1.VolumeMount {
	vm := []corev1.VolumeMount{
		{
			Name:      ConfigVolume,
			MountPath: "/etc/cyborg/cyborg.conf.d/00-default.conf",
			SubPath:   DefaultsConfigFileName,
			ReadOnly:  true,
		},
	}
	if hasCustomServiceConfig {
		vm = append(vm, corev1.VolumeMount{
			Name:      ConfigVolume,
			MountPath: "/etc/cyborg/cyborg.conf.d/01-service-custom.conf",
			SubPath:   ServiceCustomConfigFileName,
			ReadOnly:  true,
		})
	}
	vm = append(vm, corev1.VolumeMount{
		Name:      ConfigVolume,
		MountPath: "/etc/my.cnf",
		SubPath:   "my.cnf",
		ReadOnly:  true,
	})
	return vm
}
