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
	initVolumeMounts := common.GetInitVolumeMounts()
	volumeMounts := common.GetVolumeMounts()
	volumes := common.GetVolumes(cr.Name)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-create-cell", cr.Name),
			Namespace: cr.Namespace,
			Labels:    common.GetLabels(cr.Name, AppLabel),
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy:      "OnFailure",
					ServiceAccountName: "nova",
					Volumes:            volumes,
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
									Value: CreateCellKollaConfig,
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
							VolumeMounts: volumeMounts,
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
		VolumeMounts:       initVolumeMounts,
	}
	job.Spec.Template.Spec.InitContainers = common.GetCtrlInitContainer(initContainerDetails)
	controllerutil.SetControllerReference(cr, job, scheme)
	return job
}
