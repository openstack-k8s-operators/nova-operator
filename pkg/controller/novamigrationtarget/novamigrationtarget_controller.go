package novamigrationtarget

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"time"

	novav1 "github.com/openstack-k8s-operators/nova-operator/pkg/apis/nova/v1"
	nova "github.com/openstack-k8s-operators/nova-operator/pkg/novamigrationtarget"
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

var log = logf.Log.WithName("controller_novamigrationtarget")
var ospHostAliases = []corev1.HostAlias{}

// TODO move to spec like image urls?
const (
	CommonConfigMAP string = "common-config"
)

// Add creates a new NovaMigrationTarget Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileNovaMigrationTarget{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("novamigrationtarget-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource NovaMigrationTarget
	err = c.Watch(&source.Kind{Type: &novav1.NovaMigrationTarget{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch ConfigMaps owned by NovaMigrationTarget
	err = c.Watch(&source.Kind{Type: &corev1.ConfigMap{}}, &handler.EnqueueRequestForOwner{
		IsController: false,
		OwnerType:    &novav1.NovaMigrationTarget{},
	})
	if err != nil {
		return err
	}

	// Watch Secrets owned by NovaMigrationTarget
	err = c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForOwner{
		IsController: false,
		OwnerType:    &novav1.NovaMigrationTarget{},
	})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create that are owned by the primary resource
	// Watch for changes to secondary resource Pods and requeue the owner NovaMigrationTarget
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &novav1.NovaMigrationTarget{},
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileNovaMigrationTarget implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileNovaMigrationTarget{}

// ReconcileNovaMigrationTarget reconciles a NovaMigrationTarget object
type ReconcileNovaMigrationTarget struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a NovaMigrationTarget object and makes changes based on the state read
// and what is in the NovaMigrationTarget.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileNovaMigrationTarget) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling NovaMigrationTarget")

	// Fetch the NovaMigrationTarget instance
	instance := &novav1.NovaMigrationTarget{}
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

	// Set NovaTargetMigration instance as the owner and controller
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
		//                if found.Status.ReadyNovaMigrationTargetStatus == instance.Spec.NovaMigrationTargetStatus {
		//                        reqLogger.Info("Daemonsets running:", "Daemonsets", found.Status.ReadyNovaMigrationTargetStatus)
		//                } else {
		//                        reqLogger.Info("Waiting on Nova MigrationTarget Daemonset...")
		//                        return reconcile.Result{RequeueAfter: time.Second * 5}, err
		//                }
	}

	// Daemonset already exists - don't requeue
	reqLogger.Info("Skip reconcile: Daemonset already exists", "Ds.Namespace", found.Namespace, "Ds.Name", found.Name)
	return reconcile.Result{}, nil
}

func (r *ReconcileNovaMigrationTarget) setDaemonsetHash(instance *novav1.NovaMigrationTarget, hashStr string) error {

	if hashStr != instance.Status.DaemonsetHash {
		instance.Status.DaemonsetHash = hashStr
		if err := r.client.Status().Update(context.TODO(), instance); err != nil {
			return err
		}
	}
	return nil

}

