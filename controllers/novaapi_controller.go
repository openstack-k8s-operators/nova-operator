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
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	util "github.com/openstack-k8s-operators/lib-common/pkg/util"
	novav1beta1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	common "github.com/openstack-k8s-operators/nova-operator/pkg/common"
	novaapi "github.com/openstack-k8s-operators/nova-operator/pkg/novaapi"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
)

// NovaAPIReconciler reconciles a NovaAPI object
type NovaAPIReconciler struct {
	client.Client
	Kclient kubernetes.Interface
	Log     logr.Logger
	Scheme  *runtime.Scheme
}

// +kubebuilder:rbac:groups=nova.openstack.org,resources=novaapis,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nova.openstack.org,resources=novaapis/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;create;update;delete;
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;create;update;delete;
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;create;update;delete;
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;create;update;delete;

// Reconcile - nova api
func (r *NovaAPIReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	_ = r.Log.WithValues("novaapi", req.NamespacedName)

	// Fetch the NovaAPI instance
	instance := &novav1beta1.NovaAPI{}
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

	// Check if ScriptsConfigMap is there and get hash
	configMapName := fmt.Sprintf("%s-scripts", instance.Spec.ManagingCrName)
	_, scriptsConfigMapHash, err := common.GetConfigMapAndHashWithName(r.Client, r.Log, configMapName, instance.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}
	r.Log.Info("ScriptsConfigMapHash: ", "Data Hash:", scriptsConfigMapHash)

	// Check if ConfigMap is there and get hash
	configMapName = fmt.Sprintf("%s-config-data", instance.Spec.ManagingCrName)
	_, configMapHash, err := common.GetConfigMapAndHashWithName(r.Client, r.Log, configMapName, instance.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}
	r.Log.Info("ConfigMapHash: ", "Data Hash:", configMapHash)

	// Check if CustomConfigMap is there and get hash
	configMapName = fmt.Sprintf("%s-config-data-custom", instance.Spec.ManagingCrName)
	_, customConfigMapHash, err := common.GetConfigMapAndHashWithName(r.Client, r.Log, configMapName, instance.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}
	r.Log.Info("CustomConfigMapHash: ", "Data Hash:", customConfigMapHash)

	deployment := novaapi.Deployment(instance, scriptsConfigMapHash, configMapHash, customConfigMapHash, r.Scheme)
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

		if instance.Status.NovaAPIHash != deploymentHash {
			r.Log.Info("Deployment Updated")
			foundDeployment.Spec = deployment.Spec
			err = r.Client.Update(context.TODO(), foundDeployment)
			if err != nil {
				return ctrl.Result{}, err
			}
			if err := r.setNovaAPIHash(instance, deploymentHash); err != nil {
				return ctrl.Result{}, err
			}

			return ctrl.Result{RequeueAfter: time.Second * 10}, err
		}
		if foundDeployment.Status.Replicas == instance.Spec.Replicas {
			r.Log.Info("Deployment Replicas running:", "Replicas", foundDeployment.Status.Replicas)
		} else {
			r.Log.Info("Waiting on NovaAPI Deployment...")
			return ctrl.Result{RequeueAfter: time.Second * 5}, err
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager -
func (r *NovaAPIReconciler) SetupWithManager(mgr ctrl.Manager) error {

	// watch for configmap where the CM upper-cr label AND the CR.Spec.ManagingCrName label matches
	configMapFn := handler.ToRequestsFunc(func(o handler.MapObject) []reconcile.Request {
		result := []reconcile.Request{}

		// get ConfigMap object
		cm := &corev1.ConfigMap{}
		if err := r.Client.Get(context.Background(), types.NamespacedName{Name: o.Meta.GetName(), Namespace: o.Meta.GetNamespace()}, cm); err != nil {
			r.Log.Error(err, "Unable to retrieve ConfigMap %v")
			return nil
		}

		// get all conductor CRs
		apis := &novav1beta1.NovaAPIList{}
		listOpts := []client.ListOption{
			client.InNamespace(o.Meta.GetNamespace()),
		}
		if err := r.Client.List(context.Background(), apis, listOpts...); err != nil {
			r.Log.Error(err, "Unable to retrieve Conductor CRs %v")
			return nil
		}

		label := cm.ObjectMeta.GetLabels()
		// verify object has upper-cr label
		if l, ok := label["upper-cr"]; ok {
			for _, cr := range apis.Items {
				// return reconcil event for the CR where the CM upper-cr label AND the CR.Spec.ManagingCrName label matches
				if l == cr.Spec.ManagingCrName {
					// return namespace and Name of CR
					name := client.ObjectKey{
						Namespace: cm.Namespace,
						Name:      cr.Name,
					}
					r.Log.Info(fmt.Sprintf("ConfigMap object %s and CR %s marked with label: %s", o.Meta.GetName(), cr.Name, l))
					result = append(result, reconcile.Request{NamespacedName: name})
				}
			}
		}
		if len(result) > 0 {
			return result
		}
		return nil
	})

	return ctrl.NewControllerManagedBy(mgr).
		For(&novav1beta1.NovaAPI{}).
		Owns(&appsv1.Deployment{}).
		// watch the config CMs we don't own
		Watches(&source.Kind{Type: &corev1.ConfigMap{}},
			&handler.EnqueueRequestsFromMapFunc{
				ToRequests: configMapFn,
			}).
		Complete(r)
}

func (r *NovaAPIReconciler) setNovaAPIHash(instance *novav1beta1.NovaAPI, hashStr string) error {

	if hashStr != instance.Status.NovaAPIHash {
		instance.Status.NovaAPIHash = hashStr
		if err := r.Client.Status().Update(context.TODO(), instance); err != nil {
			return err
		}
	}
	return nil

}
