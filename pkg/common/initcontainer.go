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

package common

import (
	corev1 "k8s.io/api/core/v1"
)

// CtrlInitContainer information
type CtrlInitContainer struct {
	ContainerImage     string
	DatabaseHost       string
	CellDatabase       string
	APIDatabase        string
	TransportURLSecret string
	NovaSecret         string
	NeutronSecret      string
	PlacementSecret    string
	VolumeMounts       []corev1.VolumeMount
}

// CmpInitContainer information
type CmpInitContainer struct {
	Privileged         bool
	ContainerImage     string
	TransportURLSecret string
	NovaSecret         string
	NeutronSecret      string
	PlacementSecret    string
	VolumeMounts       []corev1.VolumeMount
}

// GetCtrlInitContainer - init container for nova ctrl plane services
func GetCtrlInitContainer(init CtrlInitContainer) []corev1.Container {
	runAsUser := int64(0)

	return []corev1.Container{
		{
			Name:  "init",
			Image: init.ContainerImage,
			SecurityContext: &corev1.SecurityContext{
				RunAsUser: &runAsUser,
			},
			Command: []string{
				"/bin/bash", "-c", "/usr/local/bin/container-scripts/init.sh",
			},
			Env: []corev1.EnvVar{
				{
					Name: "TransportURL",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: init.TransportURLSecret,
							},
							Key: "TransportUrl",
						},
					},
				},
				{
					Name:  "DatabaseHost",
					Value: init.DatabaseHost,
				},
				{
					Name:  "CellDatabase",
					Value: init.CellDatabase,
				},
				{
					Name:  "ApiDatabase",
					Value: init.APIDatabase,
				},
				{
					Name: "DatabasePassword",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: init.NovaSecret,
							},
							Key: "DatabasePassword",
						},
					},
				},
				{
					Name: "NovaKeystoneAuthPassword",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: init.NovaSecret,
							},
							Key: "NovaKeystoneAuthPassword",
						},
					},
				},
				{
					Name: "NeutronKeystoneAuthPassword",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: init.NeutronSecret,
							},
							Key: "NeutronKeystoneAuthPassword",
						},
					},
				},
				{
					Name: "PlacementKeystoneAuthPassword",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: init.PlacementSecret,
							},
							Key: "PlacementKeystoneAuthPassword",
						},
					},
				},
			},
			VolumeMounts: init.VolumeMounts,
		},
	}
}
