package novamigrationtarget

import (
	"strings"

	novav1beta1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
)

// GetVolumes - Volumes used by novamigrationtarget pod
func GetVolumes(cr *novav1beta1.NovaMigrationTarget, cmName string) []corev1.Volume {

	var scriptsVolumeDefaultMode int32 = 0755
	var configVolumeDefaultMode int32 = 0644
	var config0640AccessMode int32 = 0640
	var config0600AccessMode int32 = 0600
	var dirOrCreate = corev1.HostPathDirectoryOrCreate

	return []corev1.Volume{
		{
			Name: "etc-ssh",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/etc/ssh",
				},
			},
		},
		{
			Name: "run-libvirt",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/run/libvirt",
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
			Name: strings.ToLower(cr.Kind) + "-ssh-keys-authorized-keys",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					DefaultMode: &config0640AccessMode,
					SecretName:  strings.ToLower(cr.Kind) + "-ssh-keys",
					Items: []corev1.KeyToPath{
						{
							Key:  "authorized_keys",
							Path: "authorized_keys",
						},
					},
				},
			},
		},
		{
			Name: strings.ToLower(cr.Kind) + "-ssh-keys-identity",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					DefaultMode: &config0600AccessMode,
					SecretName:  strings.ToLower(cr.Kind) + "-ssh-keys",
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

// GetInitContainerVolumeMounts - novamigrationtarget initContainer VolumeMounts
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

// GetVolumeMounts - novamigrationtarget VolumeMounts
func GetVolumeMounts(cr *novav1beta1.NovaMigrationTarget, cmName string) []corev1.VolumeMount {

	var hostToContainer = corev1.MountPropagationHostToContainer

	return []corev1.VolumeMount{
		{
			Name:             "var-lib-nova",
			MountPath:        "/var/lib/nova",
			MountPropagation: &hostToContainer,
		},
		{
			Name:      "run-libvirt",
			MountPath: "/run/libvirt",
			ReadOnly:  true,
		},
		{
			Name:      strings.ToLower(cr.Kind) + "-ssh-keys-authorized-keys",
			MountPath: "/var/lib/kolla/config_files/src/etc/nova/migration/authorized_keys",
			SubPath:   "authorized_keys",
			ReadOnly:  true,
		},
		{
			Name:      strings.ToLower(cr.Kind) + "-ssh-keys-identity",
			MountPath: "/var/lib/kolla/config_files/src/etc/nova/migration/identity",
			SubPath:   "identity",
			ReadOnly:  true,
		},
		{
			Name:      cmName + "-templates",
			MountPath: "/var/lib/kolla/config_files/src/var/lib/nova/.ssh/config",
			SubPath:   "ssh_config",
			ReadOnly:  true,
		},
		{
			Name:      "etc-ssh",
			MountPath: "/var/lib/kolla/config_files/host-ssh",
			ReadOnly:  true,
		},
	}

}
