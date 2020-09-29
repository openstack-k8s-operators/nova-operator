package novascheduler

import (
	"fmt"

	novav1beta1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	common "github.com/openstack-k8s-operators/nova-operator/pkg/common"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// StatefulSet func
func StatefulSet(cr *novav1beta1.NovaScheduler, scriptsConfigMapHash string, configHash string, customConfigHash string, scheme *runtime.Scheme) *appsv1.StatefulSet {
	runAsUser := int64(0)
	// TODO: move common.Probe to lib-common
	readinessProbe := common.Probe{ProbeType: "readiness"}
	livenessProbe := common.Probe{ProbeType: "liveness"}

	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.Namespace,
		},
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: common.GetLabels(cr.Name, AppLabel),
			},
			Replicas: &cr.Spec.Replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: common.GetLabels(cr.Name, AppLabel),
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: "nova",
					Containers: []corev1.Container{
						{
							Name:  "nova-scheduler",
							Image: cr.Spec.ContainerImage,
							SecurityContext: &corev1.SecurityContext{
								RunAsUser: &runAsUser,
							},
							ReadinessProbe: readinessProbe.GetProbe(),
							LivenessProbe:  livenessProbe.GetProbe(),
							Env: []corev1.EnvVar{
								{
									Name:  "KOLLA_CONFIG_FILE",
									Value: "/var/lib/config-data/merged/nova-scheduler-config.json",
								},
								{
									Name:  "KOLLA_CONFIG_STRATEGY",
									Value: "COPY_ALWAYS",
								},
								{
									Name:  "SCRIPTS_CONFIG_HASH",
									Value: scriptsConfigMapHash,
								},
								{
									Name:  "CONFIG_HASH",
									Value: configHash,
								},
								{
									Name:  "CUSTOM_CONFIG_HASH",
									Value: customConfigHash,
								},
							},
							VolumeMounts: common.GetCtrlVolumeMounts(),
						},
					},
				},
			},
		},
	}
	initContainerDetails := common.CtrlInitContainer{
		ContainerImage:     cr.Spec.ContainerImage,
		DatabaseHost:       cr.Spec.DatabaseHostname,
		CellDatabase:       fmt.Sprintf("%s_%s", DatabasePrefix, CellDatabase),
		APIDatabase:        fmt.Sprintf("%s_%s", DatabasePrefix, APIDatabase),
		TransportURLSecret: cr.Spec.TransportURLSecret,
		NovaSecret:         cr.Spec.NovaSecret,
		NeutronSecret:      cr.Spec.NeutronSecret,
		PlacementSecret:    cr.Spec.PlacementSecret,
	}
	statefulSet.Spec.Template.Spec.InitContainers = common.GetCtrlInitContainer(initContainerDetails)
	statefulSet.Spec.Template.Spec.Volumes = common.GetCtrlVolumes(cr.Spec.ManagingCrName)
	controllerutil.SetControllerReference(cr, statefulSet, scheme)
	return statefulSet
}
