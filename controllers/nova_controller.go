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

	// ScriptsConfigMap
	scriptsConfigMap := nova.ScriptsConfigMap(instance, r.Scheme, r.Log)
	err = common.CreateOrUpdateConfigMap(r.Client, r.Log, scriptsConfigMap)
	if err != nil {
		return ctrl.Result{}, err
	}

	// ConfigMap
	configMap := nova.ConfigMap(instance, r.Scheme)
	err = common.CreateOrUpdateConfigMap(r.Client, r.Log, configMap)
	if err != nil {
		return ctrl.Result{}, err
	}

	// CustomConfigMap
	customConfigMap := nova.CustomConfigMap(instance, r.Scheme)
	err = common.CreateOrGetCustomConfigMap(r.Client, r.Log, customConfigMap)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Create nova_api and nova_cell0 DBs
	dbs := [2]string{"nova_api", "nova_cell0"}
	for _, dbName := range dbs {
		databaseObj, err := nova.DatabaseObject(instance, dbName)
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
	novaAPI := &novav1beta1.NovaAPI{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-api", instance.Name),
			Namespace: instance.Namespace,
		},
		Spec: novav1beta1.NovaAPISpec{
			ManagingCrName:     instance.Name,
			DatabaseHostname:   instance.Spec.DatabaseHostname,
			NovaSecret:         instance.Spec.NovaSecret,
			NeutronSecret:      instance.Spec.NeutronSecret,
			PlacementSecret:    instance.Spec.PlacementSecret,
			TransportURLSecret: instance.Spec.TransportURLSecret,
			Replicas:           instance.Spec.NovaAPIReplicas,
			ContainerImage:     instance.Spec.NovaAPIContainerImage,
		},
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(instance, novaAPI, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	// calc hash
	novaAPIHash, err := util.ObjectHash(novaAPI)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error statefulSet hash: %v", err)
	}
	r.Log.Info("NovaAPIHash: ", "NovaAPI Hash:", novaAPIHash)

	foundNovaAPI := &novav1beta1.NovaAPI{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: novaAPI.Name, Namespace: novaAPI.Namespace}, foundNovaAPI)
	if err != nil && k8s_errors.IsNotFound(err) {
		r.Log.Info("Creating a new NovaAPI", "NovaAPI.Namespace", novaAPI.Namespace, "NovaAPI.Name", novaAPI.Name)
		err = r.Client.Create(context.TODO(), novaAPI)
		if err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{RequeueAfter: time.Second * 10}, err

	} else if err != nil {
		return ctrl.Result{}, err
	} else {
		if instance.Status.NovaAPIHash != novaAPIHash {
			r.Log.Info("NovaAPI Updated")
			foundNovaAPI.Spec = novaAPI.Spec
			err = r.Client.Update(context.TODO(), foundNovaAPI)
			if err != nil {
				return ctrl.Result{}, err
			}
			if err := r.setNovaAPIHash(instance, novaAPIHash); err != nil {
				return ctrl.Result{}, err
			}

			return ctrl.Result{RequeueAfter: time.Second * 10}, err
		}
	}

	// deploy nova-super-conductor
	novaConductor := &novav1beta1.NovaConductor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-super-conductor", instance.Name),
			Namespace: instance.Namespace,
		},
		Spec: novav1beta1.NovaConductorSpec{
			ManagingCrName:     instance.Name,
			Cell:               "cell0",
			DatabaseHostname:   instance.Spec.DatabaseHostname,
			NovaSecret:         instance.Spec.NovaSecret,
			NeutronSecret:      instance.Spec.NeutronSecret,
			PlacementSecret:    instance.Spec.PlacementSecret,
			TransportURLSecret: instance.Spec.TransportURLSecret,
			Replicas:           instance.Spec.NovaConductorReplicas,
			ContainerImage:     instance.Spec.NovaConductorContainerImage,
		},
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(instance, novaConductor, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	// calc hash
	novaConductorHash, err := util.ObjectHash(novaConductor)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error statefulSet hash: %v", err)
	}
	r.Log.Info("NovaConductorHash: ", "NovaConductor Hash:", novaConductorHash)

	foundNovaConductor := &novav1beta1.NovaConductor{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: novaConductor.Name, Namespace: novaConductor.Namespace}, foundNovaConductor)
	if err != nil && k8s_errors.IsNotFound(err) {
		r.Log.Info("Creating a new NovaConductor", "NovaConductor.Namespace", novaConductor.Namespace, "NovaConductor.Name", novaConductor.Name)
		err = r.Client.Create(context.TODO(), novaConductor)
		if err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{RequeueAfter: time.Second * 10}, err

	} else if err != nil {
		return ctrl.Result{}, err
	} else {
		if instance.Status.NovaConductorHash != novaConductorHash {
			r.Log.Info("NovaConductor Updated")
			foundNovaConductor.Spec = novaConductor.Spec
			err = r.Client.Update(context.TODO(), foundNovaConductor)
			if err != nil {
				return ctrl.Result{}, err
			}
			if err := r.setNovaConductorHash(instance, novaConductorHash); err != nil {
				return ctrl.Result{}, err
			}

			return ctrl.Result{RequeueAfter: time.Second * 10}, err
		}
	}

	// deploy nova-scheduler
	novaScheduler := &novav1beta1.NovaScheduler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-scheduler", instance.Name),
			Namespace: instance.Namespace,
		},
		Spec: novav1beta1.NovaSchedulerSpec{
			ManagingCrName:     instance.Name,
			DatabaseHostname:   instance.Spec.DatabaseHostname,
			NovaSecret:         instance.Spec.NovaSecret,
			NeutronSecret:      instance.Spec.NeutronSecret,
			PlacementSecret:    instance.Spec.PlacementSecret,
			TransportURLSecret: instance.Spec.TransportURLSecret,
			Replicas:           instance.Spec.NovaSchedulerReplicas,
			ContainerImage:     instance.Spec.NovaSchedulerContainerImage,
		},
	}
	// Set owner reference
	if err := controllerutil.SetControllerReference(instance, novaScheduler, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	// calc hash
	novaSchedulerHash, err := util.ObjectHash(novaScheduler)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error statefulSet hash: %v", err)
	}
	r.Log.Info("NovaSchedulerHash: ", "NovaScheduler Hash:", novaSchedulerHash)

	foundNovaScheduler := &novav1beta1.NovaScheduler{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: novaScheduler.Name, Namespace: novaScheduler.Namespace}, foundNovaScheduler)
	if err != nil && k8s_errors.IsNotFound(err) {
		r.Log.Info("Creating a new NovaScheduler", "NovaScheduler.Namespace", novaScheduler.Namespace, "NovaScheduler.Name", novaScheduler.Name)
		err = r.Client.Create(context.TODO(), novaScheduler)
		if err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{RequeueAfter: time.Second * 10}, err

	} else if err != nil {
		return ctrl.Result{}, err
	} else {
		if instance.Status.NovaSchedulerHash != novaSchedulerHash {
			r.Log.Info("NovaScheduler Updated")
			foundNovaScheduler.Spec = novaScheduler.Spec
			err = r.Client.Update(context.TODO(), foundNovaScheduler)
			if err != nil {
				return ctrl.Result{}, err
			}
			if err := r.setNovaSchedulerHash(instance, novaSchedulerHash); err != nil {
				return ctrl.Result{}, err
			}

			return ctrl.Result{RequeueAfter: time.Second * 10}, err
		}
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

	service, op, err := common.CreateOrUpdateService(r.Client, r.Log, service, &serviceInfo)
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
	var apiEndpoint string
	if !strings.HasPrefix(route.Spec.Host, "http") {
		apiEndpoint = fmt.Sprintf("http://%s", route.Spec.Host)
	} else {
		apiEndpoint = route.Spec.Host
	}
	r.setAPIEndpoint(instance, apiEndpoint)

	// deploy cells
	// calc Spec.Cells hash to verify if any of the cells changed
	cellsHash, err := util.ObjectHash(instance.Spec.Cells)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error cells hash: %v", err)
	}
	r.Log.Info("CellsHash: ", "Cells Hash:", cellsHash)

	if instance.Status.CellsHash != cellsHash {
		// Check if nodes to delete information has changed
		for _, cell := range instance.Spec.Cells {
			novaCell := &novav1beta1.NovaCell{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-%s", instance.Name, cell.Name),
					Namespace: instance.Namespace,
				},
				Spec: novav1beta1.NovaCellSpec{
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
				},
			}
			// Set owner reference
			if err := controllerutil.SetControllerReference(instance, novaCell, r.Scheme); err != nil {
				return ctrl.Result{}, err
			}

			// calc hash
			novaCellHash, err := util.ObjectHash(novaCell.Spec)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("error cellHash hash: %v", err)
			}
			r.Log.Info("NovaCellHash: ", fmt.Sprintf("NovaCell Hash %s:", novaCell.Name), novaCellHash)

			foundNovaCell := &novav1beta1.NovaCell{}
			err = r.Client.Get(context.TODO(), types.NamespacedName{Name: novaCell.Name, Namespace: novaCell.Namespace}, foundNovaCell)
			if err != nil && k8s_errors.IsNotFound(err) {
				r.Log.Info("Creating a new NovaCell", "NovaCell.Namespace", novaCell.Namespace, "NovaCell.Name", novaCell.Name)
				err = r.Client.Create(context.TODO(), novaCell)
				if err != nil {
					return ctrl.Result{}, err
				}

				return ctrl.Result{RequeueAfter: time.Second * 10}, err

			} else if err != nil {
				return ctrl.Result{}, err
			} else {
				foundNovaCellHash, err := util.ObjectHash(foundNovaCell.Spec)
				if err != nil {
					return ctrl.Result{}, fmt.Errorf("error cellHash hash: %v", err)
				}
				r.Log.Info("FoundNovaCellHash: ", fmt.Sprintf("foundNovaCell Hash %s:", foundNovaCell.Name), foundNovaCellHash)

				if foundNovaCellHash != novaCellHash {
					r.Log.Info("NovaCell Updated")
					foundNovaCell.Spec = novaCell.Spec
					err = r.Client.Update(context.TODO(), foundNovaCell)
					if err != nil {
						return ctrl.Result{}, err
					}
				}
			}
		}

		// setup sells completed
		if err := r.setCellsHash(instance, cellsHash); err != nil {
			return ctrl.Result{}, err
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

func (r *NovaReconciler) setNovaAPIHash(instance *novav1beta1.Nova, hashStr string) error {

	if hashStr != instance.Status.NovaAPIHash {
		instance.Status.NovaAPIHash = hashStr
		if err := r.Client.Status().Update(context.TODO(), instance); err != nil {
			return err
		}
	}
	return nil
}

func (r *NovaReconciler) setNovaConductorHash(instance *novav1beta1.Nova, hashStr string) error {

	if hashStr != instance.Status.NovaConductorHash {
		instance.Status.NovaConductorHash = hashStr
		if err := r.Client.Status().Update(context.TODO(), instance); err != nil {
			return err
		}
	}
	return nil
}

func (r *NovaReconciler) setNovaSchedulerHash(instance *novav1beta1.Nova, hashStr string) error {

	if hashStr != instance.Status.NovaSchedulerHash {
		instance.Status.NovaSchedulerHash = hashStr
		if err := r.Client.Status().Update(context.TODO(), instance); err != nil {
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

func (r *NovaReconciler) setCellsHash(api *novav1beta1.Nova, hashStr string) error {

	if hashStr != api.Status.CellsHash {
		api.Status.CellsHash = hashStr
		if err := r.Client.Status().Update(context.TODO(), api); err != nil {
			return err
		}
	}
	return nil
}
