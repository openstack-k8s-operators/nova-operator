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
	"strings"
	"testing"

	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	internalcommon "github.com/openstack-k8s-operators/nova-operator/internal/common"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type fakeTopologyHandler struct {
	specRef        *topologyv1.TopoRef
	lastApplied    *topologyv1.TopoRef
	lastAppliedSet *topologyv1.TopoRef
}

func (f *fakeTopologyHandler) GetSpecTopologyRef() *topologyv1.TopoRef {
	return f.specRef
}

func (f *fakeTopologyHandler) GetLastAppliedTopology() *topologyv1.TopoRef {
	return f.lastApplied
}

func (f *fakeTopologyHandler) SetLastAppliedTopology(t *topologyv1.TopoRef) {
	f.lastAppliedSet = t
}

func TestEnsureTopology_noTopologyRef(t *testing.T) {
	ctx := context.Background()
	conditions := &condition.Conditions{}
	h := newTestHelper(t, newTopologyTestScheme(t))
	handler := &fakeTopologyHandler{
		lastApplied: &topologyv1.TopoRef{Name: "old", Namespace: testNamespace},
	}

	topology, err := internalcommon.EnsureTopology(
		ctx, h, handler, "placementapi", conditions, metav1.LabelSelector{},
	)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if topology != nil {
		t.Fatalf("expected nil topology, got %#v", topology)
	}
	if handler.lastAppliedSet != nil {
		t.Fatalf("expected last applied topology cleared, got %#v", handler.lastAppliedSet)
	}
	if conditions.Get(condition.TopologyReadyCondition) != nil {
		t.Fatal("expected TopologyReady condition to be unset")
	}
}

func TestEnsureTopology_missingTopology(t *testing.T) {
	const topologyName = "missing-topology"
	ctx := context.Background()
	conditions := &condition.Conditions{}
	h := newTestHelper(t, newTopologyTestScheme(t))
	handler := &fakeTopologyHandler{
		specRef: &topologyv1.TopoRef{
			Name:      topologyName,
			Namespace: testNamespace,
		},
	}

	topology, err := internalcommon.EnsureTopology(
		ctx, h, handler, "placementapi", conditions, metav1.LabelSelector{},
	)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "waiting for Topology requirements") {
		t.Fatalf("expected wrapped topology wait error, got %v", err)
	}
	if topology != nil {
		t.Fatalf("expected nil topology, got %#v", topology)
	}

	topologyReady := conditions.Get(condition.TopologyReadyCondition)
	if topologyReady == nil {
		t.Fatal("expected TopologyReady condition to be set")
	}
	if topologyReady.Status != corev1.ConditionFalse {
		t.Fatalf("expected status False, got %q", topologyReady.Status)
	}
	if topologyReady.Reason != condition.ErrorReason {
		t.Fatalf("expected reason %q, got %q", condition.ErrorReason, topologyReady.Reason)
	}
	wantPrefix := fmt.Sprintf(condition.TopologyReadyErrorMessage, "")
	if !strings.HasPrefix(topologyReady.Message, strings.TrimSuffix(wantPrefix, "%s")) {
		t.Fatalf("unexpected condition message %q", topologyReady.Message)
	}
}

func TestEnsureTopology_topologyPresent(t *testing.T) {
	const topologyName = "az-one"
	ctx := context.Background()
	conditions := &condition.Conditions{}
	scheme := newTopologyTestScheme(t)
	topologyCR := &topologyv1.Topology{
		ObjectMeta: metav1.ObjectMeta{
			Name:      topologyName,
			Namespace: testNamespace,
		},
	}
	h := newTestHelper(t, scheme, topologyCR)
	specRef := &topologyv1.TopoRef{
		Name:      topologyName,
		Namespace: testNamespace,
	}
	handler := &fakeTopologyHandler{specRef: specRef}

	topology, err := internalcommon.EnsureTopology(
		ctx, h, handler, "placementapi", conditions, metav1.LabelSelector{},
	)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if topology == nil || topology.Name != topologyName {
		t.Fatalf("expected topology %q, got %#v", topologyName, topology)
	}
	if handler.lastAppliedSet == nil || handler.lastAppliedSet.Name != topologyName {
		t.Fatalf("expected last applied topology %q, got %#v", topologyName, handler.lastAppliedSet)
	}

	topologyReady := conditions.Get(condition.TopologyReadyCondition)
	if topologyReady == nil {
		t.Fatal("expected TopologyReady condition to be set")
	}
	if topologyReady.Status != corev1.ConditionTrue {
		t.Fatalf("expected status True, got %q", topologyReady.Status)
	}
	if topologyReady.Message != condition.TopologyReadyMessage {
		t.Fatalf("expected message %q, got %q", condition.TopologyReadyMessage, topologyReady.Message)
	}
}
