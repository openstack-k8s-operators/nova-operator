package common

import (
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ReconcilerBase provides a common set of clients scheme and loggers for all reconcilers.
type ReconcilerBase struct {
	Client  client.Client
	Kclient kubernetes.Interface
	Log     logr.Logger
	Scheme  *runtime.Scheme
}

// Managable all types that conform to this interface can be setup with a controller-runtime manager.
type Managable interface {
	SetupWithManager(mgr ctrl.Manager) error
}
