package libvirtd

import (
	corev1 "k8s.io/api/core/v1"
)

// GetVolumes - Volumes used by libvirtd pod
func GetVolumes(cmName string) []corev1.Volume {

	var scriptsVolumeDefaultMode int32 = 0755
	var configVolumeDefaultMode int32 = 0644
	var config0600AccessMode int32 = 0600
	var dirOrCreate = corev1.HostPathDirectoryOrCreate

	return []corev1.Volume{
		{
			Name: "etc-selinux-config",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/etc/selinux/config",
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
			Name: "run",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/run",
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
			Name: "sys-fs-selinux",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/sys/fs/selinux",
				},
			},
		},
		{
			Name: "var-run-libvirt",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/run/libvirt",
					Type: &dirOrCreate,
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
			Name: "var-cache-libvirt",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/cache/libvirt",
					Type: &dirOrCreate,
				},
			},
		},
		{
			Name: "var-lib-vhost-sockets",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/lib/vhost_sockets",
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
			Name: cmName + "-scripts",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					DefaultMode: &scriptsVolumeDefaultMode,
					LocalObjectReference: corev1.LocalObjectReference{
						Name: cmName + "-scripts",
					},
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
		{
			Name: "novamigrationtarget-ssh-keys-identity",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					DefaultMode: &config0600AccessMode,
					SecretName:  "novamigrationtarget-ssh-keys",
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

// GetVolumeMounts - libirtd VolumeMounts
func GetVolumeMounts(cmName string) []corev1.VolumeMount {

	var bidirectional = corev1.MountPropagationBidirectional

	return []corev1.VolumeMount{
		{
			Name:      "etc-selinux-config",
			MountPath: "/etc/selinux/config",
			ReadOnly:  true,
		},
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
			Name:      "run",
			MountPath: "/run",
		},
		{
			Name:      "sys-fs-cgroup",
			MountPath: "/sys/fs/cgroup",
		},
		{
			Name:      "sys-fs-selinux",
			MountPath: "/sys/fs/selinux",
		},
		{
			Name:      "libvirt-log",
			MountPath: "/var/log/libvirt",
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
			Name:             "var-cache-libvirt",
			MountPath:        "/var/cache/libvirt",
			MountPropagation: &bidirectional,
		},
		{
			Name:      "var-lib-vhost-sockets",
			MountPath: "/var/lib/vhost_sockets",
		},
		{
			Name:      "novamigrationtarget-ssh-keys-identity",
			MountPath: "/var/lib/kolla/config_files/src/etc/nova/migration/identity",
			SubPath:   "identity",
			ReadOnly:  true,
		},
		{
			Name:      cmName + "-templates",
			MountPath: "/var/lib/kolla/config_files/src/etc/libvirt/libvirtd.conf",
			SubPath:   "libvirtd.conf",
			ReadOnly:  true,
		},
		{
			Name:      cmName + "-scripts",
			ReadOnly:  true,
			MountPath: "/usr/local/sbin/libvirtd.sh",
			SubPath:   "libvirtd.sh",
		},
	}

}
