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

package placement

import (
	"github.com/openstack-k8s-operators/lib-common/modules/common/volume"
	internalcommon "github.com/openstack-k8s-operators/nova-operator/internal/common"

	corev1 "k8s.io/api/core/v1"
)

// getVolumes - service volumes
func getVolumes(name string) []corev1.Volume {
	var configMode int32 = 0440

	return []corev1.Volume{
		{
			Name: "config-data",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					DefaultMode: &configMode,
					SecretName:  internalcommon.GetServiceConfigSecretName(name),
				},
			},
		},
		volume.WritableDirVolume("logs"),
		volume.WritableDirVolume(volume.TmpVolumeName),
	}

}

// getVolumeMounts - API deployment VolumeMounts
func getVolumeMounts(withPolicy bool) []corev1.VolumeMount {
	vm := []corev1.VolumeMount{
		volume.WritableDirVolumeMount("logs", "/var/log/placement"),
		{
			Name:      "config-data",
			MountPath: "/etc/placement/placement.conf",
			SubPath:   "placement.conf",
			ReadOnly:  true,
		},
		{
			Name:      "config-data",
			MountPath: "/etc/placement/placement.conf.d/custom.conf",
			SubPath:   "custom.conf",
			ReadOnly:  true,
		},
		{
			Name:      "config-data",
			MountPath: "/etc/httpd/conf/httpd.conf",
			SubPath:   "httpd.conf",
			ReadOnly:  true,
		},
		{
			Name:      "config-data",
			MountPath: "/etc/httpd/conf.d/ssl.conf",
			SubPath:   "ssl.conf",
			ReadOnly:  true,
		},
		{
			Name:      "config-data",
			MountPath: "/etc/my.cnf",
			SubPath:   "my.cnf",
			ReadOnly:  true,
		},
		volume.WritableDirVolumeMount(volume.RunHttpdVolumeName, volume.RunHttpdMountPath),
		volume.WritableDirVolumeMount(volume.TmpVolumeName, volume.TmpMountPath),
		volume.WritableDirVolumeMount(volume.VarLogHttpdVolumeName, volume.VarLogHttpdMountPath),
	}
	if withPolicy {
		vm = append(vm, corev1.VolumeMount{
			Name:      "config-data",
			MountPath: "/etc/placement/policy.yaml",
			SubPath:   "policy.yaml",
			ReadOnly:  true,
		})
	}
	return vm
}

// getDBSyncVolumeMounts - db-sync job VolumeMounts
func getDBSyncVolumeMounts(withPolicy bool) []corev1.VolumeMount {
	vm := []corev1.VolumeMount{
		volume.WritableDirVolumeMount("logs", "/var/log/placement"),
		{
			Name:      "config-data",
			MountPath: "/etc/placement/placement.conf",
			SubPath:   "placement.conf",
			ReadOnly:  true,
		},
		{
			Name:      "config-data",
			MountPath: "/etc/placement/placement.conf.d/custom.conf",
			SubPath:   "custom.conf",
			ReadOnly:  true,
		},
		{
			Name:      "config-data",
			MountPath: "/etc/my.cnf",
			SubPath:   "my.cnf",
			ReadOnly:  true,
		},
		volume.WritableDirVolumeMount(volume.TmpVolumeName, volume.TmpMountPath),
	}
	if withPolicy {
		vm = append(vm, corev1.VolumeMount{
			Name:      "config-data",
			MountPath: "/etc/placement/policy.yaml",
			SubPath:   "policy.yaml",
			ReadOnly:  true,
		})
	}
	return vm
}
