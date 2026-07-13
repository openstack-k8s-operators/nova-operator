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

	corev1 "k8s.io/api/core/v1"

	"github.com/openstack-k8s-operators/lib-common/modules/common/probes"
	internalcommon "github.com/openstack-k8s-operators/nova-operator/internal/common"
)

func TestGetDefaultProbesAPI(t *testing.T) {
	defaults := internalcommon.GetDefaultProbesAPI(60)

	if defaults.ReadinessProbes.TimeoutSeconds != 18 {
		t.Fatalf("readiness timeout = %d, want 18", defaults.ReadinessProbes.TimeoutSeconds)
	}
	if defaults.ReadinessProbes.PeriodSeconds != 18 {
		t.Fatalf("readiness period = %d, want 18", defaults.ReadinessProbes.PeriodSeconds)
	}
	if defaults.ReadinessProbes.FailureThreshold != 3 {
		t.Fatalf("readiness failureThreshold = %d, want 3", defaults.ReadinessProbes.FailureThreshold)
	}
	if defaults.LivenessProbes.TimeoutSeconds != 30 {
		t.Fatalf("liveness timeout = %d, want 30", defaults.LivenessProbes.TimeoutSeconds)
	}
	if defaults.LivenessProbes.PeriodSeconds != 30 {
		t.Fatalf("liveness period = %d, want 30", defaults.LivenessProbes.PeriodSeconds)
	}
	if defaults.LivenessProbes.FailureThreshold != 10 {
		t.Fatalf("liveness failureThreshold = %d, want 10", defaults.LivenessProbes.FailureThreshold)
	}
	if defaults.StartupProbes == nil {
		t.Fatal("API services should define startup probes")
	}
	if defaults.StartupProbes.PeriodSeconds != 10 {
		t.Fatalf("startup period = %d, want 10", defaults.StartupProbes.PeriodSeconds)
	}
	if defaults.StartupProbes.FailureThreshold != 6 {
		t.Fatalf("startup failureThreshold = %d, want 6", defaults.StartupProbes.FailureThreshold)
	}
}

func TestGetDefaultProbesNoVNC(t *testing.T) {
	defaults := internalcommon.GetDefaultProbesNoVNC()

	if defaults.LivenessProbes == nil {
		t.Fatal("novncproxy should define liveness probe defaults")
	}
	if defaults.LivenessProbes.Path != "/vnc_lite.html" {
		t.Fatalf("liveness path = %q, want /vnc_lite.html", defaults.LivenessProbes.Path)
	}
	if defaults.LivenessProbes.TimeoutSeconds != 10 {
		t.Fatalf("liveness timeout = %d, want 10", defaults.LivenessProbes.TimeoutSeconds)
	}
	if defaults.LivenessProbes.PeriodSeconds != 10 {
		t.Fatalf("liveness period = %d, want 10", defaults.LivenessProbes.PeriodSeconds)
	}

	if defaults.ReadinessProbes == nil {
		t.Fatal("novncproxy should define readiness probe defaults")
	}
	if defaults.ReadinessProbes.Path != "/vnc_lite.html" {
		t.Fatalf("readiness path = %q, want /vnc_lite.html", defaults.ReadinessProbes.Path)
	}
	if defaults.ReadinessProbes.TimeoutSeconds != 5 {
		t.Fatalf("readiness timeout = %d, want 5", defaults.ReadinessProbes.TimeoutSeconds)
	}
	if defaults.ReadinessProbes.PeriodSeconds != 5 {
		t.Fatalf("readiness period = %d, want 5", defaults.ReadinessProbes.PeriodSeconds)
	}

	if defaults.StartupProbes == nil {
		t.Fatal("novncproxy should define startup probe defaults")
	}
	if defaults.StartupProbes.Path != "/vnc_lite.html" {
		t.Fatalf("startup path = %q, want /vnc_lite.html", defaults.StartupProbes.Path)
	}
	if defaults.StartupProbes.PeriodSeconds != 10 {
		t.Fatalf("startup period = %d, want 10", defaults.StartupProbes.PeriodSeconds)
	}
	if defaults.StartupProbes.FailureThreshold != 6 {
		t.Fatalf("startup failureThreshold = %d, want 6", defaults.StartupProbes.FailureThreshold)
	}
}

