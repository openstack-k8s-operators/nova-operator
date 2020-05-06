package libvirtd

import (
	"context"
	"fmt"
	"reflect"
	"time"

	novav1 "github.com/openstack-k8s-operators/nova-operator/pkg/apis/nova/v1"
	common "github.com/openstack-k8s-operators/nova-operator/pkg/common"
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

	// ScriptsConfigMap
	scriptsConfigMap := libvirtd.ScriptsConfigMap(instance, instance.Name+"-scripts")
	if err := controllerutil.SetControllerReference(instance, scriptsConfigMap, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	// Check if this ScriptsConfigMap already exists
	foundScriptsConfigMap := &corev1.ConfigMap{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: scriptsConfigMap.Name, Namespace: scriptsConfigMap.Namespace}, foundScriptsConfigMap)
	if err != nil && errors.IsNotFound(err) {
		reqLogger.Info("Creating a new ScriptsConfigMap", "ScriptsConfigMap.Namespace", scriptsConfigMap.Namespace, "Job.Name", scriptsConfigMap.Name)
		err = r.client.Create(context.TODO(), scriptsConfigMap)
		if err != nil {
			return reconcile.Result{}, err
		}
	} else if !reflect.DeepEqual(scriptsConfigMap.Data, foundScriptsConfigMap.Data) {
		reqLogger.Info("Updating ScriptsConfigMap")
		scriptsConfigMap.Data = foundScriptsConfigMap.Data
	}

	scriptsConfigMapHash, err := util.ObjectHash(scriptsConfigMap.Data)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("error calculating configuration hash: %v", err)
	}
	reqLogger.Info("ScriptsConfigMapHash: ", "Data Hash:", scriptsConfigMapHash)

	// TemplatesConfigMap
	templatesConfigMap := libvirtd.TemplatesConfigMap(instance, instance.Name+"-templates")
	if err := controllerutil.SetControllerReference(instance, templatesConfigMap, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	// Check if this TemplatesConfigMap already exists
	foundTemplatesConfigMap := &corev1.ConfigMap{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: templatesConfigMap.Name, Namespace: templatesConfigMap.Namespace}, foundTemplatesConfigMap)
	if err != nil && errors.IsNotFound(err) {
		reqLogger.Info("Creating a new TemplatesConfigMap", "TemplatesConfigMap.Namespace", templatesConfigMap.Namespace, "Job.Name", templatesConfigMap.Name)
		err = r.client.Create(context.TODO(), templatesConfigMap)
		if err != nil {
			return reconcile.Result{}, err
		}
	} else if !reflect.DeepEqual(templatesConfigMap.Data, foundTemplatesConfigMap.Data) {
		reqLogger.Info("Updating TemplatesConfigMap")
		templatesConfigMap.Data = foundTemplatesConfigMap.Data
	}

	templatesConfigMapHash, err := util.ObjectHash(templatesConfigMap.Data)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("error calculating configuration hash: %v", err)
	}
	reqLogger.Info("TemplatesConfigMapHash: ", "Data Hash:", templatesConfigMapHash)

	// Define a new Daemonset object
	ds := newDaemonset(instance, instance.Name, templatesConfigMapHash)
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

func newDaemonset(cr *novav1.Libvirtd, cmName string, templatesConfigHash string) *appsv1.DaemonSet {
	var trueVar = true
	var falseVar = false

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

	containerSpec := corev1.Container{
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
		Lifecycle: &corev1.Lifecycle{
			PreStop: &corev1.Handler{
				Exec: &corev1.ExecAction{
					Command: []string{
						"bash", "-c", "kill $(cat /var/run/libvirtd.pid)",
					},
				},
			},
		},
		Command: []string{},
		SecurityContext: &corev1.SecurityContext{
			Privileged:             &trueVar,
			ReadOnlyRootFilesystem: &falseVar,
		},
		Env: []corev1.EnvVar{
			{
				Name:  "TEMPLATES_CONFIG_HASH",
				Value: templatesConfigHash,
			},
			{
				Name:  "KOLLA_CONFIG_STRATEGY",
				Value: "COPY_ALWAYS",
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
	for _, volMount := range libvirtd.GetVolumeMounts(cmName) {
		containerSpec.VolumeMounts = append(containerSpec.VolumeMounts, volMount)
	}
	daemonSet.Spec.Template.Spec.Containers = append(daemonSet.Spec.Template.Spec.Containers, containerSpec)

	// Volume config
	// add common Volumes
	for _, volConfig := range common.GetVolumes(cmName) {
		daemonSet.Spec.Template.Spec.Volumes = append(daemonSet.Spec.Template.Spec.Volumes, volConfig)
	}
	// add libvird Volumes
	for _, volConfig := range libvirtd.GetVolumes(cmName) {
		daemonSet.Spec.Template.Spec.Volumes = append(daemonSet.Spec.Template.Spec.Volumes, volConfig)
	}

	return &daemonSet
}
