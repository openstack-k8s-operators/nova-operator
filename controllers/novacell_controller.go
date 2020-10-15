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
	"strings"
	"time"

	"github.com/go-logr/logr"
	routev1 "github.com/openshift/api/route/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	util "github.com/openstack-k8s-operators/lib-common/pkg/util"
	novav1beta1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	common "github.com/openstack-k8s-operators/nova-operator/pkg/common"
	novacell "github.com/openstack-k8s-operators/nova-operator/pkg/novacell"

	corev1 "k8s.io/api/core/v1"
)

// NovaCellReconciler reconciles a NovaCell object
type NovaCellReconciler struct {
	client.Client
	Kclient kubernetes.Interface
	Log     logr.Logger
	Scheme  *runtime.Scheme
}

// GetClient -
func (r *NovaCellReconciler) GetClient() client.Client {
	return r.Client
}

// GetLogger -
func (r *NovaCellReconciler) GetLogger() logr.Logger {
	return r.Log
}

// GetScheme -
func (r *NovaCellReconciler) GetScheme() *runtime.Scheme {
	return r.Scheme
}

// +kubebuilder:rbac:groups=nova.openstack.org,resources=novacells,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nova.openstack.org,resources=novacells/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=nova.openstack.org,resources=novaconductors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nova.openstack.org,resources=novaconductors/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;create;update;delete;

