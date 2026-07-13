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

package novaapi

import (
	memcachedv1 "github.com/openstack-k8s-operators/infra-operator/apis/memcached/v1beta1"
	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	common "github.com/openstack-k8s-operators/lib-common/modules/common"
	affinity "github.com/openstack-k8s-operators/lib-common/modules/common/affinity"
	env "github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/probes"
	"github.com/openstack-k8s-operators/lib-common/modules/common/service"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/nova/v1beta1"
	internalcommon "github.com/openstack-k8s-operators/nova-operator/internal/common"
	"github.com/openstack-k8s-operators/nova-operator/internal/nova"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

// StatefulSet - returns the StatefulSet definition for the nova-api service
func StatefulSet(
	instance *novav1.NovaAPI,
	configHash string,
	labels map[string]string,
	annotations map[string]string,
	topology *topologyv1.Topology,
	memcached *memcachedv1.Memcached,
) (*appsv1.StatefulSet, error) {
	scheme := corev1.URISchemeHTTP
	if instance.Spec.TLS.API.Enabled(service.EndpointPublic) {
		scheme = corev1.URISchemeHTTPS
	}

	apiProbes, err := probes.CreateProbeSet(
		int32(APIServicePort),
		&scheme,
		probes.OverrideSpec{},
		internalcommon.GetDefaultProbesAPI(instance.Spec.APITimeout),
	)
	if err != nil {
		return nil, err
	}

	args := []string{"-c", nova.KollaServiceCommand}

	envVars := map[string]env.Setter{}
	envVars["KOLLA_CONFIG_STRATEGY"] = env.SetValue("COPY_ALWAYS")
	// NOTE(gibi): The statefulset does not use this hash directly. We store it
	// in the environment to trigger a Pod restart if any input of the
	// statefulset has changed. The k8s will trigger a restart automatically if
	// the env changes.
	envVars["CONFIG_HASH"] = env.SetValue(configHash)
	env := env.MergeEnvs([]corev1.EnvVar{}, envVars)

	// create Volume and VolumeMounts
	volumes := []corev1.Volume{
		nova.GetConfigVolume(internalcommon.GetServiceConfigSecretName(instance.Name)),
		nova.GetLogVolume(),
	}
	volumeMounts := []corev1.VolumeMount{
		nova.GetConfigVolumeMount(),
		nova.GetLogVolumeMount(),
		nova.GetKollaConfigVolumeMount("nova-api"),
	}

	// add CA cert if defined
	if instance.Spec.TLS.CaBundleSecretName != "" {
		volumes = append(volumes, instance.Spec.TLS.CreateVolume())
		volumeMounts = append(volumeMounts, instance.Spec.TLS.CreateVolumeMounts(nil)...)
	}

	// add MTLS cert if defined
	if memcached.Status.MTLSCert != "" {
		volumes = append(volumes, memcached.CreateMTLSVolume())
		volumeMounts = append(volumeMounts, memcached.CreateMTLSVolumeMounts(nil, nil)...)
	}

	for _, endpt := range []service.Endpoint{service.EndpointInternal, service.EndpointPublic} {
		if instance.Spec.TLS.API.Enabled(endpt) {
			var tlsEndptCfg tls.GenericService
			switch endpt {
			case service.EndpointPublic:
				tlsEndptCfg = instance.Spec.TLS.API.Public
			case service.EndpointInternal:
				tlsEndptCfg = instance.Spec.TLS.API.Internal
			}

			svc, err := tlsEndptCfg.ToService()
			if err != nil {
				return nil, err
			}
			volumes = append(volumes, svc.CreateVolume(endpt.String()))
			volumeMounts = append(volumeMounts, svc.CreateVolumeMounts(endpt.String())...)
		}
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
			PodManagementPolicy: appsv1.ParallelPodManagement,
			Replicas:            instance.Spec.Replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: annotations,
					Labels:      labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: instance.Spec.ServiceAccount,
					Volumes:            volumes,
					Containers: []corev1.Container{
						// the first container in a pod is the default selected
						// by oc log so define the log stream container first.
						{
							Name: instance.Name + "-log",
							Command: []string{
								"/usr/bin/dumb-init",
							},
							Args: []string{
								"--single-child",
								"--",
								"/bin/sh",
								"-c",
								"/usr/bin/tail -n+1 -F /var/log/nova/nova-api.log 2>/dev/null",
							},
							Image: instance.Spec.ContainerImage,
							SecurityContext: &corev1.SecurityContext{
								RunAsUser: ptr.To(nova.NovaUserID),
							},
							Env:            env,
							VolumeMounts:   []corev1.VolumeMount{nova.GetLogVolumeMount()},
							Resources:      instance.Spec.Resources,
							ReadinessProbe: apiProbes.Readiness,
							LivenessProbe:  apiProbes.Liveness,
							StartupProbe:   apiProbes.Startup,
						},
						{
							Name: instance.Name + "-api",
							Command: []string{
								"/bin/bash",
							},
							Args:  args,
							Image: instance.Spec.ContainerImage,
							SecurityContext: &corev1.SecurityContext{
								RunAsUser: ptr.To(nova.NovaUserID),
							},
							Env:            env,
							VolumeMounts:   volumeMounts,
							Resources:      instance.Spec.Resources,
							ReadinessProbe: apiProbes.Readiness,
							LivenessProbe:  apiProbes.Liveness,
							StartupProbe:   apiProbes.Startup,
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
