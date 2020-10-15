/*
Copyright 2020 Red Hat

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

package controllers

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	util "github.com/openstack-k8s-operators/lib-common/pkg/util"
	novav1beta1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	common "github.com/openstack-k8s-operators/nova-operator/pkg/common"
	novamigrationtarget "github.com/openstack-k8s-operators/nova-operator/pkg/novamigrationtarget"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// NovaMigrationTargetReconciler reconciles a NovaMigrationTarget object
type NovaMigrationTargetReconciler struct {
	client.Client
	Kclient kubernetes.Interface
	Log     logr.Logger
	Scheme  *runtime.Scheme
}

// GetClient -
func (r *NovaMigrationTargetReconciler) GetClient() client.Client {
	return r.Client
}

// GetLogger -
func (r *NovaMigrationTargetReconciler) GetLogger() logr.Logger {
	return r.Log
}

// GetScheme -
func (r *NovaMigrationTargetReconciler) GetScheme() *runtime.Scheme {
	return r.Scheme
}

// +kubebuilder:rbac:groups=core,namespace=openstack,resources=pods;services;services/finalizers;endpoints;persistentvolumeclaims;events;configmaps;secrets,verbs=create;delete;get;list;patch;update;watch
// +kubebuilder:rbac:groups=apps,namespace=openstack,resources=daemonsets,verbs=create;delete;get;list;patch;update;watch
// +kubebuilder:rbac:groups=nova.openstack.org,namespace=openstack,resources=novamigrationtargets,verbs=create;delete;get;list;patch;update;watch
// +kubebuilder:rbac:groups=nova.openstack.org,namespace=openstack,resources=novamigrationtargets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=nova.openstack.org,namespace=openstack,resources=deployments/finalizers,verbs=update
// +kubebuilder:rbac:groups=security.openshift.io,namespace=openstack,resources=securitycontextconstraints,resourceNames=privileged,verbs=use

// Reconcile reconcile nova migration target API requests
func (r *NovaMigrationTargetReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	_ = r.Log.WithValues("novamigrationtarget", req.NamespacedName)

	// Fetch the NovaMigrationTarget instance
	instance := &novav1beta1.NovaMigrationTarget{}
	err := r.Client.Get(context.TODO(), req.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	envVars := make(map[string]util.EnvSetter)

	// Create/update configmaps from templates
	cmLabels := common.GetLabels(instance.Name, novamigrationtarget.AppLabel)
	cmLabels["upper-cr"] = instance.Name

	templateParameters := make(map[string]string)
	templateParameters["SshdPort"] = strconv.Itoa(int(instance.Spec.SshdPort))

	cms := []common.ConfigMap{
		// ScriptsConfigMap
		{
			Name:           fmt.Sprintf("%s-scripts", instance.Name),
			Namespace:      instance.Namespace,
			CMType:         common.CMTypeScripts,
			InstanceType:   instance.Kind,
			AdditionalData: map[string]string{"common.sh": "/common/common.sh"},
			Labels:         cmLabels,
		},
		// ConfigMap
		{
			Name:           fmt.Sprintf("%s-config-data", instance.Name),
			Namespace:      instance.Namespace,
			CMType:         common.CMTypeConfig,
			InstanceType:   instance.Kind,
			AdditionalData: map[string]string{},
			Labels:         cmLabels,
			ConfigOptions:  templateParameters,
		},
		// CustomConfigMap
		{
			Name:      fmt.Sprintf("%s-config-data-custom", instance.Name),
			Namespace: instance.Namespace,
			CMType:    common.CMTypeCustom,
			Labels:    cmLabels,
		},
	}
	err = common.EnsureConfigMaps(r, instance, cms, &envVars)
	if err != nil {
		return ctrl.Result{}, nil
	}

	// Secret - used for migration
	// the secret is used in libvirtd, nova-compute and nova-migration-target
	secretName := strings.ToLower(novamigrationtarget.AppLabel) + "-ssh-keys"
	secret, secretHash, err := common.GetSecret(r.Client, secretName, instance.Namespace)
	if err != nil && errors.IsNotFound(err) {
		var op controllerutil.OperationResult
		secret, err = common.SSHKeySecret(secretName, instance.Namespace, map[string]string{secretName: ""})
		if err != nil {
			return ctrl.Result{}, err
		}
		secretHash, op, err = common.CreateOrUpdateSecret(r, instance, secret)
		if err != nil {
			return ctrl.Result{}, err
		}
		if op != controllerutil.OperationResultNone {
			r.Log.Info(fmt.Sprintf("Secret %s successfully reconciled - operation: %s", secret.Name, string(op)))
			return ctrl.Result{}, nil
		}
	} else if err != nil {
		return ctrl.Result{}, fmt.Errorf("error get secret %s: %v", secretName, err)
	}
	envVars[secret.Name] = util.EnvValue(secretHash)

	// Create or update the Daemonset object
	op, err := r.daemonsetCreateOrUpdate(instance, envVars)
	if err != nil {
		return ctrl.Result{}, err
	}
	if op != controllerutil.OperationResultNone {
		r.Log.Info(fmt.Sprintf("DaemonSet %s successfully reconciled - operation: %s", instance.Name, string(op)))
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

// SetupWithManager -
func (r *NovaMigrationTargetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&novav1beta1.NovaMigrationTarget{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Owns(&appsv1.DaemonSet{}).
		Complete(r)
}

func (r *NovaMigrationTargetReconciler) daemonsetCreateOrUpdate(instance *novav1beta1.NovaMigrationTarget, envVars map[string]util.EnvSetter) (controllerutil.OperationResult, error) {
	var trueVar = true
	var runAsUser = int64(0)

	// set KOLLA_CONFIG env vars
	envVars["KOLLA_CONFIG_FILE"] = util.EnvValue(novamigrationtarget.KollaConfig)
	envVars["KOLLA_CONFIG_STRATEGY"] = util.EnvValue("COPY_ALWAYS")

	// get readinessProbes
	readinessProbe := util.Probe{ProbeType: "readiness"}
	livenessProbe := util.Probe{ProbeType: "liveness"}

	// get volumes
	initVolumeMounts := common.GetInitVolumeMounts()
	// add novamigrationtarget init specific VolumeMounts
	for _, volMount := range novamigrationtarget.GetInitVolumeMounts(instance.Name) {
		initVolumeMounts = append(initVolumeMounts, volMount)
	}
	volumeMounts := common.GetVolumeMounts()
	// add novamigrationtarget specific VolumeMounts
	for _, volMount := range novamigrationtarget.GetVolumeMounts(instance) {
		volumeMounts = append(volumeMounts, volMount)
	}
	volumes := common.GetVolumes(instance.Name)
	// add novamigrationtarget Volumes
	for _, vol := range novamigrationtarget.GetVolumes(instance) {
		volumes = append(volumes, vol)
	}

	// tolerations
	tolerations := []corev1.Toleration{}
	// add compute worker nodes tolerations
	for _, toleration := range common.GetComputeWorkerTolerations(instance.Spec.RoleName) {
		tolerations = append(tolerations, toleration)
	}

	daemonSet := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(context.TODO(), r.Client, daemonSet, func() error {

		// Daemonset selector is immutable so we set this value only if
		// a new object is going to be created
		if daemonSet.ObjectMeta.CreationTimestamp.IsZero() {
			daemonSet.Spec.Selector = &metav1.LabelSelector{
				MatchLabels: common.GetLabels(instance.Name, novamigrationtarget.AppLabel),
			}
		}

		if len(daemonSet.Spec.Template.Spec.Containers) != 1 {
			daemonSet.Spec.Template.Spec.Containers = make([]corev1.Container, 1)
		}
		envs := util.MergeEnvs(daemonSet.Spec.Template.Spec.Containers[0].Env, envVars)

		// labels
		common.InitLabelMap(&daemonSet.Spec.Template.Labels)
		for k, v := range common.GetLabels(instance.Name, novamigrationtarget.AppLabel) {
			daemonSet.Spec.Template.Labels[k] = v
		}

		// add PodIP to init container to set local ip on sshd_config
		initEnvVars := util.MergeEnvs([]corev1.EnvVar{}, util.EnvSetterMap{
			"PodIP": util.EnvDownwardAPI("status.podIP"),
		})

		daemonSet.Spec.Template.Spec = corev1.PodSpec{
			ServiceAccountName: serviceAccountName,
			NodeSelector:       common.GetComputeWorkerNodeSelector(instance.Spec.RoleName),
			HostIPC:            true,
			HostNetwork:        true,
			DNSPolicy:          "ClusterFirstWithHostNet",
			Volumes:            volumes,
			Tolerations:        tolerations,
			InitContainers: []corev1.Container{
				{
					Name:  "init",
					Image: instance.Spec.NovaComputeImage,
					SecurityContext: &corev1.SecurityContext{
						RunAsUser:  &runAsUser,
						Privileged: &trueVar,
					},
					Command: []string{
						"/bin/bash", "-c", "/usr/local/bin/container-scripts/init.sh",
					},
					Env:          initEnvVars,
					VolumeMounts: initVolumeMounts,
				},
			},
			Containers: []corev1.Container{
				{
					Name:           "nova-migration-target",
					Image:          instance.Spec.NovaComputeImage,
					ReadinessProbe: readinessProbe.GetProbe(),
					LivenessProbe:  livenessProbe.GetProbe(),
					SecurityContext: &corev1.SecurityContext{
						Privileged: &trueVar,
						RunAsUser:  &runAsUser,
					},
					Env:          envs,
					VolumeMounts: volumeMounts,
				},
			},
		}

		err := controllerutil.SetControllerReference(instance, daemonSet, r.Scheme)
		if err != nil {
			return err
		}

		return nil
	})

	return op, err
}
