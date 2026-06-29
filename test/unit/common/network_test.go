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

	networkv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	internalcommon "github.com/openstack-k8s-operators/nova-operator/internal/common"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

func TestEnsureNetworkAttachments_noAttachments(t *testing.T) {
	ctx := context.Background()
	requeueTimeout := 5 * time.Second
	conditions := &condition.Conditions{}
	h := newTestHelper(t, newNetworkTestScheme(t))

	annotations, result, err := internalcommon.EnsureNetworkAttachments(
		ctx, h, nil, conditions, requeueTimeout,
	)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result != (ctrl.Result{}) {
		t.Fatalf("expected empty result, got %#v", result)
	}
	want := map[string]string{networkv1.NetworkAttachmentAnnot: "[]"}
	if annotations == nil || annotations[networkv1.NetworkAttachmentAnnot] != want[networkv1.NetworkAttachmentAnnot] {
		t.Fatalf("expected annotations %v, got %v", want, annotations)
	}
	if conditions.Get(condition.NetworkAttachmentsReadyCondition) != nil {
		t.Fatal("expected NetworkAttachmentsReady condition to be unset")
	}
}

func TestEnsureNetworkAttachments_missingNAD(t *testing.T) {
	const nadName = "missing-nad"
	ctx := context.Background()
	requeueTimeout := 5 * time.Second
	conditions := &condition.Conditions{}
	h := newTestHelper(t, newNetworkTestScheme(t))

	annotations, result, err := internalcommon.EnsureNetworkAttachments(
		ctx, h, []string{nadName}, conditions, requeueTimeout,
	)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.RequeueAfter != requeueTimeout {
		t.Fatalf("expected RequeueAfter %s, got %s", requeueTimeout, result.RequeueAfter)
	}
	if annotations != nil {
		t.Fatalf("expected nil annotations, got %v", annotations)
	}

	nadReady := conditions.Get(condition.NetworkAttachmentsReadyCondition)
	if nadReady == nil {
		t.Fatal("expected NetworkAttachmentsReady condition to be set")
	}
	if nadReady.Status != corev1.ConditionFalse {
		t.Fatalf("expected status False, got %q", nadReady.Status)
	}
	if nadReady.Reason != condition.ErrorReason {
		t.Fatalf("expected reason %q, got %q", condition.ErrorReason, nadReady.Reason)
	}
	wantMessage := fmt.Sprintf(condition.NetworkAttachmentsReadyWaitingMessage, nadName)
	if nadReady.Message != wantMessage {
		t.Fatalf("expected message %q, got %q", wantMessage, nadReady.Message)
	}
}

func TestEnsureNetworkAttachments_nadPresent(t *testing.T) {
	const nadName = "internalapi"
	ctx := context.Background()
	requeueTimeout := 5 * time.Second
	conditions := &condition.Conditions{}
	scheme := newNetworkTestScheme(t)
	nad := &networkv1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nadName,
			Namespace: testNamespace,
		},
	}
	h := newTestHelper(t, scheme, nad)

	annotations, result, err := internalcommon.EnsureNetworkAttachments(
		ctx, h, []string{nadName}, conditions, requeueTimeout,
	)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result != (ctrl.Result{}) {
		t.Fatalf("expected empty result, got %#v", result)
	}
	want := fmt.Sprintf(
		`[{"name":"%s","namespace":"%s","interface":"%s"}]`,
		nadName, testNamespace, nadName,
	)
	if annotations == nil || annotations[networkv1.NetworkAttachmentAnnot] != want {
		t.Fatalf("expected annotation %q, got %v", want, annotations)
	}
	if conditions.Get(condition.NetworkAttachmentsReadyCondition) != nil {
		t.Fatal("expected NetworkAttachmentsReady condition to be unset")
	}
}
