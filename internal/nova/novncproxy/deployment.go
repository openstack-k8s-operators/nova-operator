/*
Copyright 2023.

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

package novncproxy

import (
	"fmt"

	memcachedv1 "github.com/openstack-k8s-operators/infra-operator/apis/memcached/v1beta1"
	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	common "github.com/openstack-k8s-operators/lib-common/modules/common"
	affinity "github.com/openstack-k8s-operators/lib-common/modules/common/affinity"
	env "github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/pod"
	"github.com/openstack-k8s-operators/lib-common/modules/common/probes"
	"github.com/openstack-k8s-operators/lib-common/modules/users"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/nova/v1beta1"
	internalcommon "github.com/openstack-k8s-operators/nova-operator/internal/common"
	"github.com/openstack-k8s-operators/nova-operator/internal/nova"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

// StatefulSet - returns the StatefulSet definition for the nova-novanovncproxy service
func StatefulSet(
	instance *novav1.NovaNoVNCProxy,
	configHash string,
	labels map[string]string,
	annotations map[string]string,
	topology *topologyv1.Topology,
	memcached *memcachedv1.Memcached,
) (*appsv1.StatefulSet, error) {
	scheme := corev1.URISchemeHTTP
	if instance.Spec.TLS.Service.Enabled() {
		scheme = corev1.URISchemeHTTPS
	}

	novncProbes, err := probes.CreateProbeSet(
		int32(NoVNCProxyPort),
		&scheme,
		instance.Spec.Override.Probes,
		internalcommon.GetDefaultProbesNoVNC(),
	)
	if err != nil {
		return nil, err
	}

	envVars := map[string]env.Setter{}
	// NOTE(gibi): The statefulset does not use this hash directly. We store it
	// in the environment to trigger a Pod restart if any input of the
	// statefulset has changed. The k8s will trigger a restart automatically if
	// the env changes.
	envVars["CONFIG_HASH"] = env.SetValue(configHash)
	env := env.MergeEnvs([]corev1.EnvVar{}, envVars)

	// create Volume and VolumeMounts
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

	// add service certs if defined
	if instance.Spec.TLS.Service.Enabled() {
		svc, err := instance.Spec.TLS.Service.ToService()
		if err != nil {
			return nil, err
		}
		svc.CertMount = ptr.To(fmt.Sprintf("/etc/pki/tls/certs/%s.crt", ServiceName))
		svc.KeyMount = ptr.To(fmt.Sprintf("/etc/pki/tls/private/%s.key", ServiceName))
		volumes = append(volumes, svc.CreateVolume(ServiceName))
		volumeMounts = append(volumeMounts, svc.CreateVolumeMounts(ServiceName)...)
	}

	// add Vencrypt certs if defined
	if instance.Spec.TLS.Vencrypt.Enabled() {
		svc, err := instance.Spec.TLS.Vencrypt.ToService()
		if err != nil {
			return nil, err
		}
		svc.CertMount = ptr.To("/etc/pki/tls/certs/vencrypt.crt")
		svc.KeyMount = ptr.To("/etc/pki/tls/private/vencrypt.key")
		volumes = append(volumes, svc.CreateVolume(VencryptName))
		volumeMounts = append(volumeMounts, svc.CreateVolumeMounts(VencryptName)...)
	}

	statefulset := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
		},
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Replicas:            instance.Spec.Replicas,
			PodManagementPolicy: appsv1.ParallelPodManagement,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: annotations,
					Labels:      labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName:           instance.Spec.ServiceAccount,
					AutomountServiceAccountToken: ptr.To(false),
					SecurityContext:              pod.RestrictivePodSecurityContext(users.NovaUID, users.NovaGID),
					Volumes:                      volumes,
					Containers: []corev1.Container{
						{
							Name: instance.Name + "-novncproxy",
							Command: []string{
								"/usr/bin/nova-novncproxy",
							},
							Args:            []string{"--web", "/usr/share/novnc/"},
							Image:           instance.Spec.ContainerImage,
							SecurityContext: pod.RestrictiveSecurityContext(users.NovaUID, users.NovaGID),
							Env:             env,
							VolumeMounts:    volumeMounts,
							Resources:       instance.Spec.Resources,
							ReadinessProbe:  novncProbes.Readiness,
							LivenessProbe:   novncProbes.Liveness,
						},
					},
				},
			},
		},
	}

	if instance.Spec.NodeSelector != nil {
		statefulset.Spec.Template.Spec.NodeSelector = *instance.Spec.NodeSelector
	}

	if topology != nil {
		topology.ApplyTo(&statefulset.Spec.Template)
	} else {
		// If possible two pods of the same service should not
		// run on the same worker node. If this is not possible
		// the get still created on the same worker node.
		statefulset.Spec.Template.Spec.Affinity = affinity.DistributePods(
			common.AppSelector,
			[]string{
				instance.Name,
			},
			corev1.LabelHostname,
		)
	}

	return statefulset, nil
}
