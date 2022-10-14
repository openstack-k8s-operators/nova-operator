package controllers_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	nova_controllers "github.com/openstack-k8s-operators/nova-operator/controllers"
)

func TestSomething(t *testing.T) {
	assert.Equal(t, 123, 123, "they should be equal")
}

func Test_NewReconcilerBase(t *testing.T) {

	cfg, _ := config.GetConfig()
	kclient, _ := kubernetes.NewForConfig(cfg)
	mgr, _ := manager.New(cfg, manager.Options{})
	name := "test"
	base := nova_controllers.NewReconcilerBase(name, mgr, kclient)
	assert.NotNil(t, base)
	assert.Equal(t, mgr.GetClient(), base.Client)
	assert.Equal(t, mgr.GetScheme(), base.Scheme)
	assert.Equal(t, kclient, base.Kclient)
	log := ctrl.Log.WithName("controllers").WithName(name)
	assert.Equal(t, log, base.Log)
}
