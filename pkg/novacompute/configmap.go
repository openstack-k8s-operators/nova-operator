package novacompute

import (
        util "github.com/openstack-k8s-operators/nova-operator/pkg/util"
        novav1 "github.com/openstack-k8s-operators/nova-operator/pkg/apis/nova/v1"
        corev1 "k8s.io/api/core/v1"
        metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type novaComputeConfigOptions struct {
        PublicVip                  string
        InternalApiVip             string
        MemcacheServers            string
        CinderPassword             string
        NovaPassword               string
        NeutronPassword            string
        PlacementPassword          string
        RabbitTransportUrl         string
        NovaComputeCpuDedicatedSet string
        NovaComputeCpuSharedSet    string
}

// custom nova config map
func ConfigMap(cr *novav1.NovaCompute, cmName string) *corev1.ConfigMap {
        opts := novaComputeConfigOptions{cr.Spec.PublicVip,
                                         cr.Spec.InternalApiVip,
                                         cr.Spec.MemcacheServers,
                                         cr.Spec.CinderPassword,
                                         cr.Spec.NovaPassword,
                                         cr.Spec.NeutronPassword,
                                         cr.Spec.PlacementPassword,
                                         cr.Spec.RabbitTransportUrl,
                                         cr.Spec.NovaComputeCpuDedicatedSet,
                                         cr.Spec.NovaComputeCpuSharedSet}

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
                        "nova.conf":                   util.ExecuteTemplateFile("nova.conf", &opts),
                        "tripleo.cnf":                 util.ExecuteTemplateFile("tripleo.cnf", nil),
                        "logging.conf":                util.ExecuteTemplateFile("logging.conf", nil),
                },
        }

        return cm
}
