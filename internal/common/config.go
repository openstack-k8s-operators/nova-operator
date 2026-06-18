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
	"maps"

	env "github.com/openstack-k8s-operators/lib-common/modules/common/env"
	helper "github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"github.com/openstack-k8s-operators/lib-common/modules/common/secret"
	util "github.com/openstack-k8s-operators/lib-common/modules/common/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ConfigGeneratorOptions configures template-based service config generation.
type ConfigGeneratorOptions struct {
	ConfigName          string
	TemplateDir         string
	BaseExtraTemplates  map[string]string
	AdditionalTemplates map[string]string
	CommonTemplates     []string
	TemplateParameters  map[string]any
	ExtraData           map[string]string
	Labels              map[string]string
	WithScripts         bool
}

// GenerateConfigs renders service configuration secrets from templates.
func (r *ReconcilerBase) GenerateConfigs(
	ctx context.Context,
	h *helper.Helper,
	instance client.Object,
	envVars *map[string]env.Setter,
	opts ConfigGeneratorOptions,
) error {
	if opts.TemplateDir == "" {
		return ErrTemplateDirUnset
	}

	extraTemplates := map[string]string{}
	maps.Copy(extraTemplates, opts.BaseExtraTemplates)
	maps.Copy(extraTemplates, opts.AdditionalTemplates)

	cms := []util.Template{
		{
			Name:               opts.ConfigName,
			Namespace:          instance.GetNamespace(),
			Type:               util.TemplateTypeConfig,
			InstanceType:       instance.GetObjectKind().GroupVersionKind().Kind,
			MultiTemplateDir:   opts.TemplateDir,
			ConfigOptions:      opts.TemplateParameters,
			Labels:             opts.Labels,
			CustomData:         opts.ExtraData,
			Annotations:        map[string]string{},
			AdditionalTemplate: extraTemplates,
			CommonTemplates:    opts.CommonTemplates,
		},
	}
	if opts.WithScripts {
		cms = append(cms, util.Template{
			Name:               GetScriptSecretName(instance.GetName()),
			Namespace:          instance.GetNamespace(),
			Type:               util.TemplateTypeScripts,
			InstanceType:       instance.GetObjectKind().GroupVersionKind().Kind,
			MultiTemplateDir:   opts.TemplateDir,
			AdditionalTemplate: map[string]string{},
			Annotations:        map[string]string{},
			Labels:             opts.Labels,
		})
	}
	return secret.EnsureSecrets(ctx, h, instance, cms, envVars)
}
