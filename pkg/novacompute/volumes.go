package novacompute

import (
	corev1 "k8s.io/api/core/v1"
)

// GetVolumes - Volumes used by novacompute pod
func GetVolumes(cmName string) []corev1.Volume {

	var scriptsVolumeDefaultMode int32 = 0755
	var config0640AccessMode int32 = 0640
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
					DefaultMode: &config0640AccessMode,
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

// GetInitContainerVolumeMounts - novacompute initContainer VolumeMounts
func GetInitContainerVolumeMounts(cmName string) []corev1.VolumeMount {

	return []corev1.VolumeMount{
		{
			Name:      cmName + "-scripts",
			ReadOnly:  true,
			MountPath: "/tmp/container-scripts",
		},
		{
			Name:      cmName + "-templates",
			ReadOnly:  true,
			MountPath: "/tmp/container-templates",
		},
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
			MountPath: "/var/lib/kolla/config_files/src/etc/nova/migration/identity",
			SubPath:   "identity",
			ReadOnly:  true,
		},
	}

}
