package virtlogd

import (
	novav1 "github.com/openstack-k8s-operators/nova-operator/pkg/apis/nova/v1"
	util "github.com/openstack-k8s-operators/nova-operator/pkg/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TemplatesConfigMap - custom virtlogd config map
func TemplatesConfigMap(cr *novav1.Virtlogd, cmName string) *corev1.ConfigMap {

	cm := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: cr.Namespace,
		},
		Data: map[string]string{
			"virtlogd.conf": util.ExecuteTemplateFile(cr.Name+"/config/virtlogd.conf", nil),
			"config.json":   util.ExecuteTemplateFile(cr.Name+"/kolla_config.json", nil),
		},
	}

	return cm
}
