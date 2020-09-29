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
			VolumeMounts: GetCtrlInitVolumeMounts(),
		},
	}
}
