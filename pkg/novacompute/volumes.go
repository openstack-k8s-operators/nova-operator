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

package novacompute

import (
	"strings"

	"github.com/openstack-k8s-operators/nova-operator/pkg/novamigrationtarget"
	corev1 "k8s.io/api/core/v1"
)

// GetVolumes - Volumes used by novacompute pod
func GetVolumes(cmName string) []corev1.Volume {
	var config0600AccessMode int32 = 0600
	var dirOrCreate = corev1.HostPathDirectoryOrCreate

	return []corev1.Volume{
		{
			Name: "boot",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/boot",
				},
			},
		},
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
			Name: "etc-iscsi",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/etc/iscsi",
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
			Name: "lib-modules",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/lib/modules",
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
			Name: "var-lib-nova",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/lib/nova",
					Type: &dirOrCreate,
				},
			},
		},
		{
			Name: "etc-nova-nova-conf-d",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/etc/nova/nova.conf.d",
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
			Name: "var-lib-iscsi",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/lib/iscsi",
				},
			},
		},
		{
			Name: "nova-log",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/log/containers/nova",
					Type: &dirOrCreate,
				},
			},
		},
		{
			Name: "novamigrationtarget-ssh-keys-identity",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					DefaultMode: &config0600AccessMode,
					SecretName:  strings.ToLower(novamigrationtarget.AppLabel) + "-ssh-keys",
					Items: []corev1.KeyToPath{
						{
							Key:  "identity",
							Path: "identity",
						},
					},
				},
			},
		},
	}

}

// GetInitVolumeMounts - novacompute initContainer VolumeMounts
func GetInitVolumeMounts() []corev1.VolumeMount {

	return []corev1.VolumeMount{
		{
			Name:      "var-lib-nova",
			MountPath: "/var/lib/nova",
		},
	}

}

// GetVolumeMounts - novacompute VolumeMounts
func GetVolumeMounts(cmName string) []corev1.VolumeMount {

	var bidirectional = corev1.MountPropagationBidirectional

	return []corev1.VolumeMount{
		{
			Name:      "etc-libvirt-qemu",
			MountPath: "/etc/libvirt/qemu",
		},
		{
			Name:      "etc-iscsi",
			MountPath: "/etc/iscsi",
			ReadOnly:  true,
		},
		{
			Name:      "boot",
			MountPath: "/boot",
			ReadOnly:  true,
		},
		{
			Name:      "dev",
			MountPath: "/dev",
		},
		{
			Name:      "lib-modules",
			MountPath: "/lib/modules",
			ReadOnly:  true,
		},
		{
			Name:      "run",
			MountPath: "/run",
		},
		{
			Name:      "sys-fs-cgroup",
			MountPath: "/sys/fs/cgroup",
			ReadOnly:  true,
		},
		{
			Name:      "nova-log",
			MountPath: "/var/log/nova",
		},
		{
			Name:             "var-lib-nova",
			MountPath:        "/var/lib/nova",
			MountPropagation: &bidirectional,
		},
		{
			Name:             "var-lib-libvirt",
			MountPath:        "/var/lib/libvirt",
			MountPropagation: &bidirectional,
		},
		{
			Name:      "var-lib-iscsi",
			MountPath: "/var/lib/iscsi",
		},
		{
			Name:      "etc-nova-nova-conf-d",
			MountPath: "/etc/nova/nova.conf.d",
		},
		{
			Name:      "novamigrationtarget-ssh-keys-identity",
			MountPath: "/var/lib/config-data/merged/identity",
			SubPath:   "identity",
			ReadOnly:  true,
		},
	}

}
