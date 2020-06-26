package novacompute

import (
	v1 "github.com/openstack-k8s-operators/nova-operator/pkg/apis/nova/v1"

	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestReconcilerImplementsInterface(t *testing.T) {
	reconciler := ReconcileNovaCompute{}
	var i interface{} = &reconciler
	_, ok := i.(reconcile.Reconciler)
	assert.True(t, ok)
}

func TestNonWatchedResourceNameNotFound(t *testing.T) {
	_, request, reconciler := getTestParams(t)
	request.Name = "foo"

	_, err := reconciler.Reconcile(request)
	assert.NoError(t, err)
}

func getTestParams(t *testing.T) (v1.NovaCompute, reconcile.Request, ReconcileNovaCompute) {
	var request reconcile.Request
	request = reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "openstack",
			Namespace: "openstack",
		},
	}
	compute := v1.NovaCompute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      request.Name,
			Namespace: request.Namespace,
		},
	}
	return compute, request, getReconciler(t, &compute)
}

func getReconciler(t *testing.T, objs ...runtime.Object) ReconcileNovaCompute {
	registerObjs := []runtime.Object{&v1.NovaCompute{}, &appsv1.Deployment{}}
	registerObjs = append(registerObjs)
	v1.SchemeBuilder.Register(registerObjs...)

	scheme, err := v1.SchemeBuilder.Build()
	if err != nil {
		assert.Fail(t, "unable to build scheme")
	}
	client := fake.NewFakeClientWithScheme(scheme, objs...)

	return ReconcileNovaCompute{
		scheme: scheme,
		client: client,
	}
}
