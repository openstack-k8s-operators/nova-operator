/*
Copyright 2022.

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

package novaconductor

import (
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/nova-operator/pkg/nova"

	common "github.com/openstack-k8s-operators/lib-common/modules/common"
	env "github.com/openstack-k8s-operators/lib-common/modules/common/env"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// cellDBSyncCommand - the command to be used to run db sync for the cell DB
	cellDBSyncCommand = "/usr/local/bin/kolla_set_configs && /bin/sh -c /usr/local/bin/container-scripts/dbsync.sh"
)

// CellDBSyncJob - define a batchv1.Job to be run to apply the cel DB schema
func CellDBSyncJob(
	instance *novav1.NovaConductor,
	labels map[string]string,
	annotations map[string]string,
) *batchv1.Job {

	initContainerDetails := ContainerInput{
		ContainerImage:       instance.Spec.ContainerImage,
		CellDatabaseHostname: instance.Spec.CellDatabaseHostname,
		CellDatabaseUser:     instance.Spec.CellDatabaseUser,
		CellDatabaseName:     "nova_" + instance.Spec.CellName,
		Secret:               instance.Spec.Secret,
		// NOTE(gibi): this is a hack until we implement proper secret handling
		// per cell
		CellDatabasePasswordSelector:        "NovaCell0DatabasePassword",
		KeystoneServiceUserPasswordSelector: instance.Spec.PasswordSelectors.Service,
		// NOTE(gibi): these might be empty if the conductor does not support
		// upcalls but that is OK
		APIDatabaseHostname:         instance.Spec.APIDatabaseHostname,
		APIDatabaseUser:             instance.Spec.APIDatabaseUser,
		APIDatabaseName:             nova.NovaAPIDatabaseName,
		APIDatabasePasswordSelector: instance.Spec.PasswordSelectors.APIDatabase,
		VolumeMounts:                nova.GetAllVolumeMounts(),
		CellMessageBusSecretName:    instance.Spec.CellMessageBusSecretName,
	}
	runAsUser := int64(0)

	args := []string{"-c"}
	if instance.Spec.Debug.StopDBSync {
		args = append(args, common.DebugCommand)
	} else {
		args = append(args, cellDBSyncCommand)
	}

	envVars := map[string]env.Setter{}
	envVars["KOLLA_CONFIG_FILE"] = env.SetValue(MergedServiceConfigPath)
	envVars["KOLLA_CONFIG_STRATEGY"] = env.SetValue("COPY_ALWAYS")
	envVars["KOLLA_BOOTSTRAP"] = env.SetValue("true")

	envVars["CELL_NAME"] = env.SetValue(instance.Spec.CellName)

	env := env.MergeEnvs([]corev1.EnvVar{}, envVars)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:        instance.Name + "-db-sync",
			Namespace:   instance.Namespace,
			Annotations: annotations,
			Labels:      labels,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy:      "OnFailure",
					ServiceAccountName: nova.ServiceAccount,
					Volumes: nova.GetVolumes(
						nova.GetScriptConfigMapName(instance.Name),
						nova.GetServiceConfigConfigMapName(instance.Name),
					),
					Containers: []corev1.Container{
						{
							Name: instance.Name + "-db-sync",
							Command: []string{
								"/bin/bash",
							},
							Args:  args,
							Image: instance.Spec.ContainerImage,
							SecurityContext: &corev1.SecurityContext{
								RunAsUser: &runAsUser,
							},
							Env:          env,
							VolumeMounts: nova.GetServiceVolumeMounts(),
						},
					},
					InitContainers: initContainer(initContainerDetails),
				},
			},
		},
	}

	return job
}
