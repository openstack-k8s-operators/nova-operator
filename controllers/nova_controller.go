/*
Copyright 2022.

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

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/go-logr/logr"
	"github.com/openstack-k8s-operators/lib-common/modules/common/configmap"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	helper "github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	labels "github.com/openstack-k8s-operators/lib-common/modules/common/labels"
	util "github.com/openstack-k8s-operators/lib-common/modules/common/util"

	novav1beta1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"

	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
)

// NovaReconciler reconciles a Nova object
type NovaReconciler struct {
	client.Client
	Kclient kubernetes.Interface
	Log     logr.Logger
	Scheme  *runtime.Scheme
}

//+kubebuilder:rbac:groups=nova.openstack.org,resources=nova,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=nova.openstack.org,resources=nova/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=nova.openstack.org,resources=nova/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Nova object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.2/pkg/reconcile
func (r *NovaReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// TODO(user): your logic here
	instance := &novav1beta1.Nova{}
	err := r.Client.Get(ctx, req.NamespacedName, instance)
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

	helper, err := helper.NewHelper(
		instance,
		r.Client,
		r.Kclient,
		r.Scheme,
		r.Log,
	)
	if err != nil {
		return ctrl.Result{}, err
	}
	// ConfigMap
	configMapVars := make(map[string]env.Setter)
	err = r.generateServiceConfigMaps(ctx, helper, instance, &configMapVars)
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *NovaReconciler) generateServiceConfigMaps(
	ctx context.Context, h *helper.Helper,
	instance *novav1beta1.Nova, envVars *map[string]env.Setter,
) error {
	_ = log.FromContext(ctx)
	r.Log.Info("Reconciling Service")

	templateParameters := make(map[string]interface{})
	templateParameters["fqdn"] = "nova.test.example.com"
	templateParameters["debug"] = true

	cmLabels := labels.GetLabels(instance, labels.GetGroupLabel("nova"), map[string]string{})
	customData := map[string]string{
		"test": "42",
	}
	additionalTemplates := map[string]string{
		"01-nova.conf":        "/nova.conf",
		"02-nova-secret.conf": "/nova-secret.conf",
	}
	cms := []util.Template{
		// ConfigMap
		{
			Name:               fmt.Sprintf("%s-config-data", instance.Name),
			Namespace:          instance.Namespace,
			Type:               util.TemplateTypeConfig,
			InstanceType:       instance.Kind,
			CustomData:         customData,
			ConfigOptions:      templateParameters,
			Labels:             cmLabels,
			Annotations:        map[string]string{},
			AdditionalTemplate: additionalTemplates,
		},
	}
	err := configmap.EnsureConfigMaps(ctx, h, instance, cms, envVars)
	if err != nil {
		return nil
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NovaReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&novav1beta1.Nova{}).
		Complete(r)
}
