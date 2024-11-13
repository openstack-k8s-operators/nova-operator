package novaconductor

import (
	"fmt"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/nova-operator/pkg/nova"
)

func DBPurgeCronJob(
	instance *novav1.NovaConductor,
	labels map[string]string,
	annotations map[string]string,
) *batchv1.CronJob {
	args := []string{"-c", nova.KollaServiceCommand}

	envVars := map[string]env.Setter{}
	envVars["KOLLA_CONFIG_STRATEGY"] = env.SetValue("COPY_ALWAYS")
	envVars["KOLLA_BOOTSTRAP"] = env.SetValue("true")

	envVars["ARCHIVE_AGE"] = env.SetValue(fmt.Sprintf("%d", *instance.Spec.DBPurge.ArchiveAge))
	envVars["PURGE_AGE"] = env.SetValue(fmt.Sprintf("%d", *instance.Spec.DBPurge.PurgeAge))

	env := env.MergeEnvs([]corev1.EnvVar{}, envVars)

	volumes := []corev1.Volume{
		nova.GetConfigVolume(nova.GetServiceConfigSecretName(instance.Name)),
		nova.GetScriptVolume(nova.GetScriptSecretName(instance.Name)),
	}
	volumeMounts := []corev1.VolumeMount{
		nova.GetConfigVolumeMount(),
		nova.GetScriptVolumeMount(),
		nova.GetKollaConfigVolumeMount("nova-conductor-dbpurge"),
	}

	// add CA cert if defined
	if instance.Spec.TLS.CaBundleSecretName != "" {
		volumes = append(volumes, instance.Spec.TLS.CreateVolume())
		volumeMounts = append(volumeMounts, instance.Spec.TLS.CreateVolumeMounts(nil)...)
	}

	// we want to hide the fact that the job is created by the conductor
	// controller, but we don't have direct access to the Cell CR name, so we
	// remove the known conductor suffix from the Conductor CR name.
	name := strings.TrimSuffix(instance.Name, "-conductor") + "-db-purge"

	cron := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: instance.Namespace,
			Labels:    labels,
		},
		Spec: batchv1.CronJobSpec{
			Schedule:          *instance.Spec.DBPurge.Schedule,
			ConcurrencyPolicy: batchv1.ForbidConcurrent,
			JobTemplate: batchv1.JobTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: annotations,
				},
				Spec: batchv1.JobSpec{
					Parallelism: ptr.To[int32](1),
					Completions: ptr.To[int32](1),
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							RestartPolicy:      corev1.RestartPolicyOnFailure,
							ServiceAccountName: instance.Spec.ServiceAccount,
							Volumes:            volumes,
							Containers: []corev1.Container{
								{
									Name: "nova-manage",
									Command: []string{
										"/bin/bash",
									},
									Args:  args,
									Image: instance.Spec.ContainerImage,
									SecurityContext: &corev1.SecurityContext{
										RunAsUser: ptr.To(nova.NovaUserID),
									},
									Env:          env,
									VolumeMounts: volumeMounts,
								},
							},
						},
					},
				},
			},
		},
	}

	if instance.Spec.NodeSelector != nil && len(*instance.Spec.NodeSelector) > 0 {
		cron.Spec.JobTemplate.Spec.Template.Spec.NodeSelector = *instance.Spec.NodeSelector
	}

	return cron
}
