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

// Package common provides shared helpers for nova, placement, and cyborg controllers.
package common //nolint:revive // common is the established package name for multi-group shared code

import "fmt"

const (
	// ServiceCommand is the command used to start a service binary in Kolla containers.
	ServiceCommand = "/usr/local/bin/kolla_start"
)

// GetScriptSecretName returns the name of the Secret used for db sync scripts.
func GetScriptSecretName(crName string) string {
	return fmt.Sprintf("%s-scripts", crName)
}

// GetServiceConfigSecretName returns the name of the Secret used to store service configuration.
func GetServiceConfigSecretName(crName string) string {
	return fmt.Sprintf("%s-config-data", crName)
}
