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
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

const (
	// preStopStartupPollAttempts caps how many times (once a second) the preStop
	// hook polls for the service process to come up before giving up.
	preStopStartupPollAttempts = 10
)

// GetStartupAwarePreStop returns a PreStop hook that delays SIGTERM until the
// service process is up, so a pod killed early in startup shuts down gracefully
// instead of waiting out TerminationGracePeriodSeconds and being SIGKILLed. It
// polls "pgrep -r DRST processName" (the setproctitle name, as the liveness
// probe does) once a second and, once the process appears, waits five seconds
// for it to finish loading before returning. It writes progress messages to
// the container's stdout so they are visible in pod logs.
func GetStartupAwarePreStop(processName string) *corev1.Lifecycle {
	return &corev1.Lifecycle{
		PreStop: &corev1.LifecycleHandler{
			Exec: &corev1.ExecAction{
				Command: []string{
					"/bin/sh", "-c",
					fmt.Sprintf(
						`log() { echo "preStop: $(date +%%Y-%%m-%%dT%%H:%%M:%%S%%z): $1" > /proc/1/fd/1; }; i=0; while [ $i -lt %[2]d ]; do /usr/bin/pgrep -r DRST %[1]s >/dev/null 2>&1 && { sleep 5; log "found %[1]s; waited 5s before returning to kubelet"; exit 0; }; i=$((i+1)); sleep 1; done; log "timed out waiting for %[1]s; returning to kubelet"`,
						processName, preStopStartupPollAttempts),
				},
			},
		},
	}
}