// Reconcile - nova cell
func (r *NovaCellReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	_ = r.Log.WithValues("novacell", req.NamespacedName)

	// Fetch the NovaCell instance
	instance := &novav1beta1.NovaCell{}
	err := r.Client.Get(context.TODO(), req.NamespacedName, instance)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected.
			// For additional cleanup logic use finalizers. Return and don't requeue.
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	// TODO:
	// add finalizer to
	// a) delete cell in OSP api when delete
	// b) wait for all cell computes to be deleted before. Since we don't
	//    own the compute-node CRs just wait

	envVars := make(map[string]util.EnvSetter)

	// check for required secrets
	_, hash, err := common.GetSecret(r.Client, instance.Spec.NovaSecret, instance.Namespace)
	if err != nil {
		return ctrl.Result{RequeueAfter: time.Second * 10}, err
	}
	envVars[instance.Spec.NovaSecret] = util.EnvValue(hash)

	_, hash, err = common.GetSecret(r.Client, instance.Spec.PlacementSecret, instance.Namespace)
	if err != nil {
		return ctrl.Result{RequeueAfter: time.Second * 10}, err
	}
	envVars[instance.Spec.PlacementSecret] = util.EnvValue(hash)

	_, hash, err = common.GetSecret(r.Client, instance.Spec.NeutronSecret, instance.Namespace)
	if err != nil {
		return ctrl.Result{RequeueAfter: time.Second * 10}, err
	}
	envVars[instance.Spec.NeutronSecret] = util.EnvValue(hash)

	// Create/update configmaps from templates
	cmLabels := common.GetLabels(instance.Name, novacell.AppLabel)
	cmLabels["upper-cr"] = instance.Name

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

	// Create the cell DB
	dbName := fmt.Sprintf("nova_%s", instance.Spec.Cell)
	db := common.Database{
		DatabaseName:     dbName,
		DatabaseHostname: instance.Spec.DatabaseHostname,
		Secret:           instance.Spec.NovaSecret,
	}
	databaseObj, err := common.DatabaseObject(r, instance, db)
	if err != nil {
		return ctrl.Result{}, err
	}

	// set owner reference on databaseObj
	oref := metav1.NewControllerRef(instance, instance.GroupVersionKind())
	databaseObj.SetOwnerReferences([]metav1.OwnerReference{*oref})

	foundDatabase := &unstructured.Unstructured{}
	foundDatabase.SetGroupVersionKind(databaseObj.GroupVersionKind())
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: databaseObj.GetName(), Namespace: databaseObj.GetNamespace()}, foundDatabase)
	if err != nil && k8s_errors.IsNotFound(err) {
		err := r.Client.Create(context.TODO(), &databaseObj)
		if err != nil {
			return ctrl.Result{}, err
		}
	} else if err != nil {
		return ctrl.Result{}, err
	} else {
		completed, _, err := unstructured.NestedBool(foundDatabase.UnstructuredContent(), "status", "completed")
		if !completed {
			r.Log.Info(fmt.Sprintf("Waiting on %s DB to be created...", dbName))
			return ctrl.Result{RequeueAfter: time.Second * 5}, err
		}
	}

	// run dbsync job
	job := novacell.DbSyncJob(instance, r.Scheme)
	dbSyncHash, err := util.ObjectHash(job)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error calculating DB sync hash: %v", err)
	}

	requeue := true
	if instance.Status.DbSyncHash != dbSyncHash {
		requeue, err = util.EnsureJob(job, r.Client, r.Log)
		r.Log.Info("Running DB sync")
		if err != nil {
			return ctrl.Result{}, err
		} else if requeue {
			r.Log.Info("Waiting on DB sync")
			return ctrl.Result{RequeueAfter: time.Second * 5}, err
		}
	}
	// db sync completed... okay to store the hash to disable it
	if err := r.setDbSyncHash(instance, dbSyncHash); err != nil {
		return ctrl.Result{}, err
	}

	// delete the dbsync job
	requeue, err = util.DeleteJob(job, r.Kclient, r.Log)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Create or update the nova-conductor Deployment object
	op, err := r.conductorDeploymentCreateOrUpdate(instance)
	if err != nil {
		return ctrl.Result{}, err
	}
	if op != controllerutil.OperationResultNone {
		r.Log.Info(fmt.Sprintf("Deployment %s successfully reconciled - operation: %s", instance.Name, string(op)))
		return ctrl.Result{}, nil
	}

	// Create or update the nova-matadata Deployment object
	op, err = r.metadataDeploymentCreateOrUpdate(instance)
	if err != nil {
		return ctrl.Result{}, err
	}
	if op != controllerutil.OperationResultNone {
		r.Log.Info(fmt.Sprintf("Deployment %s successfully reconciled - operation: %s", instance.Name, string(op)))
		return ctrl.Result{}, nil
	}

	// deploy cell nova-novncproxy
	// Create or update the nova-novncproxy Deployment object
	op, err = r.novncproxyDeploymentCreateOrUpdate(instance)
	if err != nil {
		return ctrl.Result{}, err
	}
	if op != controllerutil.OperationResultNone {
		r.Log.Info(fmt.Sprintf("Deployment %s successfully reconciled - operation: %s", instance.Name, string(op)))
		return ctrl.Result{}, nil
	}
	// nova noVNC service
	selector := make(map[string]string)
	selector["cr"] = fmt.Sprintf("%s-novncproxy", instance.Name)

	serviceInfo := common.ServiceDetails{
		Name:      fmt.Sprintf("%s-novncproxy", instance.Name),
		Namespace: instance.Namespace,
		AppLabel:  "nova-novncproxy",
		Selector:  selector,
		Port:      6080,
	}

	service := &corev1.Service{}
	service.Name = serviceInfo.Name
	service.Namespace = serviceInfo.Namespace
	if err := controllerutil.SetControllerReference(instance, service, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	service, op, err = common.CreateOrUpdateService(r.Client, r.Log, service, &serviceInfo)
	if err != nil {
		return ctrl.Result{}, err
	}
	r.Log.Info("Service successfully reconciled", "operation", op)

	// Create the noVNC route if none exists
	routeInfo := common.RouteDetails{
		Name:      instance.Name,
		Namespace: instance.Namespace,
		AppLabel:  "nova-novncproxy",
		Port:      "api",
	}
	route := common.Route(routeInfo)
	if err := controllerutil.SetControllerReference(instance, route, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	err = common.CreateOrUpdateRoute(r.Client, r.Log, route)
	if err != nil {
		return ctrl.Result{}, err
	}

	// update status with endpoint information
	var noVncEndpoint string
	if !strings.HasPrefix(route.Spec.Host, "http") {
		noVncEndpoint = fmt.Sprintf("http://%s", route.Spec.Host)
	} else {
		noVncEndpoint = route.Spec.Host
	}
	r.setNoVNCProxyEndpoint(instance, noVncEndpoint)

	// create cell
	job = novacell.CreateCellJob(instance, r.Scheme)
	createCellHash, err := util.ObjectHash(job)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error calculating sync hash: %v", err)
	}

	requeue = true
	if instance.Status.CreateCellHash != createCellHash {
		requeue, err = util.EnsureJob(job, r.Client, r.Log)
		r.Log.Info("Running create Cell")
		if err != nil {
			return ctrl.Result{}, err
		} else if requeue {
			r.Log.Info("Waiting on create Cell")
			return ctrl.Result{RequeueAfter: time.Second * 5}, err
		}

		// restart nova-api and nova-scheduler to make aware of new cell only when a new cell got created
		labels := [2]string{"nova-api", "nova-scheduler"}
		for _, label := range labels {
			labelSelector := map[string]string{
				"app": label,
			}
			err = common.DeleteAllNamespacedPodsWithLabel(r.Kclient, r.Log, labelSelector, instance.Namespace)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
	}
	// create cell completed... okay to store the hash to disable it
	if err := r.setCreateCellHash(instance, createCellHash); err != nil {
		return ctrl.Result{}, err
	}

	// delete the creat Cell job
	requeue, err = util.DeleteJob(job, r.Kclient, r.Log)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager -
func (r *NovaCellReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&novav1beta1.NovaCell{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&novav1beta1.NovaConductor{}).
		Owns(&novav1beta1.NovaNoVNCProxy{}).
		Owns(&novav1beta1.NovaMetadata{}).
		Owns(&routev1.Route{}).
		Owns(&corev1.Service{}).
		Complete(r)
}

func (r *NovaCellReconciler) setDbSyncHash(api *novav1beta1.NovaCell, hashStr string) error {

	if hashStr != api.Status.DbSyncHash {
		api.Status.DbSyncHash = hashStr
		if err := r.Client.Status().Update(context.TODO(), api); err != nil {
			return err
		}
	}
	return nil
}

func (r *NovaCellReconciler) setCreateCellHash(api *novav1beta1.NovaCell, hashStr string) error {

	if hashStr != api.Status.CreateCellHash {
		api.Status.CreateCellHash = hashStr
		if err := r.Client.Status().Update(context.TODO(), api); err != nil {
			return err
		}
	}
	return nil
}

func (r *NovaCellReconciler) setNoVNCProxyEndpoint(instance *novav1beta1.NovaCell, endpoint string) error {

	if endpoint != instance.Status.NoVNCProxyEndpoint {
		instance.Status.NoVNCProxyEndpoint = endpoint
		if err := r.Client.Status().Update(context.TODO(), instance); err != nil {
			return err
		}
	}
	return nil

}

func (r *NovaCellReconciler) conductorDeploymentCreateOrUpdate(instance *novav1beta1.NovaCell) (controllerutil.OperationResult, error) {
	deployment := &novav1beta1.NovaConductor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-conductor", instance.Name),
			Namespace: instance.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(context.TODO(), r.Client, deployment, func() error {
		deployment.Spec = novav1beta1.NovaConductorSpec{
			ManagingCrName:     instance.Name,
			Cell:               instance.Spec.Cell,
			DatabaseHostname:   instance.Spec.DatabaseHostname,
			NovaSecret:         instance.Spec.NovaSecret,
			NeutronSecret:      instance.Spec.NeutronSecret,
			PlacementSecret:    instance.Spec.PlacementSecret,
			TransportURLSecret: instance.Spec.TransportURLSecret,
			Replicas:           instance.Spec.NovaConductorReplicas,
			ContainerImage:     instance.Spec.NovaConductorContainerImage,
		}

		err := controllerutil.SetControllerReference(instance, deployment, r.Scheme)
		if err != nil {
			return err
		}

		return nil
	})

	return op, err
}

func (r *NovaCellReconciler) metadataDeploymentCreateOrUpdate(instance *novav1beta1.NovaCell) (controllerutil.OperationResult, error) {
	deployment := &novav1beta1.NovaMetadata{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-metadata", instance.Name),
			Namespace: instance.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(context.TODO(), r.Client, deployment, func() error {
		deployment.Spec = novav1beta1.NovaMetadataSpec{
			ManagingCrName:     instance.Name,
			Cell:               instance.Spec.Cell,
			DatabaseHostname:   instance.Spec.DatabaseHostname,
			NovaSecret:         instance.Spec.NovaSecret,
			NeutronSecret:      instance.Spec.NeutronSecret,
			PlacementSecret:    instance.Spec.PlacementSecret,
			TransportURLSecret: instance.Spec.TransportURLSecret,
			Replicas:           instance.Spec.NovaMetadataReplicas,
			ContainerImage:     instance.Spec.NovaMetadataContainerImage,
		}

		err := controllerutil.SetControllerReference(instance, deployment, r.Scheme)
		if err != nil {
			return err
		}

		return nil
	})

	return op, err
}

func (r *NovaCellReconciler) novncproxyDeploymentCreateOrUpdate(instance *novav1beta1.NovaCell) (controllerutil.OperationResult, error) {
	deployment := &novav1beta1.NovaNoVNCProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-novncproxy", instance.Name),
			Namespace: instance.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(context.TODO(), r.Client, deployment, func() error {
		deployment.Spec = novav1beta1.NovaNoVNCProxySpec{
			ManagingCrName:     instance.Name,
			Cell:               instance.Spec.Cell,
			DatabaseHostname:   instance.Spec.DatabaseHostname,
			NovaSecret:         instance.Spec.NovaSecret,
			NeutronSecret:      instance.Spec.NeutronSecret,
			PlacementSecret:    instance.Spec.PlacementSecret,
			TransportURLSecret: instance.Spec.TransportURLSecret,
			Replicas:           instance.Spec.NovaNoVNCProxyReplicas,
			ContainerImage:     instance.Spec.NovaNoVNCProxyContainerImage,
		}

		err := controllerutil.SetControllerReference(instance, deployment, r.Scheme)
		if err != nil {
			return err
		}

		return nil
	})

	return op, err
}
