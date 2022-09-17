package common

import (
	"context"
	"fmt"

	"github.com/openstack-k8s-operators/lib-common/modules/common/configmap"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"github.com/openstack-k8s-operators/lib-common/modules/common/labels"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *ReconcilerBase) GenerateServiceConfigMaps(
	ctx context.Context, h *helper.Helper,
	instance client.Object, envVars *map[string]env.Setter,
	templateParameters *map[string]interface{},
	extraData *map[string]string,
) error {
	r.Log.Info("Reconciling Service")
	cmLabels := labels.GetLabels(instance, labels.GetGroupLabel("nova"), map[string]string{})
	additionalTemplates := map[string]string{
		"01-nova.conf":        "/nova.conf",
		"02-nova-secret.conf": "/nova-secret.conf",
	}
	cms := []util.Template{
		// ConfigMap
		{
			Name:               fmt.Sprintf("%s-config-data", instance.GetName()),
			Namespace:          instance.GetNamespace(),
			Type:               util.TemplateTypeConfig,
			InstanceType:       instance.GetObjectKind().GroupVersionKind().Kind,
			ConfigOptions:      *templateParameters,
			Labels:             cmLabels,
			CustomData:         *extraData,
			Annotations:        map[string]string{},
			AdditionalTemplate: additionalTemplates,
		},
	}
	err := configmap.EnsureConfigMaps(ctx, h, instance, cms, envVars)
	if err != nil {
		return nil
	}

	return nil
}
