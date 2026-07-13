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

package common

import (
	"math"

	"github.com/openstack-k8s-operators/lib-common/modules/common/probes"
)

const (
	// DefaultServiceDownTime is the upstream OpenStack default for service_down_time,
	// the maximum time since last check-in for a service to be considered up.
	// RPC worker probe defaults are derived from this value.
	// For nova: https://opendev.org/openstack/nova/src/branch/master/nova/conf/service.py
	DefaultServiceDownTime = 60
)

// GetDefaultProbesAPI returns probe defaults for nova-api, nova-metadata, and
// placement-api. Readiness is tuned to detect overload within the APITimeout
// window; liveness is more forgiving to avoid restarts during transient load.
// Startup defaults are included for lib-common CreateProbeSet compatibility;
// placement-api deployment wires liveness and readiness only.
func GetDefaultProbesAPI(apiTimeout int) probes.OverrideSpec {
	t := float64(apiTimeout)
	readinessPeriod := int32(math.Floor(0.3 * t))
	livenessPeriod := int32(math.Floor(0.5 * t))

	return probes.OverrideSpec{
		LivenessProbes: &probes.ProbeConf{
			TimeoutSeconds:   livenessPeriod,
			PeriodSeconds:    livenessPeriod,
			FailureThreshold: 10,
		},
		ReadinessProbes: &probes.ProbeConf{
			TimeoutSeconds:   readinessPeriod,
			PeriodSeconds:    readinessPeriod,
			FailureThreshold: 3,
		},
		StartupProbes: &probes.ProbeConf{
			PeriodSeconds:    10,
			FailureThreshold: 6,
		},
	}
}

// GetDefaultProbesNoVNC returns fixed-timing HTTP probe defaults for nova-novncproxy.
func GetDefaultProbesNoVNC() probes.OverrideSpec {
	return probes.OverrideSpec{
		LivenessProbes: &probes.ProbeConf{
			Path:           "/vnc_lite.html",
			TimeoutSeconds: 10,
			PeriodSeconds:  10,
		},
		ReadinessProbes: &probes.ProbeConf{
			Path:           "/vnc_lite.html",
			TimeoutSeconds: 5,
			PeriodSeconds:  5,
		},
		StartupProbes: &probes.ProbeConf{
			Path:             "/vnc_lite.html",
			PeriodSeconds:    10,
			FailureThreshold: 6,
		},
	}
}

// GetDefaultProbesRPC returns exec probe defaults for nova RPC worker services
// (conductor, scheduler, compute). Pods wire liveness and startup only.
// Readiness defaults are included for lib-common CreateProbeSetV2 compatibility.
// A dedicated serviceDownTime field is not currently exposed in the operator's
// API; we rely on the upstream nova default to compute probe timings.
// https://opendev.org/openstack/nova/src/branch/master/nova/conf/service.py
func GetDefaultProbesRPC(serviceDownTime int, command []string) probes.OverrideSpec {
	const failureCount = 3
	period := int32(math.Floor(float64(serviceDownTime) / float64(failureCount)))
	startupPeriod := int32(math.Max(5, float64(period)/2))

	return probes.OverrideSpec{
		LivenessProbes: &probes.ProbeConf{
			Type:                probes.ProbeHandlerExec,
			Command:             command,
			TimeoutSeconds:      10,
			PeriodSeconds:       period,
			InitialDelaySeconds: 15,
		},
		ReadinessProbes: &probes.ProbeConf{
			Type:           probes.ProbeHandlerExec,
			Command:        command,
			TimeoutSeconds: 5,
			PeriodSeconds:  5,
		},
		StartupProbes: &probes.ProbeConf{
			Type:                probes.ProbeHandlerExec,
			Command:             command,
			TimeoutSeconds:      10,
			PeriodSeconds:       startupPeriod,
			InitialDelaySeconds: period,
			FailureThreshold:    12,
		},
	}
}
