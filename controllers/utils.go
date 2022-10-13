package controllers

import (
	"os"

	"github.com/go-logr/logr"
	nova_common "github.com/openstack-k8s-operators/nova-operator/pkg/common"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
)

// SetupReconcilers registers all reconcilers with a provided manager.
func SetupReconcilers(mgr ctrl.Manager, setupLog logr.Logger, cfg *rest.Config) {
	kclient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		setupLog.Error(err, "unable to create k8s client")
		os.Exit(1)
	}
	reconcilers := map[string]nova_common.Managable{
		"Nova": &NovaReconciler{
			ReconcilerBase: nova_common.NewReconcilerBase("Nova", mgr, kclient),
		},
		"NovaCell": &NovaCellReconciler{
			ReconcilerBase: nova_common.NewReconcilerBase("NovaCell", mgr, kclient),
		},
		"NovaAPI": &NovaAPIReconciler{
			ReconcilerBase: nova_common.NewReconcilerBase("NovaAPI", mgr, kclient),
		},
		"NovaScheduler": &NovaSchedulerReconciler{
			ReconcilerBase: nova_common.NewReconcilerBase("NovaScheduler", mgr, kclient),
		},
		"NovaConductor": &NovaConductorReconciler{
			ReconcilerBase: nova_common.NewReconcilerBase("NovaConductor", mgr, kclient),
		},
		"NovaMetadata": &NovaMetadataReconciler{
			ReconcilerBase: nova_common.NewReconcilerBase("NovaMetadata", mgr, kclient),
		},
		"NovaNoVNCProxy": &NovaNoVNCProxyReconciler{
			ReconcilerBase: nova_common.NewReconcilerBase("NovaNoVNCProxy", mgr, kclient),
		},
	}
	for name, controller := range reconcilers {
		if err = controller.SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", name)
			os.Exit(1)
		}
	}
}
