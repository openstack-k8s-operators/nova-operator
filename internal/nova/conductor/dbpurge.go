// Package novaconductor contains Nova Conductor service deployment and management functionality.
package novaconductor

import (
	"fmt"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	memcachedv1 "github.com/openstack-k8s-operators/infra-operator/apis/memcached/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common/pod"
	"github.com/openstack-k8s-operators/lib-common/modules/users"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/nova/v1beta1"
	internalcommon "github.com/openstack-k8s-operators/nova-operator/internal/common"
	"github.com/openstack-k8s-operators/nova-operator/internal/nova"
)

// DBPurgeCronJob creates a Kubernetes CronJob for purging old Nova database records
func DBPurgeCronJob(
	instance *novav1.NovaConductor,
	labels map[string]string,
	annotations map[string]string,
	memcached *memcachedv1.Memcached,
) *batchv1.CronJob {
	volumes := []corev1.Volume{
		nova.GetConfigVolume(internalcommon.GetServiceConfigSecretName(instance.Name)),
	}
	volumeMounts := nova.GetConfVolumeMounts(instance.Spec.CustomServiceConfig != "")

	// add CA cert if defined
	if instance.Spec.TLS.CaBundleSecretName != "" {
		volumes = append(volumes, instance.Spec.TLS.CreateVolume())
		volumeMounts = append(volumeMounts, instance.Spec.TLS.CreateVolumeMounts(nil)...)
	}

	// add MTLS cert if defined
	if memcached.Status.MTLSCert != "" {
		certMountPath := memcachedv1.CertPathDst
		keyMountPath := memcachedv1.KeyPathDst
		volumes = append(volumes, memcached.CreateMTLSVolume())
		volumeMounts = append(volumeMounts, memcached.CreateMTLSVolumeMounts(&certMountPath, &keyMountPath)...)
	}

	// we want to hide the fact that the job is created by the conductor
	// controller, but we don't have direct access to the Cell CR name, so we
	// remove the known conductor suffix from the Conductor CR name.
	name := strings.TrimSuffix(instance.Name, "-conductor") + "-db-purge"

	// The cutoff dates are resolved at job execution time via date(1) so they
	// are always "<age> days ago" relative to when the CronJob runs, and so the
	// generated command line stays stable across reconciles (it only changes
	// when ArchiveAge/PurgeAge change), avoiding needless CronJob updates.
	//
	// archive_deleted_rows exits 0 (nothing archived) or 1 (rows archived) on
	// success; any other exit code is an error and aborts the script.
	archiveCmd := fmt.Sprintf(
		`nova-manage db archive_deleted_rows --verbose --until-complete --task-log --before "$(date --date="%d days ago" +%%Y-%%m-%%d)" || [ $? -eq 1 ]`,
		*instance.Spec.DBPurge.ArchiveAge)
	// purge exits 0 (rows deleted) or 3 (nothing to delete) on success; any
	// other exit code is an error and aborts the script.
	purgeCmd := fmt.Sprintf(
		`nova-manage db purge --verbose --before "$(date --date="%d days ago" +%%Y-%%m-%%d)" || [ $? -eq 3 ]`,
		*instance.Spec.DBPurge.PurgeAge)
	script := archiveCmd + " && " + purgeCmd

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
							RestartPolicy:                corev1.RestartPolicyOnFailure,
							ServiceAccountName:           instance.Spec.ServiceAccount,
							AutomountServiceAccountToken: ptr.To(false),
							SecurityContext:              pod.RestrictivePodSecurityContext(users.NovaUID, users.NovaGID),
							Volumes:                      volumes,
							Containers: []corev1.Container{
								{
									Name:            "nova-manage",
									Command:         []string{"/bin/bash", "-c", script},
									Image:           instance.Spec.ContainerImage,
									SecurityContext: pod.RestrictiveSecurityContext(users.NovaUID, users.NovaGID),
									VolumeMounts:    volumeMounts,
								},
							},
						},
					},
				},
			},
		},
	}

	if instance.Spec.NodeSelector != nil {
		cron.Spec.JobTemplate.Spec.Template.Spec.NodeSelector = *instance.Spec.NodeSelector
	}

	return cron
}
