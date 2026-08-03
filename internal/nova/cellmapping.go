package nova

import (
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/pod"
	"github.com/openstack-k8s-operators/lib-common/modules/users"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/nova/v1beta1"
)

// CellMappingJob creates a Kubernetes job to create Nova cell mappings
func CellMappingJob(
	instance *novav1.Nova,
	cell *novav1.NovaCell,
	configName string,
	scriptName string,
	inputHash string,
	labels map[string]string,
) *batchv1.Job {
	envVars := map[string]env.Setter{}
	envVars["CELL_NAME"] = env.SetValue(cell.Spec.CellName)

	// This is stored in the Job so that if the input of the job changes
	// then it results in a new job hash and therefore lib-common will re-run
	// the job
	envVars["INPUT_HASH"] = env.SetValue(inputHash)

	env := env.MergeEnvs([]corev1.EnvVar{}, envVars)

	jobName := instance.Name + "-" + cell.Spec.CellName + "-cell-mapping"

	volumes := []corev1.Volume{
		GetConfigVolume(configName),
		GetScriptVolume(scriptName),
	}
	volumeMounts := GetConfVolumeMounts(false)
	volumeMounts = append(volumeMounts, GetScriptVolumeMount())

	// add CA cert if defined
	if instance.Spec.APIServiceTemplate.TLS.CaBundleSecretName != "" {
		volumes = append(volumes, instance.Spec.APIServiceTemplate.TLS.CreateVolume())
		volumeMounts = append(volumeMounts, instance.Spec.APIServiceTemplate.TLS.CreateVolumeMounts(nil)...)
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: instance.Namespace,
			Labels:    labels,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy:                corev1.RestartPolicyOnFailure,
					ServiceAccountName:           instance.RbacResourceName(),
					AutomountServiceAccountToken: ptr.To(false),
					SecurityContext:              pod.RestrictivePodSecurityContext(users.NovaUID, users.NovaGID),
					Volumes:                      volumes,
					Containers: []corev1.Container{
						{
							Name: "nova-manage",
							Command: []string{
								"/usr/local/bin/container-scripts/ensure_cell_mapping.sh",
							},
							Image:           cell.Spec.ConductorContainerImageURL,
							SecurityContext: pod.RestrictiveSecurityContext(users.NovaUID, users.NovaGID),
							Env:             env,
							VolumeMounts:    volumeMounts,
						},
					},
				},
			},
		},
	}

	if cell.Spec.NodeSelector != nil {
		job.Spec.Template.Spec.NodeSelector = *cell.Spec.NodeSelector
	}

	return job
}
