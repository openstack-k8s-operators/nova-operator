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
	"strings"
	"time"

	"github.com/go-logr/logr"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	routev1 "github.com/openshift/api/route/v1"
	util "github.com/openstack-k8s-operators/lib-common/pkg/util"
	placementv1beta1 "github.com/openstack-k8s-operators/placement-operator/api/v1beta1"
	placement "github.com/openstack-k8s-operators/placement-operator/pkg"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
)

// AppLabel -
const AppLabel = "placement-api"

// PlacementAPIReconciler reconciles a PlacementAPI object
type PlacementAPIReconciler struct {
	client.Client
	Kclient kubernetes.Interface
	Log     logr.Logger
	Scheme  *runtime.Scheme
}

// +kubebuilder:rbac:groups=placement.openstack.org,resources=placementapis,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=placement.openstack.org,resources=placementapis/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;create;update;delete;
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;create;update;delete;
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;create;update;delete;
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;create;update;delete;

// Reconcile - placement api
func (r *PlacementAPIReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	_ = r.Log.WithValues("placementapi", req.NamespacedName)

	// Fetch the Placement instance
	instance := &placementv1beta1.PlacementAPI{}
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

	service := placement.Service(instance, r.Scheme)

	// Check if this Service already exists
	foundService := &corev1.Service{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, foundService)
	if err != nil && k8s_errors.IsNotFound(err) {

		r.Log.Info("Creating a new Service", "Service.Namespace", service.Namespace, "Service.Name", service.Name)
		err = r.Client.Create(context.TODO(), service)
		if err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{RequeueAfter: time.Second * 5}, err
	} else if err != nil {
		return ctrl.Result{}, err
	}

	// ScriptsConfigMap
	scriptsConfigMap := placement.ScriptsConfigMap(instance, r.Scheme)
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
		foundScriptsConfigMap.Data = scriptsConfigMap.Data
		err = r.Client.Update(context.TODO(), foundScriptsConfigMap)
		if err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: time.Second * 5}, err
	}

	// ConfigMap
	configMap := placement.ConfigMap(instance, r.Scheme)
	// Check if this ConfigMap already exists
	foundConfigMap := &corev1.ConfigMap{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: configMap.Name, Namespace: configMap.Namespace}, foundConfigMap)
	if err != nil && k8s_errors.IsNotFound(err) {
		r.Log.Info("Creating a new ConfigMap", "ConfigMap.Namespace", configMap.Namespace, "Job.Name", configMap.Name)
		err = r.Client.Create(context.TODO(), configMap)
		if err != nil {
			return ctrl.Result{}, err
		}
	} else if !reflect.DeepEqual(configMap.Data, foundConfigMap.Data) {
		r.Log.Info("Updating ConfigMap")
		foundConfigMap.Data = configMap.Data
		err = r.Client.Update(context.TODO(), foundConfigMap)
		if err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: time.Second * 5}, err
	}

	// CustomConfigMap
	customConfigMap := placement.CustomConfigMap(instance, r.Scheme)
	// Check if this CustomConfigMap already exists
	foundCustomConfigMap := &corev1.ConfigMap{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: customConfigMap.Name, Namespace: customConfigMap.Namespace}, foundCustomConfigMap)
	if err != nil && k8s_errors.IsNotFound(err) {
		r.Log.Info("Creating a new CustomConfigMap", "CustomConfigMap.Namespace", customConfigMap.Namespace, "Job.Name", customConfigMap.Name)
		err = r.Client.Create(context.TODO(), customConfigMap)
		if err != nil {
			return ctrl.Result{}, err
		}
	} else {
		// use data from already existing custom configmap
		customConfigMap.Data = foundCustomConfigMap.Data
	}

	// Create the DB Schema (unstructured so we don't explicitly import mariadb-operator code)
	databaseObj, err := placement.DatabaseObject(instance)
	if err != nil {
		return ctrl.Result{}, err
	}

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
			r.Log.Info("Waiting on DB to be created...")
			return ctrl.Result{RequeueAfter: time.Second * 5}, err
		}
	}

	// Define a new Job object
	job := placement.DbSyncJob(instance, r.Scheme)
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
	// delete the job
	requeue, err = util.DeleteJob(job, r.Kclient, r.Log)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Define a new Deployment object
	scriptsConfigMapHash, err := util.ObjectHash(scriptsConfigMap)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error calculating configuration hash: %v", err)
	}
	r.Log.Info("ScriptsConfigMapHash: ", "Data Hash:", scriptsConfigMapHash)

	configMapHash, err := util.ObjectHash(configMap)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error calculating config map hash: %v", err)
	}
	r.Log.Info("ConfigMapHash: ", "Data Hash:", configMapHash)

	customConfigMapHash, err := util.ObjectHash(customConfigMap)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error calculating custom config map hash: %v", err)
	}
	r.Log.Info("CustomConfigMapHash: ", "Data Hash:", customConfigMapHash)

	deployment := placement.Deployment(instance, scriptsConfigMapHash, configMapHash, customConfigMapHash, r.Scheme)
	deploymentHash, err := util.ObjectHash(deployment)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error deployment hash: %v", err)
	}
	r.Log.Info("DeploymentHash: ", "Deployment Hash:", deploymentHash)

	// Check if this Deployment already exists
	foundDeployment := &appsv1.Deployment{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, foundDeployment)
	if err != nil && k8s_errors.IsNotFound(err) {
		r.Log.Info("Creating a new Deployment", "Deployment.Namespace", deployment.Namespace, "Deployment.Name", deployment.Name)
		err = r.Client.Create(context.TODO(), deployment)
		if err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{RequeueAfter: time.Second * 10}, err

	} else if err != nil {
		return ctrl.Result{}, err
	} else {

		if instance.Status.DeploymentHash != deploymentHash {
			r.Log.Info("Deployment Updated")
			foundDeployment.Spec = deployment.Spec
			err = r.Client.Update(context.TODO(), foundDeployment)
			if err != nil {
				return ctrl.Result{}, err
			}
			if err := r.setDeploymentHash(instance, deploymentHash); err != nil {
				return ctrl.Result{}, err
			}

			return ctrl.Result{RequeueAfter: time.Second * 10}, err
		}
		if foundDeployment.Status.ReadyReplicas == instance.Spec.Replicas {
			r.Log.Info("Deployment Replicas running:", "Replicas", foundDeployment.Status.ReadyReplicas)
		} else {
			r.Log.Info("Waiting on Placement Deployment...")
			return ctrl.Result{RequeueAfter: time.Second * 5}, err
		}
	}

	// Create the route if none exists
	route := placement.Route(instance, r.Scheme)

	// Check if this Route already exists
	foundRoute := &routev1.Route{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: route.Name, Namespace: route.Namespace}, foundRoute)
	if err != nil && k8s_errors.IsNotFound(err) {
		r.Log.Info("Creating a new Route", "Route.Namespace", route.Namespace, "Route.Name", route.Name)
		err = r.Client.Create(context.TODO(), route)
		if err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{RequeueAfter: time.Second * 5}, err
	} else if err != nil {
		return ctrl.Result{}, err
	}

	var apiEndpoint string
	if !strings.HasPrefix(foundRoute.Spec.Host, "http") {
		apiEndpoint = fmt.Sprintf("http://%s", foundRoute.Spec.Host)
	} else {
		apiEndpoint = foundRoute.Spec.Host
	}
	r.setAPIEndpoint(instance, apiEndpoint)

	return ctrl.Result{}, nil
}

// SetupWithManager -
func (r *PlacementAPIReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&placementv1beta1.PlacementAPI{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&appsv1.Deployment{}).
		Owns(&routev1.Route{}).
		Complete(r)
}

func (r *PlacementAPIReconciler) setDbSyncHash(api *placementv1beta1.PlacementAPI, hashStr string) error {

	if hashStr != api.Status.DbSyncHash {
		api.Status.DbSyncHash = hashStr
		if err := r.Client.Status().Update(context.TODO(), api); err != nil {
			return err
		}
	}
	return nil
}

func (r *PlacementAPIReconciler) setDeploymentHash(instance *placementv1beta1.PlacementAPI, hashStr string) error {

	if hashStr != instance.Status.DeploymentHash {
		instance.Status.DeploymentHash = hashStr
		if err := r.Client.Status().Update(context.TODO(), instance); err != nil {
			return err
		}
	}
	return nil

}

func (r *PlacementAPIReconciler) setAPIEndpoint(instance *placementv1beta1.PlacementAPI, endpoint string) error {

	if endpoint != instance.Status.APIEndpoint {
		instance.Status.APIEndpoint = endpoint
		if err := r.Client.Status().Update(context.TODO(), instance); err != nil {
			return err
		}
	}
	return nil

}
