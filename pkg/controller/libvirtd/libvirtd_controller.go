package libvirtd

import (
	"context"
	"fmt"
	"reflect"
	"time"

	novav1 "github.com/openstack-k8s-operators/nova-operator/pkg/apis/nova/v1"
	libvirtd "github.com/openstack-k8s-operators/nova-operator/pkg/libvirtd"
	util "github.com/openstack-k8s-operators/nova-operator/pkg/util"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_libvirtd")

// TODO move to spec like image urls?
const (
	CommonConfigMAP string = "common-config"
)

// Add creates a new Libvirtd Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileLibvirtd{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("libvirtd-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource Libvirtd
	err = c.Watch(&source.Kind{Type: &novav1.Libvirtd{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch ConfigMaps owned by Libvirtd
	err = c.Watch(&source.Kind{Type: &corev1.ConfigMap{}}, &handler.EnqueueRequestForOwner{
		IsController: false,
		OwnerType:    &novav1.Libvirtd{},
	})
	if err != nil {
		return err
	}

	// Watch Secrets owned by Libvirtd
	err = c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForOwner{
		IsController: false,
		OwnerType:    &novav1.Libvirtd{},
	})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create that are owned by the primary resource
	// Watch for changes to secondary resource Pods and requeue the owner Libvirtd
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &novav1.Libvirtd{},
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileLibvirtd implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileLibvirtd{}

// ReconcileLibvirtd reconciles a Libvirtd object
type ReconcileLibvirtd struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a Libvirtd object and makes changes based on the state read
// and what is in the Libvirtd.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileLibvirtd) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling Libvirtd")

	// Fetch the Libvirtd instance
	instance := &novav1.Libvirtd{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// ConfigMap
	configMap := libvirtd.ConfigMap(instance, instance.Name)
	if err := controllerutil.SetControllerReference(instance, configMap, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	// Check if this ConfigMap already exists
	foundConfigMap := &corev1.ConfigMap{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: configMap.Name, Namespace: configMap.Namespace}, foundConfigMap)
	if err != nil && errors.IsNotFound(err) {
		reqLogger.Info("Creating a new ConfigMap", "ConfigMap.Namespace", configMap.Namespace, "Job.Name", configMap.Name)
		err = r.client.Create(context.TODO(), configMap)
		if err != nil {
			return reconcile.Result{}, err
		}
	} else if !reflect.DeepEqual(configMap.Data, foundConfigMap.Data) {
		reqLogger.Info("Updating ConfigMap")

		configMap.Data = foundConfigMap.Data
	}

	configMapHash, err := util.ObjectHash(configMap)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("error calculating configuration hash: %v", err)
	} 
  reqLogger.Info("ConfigMapHash: ", "Data Hash:", configMapHash)

	// Define a new Daemonset object
	ds := newDaemonset(instance, instance.Name, configMapHash)
	dsHash, err := util.ObjectHash(ds)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("error calculating configuration hash: %v", err)
	}
	reqLogger.Info("DaemonsetHash: ", "Daemonset Hash:", dsHash)

	// Set Libvirtd instance as the owner and controller
	if err := controllerutil.SetControllerReference(instance, ds, r.scheme); err != nil {
		return reconcile.Result{}, err
	}

	// Check if this Daemonset already exists
	found := &appsv1.DaemonSet{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: ds.Name, Namespace: ds.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		reqLogger.Info("Creating a new Daemonset", "Ds.Namespace", ds.Namespace, "Ds.Name", ds.Name)
		err = r.client.Create(context.TODO(), ds)
		if err != nil {
			return reconcile.Result{}, err
		}

		// Daemonset created successfully - don't requeue
		return reconcile.Result{}, nil
	} else if err != nil {
		return reconcile.Result{}, err
	} else {

		if instance.Status.DaemonsetHash != dsHash {
			reqLogger.Info("Daemonset Updated")
			found.Spec = ds.Spec
			err = r.client.Update(context.TODO(), found)
			if err != nil {
				return reconcile.Result{}, err
			}
			r.setDaemonsetHash(instance, dsHash)
			return reconcile.Result{RequeueAfter: time.Second * 10}, err
		}
	}

	// Daemonset already exists - don't requeue
	reqLogger.Info("Skip reconcile: Daemonset already exists", "Ds.Namespace", found.Namespace, "Ds.Name", found.Name)
	return reconcile.Result{}, nil
}

func (r *ReconcileLibvirtd) setDaemonsetHash(instance *novav1.Libvirtd, hashStr string) error {

	if hashStr != instance.Status.DaemonsetHash {
		instance.Status.DaemonsetHash = hashStr
		if err := r.client.Status().Update(context.TODO(), instance); err != nil {
			return err
		}
	}
	return nil

}

func newDaemonset(cr *novav1.Libvirtd, cmName string, configHash string) *appsv1.DaemonSet {
	var bidirectional = corev1.MountPropagationBidirectional
	var trueVar = true
	var falseVar = false
	var configVolumeDefaultMode int32 = 0644
	var configVolumeBinMode int32 = 0755
	var dirOrCreate = corev1.HostPathDirectoryOrCreate

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
					NodeSelector:       map[string]string{"daemon": cr.Spec.Label},
					HostNetwork:        true,
					HostPID:            true,
					DNSPolicy:          "ClusterFirstWithHostNet",
					InitContainers:     []corev1.Container{},
					Containers:         []corev1.Container{},
					Tolerations:        []corev1.Toleration{},
					ServiceAccountName: cr.Spec.ServiceAccount,
				},
			},
		},
	}

	tolerationSpec := corev1.Toleration{
		Operator: "Exists",
	}
	daemonSet.Spec.Template.Spec.Tolerations = append(daemonSet.Spec.Template.Spec.Tolerations, tolerationSpec)

	initContainerSpec := corev1.Container{
		Name:  "libvirtd-config-init",
		Image: cr.Spec.NovaLibvirtImage,
		SecurityContext: &corev1.SecurityContext{
			Privileged: &trueVar,
		},
		Command: []string{
			//Set correct owner/permissions for the ssh identity file
			"/bin/bash", "-c", "cp -a /etc/nova/migration/* /tmp/nova/ && cp -f /tmp/identity /tmp/nova/ && chown nova:nova /tmp/nova/identity && chmod 600 /tmp/nova/identity",
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "etc-machine-id",
				MountPath: "/etc/machine-id",
				ReadOnly:  true,
			},
			{
				// TODO: move to secret
				Name:      cmName,
				ReadOnly:  true,
				MountPath: "/tmp/identity",
				SubPath:   "migration_ssh_identity",
			},
			{
				Name:      "nova-config",
				MountPath: "/tmp/nova",
			},
		},
	}
	daemonSet.Spec.Template.Spec.InitContainers = append(daemonSet.Spec.Template.Spec.InitContainers, initContainerSpec)

	libvirtContainerSpec := corev1.Container{
		Name:  "libvirtd",
		Image: cr.Spec.NovaLibvirtImage,
		//ReadinessProbe: &corev1.Probe{
		//        Handler: corev1.Handler{
		//                Exec: &corev1.ExecAction{
		//                        Command: []string{
		//                                "/openstack/healthcheck", "libvirtd",
		//                        },
		//                },
		//        },
		//        InitialDelaySeconds: 30,
		//        PeriodSeconds:       30,
		//        TimeoutSeconds:      1,
		//},
		Command: []string{
			//"/bin/sleep", "86400",
			"bash", "-c", "/tmp/libvirtd.sh",
		},
		Lifecycle: &corev1.Lifecycle{
			PreStop: &corev1.Handler{
				Exec: &corev1.ExecAction{
					Command: []string{
						"bash", "-c", "kill $(cat /var/run/libvirtd.pid)",
					},
				},
			},
		},
		SecurityContext: &corev1.SecurityContext{
			Privileged:             &trueVar,
			ReadOnlyRootFilesystem: &falseVar,
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      cmName,
				ReadOnly:  true,
				MountPath: "/etc/libvirt/libvirtd.conf",
				SubPath:   "libvirtd.conf",
			},
			{
				Name:      cmName,
				ReadOnly:  true,
				MountPath: "/tmp/libvirtd.sh",
				SubPath:   "libvirtd.sh",
			},
			{
				Name:      "etc-machine-id",
				MountPath: "/etc/machine-id",
				ReadOnly:  true,
			},
			{
				Name:      "etc-libvirt-qemu",
				MountPath: "/etc/libvirt/qemu",
			},
			{
				Name:      "lib-modules",
				MountPath: "/lib/modules",
				ReadOnly:  true,
			},
			{
				Name:      "dev",
				MountPath: "/dev",
			},
			{
				Name:      "run",
				MountPath: "/run",
			},
			{
				Name:      "sys-fs-cgroup",
				MountPath: "/sys/fs/cgroup",
			},
			{
				Name:      "libvirt-log",
				MountPath: "/var/log/libvirt",
			},
			{
				Name:             "var-lib-nova",
				MountPath:        "/var/lib/nova",
				MountPropagation: &bidirectional,
			},
			{
				Name:             "var-lib-libvirt",
				MountPath:        "/var/lib/libvirt",
				MountPropagation: &bidirectional,
			},
			{
				Name:      "var-lib-vhost-sockets",
				MountPath: "/var/lib/vhost_sockets",
			},
			{
				Name:      "nova-config",
				MountPath: "/etc/nova/migration",
				ReadOnly:  true,
			},
		},
	}
	daemonSet.Spec.Template.Spec.Containers = append(daemonSet.Spec.Template.Spec.Containers, libvirtContainerSpec)

	volConfigs := []corev1.Volume{
		{
			Name: "etc-libvirt-qemu",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/etc/libvirt/qemu",
					Type: &dirOrCreate,
				},
			},
		},
		{
			Name: "etc-machine-id",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/etc/machine-id",
				},
			},
		},
		{
			Name: "run",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/run",
				},
			},
		},
		{
			Name: "dev",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/dev",
				},
			},
		},
		{
			Name: "sys-fs-cgroup",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/sys/fs/cgroup",
				},
			},
		},
		{
			Name: "var-run-libvirt",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/run/libvirt",
					Type: &dirOrCreate,
				},
			},
		},
		{
			Name: "var-lib-nova",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/lib/nova",
					Type: &dirOrCreate,
				},
			},
		},
		{
			Name: "var-lib-libvirt",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/lib/libvirt",
					Type: &dirOrCreate,
				},
			},
		},
		{
			Name: "var-lib-vhost-sockets",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/lib/vhost_sockets",
					Type: &dirOrCreate,
				},
			},
		},
		{
			Name: "lib-modules",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/lib/modules",
				},
			},
		},
		{
			Name: "libvirt-log",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/log/containers/libvirt",
					Type: &dirOrCreate,
				},
			},
		},
		{
			Name: "nova-config",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: cmName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					// otherwise the libvirtd.sh script can not be excecuted
					// even with using bash -c
					DefaultMode: &configVolumeBinMode,
					LocalObjectReference: corev1.LocalObjectReference{
						Name: cmName,
					},
				},
			},
		},
		{
			Name: "common-config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					DefaultMode: &configVolumeDefaultMode,
					LocalObjectReference: corev1.LocalObjectReference{
						Name: CommonConfigMAP,
					},
				},
			},
		},
	}
	for _, volConfig := range volConfigs {
		daemonSet.Spec.Template.Spec.Volumes = append(daemonSet.Spec.Template.Spec.Volumes, volConfig)
	}

	return &daemonSet
}
