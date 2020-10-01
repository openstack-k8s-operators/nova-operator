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

package nova

import (
	"fmt"

	novav1beta1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	common "github.com/openstack-k8s-operators/nova-operator/pkg/common"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// DbSyncJob func
func DbSyncJob(cr *novav1beta1.Nova, scheme *runtime.Scheme) *batchv1.Job {

	runAsUser := int64(0)

	initVolumeMounts := common.GetInitVolumeMounts()
	volumeMounts := common.GetVolumeMounts()
	volumes := common.GetVolumes(cr.Name)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-db-sync",
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
							Name:  cr.Name + "-db-sync",
							Image: cr.Spec.NovaAPIContainerImage,
							SecurityContext: &corev1.SecurityContext{
								RunAsUser: &runAsUser,
							},
							Env: []corev1.EnvVar{
								{
									Name:  "KOLLA_CONFIG_FILE",
									Value: DBSyncKollaConfig,
								},
								{
									Name:  "KOLLA_CONFIG_STRATEGY",
									Value: "COPY_ALWAYS",
								},
								{
									Name:  "KOLLA_BOOTSTRAP",
									Value: "TRUE",
								},
								{
									Name:  "DatabaseHost",
									Value: cr.Spec.DatabaseHostname,
								},
								{
									Name:  "CellDatabase",
									Value: fmt.Sprintf("nova_%s", CellDatabase),
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
							},
							VolumeMounts: volumeMounts,
						},
					},
				},
			},
		},
	}
	initContainerDetails := common.CtrlInitContainer{
		ContainerImage:     cr.Spec.NovaAPIContainerImage,
		DatabaseHost:       cr.Spec.DatabaseHostname,
		CellDatabase:       fmt.Sprintf("%s_%s", DatabasePrefix, CellDatabase),
		APIDatabase:        fmt.Sprintf("%s_%s", DatabasePrefix, APIDatabase),
		TransportURLSecret: cr.Spec.TransportURLSecret,
		NovaSecret:         cr.Spec.NovaSecret,
		NeutronSecret:      cr.Spec.NeutronSecret,
		PlacementSecret:    cr.Spec.PlacementSecret,
		VolumeMounts:       initVolumeMounts,
	}
	job.Spec.Template.Spec.InitContainers = common.GetCtrlInitContainer(initContainerDetails)

	return job
}
