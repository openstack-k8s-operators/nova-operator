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

	// ScriptsConfigMap - create if not exist, update if changed manually
	scriptsConfigMap := novacell.ScriptsConfigMap(instance, r.Scheme, r.Log)
	err = common.CreateOrUpdateConfigMap(r.Client, r.Log, scriptsConfigMap)
	if err != nil {
		return ctrl.Result{}, err
	}

	// ConfigMap - create if not exist, update if changed manually
	configMap := novacell.ConfigMap(instance, r.Scheme)
	err = common.CreateOrUpdateConfigMap(r.Client, r.Log, configMap)
	if err != nil {
		return ctrl.Result{}, err
	}

	// CustomConfigMap - only create create if not exist
	customConfigMap := novacell.CustomConfigMap(instance, r.Scheme)
	err = common.CreateOrGetCustomConfigMap(r.Client, r.Log, customConfigMap)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Create the cell DB
	dbName := fmt.Sprintf("nova_%s", instance.Spec.Cell)
	databaseObj, err := novacell.DatabaseObject(instance, dbName)
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

	// deploy cell nova-conductor
	novaConductor := &novav1beta1.NovaConductor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-conductor", instance.Name),
			Namespace: instance.Namespace,
		},
		Spec: novav1beta1.NovaConductorSpec{
			ManagingCrName:     instance.Name,
			Cell:               instance.Spec.Cell,
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

	// deploy cell nova-matadata
	novaMetadata := &novav1beta1.NovaMetadata{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-metadata", instance.Name),
			Namespace: instance.Namespace,
		},
		Spec: novav1beta1.NovaMetadataSpec{
			ManagingCrName:     instance.Name,
			Cell:               instance.Spec.Cell,
			DatabaseHostname:   instance.Spec.DatabaseHostname,
			NovaSecret:         instance.Spec.NovaSecret,
			NeutronSecret:      instance.Spec.NeutronSecret,
			PlacementSecret:    instance.Spec.PlacementSecret,
			TransportURLSecret: instance.Spec.TransportURLSecret,
			Replicas:           instance.Spec.NovaMetadataReplicas,
			ContainerImage:     instance.Spec.NovaMetadataContainerImage,
		},
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(instance, novaMetadata, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	// calc hash
	novaMetadataHash, err := util.ObjectHash(novaMetadata)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error statefulSet hash: %v", err)
	}
	r.Log.Info("NovaMetadataHash: ", "NovaMetadata Hash:", novaMetadataHash)

	foundNovaMetadata := &novav1beta1.NovaMetadata{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: novaMetadata.Name, Namespace: novaMetadata.Namespace}, foundNovaMetadata)
	if err != nil && k8s_errors.IsNotFound(err) {
		r.Log.Info("Creating a new NovaMetadata", "NovaMetadata.Namespace", novaMetadata.Namespace, "NovaMetadata.Name", novaMetadata.Name)
		err = r.Client.Create(context.TODO(), novaMetadata)
		if err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{RequeueAfter: time.Second * 10}, err

	} else if err != nil {
		return ctrl.Result{}, err
	} else {
		if instance.Status.NovaMetadataHash != novaMetadataHash {
			r.Log.Info("NovaMetadata Updated")
			foundNovaMetadata.Spec = novaMetadata.Spec
			err = r.Client.Update(context.TODO(), foundNovaMetadata)
			if err != nil {
				return ctrl.Result{}, err
			}
			if err := r.setNovaMetadataHash(instance, novaMetadataHash); err != nil {
				return ctrl.Result{}, err
			}

			return ctrl.Result{RequeueAfter: time.Second * 10}, err
		}
	}

	// deploy cell nova-novncproxy
	novaNoVNCProxy := &novav1beta1.NovaNoVNCProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-novncproxy", instance.Name),
			Namespace: instance.Namespace,
		},
		Spec: novav1beta1.NovaNoVNCProxySpec{
			ManagingCrName:     instance.Name,
			Cell:               instance.Spec.Cell,
			DatabaseHostname:   instance.Spec.DatabaseHostname,
			NovaSecret:         instance.Spec.NovaSecret,
			NeutronSecret:      instance.Spec.NeutronSecret,
			PlacementSecret:    instance.Spec.PlacementSecret,
			TransportURLSecret: instance.Spec.TransportURLSecret,
			Replicas:           instance.Spec.NovaNoVNCProxyReplicas,
			ContainerImage:     instance.Spec.NovaNoVNCProxyContainerImage,
		},
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(instance, novaNoVNCProxy, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	// calc hash
	novaNoVNCProxyHash, err := util.ObjectHash(novaNoVNCProxy)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error statefulSet hash: %v", err)
	}
	r.Log.Info("NovaNoVNCProxyHash: ", "NovaNoVNCProxy Hash:", novaNoVNCProxyHash)

	foundNovaNoVNCProxy := &novav1beta1.NovaNoVNCProxy{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: novaNoVNCProxy.Name, Namespace: novaNoVNCProxy.Namespace}, foundNovaNoVNCProxy)
	if err != nil && k8s_errors.IsNotFound(err) {
		r.Log.Info("Creating a new NovaNoVNCProxy", "NovaNoVNCProxy.Namespace", novaNoVNCProxy.Namespace, "NovaNoVNCProxy.Name", novaNoVNCProxy.Name)
		err = r.Client.Create(context.TODO(), novaNoVNCProxy)
		if err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{RequeueAfter: time.Second * 10}, err

	} else if err != nil {
		return ctrl.Result{}, err
	} else {
		if instance.Status.NovaNoVNCProxyHash != novaNoVNCProxyHash {
			r.Log.Info("NovaNoVNCProxy Updated")
			foundNovaNoVNCProxy.Spec = novaNoVNCProxy.Spec
			err = r.Client.Update(context.TODO(), foundNovaNoVNCProxy)
			if err != nil {
				return ctrl.Result{}, err
			}
			if err := r.setNovaNoVNCProxyHash(instance, novaNoVNCProxyHash); err != nil {
				return ctrl.Result{}, err
			}

			return ctrl.Result{RequeueAfter: time.Second * 10}, err
		}
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

	service, op, err := common.CreateOrUpdateService(r.Client, r.Log, service, &serviceInfo)
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

func (r *NovaCellReconciler) setNovaConductorHash(instance *novav1beta1.NovaCell, hashStr string) error {

	if hashStr != instance.Status.NovaConductorHash {
		instance.Status.NovaConductorHash = hashStr
		if err := r.Client.Status().Update(context.TODO(), instance); err != nil {
			return err
		}
	}
	return nil
}

func (r *NovaCellReconciler) setNovaMetadataHash(instance *novav1beta1.NovaCell, hashStr string) error {

	if hashStr != instance.Status.NovaMetadataHash {
		instance.Status.NovaMetadataHash = hashStr
		if err := r.Client.Status().Update(context.TODO(), instance); err != nil {
			return err
		}
	}
	return nil
}

func (r *NovaCellReconciler) setNovaNoVNCProxyHash(instance *novav1beta1.NovaCell, hashStr string) error {

	if hashStr != instance.Status.NovaNoVNCProxyHash {
		instance.Status.NovaNoVNCProxyHash = hashStr
		if err := r.Client.Status().Update(context.TODO(), instance); err != nil {
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
