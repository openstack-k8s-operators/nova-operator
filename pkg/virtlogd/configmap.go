package virtlogd

import (
        util "github.com/nova-operator/pkg/util"
        novav1 "github.com/nova-operator/pkg/apis/nova/v1"
        corev1 "k8s.io/api/core/v1"
        metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// custom virtlogd config map
func ConfigMap(cr *novav1.Virtlogd, cmName string) *corev1.ConfigMap {

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
                        "virtlogd.conf":   util.ExecuteTemplateFile("virtlogd.conf", nil),
                },
        }

        return cm
}
