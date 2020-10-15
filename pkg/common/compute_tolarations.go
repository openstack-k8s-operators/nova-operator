/*
Copyright 2020 Red Hat

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
	corev1 "k8s.io/api/core/v1"
)

// GetComputeWorkerTolerations - common Volumes used by many service pods
func GetComputeWorkerTolerations(roleName string) []corev1.Toleration {

	return []corev1.Toleration{
		// Add toleration
		{
			Operator: "Equal",
			Effect:   "NoSchedule",
			Key:      "dedicated",
			Value:    roleName,
		},
	}
}