func TestGetDefaultProbesRPC(t *testing.T) {
	command := []string{"/usr/bin/pgrep", "-r", "DRST", "nova-scheduler"}
	defaults := internalcommon.GetDefaultProbesRPC(internalcommon.DefaultServiceDownTime, command)

	if defaults.LivenessProbes.TimeoutSeconds != 10 {
		t.Fatalf("liveness timeout = %d, want 10", defaults.LivenessProbes.TimeoutSeconds)
	}
	if defaults.LivenessProbes.PeriodSeconds != 20 {
		t.Fatalf("liveness period = %d, want 20", defaults.LivenessProbes.PeriodSeconds)
	}
	if defaults.LivenessProbes.InitialDelaySeconds != 15 {
		t.Fatalf("liveness initialDelay = %d, want 15", defaults.LivenessProbes.InitialDelaySeconds)
	}
	if defaults.StartupProbes.PeriodSeconds != 10 {
		t.Fatalf("startup period = %d, want 10", defaults.StartupProbes.PeriodSeconds)
	}
	if defaults.StartupProbes.InitialDelaySeconds != 20 {
		t.Fatalf("startup initialDelay = %d, want 20", defaults.StartupProbes.InitialDelaySeconds)
	}
	if defaults.StartupProbes.FailureThreshold != 12 {
		t.Fatalf("startup failureThreshold = %d, want 12", defaults.StartupProbes.FailureThreshold)
	}
	if defaults.ReadinessProbes == nil || defaults.ReadinessProbes.Type != probes.ProbeHandlerExec {
		t.Fatal("RPC readiness probe must be exec type")
	}
	if len(defaults.ReadinessProbes.Command) == 0 {
		t.Fatal("RPC readiness probe requires exec command")
	}
	if defaults.ReadinessProbes.PeriodSeconds != 5 {
		t.Fatalf("readiness period = %d, want 5", defaults.ReadinessProbes.PeriodSeconds)
	}
	if defaults.ReadinessProbes.TimeoutSeconds != 5 {
		t.Fatalf("readiness timeout = %d, want 5", defaults.ReadinessProbes.TimeoutSeconds)
	}
}

func TestCreateProbeSetAPI(t *testing.T) {
	scheme := corev1.URISchemeHTTP
	probeSet, err := probes.CreateProbeSet(
		8774,
		&scheme,
		probes.OverrideSpec{},
		internalcommon.GetDefaultProbesAPI(60),
	)
	if err != nil {
		t.Fatalf("CreateProbeSet failed: %v", err)
	}
	if probeSet.Liveness == nil || probeSet.Readiness == nil || probeSet.Startup == nil {
		t.Fatal("expected liveness, readiness, and startup probes")
	}
	if probeSet.Liveness.HTTPGet == nil {
		t.Fatal("expected HTTP GET liveness probe")
	}
	if probeSet.Liveness.PeriodSeconds != 30 {
		t.Fatalf("liveness period = %d, want 30", probeSet.Liveness.PeriodSeconds)
	}
	if probeSet.Readiness.PeriodSeconds != 18 {
		t.Fatalf("readiness period = %d, want 18", probeSet.Readiness.PeriodSeconds)
	}
}

func TestCreateProbeSetPlacement(t *testing.T) {
	scheme := corev1.URISchemeHTTP
	defaults := internalcommon.GetDefaultProbesAPI(60)
	defaults.LivenessProbes.InitialDelaySeconds = 5
	defaults.ReadinessProbes.InitialDelaySeconds = 5

	probeSet, err := probes.CreateProbeSet(8778, &scheme, probes.OverrideSpec{}, defaults)
	if err != nil {
		t.Fatalf("CreateProbeSet failed: %v", err)
	}
	if probeSet.Liveness == nil || probeSet.Readiness == nil {
		t.Fatal("expected liveness and readiness probes")
	}
	if probeSet.Liveness.InitialDelaySeconds != 5 {
		t.Fatalf("liveness initialDelay = %d, want 5", probeSet.Liveness.InitialDelaySeconds)
	}
	if probeSet.Readiness.InitialDelaySeconds != 5 {
		t.Fatalf("readiness initialDelay = %d, want 5", probeSet.Readiness.InitialDelaySeconds)
	}
}

