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
	"fmt"
	"testing"
	"time"

	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	internalcommon "github.com/openstack-k8s-operators/nova-operator/internal/common"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// placement uses EnsureSecret for the OpenStack password secret referenced in
// PlacementAPI.Spec.Secret with PasswordSelectors.Service defaulting to
// PlacementPassword.
func TestEnsureSecret_placementMissingSecret(t *testing.T) {
	const (
		namespace          = "test-ns"
		secretName         = "test-osp-secret"
		servicePasswordKey = "PlacementPassword"
	)
	ctx := context.Background()
	requeueTimeout := 5 * time.Second
	conditions := &condition.Conditions{}
	reader := fake.NewClientBuilder().Build()

	hash, result, _, err := internalcommon.EnsureSecret(
		ctx,
		types.NamespacedName{Namespace: namespace, Name: secretName},
		[]string{servicePasswordKey},
		reader,
		conditions,
		requeueTimeout,
	)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if hash != "" {
		t.Fatalf("expected empty hash, got %q", hash)
	}
	if result.RequeueAfter != requeueTimeout {
		t.Fatalf("expected RequeueAfter %s, got %s", requeueTimeout, result.RequeueAfter)
	}

	inputReady := conditions.Get(condition.InputReadyCondition)
	if inputReady == nil {
		t.Fatal("expected InputReady condition to be set")
	}
	if inputReady.Status != corev1.ConditionFalse {
		t.Fatalf("expected InputReady status False, got %q", inputReady.Status)
	}
	if inputReady.Reason != condition.ErrorReason {
		t.Fatalf("expected reason %q, got %q", condition.ErrorReason, inputReady.Reason)
	}
	wantMessage := fmt.Sprintf("Input data resources missing: secret/%s", secretName)
	if inputReady.Message != wantMessage {
		t.Fatalf("expected message %q, got %q", wantMessage, inputReady.Message)
	}
}

func TestEnsureSecret_placementMissingField(t *testing.T) {
	const (
		namespace          = "test-ns"
		secretName         = "test-osp-secret"
		servicePasswordKey = "PlacementPassword"
	)
	ctx := context.Background()
	requeueTimeout := 5 * time.Second
	conditions := &condition.Conditions{}
	reader := fake.NewClientBuilder().WithObjects(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
		Data: map[string][]byte{},
	}).Build()

	hash, result, _, err := internalcommon.EnsureSecret(
		ctx,
		types.NamespacedName{Namespace: namespace, Name: secretName},
		[]string{servicePasswordKey},
		reader,
		conditions,
		requeueTimeout,
	)

	wantErr := fmt.Sprintf("field not found in Secret: '%s' not found in secret/%s", servicePasswordKey, secretName)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != wantErr {
		t.Fatalf("expected error %q, got %q", wantErr, err.Error())
	}
	if hash != "" {
		t.Fatalf("expected empty hash, got %q", hash)
	}
	if result != (ctrl.Result{}) {
		t.Fatalf("expected empty result, got %#v", result)
	}

	inputReady := conditions.Get(condition.InputReadyCondition)
	if inputReady == nil {
		t.Fatal("expected InputReady condition to be set")
	}
	wantMessage := fmt.Sprintf("Input data error occurred %s", wantErr)
	if inputReady.Message != wantMessage {
		t.Fatalf("expected condition message %q, got %q", wantMessage, inputReady.Message)
	}
}

func TestEnsureSecret_placementSecretPresent(t *testing.T) {
	const (
		namespace          = "test-ns"
		secretName         = "test-osp-secret"
		servicePasswordKey = "PlacementPassword"
	)
	ctx := context.Background()
	requeueTimeout := 5 * time.Second
	conditions := &condition.Conditions{}
	reader := fake.NewClientBuilder().WithObjects(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			servicePasswordKey: []byte("12345678"),
		},
	}).Build()

	hash, result, secret, err := internalcommon.EnsureSecret(
		ctx,
		types.NamespacedName{Namespace: namespace, Name: secretName},
		[]string{servicePasswordKey},
		reader,
		conditions,
		requeueTimeout,
	)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if hash == "" {
		t.Fatal("expected non-empty hash")
	}
	if result != (ctrl.Result{}) {
		t.Fatalf("expected empty result, got %#v", result)
	}
	if string(secret.Data[servicePasswordKey]) != "12345678" {
		t.Fatalf("unexpected secret data: %#v", secret.Data)
	}
}
