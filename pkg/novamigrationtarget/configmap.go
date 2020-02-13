package novamigrationtarget

import (
        "strconv"

        util "github.com/openstack-k8s-operators/nova-operator/pkg/util"
        novav1 "github.com/openstack-k8s-operators/nova-operator/pkg/apis/nova/v1"
        corev1 "k8s.io/api/core/v1"
        metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type novaMigrationTargetConfigOptions struct {
        SshdPort          string
}

// custom nova config map
func ConfigMap(cr *novav1.NovaMigrationTarget, cmName string) *corev1.ConfigMap {
        //var sshdPort string
        sshdPort := strconv.FormatUint(uint64(cr.Spec.SshdPort), 10)
        opts := novaMigrationTargetConfigOptions{ sshdPort }

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
                        "migration_ssh_config":        util.ExecuteTemplateFile("migration_ssh_config", &opts),
                        "migration_sshd_config":       util.ExecuteTemplateFile("migration_sshd_config", nil),
                        "migration_authorized_keys":   util.ExecuteTemplateFile("migration_authorized_keys", nil),
                },
        }

        return cm
}
