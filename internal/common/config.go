/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package common //nolint:revive // common is the established package name for multi-group shared code

import (
	"context"

	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	helper "github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"github.com/openstack-k8s-operators/lib-common/modules/common/secret"
	util "github.com/openstack-k8s-operators/lib-common/modules/common/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GenerateConfigs helper function to generate config maps
func GenerateConfigs(
	ctx context.Context, h *helper.Helper,
	instance client.Object, configName string, envVars *map[string]env.Setter,
	templateParameters map[string]any,
	extraData map[string]string, cmLabels map[string]string,
	additionalTemplates map[string]string,
	commonTemplates []string,
	templateDir string,
) error {
	return generateConfigsGeneric(ctx, h, instance, configName, envVars, templateParameters, extraData,
		cmLabels, additionalTemplates, commonTemplates, templateDir, false)
}

// GenerateConfigsWithScripts helper function to generate config maps
// for service configs and scripts
func GenerateConfigsWithScripts(
	ctx context.Context, h *helper.Helper,
	instance client.Object, envVars *map[string]env.Setter,
	templateParameters map[string]any,
	extraData map[string]string, cmLabels map[string]string,
	additionalTemplates map[string]string,
	commonTemplates []string,
	templateDir string,
) error {
	return generateConfigsGeneric(ctx, h, instance, GetServiceConfigSecretName(instance.GetName()),
		envVars, templateParameters, extraData,
		cmLabels, additionalTemplates, commonTemplates, templateDir, true)
}

// generateConfigsGeneric helper function to generate config maps
func generateConfigsGeneric(
	ctx context.Context, h *helper.Helper,
	instance client.Object, configName string, envVars *map[string]env.Setter,
	templateParameters map[string]any,
	extraData map[string]string, cmLabels map[string]string,
	additionalTemplates map[string]string,
	commonTemplates []string,
	templateDir string,
	withScripts bool,
) error {
	if templateDir == "" {
		return ErrTemplateDirUnset
	}

	cms := []util.Template{
		{
			Name:               configName,
			Namespace:          instance.GetNamespace(),
			Type:               util.TemplateTypeConfig,
			InstanceType:       instance.GetObjectKind().GroupVersionKind().Kind,
			MultiTemplateDir:   templateDir,
			ConfigOptions:      templateParameters,
			Labels:             cmLabels,
			CustomData:         extraData,
			Annotations:        map[string]string{},
			AdditionalTemplate: additionalTemplates,
			CommonTemplates:    commonTemplates,
		},
	}
	if withScripts {
		cms = append(cms, util.Template{
			Name:               GetScriptSecretName(instance.GetName()),
			Namespace:          instance.GetNamespace(),
			Type:               util.TemplateTypeScripts,
			InstanceType:       instance.GetObjectKind().GroupVersionKind().Kind,
			MultiTemplateDir:   templateDir,
			AdditionalTemplate: map[string]string{},
			Annotations:        map[string]string{},
			Labels:             cmLabels,
		})
	}
	return secret.EnsureSecrets(ctx, h, instance, cms, envVars)
}
