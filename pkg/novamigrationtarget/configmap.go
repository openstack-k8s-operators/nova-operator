package novamigrationtarget

import (
	"strconv"

	novav1 "github.com/openstack-k8s-operators/nova-operator/pkg/apis/nova/v1"
	util "github.com/openstack-k8s-operators/nova-operator/pkg/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SshdPort
type novaMigrationTargetConfigOptions struct {
	SshdPort string
}

// ScriptsConfigMap - scripts config map
func ScriptsConfigMap(cr *novav1.NovaMigrationTarget, cmName string) *corev1.ConfigMap {

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
			"common.sh": util.ExecuteTemplateFile("common/common.sh", nil),
			"init.sh":   util.ExecuteTemplateFile(cr.Name+"/bin/init.sh", nil),
		},
	}

	return cm
}

// TemplatesConfigMap - custom nova config map
func TemplatesConfigMap(cr *novav1.NovaMigrationTarget, cmName string) *corev1.ConfigMap {
	//var sshdPort string
	sshdPort := strconv.FormatUint(uint64(cr.Spec.SshdPort), 10)
	opts := novaMigrationTargetConfigOptions{sshdPort}

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
			"config.json": util.ExecuteTemplateFile(cr.Name+"/kolla_config.json", &opts),
			"ssh_config":  util.ExecuteTemplateFile(cr.Name+"/config/ssh_config", &opts),
			"sshd_config": util.ExecuteTemplateFile(cr.Name+"/config/sshd_config", nil),
		},
	}

	return cm
}
