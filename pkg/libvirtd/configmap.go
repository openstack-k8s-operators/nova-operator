package libvirtd

import (
	util "github.com/openstack-k8s-operators/lib-common/pkg/util"
	novav1 "github.com/openstack-k8s-operators/nova-operator/pkg/apis/nova/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ScriptsConfigMap - scripts config map
func ScriptsConfigMap(cr *novav1.Libvirtd, cmName string) *corev1.ConfigMap {

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
			"libvirtd.sh": util.ExecuteTemplateFile(cr.Name+"/bin/libvirtd.sh", nil),
		},
	}

	return cm
}

// TemplatesConfigMap - custom nova config map
func TemplatesConfigMap(cr *novav1.Libvirtd, cmName string) *corev1.ConfigMap {

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
			"libvirtd.conf": util.ExecuteTemplateFile(cr.Name+"/config/libvirtd.conf", nil),
			"config.json":   util.ExecuteTemplateFile(cr.Name+"/kolla_config.json", nil),
		},
	}

	return cm
}
