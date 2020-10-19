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

	keystonev1beta1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	util "github.com/openstack-k8s-operators/lib-common/pkg/util"
	novav1beta1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	common "github.com/openstack-k8s-operators/nova-operator/pkg/common"
	nova "github.com/openstack-k8s-operators/nova-operator/pkg/nova"

	corev1 "k8s.io/api/core/v1"
)

// NovaReconciler reconciles a Nova object
type NovaReconciler struct {
	client.Client
	Kclient kubernetes.Interface
	Log     logr.Logger
	Scheme  *runtime.Scheme
}

// GetClient -
func (r *NovaReconciler) GetClient() client.Client {
	return r.Client
}

// GetLogger -
func (r *NovaReconciler) GetLogger() logr.Logger {
	return r.Log
}

// GetScheme -
func (r *NovaReconciler) GetScheme() *runtime.Scheme {
	return r.Scheme
}

// +kubebuilder:rbac:groups=nova.openstack.org,resources=nova,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nova.openstack.org,resources=nova/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=nova.openstack.org,resources=novaapis,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nova.openstack.org,resources=novaapis/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=nova.openstack.org,resources=novaschedulers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nova.openstack.org,resources=novaschedulers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=nova.openstack.org,resources=novaconductors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nova.openstack.org,resources=novaconductors/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=nova.openstack.org,resources=novacells,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nova.openstack.org,resources=novacells/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;create;update;delete;

