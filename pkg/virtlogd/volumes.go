/*
Copyright 2020 Red Hat

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

package virtlogd

import (
	corev1 "k8s.io/api/core/v1"
)

// GetVolumes - Volumes used by virtlogd pod
func GetVolumes(cmName string) []corev1.Volume {
	var dirOrCreate = corev1.HostPathDirectoryOrCreate

	return []corev1.Volume{
		{
			Name: "etc-libvirt-qemu",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/etc/libvirt/qemu",
					Type: &dirOrCreate,
				},
			},
		},
		{
			Name: "dev",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/dev",
				},
			},
		},
		{
			Name: "sys-fs-cgroup",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/sys/fs/cgroup",
				},
			},
		},
		{
			Name: "run",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/run",
				},
			},
		},
		{
			Name: "var-lib-nova",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/lib/nova",
					Type: &dirOrCreate,
				},
			},
		},
		{
			Name: "var-lib-libvirt",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/lib/libvirt",
					Type: &dirOrCreate,
				},
			},
		},
		{
			Name: "lib-modules",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/lib/modules",
				},
			},
		},
		{
			Name: "libvirt-log",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/log/containers/libvirt",
					Type: &dirOrCreate,
				},
			},
		},
	}

}

// GetVolumeMounts - virtlogd VolumeMounts
func GetVolumeMounts(cmName string) []corev1.VolumeMount {

	var hostToContainer = corev1.MountPropagationHostToContainer

	return []corev1.VolumeMount{
		{
			Name:      "etc-libvirt-qemu",
			MountPath: "/etc/libvirt/qemu",
		},
		{
			Name:      "lib-modules",
			MountPath: "/lib/modules",
			ReadOnly:  true,
		},
		{
			Name:      "dev",
			MountPath: "/dev",
		},
		{
			Name:      "sys-fs-cgroup",
			MountPath: "/sys/fs/cgroup",
			ReadOnly:  true,
		},
		{
			Name:      "run",
			MountPath: "/run",
		},
		{
			Name:      "libvirt-log",
			MountPath: "/var/log/libvirt",
		},
		{
			Name:             "var-lib-nova",
			MountPath:        "/var/lib/nova",
			MountPropagation: &hostToContainer,
		},
		{
			Name:             "var-lib-libvirt",
			MountPath:        "/var/lib/libvirt",
			MountPropagation: &hostToContainer,
		},
	}

}
