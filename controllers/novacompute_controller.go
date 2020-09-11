/*


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
	"reflect"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	util "github.com/openstack-k8s-operators/lib-common/pkg/util"
	novav1beta1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	common "github.com/openstack-k8s-operators/nova-operator/pkg/common"
	"github.com/openstack-k8s-operators/nova-operator/pkg/novacompute"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

var ospHostAliases = []corev1.HostAlias{}

// NovaComputeReconciler reconciles a NovaCompute object
type NovaComputeReconciler struct {
	client.Client
	Kclient kubernetes.Interface
	Log     logr.Logger
	Scheme  *runtime.Scheme
}

// +kubebuilder:rbac:groups=core,namespace=openstack,resources=pods;services;services/finalizers;endpoints;persistentvolumeclaims;events;configmaps;secrets,verbs=create;delete;get;list;patch;update;watch
// +kubebuilder:rbac:groups=apps,namespace=openstack,resources=daemonsets,verbs=create;delete;get;list;patch;update;watch
// +kubebuilder:rbac:groups=nova.openstack.org,namespace=openstack,resources=novacomputes,verbs=create;delete;get;list;patch;update;watch
// +kubebuilder:rbac:groups=nova.openstack.org,namespace=openstack,resources=novacomputes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=nova.openstack.org,namespace=openstack,resources=deployments/finalizers,verbs=update
// +kubebuilder:rbac:groups=security.openshift.io,namespace=openstack,resources=securitycontextconstraints,resourceNames=privileged,verbs=use

// Reconcile reconcile nova compute API requests
func (r *NovaComputeReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	_ = r.Log.WithValues("novacompute", req.NamespacedName)

	// Fetch the NovaCompute instance
	instance := &novav1beta1.NovaCompute{}
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

	// get instance.Spec.CommonConfigMap which holds general information on the OSP environment
	// TODO: handle commonConfigMap data change
	commonConfigMap := &corev1.ConfigMap{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: instance.Spec.CommonConfigMap, Namespace: instance.Namespace}, commonConfigMap)
	if err != nil && errors.IsNotFound(err) {
		r.Log.Error(err, instance.Spec.CommonConfigMap+" ConfigMap not found!", "Instance.Namespace", instance.Namespace, "Instance.Name", instance.Name)
		return ctrl.Result{}, err
	}
	if err := controllerutil.SetControllerReference(instance, commonConfigMap, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	// get insatnce.Spec.OspSecret which holds passwords and other sensitive information from the OSP environment
	// TODO: handle secrets data change, like pwd change
	ospSecrets := &corev1.Secret{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: instance.Spec.OspSecrets, Namespace: instance.Namespace}, ospSecrets)
	if err != nil && errors.IsNotFound(err) {
		r.Log.Error(err, instance.Spec.OspSecrets+" Secret not found!", "Instance.Namespace", instance.Namespace, "Instance.Name", instance.Name)
		return ctrl.Result{}, err
	}
	if err := controllerutil.SetControllerReference(instance, ospSecrets, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	// Create additional host entries added to the /etc/hosts file of the containers
	ospHostAliases, err = util.CreateOspHostsEntries(commonConfigMap)
	if err != nil {
		r.Log.Error(err, "Failed ospHostAliases", "Instance.Namespace", instance.Namespace, "Instance.Name", instance.Name)
		return ctrl.Result{}, err
	}

	// Create additional host entries added to the /etc/hosts file of the containers
	ospHostAliases, err = util.CreateOspHostsEntries(commonConfigMap)
	if err != nil {
		r.Log.Error(err, "Failed ospHostAliases", "Instance.Namespace", instance.Namespace, "Instance.Name", instance.Name)
		return ctrl.Result{}, err
	}

	// ScriptsConfigMap
	scriptsConfigMap := novacompute.ScriptsConfigMap(instance, instance.Name+"-scripts")
	if err := controllerutil.SetControllerReference(instance, scriptsConfigMap, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}
	// Check if this ScriptsConfigMap already exists
	foundScriptsConfigMap := &corev1.ConfigMap{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: scriptsConfigMap.Name, Namespace: scriptsConfigMap.Namespace}, foundScriptsConfigMap)
	if err != nil && errors.IsNotFound(err) {
		r.Log.Info("Creating a new ScriptsConfigMap", "ScriptsConfigMap.Namespace", scriptsConfigMap.Namespace, "Job.Name", scriptsConfigMap.Name)
		err = r.Client.Create(context.TODO(), scriptsConfigMap)
		if err != nil {
			return ctrl.Result{}, err
		}
	} else if !reflect.DeepEqual(scriptsConfigMap.Data, foundScriptsConfigMap.Data) {
		r.Log.Info("Updating ScriptsConfigMap")

		scriptsConfigMap.Data = foundScriptsConfigMap.Data
	}
	scriptsConfigMapHash, err := util.ObjectHash(scriptsConfigMap.Data)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error calculating configuration hash: %v", err)
	}
	r.Log.Info("ScriptsConfigMapHash: ", "Data Hash:", scriptsConfigMapHash)

	// templatesConfigMap
	// TODO: when config handling is set this needs to be changed!! Right now passwords get stored in the resulting CM
	templatesConfigMap := novacompute.TemplatesConfigMap(instance, commonConfigMap, ospSecrets, instance.Name+"-templates")
	if err := controllerutil.SetControllerReference(instance, templatesConfigMap, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}
	// Check if this TemplatesConfigMap already exists
	foundTemplatesConfigMap := &corev1.ConfigMap{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: templatesConfigMap.Name, Namespace: templatesConfigMap.Namespace}, foundTemplatesConfigMap)
	if err != nil && errors.IsNotFound(err) {
		r.Log.Info("Creating a new TemplatesConfigMap", "TemplatesConfigMap.Namespace", templatesConfigMap.Namespace, "Job.Name", templatesConfigMap.Name)
		err = r.Client.Create(context.TODO(), templatesConfigMap)
		if err != nil {
			return ctrl.Result{}, err
		}
	} else if !reflect.DeepEqual(templatesConfigMap.Data, foundTemplatesConfigMap.Data) {
		r.Log.Info("Updating TemplatesConfigMap")

		templatesConfigMap.Data = foundTemplatesConfigMap.Data
	}

	templatesConfigMapHash, err := util.ObjectHash(templatesConfigMap.Data)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error calculating configuration hash: %v", err)
	}
	r.Log.Info("TemplatesConfigMapHash: ", "Data Hash:", templatesConfigMapHash)

	// Define a new Daemonset object
	ds := newDaemonset(instance, instance.Name, templatesConfigMapHash, scriptsConfigMapHash)
	dsHash, err := util.ObjectHash(ds)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error calculating configuration hash: %v", err)
	}
	r.Log.Info("DaemonsetHash: ", "Daemonset Hash:", dsHash)

	// Set NovaCompute instance as the owner and controller
	if err := controllerutil.SetControllerReference(instance, ds, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	// Check if this Daemonset already exists
	found := &appsv1.DaemonSet{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: ds.Name, Namespace: ds.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		r.Log.Info("Creating a new Daemonset", "Ds.Namespace", ds.Namespace, "Ds.Name", ds.Name)
		err = r.Client.Create(context.TODO(), ds)
		if err != nil {
			return ctrl.Result{}, err
		}

		// Daemonset created successfully - don't requeue
		return ctrl.Result{}, nil
	} else if err != nil {
		return ctrl.Result{}, err
	} else {

		if instance.Status.DaemonsetHash != dsHash {
			r.Log.Info("Daemonset Updated")
			found.Spec = ds.Spec
			err = r.Client.Update(context.TODO(), found)
			if err != nil {
				return ctrl.Result{}, err
			}
			r.setDaemonsetHash(instance, dsHash)
			return ctrl.Result{RequeueAfter: time.Second * 10}, err
		}
		//                if found.Status.ReadyNovaComputeStatus == instance.Spec.NovaComputeStatus {
		//                        reqLogger.Info("Daemonsets running:", "Daemonsets", found.Status.ReadyNovaComputeStatus)
		//                } else {
		//                        reqLogger.Info("Waiting on Nova Compute Daemonset...")
		//                        return ctrl.Result{RequeueAfter: time.Second * 5}, err
		//                }
	}

	return ctrl.Result{}, nil
}

// SetupWithManager -
func (r *NovaComputeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&novav1beta1.NovaCompute{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Owns(&appsv1.DaemonSet{}).
		Complete(r)
}

func (r *NovaComputeReconciler) setDaemonsetHash(instance *novav1beta1.NovaCompute, hashStr string) error {

	if hashStr != instance.Status.DaemonsetHash {
		instance.Status.DaemonsetHash = hashStr
		if err := r.Client.Status().Update(context.TODO(), instance); err != nil {
			return err
		}
	}
	return nil

}

func newDaemonset(cr *novav1beta1.NovaCompute, cmName string, templatesConfigHash string, scriptsConfigHash string) *appsv1.DaemonSet {
	var trueVar = true
	var terminationGracePeriodSeconds int64 = 0

	daemonSet := appsv1.DaemonSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DaemonSet",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: cr.Namespace,
			//OwnerReferences: []metav1.OwnerReference{
			//      *metav1.NewControllerRef(cr, schema.GroupVersionKind{
			//              Group:   v1beta1.SchemeGroupVersion.Group,
			//              Version: v1beta1.SchemeGroupVersion.Version,
			//              Kind:    "GenericDaemon",
			//      }),
			//},
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"daemonset": cr.Name + "-daemonset"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"daemonset": cr.Name + "-daemonset"},
				},
				Spec: corev1.PodSpec{
					NodeSelector:                  common.GetComputeWorkerNodeSelector(cr.Spec.RoleName),
					HostNetwork:                   true,
					HostPID:                       true,
					DNSPolicy:                     "ClusterFirstWithHostNet",
					HostAliases:                   ospHostAliases,
					InitContainers:                []corev1.Container{},
					Containers:                    []corev1.Container{},
					Tolerations:                   []corev1.Toleration{},
					ServiceAccountName:            cr.Spec.ServiceAccount,
					TerminationGracePeriodSeconds: &terminationGracePeriodSeconds,
				},
			},
		},
	}

	// add compute worker nodes tolerations
	for _, toleration := range common.GetComputeWorkerTolerations(cr.Spec.RoleName) {
		daemonSet.Spec.Template.Spec.Tolerations = append(daemonSet.Spec.Template.Spec.Tolerations, toleration)
	}

	initContainerSpec := corev1.Container{
		Name:  "init",
		Image: cr.Spec.NovaComputeImage,
		SecurityContext: &corev1.SecurityContext{
			Privileged: &trueVar,
		},
		Command: []string{
			"/bin/bash", "-c", "/tmp/container-scripts/init.sh",
		},
		Env: []corev1.EnvVar{
			{
				Name:  "CONFIG_VOLUME",
				Value: "/var/lib/kolla/config_files/src",
			},
			{
				Name:  "TEMPLATES_VOLUME",
				Value: "/tmp/container-templates",
			},
			{
				// TODO: mschuppert- change to get the info per route
				// for now we get the keystoneAPI from common-config
				Name: "CTRL_PLANE_ENDPOINT",
				ValueFrom: &corev1.EnvVarSource{
					ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "common-config"},
						Key:                  "keystoneAPI",
					},
				},
			},
		},
		VolumeMounts: []corev1.VolumeMount{},
	}
	// initContainer VolumeMounts
	// add common VolumeMounts
	for _, volMount := range common.GetVolumeMounts() {
		initContainerSpec.VolumeMounts = append(initContainerSpec.VolumeMounts, volMount)
	}
	// add novacompute init specific VolumeMounts
	for _, volMount := range novacompute.GetInitContainerVolumeMounts(cmName) {
		initContainerSpec.VolumeMounts = append(initContainerSpec.VolumeMounts, volMount)
	}
	daemonSet.Spec.Template.Spec.InitContainers = append(daemonSet.Spec.Template.Spec.InitContainers, initContainerSpec)

	containerSpec := corev1.Container{
		Name:  "nova-compute",
		Image: cr.Spec.NovaComputeImage,
		ReadinessProbe: &corev1.Probe{
			Handler: corev1.Handler{
				Exec: &corev1.ExecAction{
					Command: []string{
						"/openstack/healthcheck",
					},
				},
			},
			InitialDelaySeconds: 30,
			PeriodSeconds:       30,
			TimeoutSeconds:      3,
		},
		LivenessProbe: &corev1.Probe{
			Handler: corev1.Handler{
				Exec: &corev1.ExecAction{
					Command: []string{
						"/openstack/healthcheck",
					},
				},
			},
			InitialDelaySeconds: 30,
			PeriodSeconds:       60,
			TimeoutSeconds:      3,
			FailureThreshold:    5,
		},
		Command: []string{},
		SecurityContext: &corev1.SecurityContext{
			Privileged: &trueVar,
		},
		Env: []corev1.EnvVar{
			{
				Name:  "TEMPLATES_CONFIG_HASH",
				Value: templatesConfigHash,
			},
			{
				Name:  "SCRIPTS_CONFIG_HASH",
				Value: scriptsConfigHash,
			},
			{
				Name:  "KOLLA_CONFIG_STRATEGY",
				Value: "COPY_ALWAYS",
			},
			{
				// TODO: mschuppert- change to get the info per route
				// for now we get the keystoneAPI from common-config
				Name: "CTRL_PLANE_ENDPOINT",
				ValueFrom: &corev1.EnvVarSource{
					ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "common-config"},
						Key:                  "keystoneAPI",
					},
				},
			},
		},
		VolumeMounts: []corev1.VolumeMount{},
	}

	// VolumeMounts
	// add common VolumeMounts
	for _, volMount := range common.GetVolumeMounts() {
		containerSpec.VolumeMounts = append(containerSpec.VolumeMounts, volMount)
	}
	// add libvirtd specific VolumeMounts
	for _, volMount := range novacompute.GetVolumeMounts(cmName) {
		containerSpec.VolumeMounts = append(containerSpec.VolumeMounts, volMount)
	}
	daemonSet.Spec.Template.Spec.Containers = append(daemonSet.Spec.Template.Spec.Containers, containerSpec)

	// Volume config
	// add common Volumes
	for _, volConfig := range common.GetVolumes(cmName) {
		daemonSet.Spec.Template.Spec.Volumes = append(daemonSet.Spec.Template.Spec.Volumes, volConfig)
	}
	// add libvird Volumes
	for _, volConfig := range novacompute.GetVolumes(cmName) {
		daemonSet.Spec.Template.Spec.Volumes = append(daemonSet.Spec.Template.Spec.Volumes, volConfig)
	}

	return &daemonSet
}
