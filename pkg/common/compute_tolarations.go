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
