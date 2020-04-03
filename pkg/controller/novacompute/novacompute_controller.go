package novacompute

import (
	"context"
	"fmt"
	"reflect"
	"time"

	novav1 "github.com/openstack-k8s-operators/nova-operator/pkg/apis/nova/v1"
	common "github.com/openstack-k8s-operators/nova-operator/pkg/common"
	novacompute "github.com/openstack-k8s-operators/nova-operator/pkg/novacompute"
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
	COMMON_CONFIGMAP string = "common-config"
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

	reqLogger.Info("Creating host entries from config map:", "configMap: ", COMMON_CONFIGMAP)
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: COMMON_CONFIGMAP, Namespace: instance.Namespace}, commonConfigMap)
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

	// ScriptsConfigMap
	scriptsConfigMap := novacompute.ScriptsConfigMap(instance, instance.Name+"-scripts")
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
	} else {
		reqLogger.Info("ScriptsConfigMapHash: ", "Data Hash:", scriptsConfigMapHash)
	}

	// TemplatesConfigMap
	templatesConfigMap := novacompute.TemplatesConfigMap(instance, instance.Name+"-templates")
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
	} else {
		reqLogger.Info("TemplatesConfigMapHash: ", "Data Hash:", templatesConfigMapHash)
	}

	// Define a new Daemonset object
	ds := newDaemonset(instance, instance.Name, templatesConfigMapHash, scriptsConfigMapHash)
	dsHash, err := util.ObjectHash(ds)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("error calculating configuration hash: %v", err)
	} else {
		reqLogger.Info("DaemonsetHash: ", "Daemonset Hash:", dsHash)
	}

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

func newDaemonset(cr *novav1.NovaCompute, cmName string, templatesConfigHash string, scriptsConfigHash string) *appsv1.DaemonSet {
	var trueVar bool = true

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
