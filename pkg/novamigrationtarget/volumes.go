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

package novamigrationtarget

import (
	"strings"

	novav1beta1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
)

// GetVolumes - Volumes used by novamigrationtarget pod
func GetVolumes(cr *novav1beta1.NovaMigrationTarget) []corev1.Volume {
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
			Name: strings.ToLower(cr.Kind) + "-ssh-keys-authorized-keys",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					DefaultMode: &config0640AccessMode,
					SecretName:  strings.ToLower(AppLabel) + "-ssh-keys",
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
					SecretName:  strings.ToLower(AppLabel) + "-ssh-keys",
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

// GetInitVolumeMounts - novamigrationtarget initContainer VolumeMounts
func GetInitVolumeMounts(cmName string) []corev1.VolumeMount {

	return []corev1.VolumeMount{
		{
			Name:      "var-lib-nova",
			MountPath: "/var/lib/nova",
		},
	}
}

// GetVolumeMounts - novamigrationtarget VolumeMounts
func GetVolumeMounts(cr *novav1beta1.NovaMigrationTarget) []corev1.VolumeMount {

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
			MountPath: "/var/lib/config-data/merged/authorized_keys",
			SubPath:   "authorized_keys",
			ReadOnly:  true,
		},
		{
			Name:      strings.ToLower(cr.Kind) + "-ssh-keys-identity",
			MountPath: "/var/lib/config-data/merged/identity",
			SubPath:   "identity",
			ReadOnly:  true,
		},
		{
			Name:      "etc-ssh",
			MountPath: "/var/lib/config-data/host-ssh",
			ReadOnly:  true,
		},
	}
}
