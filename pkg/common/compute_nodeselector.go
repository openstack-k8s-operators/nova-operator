package common

// GetComputeWorkerNodeSelector - returns the NodeSelector for all compute worker DS
func GetComputeWorkerNodeSelector(roleName string) map[string]string {

	// Change nodeSelector
	nodeSelector := "node-role.kubernetes.io/" + roleName
	return map[string]string{nodeSelector: ""}
}
