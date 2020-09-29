package common

// GetLabels - get labels to be set on objects created by controller
func GetLabels(name string, appLabel string) map[string]string {
	return map[string]string{
		"owner": "nova-operator",
		"cr":    name,
		"app":   appLabel,
	}
}
