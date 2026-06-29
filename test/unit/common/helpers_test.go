/*
Copyright 2026.

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

package common_test

import (
	"testing"

	networkv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	helper "github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const testNamespace = "test-ns"

func newTestInstance() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-instance",
			Namespace: testNamespace,
		},
	}
}

func newTestHelper(t *testing.T, s *runtime.Scheme, objs ...client.Object) *helper.Helper {
	t.Helper()

	instance := newTestInstance()
	allObjs := append([]client.Object{instance}, objs...)
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(allObjs...).Build()

	h, err := helper.NewHelper(instance, cl, nil, s, log.Log)
	if err != nil {
		t.Fatalf("helper.NewHelper: %v", err)
	}
	return h
}

func newNetworkTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	s := runtime.NewScheme()
	if err := scheme.AddToScheme(s); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	if err := networkv1.AddToScheme(s); err != nil {
		t.Fatalf("add network scheme: %v", err)
	}
	return s
}

func newTopologyTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	s := runtime.NewScheme()
	if err := scheme.AddToScheme(s); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	if err := topologyv1.AddToScheme(s); err != nil {
		t.Fatalf("add topology scheme: %v", err)
	}
	return s
}
