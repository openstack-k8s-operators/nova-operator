package libvirtd

import (
	"context"
        "reflect"
        "time"

	novav1 "github.com/nova-operator/pkg/apis/nova/v1"
        appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
        libvirtd "github.com/nova-operator/pkg/libvirtd"
        util "github.com/nova-operator/pkg/util"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_libvirtd")

// TODO move to spec like image urls?
const (
        COMMON_CONFIGMAP      string = "common-config"
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
        } else if !reflect.DeepEqual(util.ObjectHash(configMap.Data), util.ObjectHash(foundConfigMap.Data)) {
                reqLogger.Info("Updating ConfigMap")

                configMap.Data = foundConfigMap.Data
        }

        configMapHash := util.ObjectHash(configMap)
        reqLogger.Info("ConfigMapHash: ", "Data Hash:", configMapHash)

        // Define a new Daemonset object
        ds := newDaemonset(instance, instance.Name, configMapHash)
        dsHash := util.ObjectHash(ds)
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
        var bidirectional corev1.MountPropagationMode = corev1.MountPropagationBidirectional
        var hostToContainer corev1.MountPropagationMode = corev1.MountPropagationHostToContainer
        var trueVar bool = true
        var falseVar bool = false
        var configVolumeDefaultMode int32 = 0644
        var configVolumeBinMode     int32 = 0755
        var dirOrCreate corev1.HostPathType = corev1.HostPathDirectoryOrCreate

        daemonSet := appsv1.DaemonSet{
                TypeMeta: metav1.TypeMeta{
                        Kind:       "DaemonSet",
                        APIVersion: "apps/v1",
                },
                ObjectMeta: metav1.ObjectMeta{
                        Name:      cmName,
                        //Name:      fmt.Sprintf("%s-nova-%s",cr.Name, cr.Spec.NodeName),
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
                                        NodeSelector:   map[string]string{"daemon": cr.Spec.Label},
                                        HostNetwork:    true,
                                        HostPID:        true,
                                        DNSPolicy:      "ClusterFirstWithHostNet",
                                        InitContainers: []corev1.Container{},
                                        Containers:     []corev1.Container{},
                                },
                        },
                },
        }

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
                        "bash", "-c", "/tmp/libvirtd.sh",
                },
                Lifecycle: &corev1.Lifecycle {
                        PreStop: &corev1.Handler{
                                Exec: &corev1.ExecAction{
                                        Command: []string{
                                                "bash", "-c", "kill $(cat /var/run/libvirtd.pid)",
                                        },
                                },
                        },
                },
                SecurityContext: &corev1.SecurityContext{
                        Privileged:  &trueVar,
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
                                Name:      "etc-libvirt-qemu-volume",
                                MountPath: "/etc/libvirt/qemu",
                                MountPropagation: &bidirectional,
                        },
                        {
                                Name:      "lib-modules-volume",
                                MountPath: "/lib/modules",
                                MountPropagation: &hostToContainer,
                        },
                        {
                                Name:      "dev-volume",
                                MountPath: "/dev",
                                MountPropagation: &hostToContainer,
                        },
                        {
                                Name:      "run-volume",
                                MountPath: "/run",
                                MountPropagation: &hostToContainer,
                        },
                        {
                                Name:      "sys-fs-cgroup-volume",
                                MountPath: "/sys/fs/cgroup",
                        },
                        {
                                Name:      "run-libvirt-volume",
                                MountPath: "/var/run/libvirt",
                                MountPropagation: &bidirectional,
                        },
                        {
                                Name:      "libvirt-log-volume",
                                MountPath: "/var/log/libvirt",
                                MountPropagation: &bidirectional,
                        },
                        {
                                Name:      "var-lib-nova-volume",
                                MountPath: "/var/lib/nova",
                                MountPropagation: &bidirectional,
                        },
                        {
                                Name:      "var-lib-libvirt-volume",
                                MountPath: "/var/lib/libvirt",
                                MountPropagation: &bidirectional,
                        },
                },
        }
        daemonSet.Spec.Template.Spec.Containers = append(daemonSet.Spec.Template.Spec.Containers, libvirtContainerSpec)

        volConfigs := []corev1.Volume{
                {
                        Name: "etc-libvirt-qemu-volume",
                        VolumeSource: corev1.VolumeSource{
                                HostPath: &corev1.HostPathVolumeSource{
                                        Path: "/opt/osp/etc/libvirt/qemu",
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
                        Name: "run-volume",
                        VolumeSource: corev1.VolumeSource{
                                HostPath: &corev1.HostPathVolumeSource{
                                        Path: "/run",
                                },
                        },
                },
                {
                        Name: "dev-volume",
                        VolumeSource: corev1.VolumeSource{
                                HostPath: &corev1.HostPathVolumeSource{
                                        Path: "/dev",
                                },
                        },
                },
                {
                        Name: "sys-fs-cgroup-volume",
                        VolumeSource: corev1.VolumeSource{
                                HostPath: &corev1.HostPathVolumeSource{
                                        Path: "/sys/fs/cgroup",
                                },
                        },
                },
                {
                        Name: "run-libvirt-volume",
                        VolumeSource: corev1.VolumeSource{
                                HostPath: &corev1.HostPathVolumeSource{
                                        Path: "/var/run/libvirt",
                                        Type: &dirOrCreate,
                                },
                        },
                },
                {
                        Name: "var-lib-nova-volume",
                        VolumeSource: corev1.VolumeSource{
                                HostPath: &corev1.HostPathVolumeSource{
                                        Path: "/var/lib/nova",
                                        Type: &dirOrCreate,
                                },
                        },
                },
                {
                        Name: "var-lib-libvirt-volume",
                        VolumeSource: corev1.VolumeSource{
                                HostPath: &corev1.HostPathVolumeSource{
                                        Path: "/var/lib/libvirt",
                                        Type: &dirOrCreate,
                                },
                        },
                },
                {
                        Name: "lib-modules-volume",
                        VolumeSource: corev1.VolumeSource{
                                HostPath: &corev1.HostPathVolumeSource{
                                        Path: "/lib/modules",
                                },
                        },
                },
                {
                        Name: "libvirt-log-volume",
                        VolumeSource: corev1.VolumeSource{
                                HostPath: &corev1.HostPathVolumeSource{
                                        Path: "/var/log/containers/libvirt",
                                        Type: &dirOrCreate,
                                },
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
                                                 Name: COMMON_CONFIGMAP,
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
