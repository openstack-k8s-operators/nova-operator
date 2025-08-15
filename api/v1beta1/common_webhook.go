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

// ValidateDefaultConfigOverwriteKeyValuePairs checks if the file names in the overwrite pairs
// are allowed and return an error for each unsupported files. The allowedKeys
// list supports direct string match and globs like provider*.yaml
func ValidateDefaultConfigOverwriteKeyValuePairs(
	basePath *field.Path,
	defaultConfigOverwrite []KeyValuePair,
	allowedKeys []string,
) field.ErrorList {
	var errors field.ErrorList
	for _, pair := range defaultConfigOverwrite {
		if !matchAny(pair.Key, allowedKeys) {
			errors = append(
				errors,
				field.Invalid(
					basePath,
					pair.Key,
					fmt.Sprintf(
						"Only the following keys are valid: %s",
						strings.Join(allowedKeys, ", ")),
				),
			)
		}
	}
	return errors
}

// ValidateAPIDefaultConfigOverwrite validates API-specific default config overwrite
func ValidateAPIDefaultConfigOverwrite(
	defaultConfigOverwrite []KeyValuePair,
) field.ErrorList {
	allowedKeys := []string{
		"api-paste.ini",
		"policy.yaml",
		"policy.json",
	}
	return ValidateDefaultConfigOverwriteKeyValuePairs(
		field.NewPath("spec").Child("defaultConfigOverwrite"),
		defaultConfigOverwrite,
		allowedKeys,
	)
}

// ValidateComputeDefaultConfigOverwrite validates Compute-specific default config overwrite
func ValidateComputeDefaultConfigOverwrite(
	defaultConfigOverwrite []KeyValuePair,
) field.ErrorList {
	allowedKeys := []string{
		"nova.conf",
		"provider*.yaml",
		"policy.yaml",
		"policy.json",
	}
	return ValidateDefaultConfigOverwriteKeyValuePairs(
		field.NewPath("spec").Child("defaultConfigOverwrite"),
		defaultConfigOverwrite,
		allowedKeys,
	)
}

// ValidateMetadataDefaultConfigOverwrite validates Metadata-specific default config overwrite
func ValidateMetadataDefaultConfigOverwrite(
	defaultConfigOverwrite []KeyValuePair,
) field.ErrorList {
	allowedKeys := []string{
		"api-paste.ini",
		"policy.yaml",
		"policy.json",
	}
	return ValidateDefaultConfigOverwriteKeyValuePairs(
		field.NewPath("spec").Child("defaultConfigOverwrite"),
		defaultConfigOverwrite,
		allowedKeys,
	)
}

func matchAny(requested string, allowed []string) bool {
	for _, a := range allowed {
		if matched, _ := filepath.Match(a, requested); matched {
			return true
		}
	}
	return false
}
