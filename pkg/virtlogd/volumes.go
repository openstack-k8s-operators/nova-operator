package virtlogd

import (
	corev1 "k8s.io/api/core/v1"
)

// Volumes used by virtlogd pod
func GetVolumes(cmName string) []corev1.Volume {

	var configVolumeDefaultMode int32 = 0644
	var dirOrCreate corev1.HostPathType = corev1.HostPathDirectoryOrCreate

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
		{
			Name: cmName + "-templates",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					DefaultMode: &configVolumeDefaultMode,
					LocalObjectReference: corev1.LocalObjectReference{
						Name: cmName + "-templates",
					},
				},
			},
		},
	}

}

// virtlogd VolumeMounts
func GetVolumeMounts(cmName string) []corev1.VolumeMount {

	var hostToContainer corev1.MountPropagationMode = corev1.MountPropagationHostToContainer

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
		{
			Name:      cmName + "-templates",
			MountPath: "/var/lib/kolla/config_files/src//etc/libvirt/virtlogd.conf",
			SubPath:   "virtlogd.conf",
			ReadOnly:  true,
		},
	}

}
