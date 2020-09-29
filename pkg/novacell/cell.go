package novacell

import (
	"fmt"

	novav1beta1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	common "github.com/openstack-k8s-operators/nova-operator/pkg/common"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// CreateCellJob func
func CreateCellJob(cr *novav1beta1.NovaCell, scheme *runtime.Scheme) *batchv1.Job {

	runAsUser := int64(0)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-create-cell-%s", cr.Name, cr.Spec.Cell),
			Namespace: cr.Namespace,
			Labels:    common.GetLabels(cr.Name, AppLabel),
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy:      "OnFailure",
					ServiceAccountName: "nova",
					Containers: []corev1.Container{
						{
							Name:  cr.Name + "-create-cell",
							Image: cr.Spec.NovaConductorContainerImage,
							SecurityContext: &corev1.SecurityContext{
								RunAsUser: &runAsUser,
							},
							Env: []corev1.EnvVar{
								{
									Name:  "KOLLA_CONFIG_FILE",
									Value: "/var/lib/config-data/merged/create-cell-config.json",
								},
								{
									Name:  "KOLLA_CONFIG_STRATEGY",
									Value: "COPY_ALWAYS",
								},
								{
									Name:  "Cell",
									Value: cr.Spec.Cell,
								},
								{
									Name:  "DatabaseHost",
									Value: cr.Spec.DatabaseHostname,
								},
								{
									Name:  "ApiDatabase",
									Value: fmt.Sprintf("%s_%s", DatabasePrefix, APIDatabase),
								},
								{
									Name:  "CellDatabase",
									Value: fmt.Sprintf("%s_%s", DatabasePrefix, cr.Spec.Cell),
								},
								{
									Name: "DatabasePassword",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: cr.Spec.NovaSecret,
											},
											Key: "DatabasePassword",
										},
									},
								},
								{
									Name: "TransportURL",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: cr.Spec.TransportURLSecret,
											},
											Key: "TransportUrl",
										},
									},
								},
							},
							VolumeMounts: common.GetCtrlVolumeMounts(),
						},
					},
				},
			},
		},
	}
	initContainerDetails := common.CtrlInitContainer{
		ContainerImage:     cr.Spec.NovaConductorContainerImage,
		DatabaseHost:       cr.Spec.DatabaseHostname,
		CellDatabase:       fmt.Sprintf("%s_%s", DatabasePrefix, cr.Spec.Cell),
		APIDatabase:        fmt.Sprintf("%s_%s", DatabasePrefix, APIDatabase),
		TransportURLSecret: cr.Spec.TransportURLSecret,
		NovaSecret:         cr.Spec.NovaSecret,
		NeutronSecret:      cr.Spec.NeutronSecret,
		PlacementSecret:    cr.Spec.PlacementSecret,
	}
	job.Spec.Template.Spec.InitContainers = common.GetCtrlInitContainer(initContainerDetails)
	job.Spec.Template.Spec.Volumes = common.GetCtrlVolumes(cr.Name)
	controllerutil.SetControllerReference(cr, job, scheme)
	return job
}
