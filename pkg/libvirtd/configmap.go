package libvirtd

import (
        util "github.com/openstack-k8s-operators/nova-operator/pkg/util"
        novav1 "github.com/openstack-k8s-operators/nova-operator/pkg/apis/nova/v1"
        corev1 "k8s.io/api/core/v1"
        metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// custom libvirt config map
func ConfigMap(cr *novav1.Libvirtd, cmName string) *corev1.ConfigMap {

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
                        "libvirtd.conf":               util.ExecuteTemplateFile("libvirtd.conf", nil),
                        "libvirtd.sh":                 util.ExecuteTemplateFile("libvirtd.sh", nil),
                        "migration_ssh_identity":      util.ExecuteTemplateFile("migration_ssh_identity", nil),
                },
        }

        return cm
}
