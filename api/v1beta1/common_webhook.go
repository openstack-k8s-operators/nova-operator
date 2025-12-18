/*
Copyright 2024.

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

package v1beta1

import (
	"fmt"
	"path/filepath"
	"strings"

	common_webhook "github.com/openstack-k8s-operators/lib-common/modules/common/webhook"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// ValidateDefaultConfigOverwrite checks if the file names in the overwrite map
// are allowed and return an error for each unsupported files. The allowedKeys
// list supports direct string match and globs like provider*.yaml
func ValidateDefaultConfigOverwrite(
	basePath *field.Path,
	defaultConfigOverwrite map[string]string,
	allowedKeys []string,
) field.ErrorList {
	var errors field.ErrorList
	for requested := range defaultConfigOverwrite {
		if !matchAny(requested, allowedKeys) {
			errors = append(
				errors,
				field.Invalid(
					basePath,
					requested,
					fmt.Sprintf(
						"Only the following keys are valid: %s",
						strings.Join(allowedKeys, ", ")),
				),
			)
		}
	}
	return errors
}

func matchAny(requested string, allowed []string) bool {
	for _, a := range allowed {
		if matched, _ := filepath.Match(a, requested); matched {
			return true
		}
	}
	return false
}

// getDeprecatedFields returns the centralized list of deprecated fields for NovaSpecCore
func (spec *NovaSpecCore) getDeprecatedFields(old *NovaSpecCore) []common_webhook.DeprecatedFieldUpdate {
	// Get new field value (handle nil NotificationsBus)
	var newNotifBusCluster *string
	if spec.NotificationsBus != nil {
		newNotifBusCluster = &spec.NotificationsBus.Cluster
	}

	deprecatedFields := []common_webhook.DeprecatedFieldUpdate{
		{
			DeprecatedFieldName: "apiMessageBusInstance",
			NewFieldPath:        []string{"messagingBus", "cluster"},
			NewDeprecatedValue:  &spec.APIMessageBusInstance,
			NewValue:            &spec.MessagingBus.Cluster,
		},
		{
			DeprecatedFieldName: "notificationsBusInstance",
			NewFieldPath:        []string{"notificationsBus", "cluster"},
			NewDeprecatedValue:  spec.NotificationsBusInstance,
			NewValue:            newNotifBusCluster,
		},
	}

	// If old spec is provided (UPDATE operation), add old values
	if old != nil {
		deprecatedFields[0].OldDeprecatedValue = &old.APIMessageBusInstance
		deprecatedFields[1].OldDeprecatedValue = old.NotificationsBusInstance
	}

	return deprecatedFields
}

// validateDeprecatedFieldsCreate validates deprecated fields during CREATE operations
func (spec *NovaSpecCore) validateDeprecatedFieldsCreate(basePath *field.Path) ([]string, field.ErrorList) {
	// Get deprecated fields list (without old values for CREATE)
	deprecatedFieldsUpdate := spec.getDeprecatedFields(nil)

	// Convert to DeprecatedField list for CREATE validation
	deprecatedFields := make([]common_webhook.DeprecatedField, len(deprecatedFieldsUpdate))
	for i, df := range deprecatedFieldsUpdate {
		deprecatedFields[i] = common_webhook.DeprecatedField{
			DeprecatedFieldName: df.DeprecatedFieldName,
			NewFieldPath:        df.NewFieldPath,
			DeprecatedValue:     df.NewDeprecatedValue,
			NewValue:            df.NewValue,
		}
	}

	// Validate top-level NovaSpecCore fields
	warnings := common_webhook.ValidateDeprecatedFieldsCreate(deprecatedFields, basePath)

	// Validate deprecated fields in cell templates
	for cellName, cellTemplate := range spec.CellTemplates {
		cellPath := basePath.Child("cellTemplates").Key(cellName)
		cellWarnings := cellTemplate.validateDeprecatedFieldsCreate(cellPath)
		warnings = append(warnings, cellWarnings...)
	}

	return warnings, nil
}

// validateDeprecatedFieldsUpdate validates deprecated fields during UPDATE operations
func (spec *NovaSpecCore) validateDeprecatedFieldsUpdate(old NovaSpecCore, basePath *field.Path) ([]string, field.ErrorList) {
	// Get deprecated fields list with old values
	deprecatedFields := spec.getDeprecatedFields(&old)
	warnings, errors := common_webhook.ValidateDeprecatedFieldsUpdate(deprecatedFields, basePath)

	// Validate deprecated fields in cell templates
	for cellName, cellTemplate := range spec.CellTemplates {
		if oldCell, exists := old.CellTemplates[cellName]; exists {
			cellPath := basePath.Child("cellTemplates").Key(cellName)
			cellWarnings, cellErrors := cellTemplate.validateDeprecatedFieldsUpdate(oldCell, cellPath)
			warnings = append(warnings, cellWarnings...)
			errors = append(errors, cellErrors...)
		}
	}

	return warnings, errors
}

// getDeprecatedFields returns the centralized list of deprecated fields for NovaCellTemplate
func (spec *NovaCellTemplate) getDeprecatedFields(old *NovaCellTemplate) []common_webhook.DeprecatedFieldUpdate {
	deprecatedFields := []common_webhook.DeprecatedFieldUpdate{
		{
			DeprecatedFieldName: "cellMessageBusInstance",
			NewFieldPath:        []string{"messagingBus", "cluster"},
			NewDeprecatedValue:  &spec.CellMessageBusInstance,
			NewValue:            &spec.MessagingBus.Cluster,
		},
	}

	// If old spec is provided (UPDATE operation), add old values
	if old != nil {
		deprecatedFields[0].OldDeprecatedValue = &old.CellMessageBusInstance
	}

	return deprecatedFields
}

// validateDeprecatedFieldsCreate validates deprecated fields during CREATE operations for NovaCellTemplate
func (spec *NovaCellTemplate) validateDeprecatedFieldsCreate(basePath *field.Path) []string {
	// Get deprecated fields list (without old values for CREATE)
	deprecatedFieldsUpdate := spec.getDeprecatedFields(nil)

	// Convert to DeprecatedField list for CREATE validation
	deprecatedFields := make([]common_webhook.DeprecatedField, len(deprecatedFieldsUpdate))
	for i, df := range deprecatedFieldsUpdate {
		deprecatedFields[i] = common_webhook.DeprecatedField{
			DeprecatedFieldName: df.DeprecatedFieldName,
			NewFieldPath:        df.NewFieldPath,
			DeprecatedValue:     df.NewDeprecatedValue,
			NewValue:            df.NewValue,
		}
	}

	return common_webhook.ValidateDeprecatedFieldsCreate(deprecatedFields, basePath)
}

// validateDeprecatedFieldsUpdate validates deprecated fields during UPDATE operations for NovaCellTemplate
func (spec *NovaCellTemplate) validateDeprecatedFieldsUpdate(old NovaCellTemplate, basePath *field.Path) ([]string, field.ErrorList) {
	// Get deprecated fields list with old values
	deprecatedFields := spec.getDeprecatedFields(&old)
	return common_webhook.ValidateDeprecatedFieldsUpdate(deprecatedFields, basePath)
}
