package placement

import (
	placementv1beta1 "github.com/openstack-k8s-operators/placement-operator/api/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// AppLabel -
const AppLabel = "placement-api"

// Deployment func
func Deployment(cr *placementv1beta1.PlacementAPI, scriptsConfigMapHash string, configHash string, customConfigHash string, scheme *runtime.Scheme) *appsv1.Deployment {
	runAsUser := int64(0)

	labels := map[string]string{
		"app": AppLabel,
	}
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Replicas: &cr.Spec.Replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: "placement",
					Containers: []corev1.Container{
						{
							Name: "placement-api",
							//Command: []string{"/bin/sleep", "7000"},
							Image: cr.Spec.ContainerImage,
							SecurityContext: &corev1.SecurityContext{
								RunAsUser: &runAsUser,
							},
							Env: []corev1.EnvVar{
								{
									Name:  "KOLLA_CONFIG_FILE",
									Value: "/var/lib/config-data/merged/config.json",
								},
								{
									Name:  "KOLLA_CONFIG_STRATEGY",
									Value: "COPY_ALWAYS",
								},
								{
									Name:  "SCRIPTS_CONFIG_HASH",
									Value: scriptsConfigMapHash,
								},
								{
									Name:  "CONFIG_HASH",
									Value: configHash,
								},
								{
									Name:  "CUSTOM_CONFIG_HASH",
									Value: customConfigHash,
								},
							},
							VolumeMounts: getVolumeMounts(),
						},
					},
					InitContainers: []corev1.Container{
						{
							Name:  "init",
							Image: cr.Spec.ContainerImage,
							SecurityContext: &corev1.SecurityContext{
								RunAsUser: &runAsUser,
							},
							Command: []string{
								"/bin/bash", "-c", "/usr/local/bin/container-scripts/init.sh",
							},
							Env: []corev1.EnvVar{
								{
									Name:  "DatabaseHost",
									Value: cr.Spec.DatabaseHostname,
								},
								{
									Name:  "DatabaseUser",
									Value: cr.Name,
								},
								{
									Name:  "DatabaseSchema",
									Value: cr.Name,
								},
								{
									Name: "DatabasePassword",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: cr.Spec.Secret,
											},
											Key: "DatabasePassword",
										},
									},
								},
								{
									Name: "PlacementKeystoneAuthPassword",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: cr.Spec.Secret,
											},
											Key: "PlacementKeystoneAuthPassword",
										},
									},
								},
							},
							VolumeMounts: getInitVolumeMounts(),
						},
					},
				},
			},
		},
	}
	deployment.Spec.Template.Spec.Volumes = getVolumes(cr.Name)
	controllerutil.SetControllerReference(cr, deployment, scheme)
	return deployment
}
