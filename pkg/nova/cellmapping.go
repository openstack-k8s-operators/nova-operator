package nova

import (
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	common "github.com/openstack-k8s-operators/lib-common/modules/common"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
)

const (
	cellMappingCommand      = "/usr/local/bin/kolla_set_configs && /var/lib/openstack/bin/ensure_cell_mapping.sh"
)

func CellMappingJob(
	instance *novav1.Nova,
	cell *novav1.NovaCell,
	configName string,
	scriptName string,
	inputHash string,
	labels map[string]string,
) *batchv1.Job {
	runAsUser := int64(0)

	args := []string{"-c"}
	if cell.Spec.Debug.StopJob {
		args = append(args, common.DebugCommand)
	} else {
		args = append(args, cellMappingCommand)
	}

	envVars := map[string]env.Setter{}
	envVars["KOLLA_CONFIG_STRATEGY"] = env.SetValue("COPY_ALWAYS")
	envVars["KOLLA_BOOTSTRAP"] = env.SetValue("true")
	envVars["CELL_NAME"] = env.SetValue(cell.Spec.CellName)

	// This is stored in the Job so that if the input of the job changes
	// then it results in a new job hash and therefore lib-common will re-run
	// the job
	envVars["INPUT_HASH"] = env.SetValue(inputHash)

	env := env.MergeEnvs([]corev1.EnvVar{}, envVars)

	jobName := instance.Name + "-" + cell.Spec.CellName + "-cell-mapping"

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
					Volumes: []corev1.Volume{
						GetConfigVolume(configName),
						GetScriptVolume(scriptName),
					},
					Containers: []corev1.Container{
						{
							Name: "nova-manage",
							Command: []string{
								"/bin/bash",
							},
							Args:  args,
							Image: cell.Spec.ConductorServiceTemplate.ContainerImage,
							SecurityContext: &corev1.SecurityContext{
								RunAsUser: &runAsUser,
							},
							Env: env,
							VolumeMounts: []corev1.VolumeMount{
								GetConfigVolumeMount(),
								GetScriptVolumeMount(),
								GetKollaConfigVolumeMount("nova-manage"),
							},
						},
					},
				},
			},
		},
	}

	return job
}
