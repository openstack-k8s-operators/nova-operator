package novacompute

import (
	"context"
	"fmt"
	"reflect"
	"time"

	novav1 "github.com/openstack-k8s-operators/nova-operator/pkg/apis/nova/v1"
	nova "github.com/openstack-k8s-operators/nova-operator/pkg/novacompute"
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

var log = logf.Log.WithName("controller_novacompute")
var ospHostAliases = []corev1.HostAlias{}

// TODO move to spec like image urls?
const (
	CommonConfigMAP string = "common-config"
)

// Add creates a new NovaCompute Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileNovaCompute{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("novacompute-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource NovaCompute
	err = c.Watch(&source.Kind{Type: &novav1.NovaCompute{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch ConfigMaps owned by NovaCompute
	err = c.Watch(&source.Kind{Type: &corev1.ConfigMap{}}, &handler.EnqueueRequestForOwner{
		IsController: false,
		OwnerType:    &novav1.NovaCompute{},
	})
	if err != nil {
		return err
	}

	// Watch Secrets owned by NovaCompute
	err = c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForOwner{
		IsController: false,
		OwnerType:    &novav1.NovaCompute{},
	})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create that are owned by the primary resource
	// Watch for changes to secondary resource Daemonset and requeue the owner NovaCompute
	err = c.Watch(&source.Kind{Type: &appsv1.DaemonSet{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &novav1.NovaCompute{},
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileNovaCompute implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileNovaCompute{}

// ReconcileNovaCompute reconciles a NovaCompute object
type ReconcileNovaCompute struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a NovaCompute object and makes changes based on the state read
// and what is in the NovaCompute.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileNovaCompute) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling NovaCompute")

	// Fetch the NovaCompute instance
	instance := &novav1.NovaCompute{}
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

	commonConfigMap := &corev1.ConfigMap{}

	reqLogger.Info("Creating host entries from config map:", "configMap: ", CommonConfigMAP)
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: CommonConfigMAP, Namespace: instance.Namespace}, commonConfigMap)
	if err != nil && errors.IsNotFound(err) {
		reqLogger.Error(err, "common-config ConfigMap not found!", "Instance.Namespace", instance.Namespace, "Instance.Name", instance.Name)
		return reconcile.Result{}, err
	}

	if err := controllerutil.SetControllerReference(instance, commonConfigMap, r.scheme); err != nil {
		return reconcile.Result{}, err
	}

	// Create additional host entries added to the /etc/hosts file of the containers
	ospHostAliases, err = util.CreateOspHostsEntries(commonConfigMap)
	if err != nil {
		reqLogger.Error(err, "Failed ospHostAliases", "Instance.Namespace", instance.Namespace, "Instance.Name", instance.Name)
		return reconcile.Result{}, err
	}

	// ConfigMap
	configMap := nova.ConfigMap(instance, instance.Name)
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

	// Set NovaCompute instance as the owner and controller
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
		//                if found.Status.ReadyNovaComputeStatus == instance.Spec.NovaComputeStatus {
		//                        reqLogger.Info("Daemonsets running:", "Daemonsets", found.Status.ReadyNovaComputeStatus)
		//                } else {
		//                        reqLogger.Info("Waiting on Nova Compute Daemonset...")
		//                        return reconcile.Result{RequeueAfter: time.Second * 5}, err
		//                }
	}

	// Daemonset already exists - don't requeue
	reqLogger.Info("Skip reconcile: Daemonset already exists", "Ds.Namespace", found.Namespace, "Ds.Name", found.Name)
	return reconcile.Result{}, nil
}

func (r *ReconcileNovaCompute) setDaemonsetHash(instance *novav1.NovaCompute, hashStr string) error {

	if hashStr != instance.Status.DaemonsetHash {
		instance.Status.DaemonsetHash = hashStr
		if err := r.client.Status().Update(context.TODO(), instance); err != nil {
			return err
		}
	}
	return nil

}

func newDaemonset(cr *novav1.NovaCompute, cmName string, configHash string) *appsv1.DaemonSet {
	var bidirectional = corev1.MountPropagationBidirectional
	var trueVar = true
	var configVolumeDefaultMode int32 = 0644
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
					HostAliases:        ospHostAliases,
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

	// Add hosts entries rendered from the the config map to the hosts file of the containers in the pod
	// TODO:
	//       - move to some common lib to be used from nova and neutron operator
	//
	//for _, ospHostAlias := range ospHostAliases {
	//        daemonSet.Spec.Template.Spec.HostAliases = append(daemonSet.Spec.Template.Spec.HostAliases, ospHostAlias)
	//}

	initContainerSpec := corev1.Container{
		Name:  "nova-compute-config-init",
		Image: cr.Spec.NovaComputeImage,
		SecurityContext: &corev1.SecurityContext{
			Privileged: &trueVar,
		},
		Command: []string{
			"/bin/bash", "-c", "export CTRL_IP_INTRENALAPI=$(getent hosts controller-0.internalapi | awk '{print $1}') && export POD_IP_INTERNALAPI=$(ip route get $CTRL_IP_INTRENALAPI | awk '{print $5}') && cp -a /etc/nova/* /tmp/nova/ && crudini --set /tmp/nova/nova.conf DEFAULT my_ip $POD_IP_INTERNALAPI && crudini --set /tmp/nova/nova.conf libvirt live_migration_inbound_addr $POD_IP_INTERNALAPI && crudini --set /tmp/nova/nova.conf vnc server_listen $POD_IP_INTERNALAPI && crudini --set /tmp/nova/nova.conf vnc server_proxyclient_address $POD_IP_INTERNALAPI && mkdir -p /var/lib/nova/instances",
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      cmName,
				ReadOnly:  true,
				MountPath: "/etc/nova/nova.conf",
				SubPath:   "nova.conf",
			},
			{
				Name:      cmName,
				ReadOnly:  true,
				MountPath: "/etc/nova/logging.conf",
				SubPath:   "logging.conf",
			},
			{
				Name:      "common-config",
				ReadOnly:  true,
				MountPath: "/common-config",
			},
			{
				Name:      "etc-machine-id",
				MountPath: "/etc/machine-id",
				ReadOnly:  true,
			},
			{
				Name:      "var-lib-nova-volume",
				MountPath: "/var/lib/nova",
			},
			{
				Name:      "nova-config-vol",
				MountPath: "/tmp/nova",
			},
		},
	}
	daemonSet.Spec.Template.Spec.InitContainers = append(daemonSet.Spec.Template.Spec.InitContainers, initContainerSpec)

	novaContainerSpec := corev1.Container{
		Name:  "nova-compute",
		Image: cr.Spec.NovaComputeImage,
		//ReadinessProbe: &corev1.Probe{
		//        Handler: corev1.Handler{
		//                Exec: &corev1.ExecAction{
		//                        Command: []string{
		//                                "/openstack/healthcheck",
		//                        },
		//                },
		//        },
		//        InitialDelaySeconds: 30,
		//        PeriodSeconds:       30,
		//        TimeoutSeconds:      1,
		//},
		Command: []string{
			//"/bin/sleep", "86400",
			"/usr/bin/nova-compute", "--config-file", "/etc/nova/nova.conf",
		},
		SecurityContext: &corev1.SecurityContext{
			Privileged: &trueVar,
		},
		Env: []corev1.EnvVar{
			{
				Name:  "CONFIG_HASH",
				Value: configHash,
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "etc-libvirt-qemu-volume",
				MountPath: "/etc/libvirt/qemu",
			},
			{
				Name:      "etc-machine-id",
				MountPath: "/etc/machine-id",
				ReadOnly:  true,
			},
			{
				Name:      "etc-iscsi-volume",
				MountPath: "/etc/iscsi",
				ReadOnly:  true,
			},
			{
				Name:      "boot-volume",
				MountPath: "/boot",
				ReadOnly:  true,
			},
			{
				Name:      "dev-volume",
				MountPath: "/dev",
			},
			{
				Name:      "lib-modules-volume",
				MountPath: "/lib/modules",
				ReadOnly:  true,
			},
			{
				Name:      "run-volume",
				MountPath: "/run",
			},
			{
				Name:      "sys-fs-cgroup-volume",
				MountPath: "/sys/fs/cgroup",
				ReadOnly:  true,
			},
			{
				Name:      "nova-log-volume",
				MountPath: "/var/log/nova",
			},
			{
				Name:             "var-lib-nova-volume",
				MountPath:        "/var/lib/nova",
				MountPropagation: &bidirectional,
			},
			{
				Name:             "var-lib-libvirt-volume",
				MountPath:        "/var/lib/libvirt",
				MountPropagation: &bidirectional,
			},
			{
				Name:      "var-lib-iscsi-volume",
				MountPath: "/var/lib/iscsi",
			},
			{
				Name:      "nova-config-vol",
				MountPath: "/etc/nova",
				ReadOnly:  true,
			},
		},
	}
	daemonSet.Spec.Template.Spec.Containers = append(daemonSet.Spec.Template.Spec.Containers, novaContainerSpec)

	volConfigs := []corev1.Volume{
		{
			Name: "boot-volume",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/boot",
				},
			},
		},
		{
			Name: "etc-libvirt-qemu-volume",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/etc/libvirt/qemu",
					Type: &dirOrCreate,
				},
			},
		},
		{
			Name: "etc-iscsi-volume",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/etc/iscsi",
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
			Name: "lib-modules-volume",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/lib/modules",
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
			Name: "etc-machine-id",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/etc/machine-id",
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
			Name: "var-lib-iscsi-volume",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/lib/iscsi",
				},
			},
		},
		{
			Name: "nova-log-volume",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/log/containers/nova",
					Type: &dirOrCreate,
				},
			},
		},
		{
			Name: cmName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					DefaultMode: &configVolumeDefaultMode,
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
		{
			Name: "nova-config-vol",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}
	for _, volConfig := range volConfigs {
		daemonSet.Spec.Template.Spec.Volumes = append(daemonSet.Spec.Template.Spec.Volumes, volConfig)
	}

	return &daemonSet
}
