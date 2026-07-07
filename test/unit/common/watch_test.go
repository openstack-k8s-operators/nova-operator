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
	"context"
	"testing"

	commonlib "github.com/openstack-k8s-operators/lib-common/modules/common"
	internalcommon "github.com/openstack-k8s-operators/nova-operator/internal/common"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func newWatchSrc(name string, labels map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
			Labels:    labels,
		},
	}
}

func newWatchTestReader(t *testing.T, objs ...client.Object) client.Reader {
	t.Helper()
	return fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(objs...).Build()
}

func newWatchTestReaderWithFieldIndex(
	t *testing.T,
	field string,
	indexFunc func(client.Object) []string,
	objs ...client.Object,
) client.Reader {
	t.Helper()
	return fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(objs...).
		WithIndex(&corev1.ConfigMap{}, field, indexFunc).
		Build()
}

func configMapList() client.ObjectList {
	return &corev1.ConfigMapList{}
}

func TestFindObjectsForSrcInNamespace_returnsRequestsInNamespace(t *testing.T) {
	ctx := context.Background()
	src := newWatchSrc("osp-secret", nil)
	inNamespace := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "placement", Namespace: testNamespace},
	}
	otherNamespace := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "placement", Namespace: "other-ns"},
	}
	reader := newWatchTestReader(t, inNamespace, otherNamespace)

	requests := internalcommon.FindObjectsForSrcInNamespace(
		ctx, log.Log, reader, src, configMapList,
	)

	if len(requests) != 1 {
		t.Fatalf("expected 1 request, got %d: %#v", len(requests), requests)
	}
	want := reconcile.Request{NamespacedName: types.NamespacedName{Name: "placement", Namespace: testNamespace}}
	if requests[0] != want {
		t.Fatalf("expected request %#v, got %#v", want, requests[0])
	}
}

func TestFindObjectsForSrcInNamespace_listError(t *testing.T) {
	ctx := context.Background()
	src := newWatchSrc("osp-secret", nil)

	requests := internalcommon.FindObjectsForSrcInNamespace(
		ctx, log.Log, errorReader{err: errTestSecretGet}, src, configMapList,
	)

	if requests != nil {
		t.Fatalf("expected nil requests on list error, got %#v", requests)
	}
}

func TestFindObjectsForSrcByField_returnsRequests(t *testing.T) {
	ctx := context.Background()
	src := newWatchSrc("mariadb", nil)
	target := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "placement", Namespace: testNamespace},
	}
	const field = ".spec.databaseInstance"
	reader := newWatchTestReaderWithFieldIndex(t, field, func(obj client.Object) []string {
		if obj.GetName() == "placement" {
			return []string{"mariadb"}
		}
		return nil
	}, target)

	requests := internalcommon.FindObjectsForSrcByField(
		ctx,
		log.Log,
		reader,
		src,
		[]string{field},
		configMapList,
	)

	if len(requests) != 1 {
		t.Fatalf("expected 1 request, got %d: %#v", len(requests), requests)
	}
	want := reconcile.Request{NamespacedName: types.NamespacedName{Name: "placement", Namespace: testNamespace}}
	if requests[0] != want {
		t.Fatalf("expected request %#v, got %#v", want, requests[0])
	}
}

func TestFindObjectsForSrcByField_accumulatesMultipleFields(t *testing.T) {
	ctx := context.Background()
	src := newWatchSrc("shared-input", nil)
	first := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "first", Namespace: testNamespace},
	}
	second := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "second", Namespace: testNamespace},
	}
	reader := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(first, second).
		WithIndex(&corev1.ConfigMap{}, ".spec.secret", func(obj client.Object) []string {
			if obj.GetName() == "first" {
				return []string{"shared-input"}
			}
			return nil
		}).
		WithIndex(&corev1.ConfigMap{}, ".spec.tls", func(obj client.Object) []string {
			if obj.GetName() == "second" {
				return []string{"shared-input"}
			}
			return nil
		}).
		Build()

	requests := internalcommon.FindObjectsForSrcByField(
		ctx,
		log.Log,
		reader,
		src,
		[]string{".spec.secret", ".spec.tls"},
		configMapList,
	)

	if len(requests) != 2 {
		t.Fatalf("expected 2 requests, got %d: %#v", len(requests), requests)
	}
}

func TestFindObjectsForSrcByField_listError(t *testing.T) {
	ctx := context.Background()
	src := newWatchSrc("osp-secret", nil)

	requests := internalcommon.FindObjectsForSrcByField(
		ctx,
		log.Log,
		errorReader{err: errTestSecretGet},
		src,
		[]string{".spec.secret"},
		configMapList,
	)

	if len(requests) != 0 {
		t.Fatalf("expected no requests on list error, got %#v", requests)
	}
}

func TestFindObjectsWithAppSelectorLabelInNamespace_matchingLabel(t *testing.T) {
	ctx := context.Background()
	src := newWatchSrc("keystone-endpoint", map[string]string{
		commonlib.AppSelector: "placement",
	})
	target := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "placement", Namespace: testNamespace},
	}
	reader := newWatchTestReader(t, target)

	requests := internalcommon.FindObjectsWithAppSelectorLabelInNamespace(
		ctx,
		log.Log,
		reader,
		src,
		[]string{"placement", "nova"},
		configMapList,
	)

	if len(requests) != 1 {
		t.Fatalf("expected 1 request, got %d: %#v", len(requests), requests)
	}
}

func TestFindObjectsWithAppSelectorLabelInNamespace_labelNotAllowed(t *testing.T) {
	ctx := context.Background()
	src := newWatchSrc("keystone-endpoint", map[string]string{
		commonlib.AppSelector: "glance",
	})

	requests := internalcommon.FindObjectsWithAppSelectorLabelInNamespace(
		ctx,
		log.Log,
		newWatchTestReader(t),
		src,
		[]string{"placement", "nova"},
		configMapList,
	)

	if requests != nil {
		t.Fatalf("expected nil requests when label not allowed, got %#v", requests)
	}
}

func TestFindObjectsWithAppSelectorLabelInNamespace_missingLabel(t *testing.T) {
	ctx := context.Background()
	src := newWatchSrc("keystone-endpoint", nil)

	requests := internalcommon.FindObjectsWithAppSelectorLabelInNamespace(
		ctx,
		log.Log,
		newWatchTestReader(t),
		src,
		[]string{"placement"},
		configMapList,
	)

	if requests != nil {
		t.Fatalf("expected nil requests when label missing, got %#v", requests)
	}
}
