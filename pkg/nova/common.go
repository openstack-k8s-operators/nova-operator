/*
Copyright 2022.

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

package nova

import "fmt"

const (
	// ServiceAccount - the name of the account defined in
	// config/rbac/service_account.yaml providing access rights to all the nova
	// controllers
	ServiceAccount = "nova-operator-nova"
	// KollaServiceCommand - the command to start the service binary in the kolla container
	KollaServiceCommand = "/usr/local/bin/kolla_set_configs && /usr/local/bin/kolla_start"
)

// GetScriptConfigMapName returns the name of the ConfigMap used for the
// config merger and the service init scripts
func GetScriptConfigMapName(crName string) string {
	return fmt.Sprintf("%s-scripts", crName)
}

// GetServiceConfigConfigMapName returns the name of the ConfigMap used to
// store the service configuration files
func GetServiceConfigConfigMapName(crName string) string {
	return fmt.Sprintf("%s-config-data", crName)
}