// Reconcile - nova
func (r *NovaReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	_ = r.Log.WithValues("nova", req.NamespacedName)

	// Fetch the Nova instance
	instance := &novav1beta1.Nova{}
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
	cmLabels := common.GetLabels(instance.Name, nova.AppLabel)
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

	// Create nova_api and nova_cell0 DBs
	dbs := [2]string{"nova_api", "nova_cell0"}
	for _, dbName := range dbs {

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
	}

	// run dbsync job
	job := nova.DbSyncJob(instance, r.Scheme)
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

	// deploy nova-api
	// Create or update the nova-api Deployment object
	op, err := r.apiDeploymentCreateOrUpdate(instance)
	if err != nil {
		return ctrl.Result{}, err
	}
	if op != controllerutil.OperationResultNone {
		r.Log.Info(fmt.Sprintf("Deployment %s successfully reconciled - operation: %s", instance.Name, string(op)))
		return ctrl.Result{}, nil
	}

	// deploy nova-super-conductor
	// Create or update the nova-super-conductor Deployment object
	op, err = r.conductorDeploymentCreateOrUpdate(instance)
	if err != nil {
		return ctrl.Result{}, err
	}
	if op != controllerutil.OperationResultNone {
		r.Log.Info(fmt.Sprintf("Deployment %s successfully reconciled - operation: %s", instance.Name, string(op)))
		return ctrl.Result{}, nil
	}

	// deploy nova-scheduler
	// Create or update the nova-super-conductor Deployment object
	op, err = r.schedulerDeploymentCreateOrUpdate(instance)
	if err != nil {
		return ctrl.Result{}, err
	}
	if op != controllerutil.OperationResultNone {
		r.Log.Info(fmt.Sprintf("Deployment %s successfully reconciled - operation: %s", instance.Name, string(op)))
		return ctrl.Result{}, nil
	}

	// nova service
	selector := make(map[string]string)
	selector["app"] = fmt.Sprintf("%s-api", instance.Name)

	serviceInfo := common.ServiceDetails{
		Name:      instance.Name,
		Namespace: instance.Namespace,
		AppLabel:  "nova-api",
		Selector:  selector,
		Port:      8774,
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

	// Create the route if none exists
	routeInfo := common.RouteDetails{
		Name:      instance.Name,
		Namespace: instance.Namespace,
		AppLabel:  "nova-api",
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
	r.Log.Info("Setting up nova KeystoneService")
	// Keystone setup
	novaKeystoneService := &keystonev1beta1.KeystoneService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
		},
	}

	_, err = controllerutil.CreateOrUpdate(context.TODO(), r.Client, novaKeystoneService, func() error {
		novaKeystoneService.Spec.ServiceType = "compute"
		novaKeystoneService.Spec.ServiceName = "nova"
		novaKeystoneService.Spec.ServiceDescription = "nova"
		novaKeystoneService.Spec.Enabled = true
		novaKeystoneService.Spec.Region = "regionOne"
		novaKeystoneService.Spec.AdminURL = fmt.Sprintf("http://%s/v2.1", route.Spec.Host)
		novaKeystoneService.Spec.PublicURL = fmt.Sprintf("http://%s/v2.1", route.Spec.Host)
		novaKeystoneService.Spec.InternalURL = fmt.Sprintf("http://%s.openstack.svc:8774/v2.1", instance.Name)

		return nil
	})

	if err != nil {
		return ctrl.Result{}, err
	}

	r.setAPIEndpoint(instance, novaKeystoneService.Spec.PublicURL)

	// Create/Update cells
	for _, cell := range instance.Spec.Cells {
		// Create or update the nova-cell Deployment object
		op, err = r.cellDeploymentCreateOrUpdate(instance, &cell)
		if err != nil {
			return ctrl.Result{}, err
		}
		if op != controllerutil.OperationResultNone {
			r.Log.Info(fmt.Sprintf("Deployment %s successfully reconciled - operation: %s", instance.Name, string(op)))
			return ctrl.Result{}, nil
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager -
func (r *NovaReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&novav1beta1.Nova{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&novav1beta1.NovaScheduler{}).
		Owns(&novav1beta1.NovaAPI{}).
		Owns(&novav1beta1.NovaConductor{}).
		Owns(&novav1beta1.NovaCell{}).
		Owns(&routev1.Route{}).
		Owns(&corev1.Service{}).
		Complete(r)
}

func (r *NovaReconciler) setDbSyncHash(api *novav1beta1.Nova, hashStr string) error {

	if hashStr != api.Status.DbSyncHash {
		api.Status.DbSyncHash = hashStr
		if err := r.Client.Status().Update(context.TODO(), api); err != nil {
			return err
		}
	}
	return nil
}

func (r *NovaReconciler) setDbSyncStatus(api *novav1beta1.Nova, status string) error {

	if status != api.Status.DbSyncStatus {
		api.Status.DbSyncStatus = status
		if err := r.Client.Status().Update(context.TODO(), api); err != nil {
			return err
		}
	}
	return nil
}

func (r *NovaReconciler) setAPIEndpoint(instance *novav1beta1.Nova, endpoint string) error {

	if endpoint != instance.Status.APIEndpoint {
		instance.Status.APIEndpoint = endpoint
		if err := r.Client.Status().Update(context.TODO(), instance); err != nil {
			return err
		}
	}
	return nil

}

func (r *NovaReconciler) conductorDeploymentCreateOrUpdate(instance *novav1beta1.Nova) (controllerutil.OperationResult, error) {
	deployment := &novav1beta1.NovaConductor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-super-conductor", instance.Name),
			Namespace: instance.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(context.TODO(), r.Client, deployment, func() error {
		deployment.Spec = novav1beta1.NovaConductorSpec{
			ManagingCrName:     instance.Name,
			Cell:               "cell0",
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

func (r *NovaReconciler) apiDeploymentCreateOrUpdate(instance *novav1beta1.Nova) (controllerutil.OperationResult, error) {
	deployment := &novav1beta1.NovaAPI{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-api", instance.Name),
			Namespace: instance.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(context.TODO(), r.Client, deployment, func() error {
		deployment.Spec = novav1beta1.NovaAPISpec{
			ManagingCrName:     instance.Name,
			DatabaseHostname:   instance.Spec.DatabaseHostname,
			NovaSecret:         instance.Spec.NovaSecret,
			NeutronSecret:      instance.Spec.NeutronSecret,
			PlacementSecret:    instance.Spec.PlacementSecret,
			TransportURLSecret: instance.Spec.TransportURLSecret,
			Replicas:           instance.Spec.NovaAPIReplicas,
			ContainerImage:     instance.Spec.NovaAPIContainerImage,
		}

		err := controllerutil.SetControllerReference(instance, deployment, r.Scheme)
		if err != nil {
			return err
		}

		return nil
	})

	return op, err
}

func (r *NovaReconciler) schedulerDeploymentCreateOrUpdate(instance *novav1beta1.Nova) (controllerutil.OperationResult, error) {
	deployment := &novav1beta1.NovaScheduler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-scheduler", instance.Name),
			Namespace: instance.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(context.TODO(), r.Client, deployment, func() error {
		deployment.Spec = novav1beta1.NovaSchedulerSpec{
			ManagingCrName:     instance.Name,
			DatabaseHostname:   instance.Spec.DatabaseHostname,
			NovaSecret:         instance.Spec.NovaSecret,
			NeutronSecret:      instance.Spec.NeutronSecret,
			PlacementSecret:    instance.Spec.PlacementSecret,
			TransportURLSecret: instance.Spec.TransportURLSecret,
			Replicas:           instance.Spec.NovaSchedulerReplicas,
			ContainerImage:     instance.Spec.NovaSchedulerContainerImage,
		}

		err := controllerutil.SetControllerReference(instance, deployment, r.Scheme)
		if err != nil {
			return err
		}

		return nil
	})

	return op, err
}

func (r *NovaReconciler) cellDeploymentCreateOrUpdate(instance *novav1beta1.Nova, cell *novav1beta1.Cell) (controllerutil.OperationResult, error) {
	deployment := &novav1beta1.NovaCell{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", instance.Name, cell.Name),
			Namespace: instance.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(context.TODO(), r.Client, deployment, func() error {
		deployment.Spec = novav1beta1.NovaCellSpec{
			Cell:                         cell.Name,
			DatabaseHostname:             cell.DatabaseHostname,
			TransportURLSecret:           cell.TransportURLSecret,
			NovaConductorContainerImage:  cell.NovaConductorContainerImage,
			NovaMetadataContainerImage:   cell.NovaMetadataContainerImage,
			NovaNoVNCProxyContainerImage: cell.NovaNoVNCProxyContainerImage,
			NovaConductorReplicas:        cell.NovaConductorReplicas,
			NovaMetadataReplicas:         cell.NovaMetadataReplicas,
			NovaNoVNCProxyReplicas:       cell.NovaNoVNCProxyReplicas,
			NovaSecret:                   instance.Spec.NovaSecret,
			PlacementSecret:              instance.Spec.PlacementSecret,
			NeutronSecret:                instance.Spec.NeutronSecret,
		}

		err := controllerutil.SetControllerReference(instance, deployment, r.Scheme)
		if err != nil {
			return err
		}

		return nil
	})

	return op, err
}
