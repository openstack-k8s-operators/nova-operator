package novacompute

import (
	"strings"

	util "github.com/openstack-k8s-operators/lib-common/pkg/util"
	novav1 "github.com/openstack-k8s-operators/nova-operator/pkg/apis/nova/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type novaComputeConfigOptions struct {
	KeystoneAPI                string
	GlanceAPI                  string
	MemcacheServers            string
	CinderPassword             string
	NovaPassword               string
	NeutronPassword            string
	PlacementPassword          string
	RabbitTransportURL         string
	NovaComputeCPUDedicatedSet string
	NovaComputeCPUSharedSet    string
}

// ScriptsConfigMap - scripts config map
func ScriptsConfigMap(cr *novav1.NovaCompute, cmName string) *corev1.ConfigMap {

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
			"init.sh":   util.ExecuteTemplateFile(strings.ToLower(cr.Kind)+"/bin/init.sh", nil),
		},
	}

	return cm
}

// TemplatesConfigMap - mandatory settings config map
func TemplatesConfigMap(cr *novav1.NovaCompute, commonConfigMap *corev1.ConfigMap, ospSecrets *corev1.Secret, cmName string) *corev1.ConfigMap {
	opts := novaComputeConfigOptions{
		commonConfigMap.Data["keystoneAPI"],
		commonConfigMap.Data["glanceAPI"],
		commonConfigMap.Data["memcacheServers"],
		string(ospSecrets.Data["CinderPassword"]),
		string(ospSecrets.Data["NovaPassword"]),
		string(ospSecrets.Data["NeutronPassword"]),
		string(ospSecrets.Data["PlacementPassword"]),
		string(ospSecrets.Data["RabbitTransportURL"]),
		cr.Spec.NovaComputeCPUDedicatedSet,
		cr.Spec.NovaComputeCPUSharedSet}

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
			"config.json": util.ExecuteTemplateFile(strings.ToLower(cr.Kind)+"/kolla_config.json", &opts),
			// mschuppert: TODO run over all files in /configs subdir to have it more generic
			"nova.conf":    util.ExecuteTemplateFile(strings.ToLower(cr.Kind)+"/config/nova.conf", &opts),
			"logging.conf": util.ExecuteTemplateFile(strings.ToLower(cr.Kind)+"/config/logging.conf", nil),
			"policy.json":  util.ExecuteTemplateFile(strings.ToLower(cr.Kind)+"/config/policy.json", nil),
		},
	}

	return cm
}
