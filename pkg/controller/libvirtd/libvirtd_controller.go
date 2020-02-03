package libvirtd

import (
	"context"

	novav1 "github.com/nova-operator/pkg/apis/nova/v1"
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
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_libvirtd")

// TODO move to spec like image urls?
const (
        COMMON_CONFIGMAP      string = "common-config"
        LIBVIRT_CONFIGMAP     string = "libvirt-config"
        LIBVIRT_BIN_CONFIGMAP string = "libvirt-bin"
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

	// Define a new Pod object
	pod := newDaemonset(instance)

	// Set Libvirtd instance as the owner and controller
	if err := controllerutil.SetControllerReference(instance, pod, r.scheme); err != nil {
		return reconcile.Result{}, err
	}

	// Check if this Pod already exists
	found := &corev1.Pod{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		reqLogger.Info("Creating a new Pod", "Pod.Namespace", pod.Namespace, "Pod.Name", pod.Name)
		err = r.client.Create(context.TODO(), pod)
		if err != nil {
			return reconcile.Result{}, err
		}

		// Pod created successfully - don't requeue
		return reconcile.Result{}, nil
	} else if err != nil {
		return reconcile.Result{}, err
	}

	// Pod already exists - don't requeue
	reqLogger.Info("Skip reconcile: Pod already exists", "Pod.Namespace", found.Namespace, "Pod.Name", found.Name)
	return reconcile.Result{}, nil
}

func newDaemonset(cr *novav1.Libvirtd) *appsv1.DaemonSet {
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
                        Name:      cr.Name + "-daemonset",
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
                                // MatchLabels: map[string]string{"daemonset": cr.Spec.NodeName + cr.Name + "-daemonset"},
                                MatchLabels: map[string]string{"daemonset": cr.Name + "-daemonset"},
                        },
                        Template: corev1.PodTemplateSpec{
                                ObjectMeta: metav1.ObjectMeta{
                                        // Labels: map[string]string{"daemonset": cr.Spec.NodeName + cr.Name + "-daemonset"},
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
                        "bash", "-c", "/tmp/libvirt.sh",
                        //"/usr/sbin/libvirtd", "--listen", "--config", "/etc/libvirt/libvirtd.conf",
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
                                Name:      "libvirt-config",
                                ReadOnly:  true,
                                MountPath: "/etc/libvirt/libvirtd.conf",
                                SubPath:   "libvirtd.conf",
                        },
                        {
                                Name:      "libvirt-bin",
                                ReadOnly:  true,
                                MountPath: "/tmp/libvirt.sh",
                                SubPath:   "libvirt.sh",
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
                        Name: "libvirt-config",
                        VolumeSource: corev1.VolumeSource{
                                ConfigMap: &corev1.ConfigMapVolumeSource{
                                         DefaultMode: &configVolumeDefaultMode,
                                         LocalObjectReference: corev1.LocalObjectReference{
                                                 Name: LIBVIRT_CONFIGMAP,
                                         },
                                },
                        },
                },
                {
                        Name: "libvirt-bin",
                        VolumeSource: corev1.VolumeSource{
                                ConfigMap: &corev1.ConfigMapVolumeSource{
                                         DefaultMode: &configVolumeBinMode,
                                         LocalObjectReference: corev1.LocalObjectReference{
                                                 Name: LIBVIRT_BIN_CONFIGMAP,
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
