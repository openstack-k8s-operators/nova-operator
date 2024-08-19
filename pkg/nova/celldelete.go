package nova

import (
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
)

func CellDeleteJob(
	instance *novav1.Nova,
	cell *novav1.NovaCell,
	configName string,
	scriptName string,
	inputHash string,
	labels map[string]string,
) *batchv1.Job {
	args := []string{"-c", KollaServiceCommand}

	envVars := map[string]env.Setter{}
	envVars["KOLLA_CONFIG_STRATEGY"] = env.SetValue("COPY_ALWAYS")
	envVars["KOLLA_BOOTSTRAP"] = env.SetValue("true")
	envVars["CELL_NAME"] = env.SetValue(cell.Spec.CellName)

	// This is stored in the Job so that if the input of the job changes
	// then it results in a new job hash and therefore lib-common will re-run
	// the job
	envVars["INPUT_HASH"] = env.SetValue(inputHash)

	env := env.MergeEnvs([]corev1.EnvVar{}, envVars)

	jobName := instance.Name + "-" + cell.Spec.CellName + "-cell-delete"

	volumes := []corev1.Volume{
		GetConfigVolume(configName),
		GetScriptVolume(scriptName),
	}
	volumeMounts := []corev1.VolumeMount{
		GetConfigVolumeMount(),
		GetScriptVolumeMount(),
		GetKollaConfigVolumeMount("cell-delete"),
	}

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
					RestartPolicy:      corev1.RestartPolicyOnFailure,
					ServiceAccountName: instance.RbacResourceName(),
					Volumes:            volumes,
					Containers: []corev1.Container{
						{
							Name: "nova-manage",
							Command: []string{
								"/bin/bash",
							},
							Args:  args,
							Image: cell.Spec.ConductorContainerImageURL,
							SecurityContext: &corev1.SecurityContext{
								RunAsUser: ptr.To(NovaUserID),
							},
							Env:          env,
							VolumeMounts: volumeMounts,
						},
					},
				},
			},
		},
	}

	return job
}
