package placement

func GetLabels(name string) map[string]string {
	return map[string]string{"owner": "placement-operator", "cr": name, "app": AppLabel}
}
