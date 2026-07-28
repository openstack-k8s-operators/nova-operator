/*
Copyright 2024.

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

// Package api provides the StatefulSet for the cyborg-api service
//
// revive:disable:var-naming
package api

import (
	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common"
	"github.com/openstack-k8s-operators/lib-common/modules/common/affinity"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/probes"
	libservice "github.com/openstack-k8s-operators/lib-common/modules/common/service"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"

	cyborgv1beta1 "github.com/openstack-k8s-operators/nova-operator/api/cyborg/v1beta1"
	internalcommon "github.com/openstack-k8s-operators/nova-operator/internal/common"
	cyborg "github.com/openstack-k8s-operators/nova-operator/internal/cyborg"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

const (
	// ComponentName is the name used for the api container
	ComponentName = "cyborg-api"
)

// StatefulSet creates a StatefulSet for the cyborg-api service
func StatefulSet(
	instance *cyborgv1beta1.CyborgAPI,
	configHash string,
	labels map[string]string,
	topology *topologyv1.Topology,
) (*appsv1.StatefulSet, error) {
	var config0644AccessMode int32 = 0644

	envVars := make(map[string]env.Setter)
	envVars["KOLLA_CONFIG_STRATEGY"] = env.SetValue("COPY_ALWAYS")
	envVars["CONFIG_HASH"] = env.SetValue(configHash)
	args := []string{"-c", internalcommon.ServiceCommand}

	scheme := corev1.URISchemeHTTP
	if instance.Spec.TLS.API.Enabled(libservice.EndpointPublic) {
		scheme = corev1.URISchemeHTTPS
	}
	apiProbes, err := probes.CreateProbeSet(
		int32(cyborg.CyborgInternalPort),
		&scheme,
		instance.Spec.Override.Probes,
		internalcommon.GetDefaultProbesAPI(*instance.Spec.APITimeout),
	)
	if err != nil {
		return nil, err
	}

	logVolumeMount := corev1.VolumeMount{
		Name:      cyborg.LogVolume,
		MountPath: "/var/log/cyborg",
		ReadOnly:  false,
	}

	volumes := []corev1.Volume{
		{
			Name: cyborg.ConfigVolume,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					DefaultMode: &config0644AccessMode,
					SecretName:  internalcommon.GetServiceConfigSecretName(instance.Name),
				},
			},
		},
		{
			Name: cyborg.LogVolume,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{Medium: ""},
			},
		},
	}

	volumeMounts := []corev1.VolumeMount{
		{
			Name:      cyborg.ConfigVolume,
			MountPath: "/var/lib/config-data/default",
			ReadOnly:  true,
		},
		{
			Name:      cyborg.ConfigVolume,
			MountPath: "/var/lib/kolla/config_files/config.json",
			SubPath:   "cyborg-api-config.json",
			ReadOnly:  true,
		},
		{
			Name:      cyborg.ConfigVolume,
			MountPath: "/etc/my.cnf",
			SubPath:   "my.cnf",
			ReadOnly:  true,
		},
		logVolumeMount,
	}

	// Add CA bundle volume if set
	if instance.Spec.TLS.CaBundleSecretName != "" {
		volumes = append(volumes, instance.Spec.TLS.CreateVolume())
		volumeMounts = append(volumeMounts, instance.Spec.TLS.CreateVolumeMounts(nil)...)
	}

	// Add API TLS cert volumes for each enabled endpoint
	for _, endpt := range []libservice.Endpoint{libservice.EndpointInternal, libservice.EndpointPublic} {
		if instance.Spec.TLS.API.Enabled(endpt) {
			var tlsEndptCfg tls.GenericService
			switch endpt {
			case libservice.EndpointPublic:
				tlsEndptCfg = instance.Spec.TLS.API.Public
			case libservice.EndpointInternal:
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
			Labels:    labels,
		},
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Replicas:            instance.Spec.Replicas,
			PodManagementPolicy: appsv1.ParallelPodManagement,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName:           instance.Spec.ServiceAccount,
					AutomountServiceAccountToken: ptr.To(false),
					Containers: []corev1.Container{
						{
							Name: ComponentName + "-log",
							Command: []string{
								"/usr/bin/dumb-init",
							},
							Args: []string{
								"--single-child",
								"--",
								"/usr/bin/tail",
								"-n+1",
								"-F",
								cyborg.CyborgLogPath + instance.Name + ".log",
							},
							Image: instance.Spec.ContainerImage,
							SecurityContext: &corev1.SecurityContext{
								RunAsUser: ptr.To(cyborg.CyborgUserID),
							},
							Env:          env.MergeEnvs([]corev1.EnvVar{}, envVars),
							VolumeMounts: []corev1.VolumeMount{logVolumeMount},
							Resources:    instance.Spec.Resources,
						},
						{
							Name: ComponentName,
							Command: []string{
								"/bin/bash",
							},
							Args:  args,
							Image: instance.Spec.ContainerImage,
							SecurityContext: &corev1.SecurityContext{
								RunAsUser: ptr.To(cyborg.CyborgUserID),
							},
							Env:            env.MergeEnvs([]corev1.EnvVar{}, envVars),
							VolumeMounts:   volumeMounts,
							Resources:      instance.Spec.Resources,
							ReadinessProbe: apiProbes.Readiness,
							LivenessProbe:  apiProbes.Liveness,
						},
					},
					Volumes: volumes,
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
		statefulset.Spec.Template.Spec.Affinity = affinity.DistributePods(
			common.AppSelector,
			[]string{instance.Name},
			corev1.LabelHostname,
		)
	}

	return statefulset, nil
}