func newDaemonset(cr *novav1.NovaMigrationTarget, cmName string, configHash string) *appsv1.DaemonSet {
	var hostToContainer = corev1.MountPropagationHostToContainer
	var trueVar = true
	var userID int64
	var configVolumeDefaultMode int32 = 0600
	var dirOrCreate = corev1.HostPathDirectoryOrCreate

	var sshdPort = strconv.FormatUint(uint64(cr.Spec.SshdPort), 10)

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

	initContainerSpec := corev1.Container{
		Name:  "nova-migration-target-init",
		Image: cr.Spec.NovaComputeImage,
		SecurityContext: &corev1.SecurityContext{
			RunAsUser:  &userID,
			Privileged: &trueVar,
		},
		Command: []string{
			// * make sure /var/lib/nova/.ssh/config is owned by nova:nova
			// * copy /etc/ssh to ssh-config-vol because the ssh_keys group IDs
			//   don't match on host and container and sshd fail to start
			// * /etc/nova/migration/authorized_keys -> group nova_migration
			"/bin/bash", "-c", "export CTRL_IP_INTRENALAPI=$(getent hosts controller-0.internalapi | awk '{print $1}') && export POD_IP_INTERNALAPI=$(ip route get $CTRL_IP_INTRENALAPI | awk '{print $5}') && mkdir -p /var/lib/nova/.ssh && cp -f /tmp/ssh_config /var/lib/nova/.ssh/config && chown nova:nova /var/lib/nova/.ssh/config && cp -a /etc/ssh/* /tmp/ssh/ && chown -R root:root /tmp/ssh/ssh_host* && chmod 600 /tmp/ssh/ssh_host*_key && cp -f /tmp/sshd_config /tmp/ssh/ && sed -i \"s/POD_IP_INTERNALAPI/$POD_IP_INTERNALAPI/g\" /tmp/ssh/sshd_config && cp -a /etc/nova/migration/* /tmp/nova/ && cp -f /tmp/authorized_keys /tmp/nova/ && chown root:nova_migration /tmp/nova/authorized_keys && chmod 640 /tmp/nova/authorized_keys && chmod 755 /tmp/nova",
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "etc-machine-id",
				MountPath: "/etc/machine-id",
				ReadOnly:  true,
			},
			{
				Name:      "etc-ssh",
				MountPath: "/etc/ssh",
				ReadOnly:  true,
			},
			{
				Name:      "var-lib-nova",
				MountPath: "/var/lib/nova",
			},
			{
				Name:      cmName,
				ReadOnly:  true,
				MountPath: "/tmp/ssh_config",
				SubPath:   "migration_ssh_config",
			},
			{
				Name:      cmName,
				ReadOnly:  true,
				MountPath: "/tmp/sshd_config",
				SubPath:   "migration_sshd_config",
			},
			{
				Name: cmName,
				//ReadOnly:  true,
				MountPath: "/tmp/authorized_keys",
				SubPath:   "migration_authorized_keys",
			},
			{
				Name:      "ssh-config-vol",
				MountPath: "/tmp/ssh",
			},
			{
				Name:      "nova-config-vol",
				MountPath: "/tmp/nova",
			},
		},
	}
	daemonSet.Spec.Template.Spec.InitContainers = append(daemonSet.Spec.Template.Spec.InitContainers, initContainerSpec)

	novaMigrationTargetContainerSpec := corev1.Container{
		Name:  "nova-migration-target",
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
		//Env: []corev1.EnvVar{
		//        {
		//                Name:  "SSHDPORT",
		//                Value: sshdPort,
		//        },
		//},
		SecurityContext: &corev1.SecurityContext{
			RunAsUser:  &userID,
			Privileged: &trueVar,
		},
		Command: []string{
			//"/bin/sleep", "86400",
			"/usr/sbin/sshd", "-D", "-p", sshdPort,
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "etc-machine-id",
				MountPath: "/etc/machine-id",
				ReadOnly:  true,
			},
			{
				Name:      "etc-localtime",
				MountPath: "/etc/localtime",
				ReadOnly:  true,
			},
			{
				Name:             "var-lib-nova",
				MountPath:        "/var/lib/nova",
				MountPropagation: &hostToContainer,
			},
			{
				Name:      "run-libvirt",
				MountPath: "/run/libvirt",
				ReadOnly:  true,
			},
			{
				Name:      "ssh-config-vol",
				MountPath: "/etc/ssh",
				ReadOnly:  true,
			},
			{
				Name:      "nova-config-vol",
				MountPath: "/etc/nova/migration",
				ReadOnly:  true,
			},
		},
	}
	daemonSet.Spec.Template.Spec.Containers = append(daemonSet.Spec.Template.Spec.Containers, novaMigrationTargetContainerSpec)

	volConfigs := []corev1.Volume{
		{
			Name: "etc-machine-id",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/etc/machine-id",
				},
			},
		},
		{
			Name: "etc-localtime",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/etc/localtime",
				},
			},
		},
		{
			Name: "etc-ssh",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/etc/ssh",
				},
			},
		},
		{
			Name: "run-libvirt",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/run/libvirt",
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
			Name: "ssh-config-vol",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: "nova-config-vol",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
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
	}
	for _, volConfig := range volConfigs {
		daemonSet.Spec.Template.Spec.Volumes = append(daemonSet.Spec.Template.Spec.Volumes, volConfig)
	}

	return &daemonSet
}