func TestCreateProbeSetNoVNC(t *testing.T) {
	scheme := corev1.URISchemeHTTP
	probeSet, err := probes.CreateProbeSet(
		6080,
		&scheme,
		probes.OverrideSpec{},
		internalcommon.GetDefaultProbesNoVNC(),
	)
	if err != nil {
		t.Fatalf("CreateProbeSet failed: %v", err)
	}
	if probeSet.Liveness == nil || probeSet.Readiness == nil || probeSet.Startup == nil {
		t.Fatal("expected liveness, readiness, and startup probes")
	}
	if probeSet.Liveness.HTTPGet == nil || probeSet.Liveness.HTTPGet.Path != "/vnc_lite.html" {
		t.Fatalf("liveness path = %q, want /vnc_lite.html", probeSet.Liveness.HTTPGet.Path)
	}
	if probeSet.Liveness.TimeoutSeconds != 10 {
		t.Fatalf("liveness timeout = %d, want 10", probeSet.Liveness.TimeoutSeconds)
	}
	if probeSet.Liveness.PeriodSeconds != 10 {
		t.Fatalf("liveness period = %d, want 10", probeSet.Liveness.PeriodSeconds)
	}
	if probeSet.Readiness.HTTPGet == nil || probeSet.Readiness.HTTPGet.Path != "/vnc_lite.html" {
		t.Fatalf("readiness path = %q, want /vnc_lite.html", probeSet.Readiness.HTTPGet.Path)
	}
	if probeSet.Readiness.TimeoutSeconds != 5 {
		t.Fatalf("readiness timeout = %d, want 5", probeSet.Readiness.TimeoutSeconds)
	}
	if probeSet.Readiness.PeriodSeconds != 5 {
		t.Fatalf("readiness period = %d, want 5", probeSet.Readiness.PeriodSeconds)
	}
	if probeSet.Startup.FailureThreshold != 6 {
		t.Fatalf("startup failureThreshold = %d, want 6", probeSet.Startup.FailureThreshold)
	}
	if probeSet.Startup.PeriodSeconds != 10 {
		t.Fatalf("startup period = %d, want 10", probeSet.Startup.PeriodSeconds)
	}
}

// rpcProbeSetChecks verifies the probe set produced by GetDefaultProbesRPC for
// a given service name and pgrep command. CreateProbeSetV2 always builds all
// three probes from the defaults; the deployment then selects which to wire into
// the pod spec (conductor and compute: liveness + readiness; scheduler: all three).
func rpcProbeSetChecks(t *testing.T, name string, command []string) {
	t.Helper()

	probeSet, err := probes.CreateProbeSetV2(
		probes.OverrideSpec{},
		internalcommon.GetDefaultProbesRPC(internalcommon.DefaultServiceDownTime, command),
	)
	if err != nil {
		t.Fatalf("%s: CreateProbeSetV2 failed: %v", name, err)
	}

	if probeSet.Liveness == nil {
		t.Fatalf("%s: expected liveness probe", name)
	}
	if probeSet.Liveness.Exec == nil {
		t.Fatalf("%s: expected exec liveness probe", name)
	}
	if probeSet.Liveness.TimeoutSeconds != 10 {
		t.Fatalf("%s: liveness timeout = %d, want 10", name, probeSet.Liveness.TimeoutSeconds)
	}
	if probeSet.Liveness.PeriodSeconds != 20 {
		t.Fatalf("%s: liveness period = %d, want 20", name, probeSet.Liveness.PeriodSeconds)
	}
	if probeSet.Liveness.InitialDelaySeconds != 15 {
		t.Fatalf("%s: liveness initialDelay = %d, want 15", name, probeSet.Liveness.InitialDelaySeconds)
	}

	if probeSet.Readiness == nil {
		t.Fatalf("%s: expected readiness probe", name)
	}
	if probeSet.Readiness.Exec == nil {
		t.Fatalf("%s: expected exec readiness probe", name)
	}
	if probeSet.Readiness.PeriodSeconds != 5 {
		t.Fatalf("%s: readiness period = %d, want 5", name, probeSet.Readiness.PeriodSeconds)
	}
	if probeSet.Readiness.TimeoutSeconds != 5 {
		t.Fatalf("%s: readiness timeout = %d, want 5", name, probeSet.Readiness.TimeoutSeconds)
	}

	if probeSet.Startup == nil {
		t.Fatalf("%s: expected startup probe in created probe set", name)
	}
	if probeSet.Startup.Exec == nil {
		t.Fatalf("%s: expected exec startup probe", name)
	}
	if probeSet.Startup.PeriodSeconds != 10 {
		t.Fatalf("%s: startup period = %d, want 10", name, probeSet.Startup.PeriodSeconds)
	}
	if probeSet.Startup.InitialDelaySeconds != 20 {
		t.Fatalf("%s: startup initialDelay = %d, want 20", name, probeSet.Startup.InitialDelaySeconds)
	}
	if probeSet.Startup.FailureThreshold != 12 {
		t.Fatalf("%s: startup failureThreshold = %d, want 12", name, probeSet.Startup.FailureThreshold)
	}
}

func TestCreateProbeSetConductor(t *testing.T) {
	rpcProbeSetChecks(t, "conductor", []string{"/usr/bin/pgrep", "-r", "DRST", "nova-conductor"})
}

func TestCreateProbeSetScheduler(t *testing.T) {
	rpcProbeSetChecks(t, "scheduler", []string{"/usr/bin/pgrep", "-r", "DRST", "nova-scheduler"})
}

func TestCreateProbeSetCompute(t *testing.T) {
	rpcProbeSetChecks(t, "compute", []string{"/usr/bin/pgrep", "-r", "DRST", "nova-compute"})
}
