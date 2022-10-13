package controllers

import (
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ReconcilerBase provides a common set of clients scheme and loggers for all reconcilers.
type ReconcilerBase struct {
	Client         client.Client
	Kclient        kubernetes.Interface
	Log            logr.Logger
	Scheme         *runtime.Scheme
	RequeueTimeout time.Duration
}

// Managable all types that conform to this interface can be setup with a controller-runtime manager.
type Managable interface {
	SetupWithManager(mgr ctrl.Manager) error
}

// Reconciler represents a generic interface for all Reconciler objects in nova
type Reconciler interface {
	Managable
	SetRequeueTimeout(timeout time.Duration)
}

// NewReconcilerBase constructs a ReconcilerBase given a name manager and Kclient.
func NewReconcilerBase(
	name string, mgr ctrl.Manager, kclient kubernetes.Interface,
) ReconcilerBase {
	log := ctrl.Log.WithName("controllers").WithName(name)
	return ReconcilerBase{
		Client:         mgr.GetClient(),
		Scheme:         mgr.GetScheme(),
		Kclient:        kclient,
		Log:            log,
		RequeueTimeout: time.Duration(5) * time.Second,
	}
}

// SetRequeueTimeout overrides the default RequeueTimeout of the Reconciler
func (r *ReconcilerBase) SetRequeueTimeout(timeout time.Duration) {
	r.RequeueTimeout = timeout
}

// Reconcilers holds all the Reconciler objects of the nova-operator to
// allow generic managemenet of them.
type Reconcilers struct {
	reconcilers map[string]Reconciler
}

// NewReconcilers constructs all nova Reconciler objects
func NewReconcilers(mgr ctrl.Manager, kclient *kubernetes.Clientset) *Reconcilers {
	return &Reconcilers{
		reconcilers: map[string]Reconciler{
			"Nova": &NovaReconciler{
				ReconcilerBase: NewReconcilerBase("Nova", mgr, kclient),
			},
			"NovaCell": &NovaCellReconciler{
				ReconcilerBase: NewReconcilerBase("NovaCell", mgr, kclient),
			},
			"NovaAPI": &NovaAPIReconciler{
				ReconcilerBase: NewReconcilerBase("NovaAPI", mgr, kclient),
			},
			"NovaScheduler": &NovaSchedulerReconciler{
				ReconcilerBase: NewReconcilerBase("NovaScheduler", mgr, kclient),
			},
			"NovaConductor": &NovaConductorReconciler{
				ReconcilerBase: NewReconcilerBase("NovaConductor", mgr, kclient),
			},
			"NovaMetadata": &NovaMetadataReconciler{
				ReconcilerBase: NewReconcilerBase("NovaMetadata", mgr, kclient),
			},
			"NovaNoVNCProxy": &NovaNoVNCProxyReconciler{
				ReconcilerBase: NewReconcilerBase("NovaNoVNCProxy", mgr, kclient),
			},
		}}
}

// Setup starts the reconcilers by connecting them to the Manager
func (r *Reconcilers) Setup(mgr ctrl.Manager, setupLog logr.Logger) error {
	var err error
	for name, controller := range r.reconcilers {
		if err = controller.SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", name)
			return err
		}
	}
	return nil
}

// OverriedRequeueTimeout overrieds the default RequeueTimeout of our reconcilers
func (r *Reconcilers) OverriedRequeueTimeout(timeout time.Duration) {
	for _, reconciler := range r.reconcilers {
		reconciler.SetRequeueTimeout(timeout)
	}
}
