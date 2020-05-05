package common

import (
	corev1 "k8s.io/api/core/v1"
)

// common Volumes used by many service pods
func GetVolumes(cmName string) []corev1.Volume {

	return []corev1.Volume{
		{
			Name: "etc-machine-id",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/etc/machine-id",
				},
			},
		},
		{
			Name: "etc-localtime",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/etc/localtime",
				},
			},
		},
		{
			Name: "kolla-config-src",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: "kolla-config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: cmName + "-templates",
					},
					Items: []corev1.KeyToPath{
						{
							Key:  "config.json",
							Path: "config.json",
						},
					},
				},
			},
		},
	}

}

// common VolumeMounts
func GetVolumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      "etc-machine-id",
			MountPath: "/etc/machine-id",
			ReadOnly:  true,
		},
		{
			Name:      "etc-localtime",
			MountPath: "/etc/localtime",
			ReadOnly:  true,
		},
		{
			Name:      "kolla-config",
			MountPath: "/var/lib/kolla/config_files/config.json",
			SubPath:   "config.json",
			ReadOnly:  true,
		},
		{
			Name:      "kolla-config-src",
			MountPath: "/var/lib/kolla/config_files/src",
		},
	}

}
