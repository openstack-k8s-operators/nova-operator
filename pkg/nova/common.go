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

import (
	"fmt"

	"github.com/openstack-k8s-operators/lib-common/modules/database"
)

const (
	// KollaServiceCommand - the command to start the service binary in the kolla container
	KollaServiceCommand = "/usr/local/bin/kolla_set_configs && /usr/local/bin/kolla_start"
	// NovaAPIDatabaseName - the name of the DB to store tha API schema
	NovaAPIDatabaseName = "nova_api"
	// NovaCell0DatabaseName - the name of the DB to store the cell schema for
	// cell0
	NovaCell0DatabaseName = "nova_cell0"
)

// GetScriptSecretName returns the name of the Secret used for the
// db sync scripts
func GetScriptSecretName(crName string) string {
	return fmt.Sprintf("%s-scripts", crName)
}

// GetServiceConfigSecretName returns the name of the Secret used to
// store the service configuration files
func GetServiceConfigSecretName(crName string) string {
	return fmt.Sprintf("%s-config-data", crName)
}

// DatabaseStatus -
type DatabaseStatus int

const (
	// DBFailed -
	DBFailed DatabaseStatus = iota
	// DBCreating -
	DBCreating DatabaseStatus = iota
	// DBCompleted -
	DBCompleted DatabaseStatus = iota
)

// MessageBusStatus -
type MessageBusStatus int

const (
	// MQFailed -
	MQFailed MessageBusStatus = iota
	// MQCreating -
	MQCreating MessageBusStatus = iota
	// MQCompleted -
	MQCompleted MessageBusStatus = iota
)

type CellDeploymentStatus int
type NovaComputeStatus int

// CellDeploymentStatus -
const (
	// CellDeploying indicates that NovaCell is created and waiting to reach
	// Ready status
	CellDeploying CellDeploymentStatus = iota
	// CellMapping indicates that NovaCell reached the Ready status and it is
	// being mapped to the Nova API database
	CellMapping CellDeploymentStatus = iota
	// CellMappingFailed indicates that NovaCell reached the Ready status but
	// mapping it to the Nova API database failed
	CellMappingFailed CellDeploymentStatus = iota
	// CellMappingReady indicates that NovaCell reached the Ready status and
	// it is mapped to the Nova API database
	CellMappingReady CellDeploymentStatus = iota
	// CellReady indicates that the NovaCell is Ready and it is mapped to
	// Nova API database so it is accessible.
	CellReady CellDeploymentStatus = iota
	// CellFailed indicates that the NovaCell deployment failed.
	CellFailed CellDeploymentStatus = iota
)

// NovaComputeStatus -
const (
	// ComputeDiscovering indicates that NovaCompute reached the Ready status and it is
	// being discover
	ComputeDiscovering NovaComputeStatus = iota
	// ComputeDiscoverFailed indicates that NovaCompute reached the Ready status but
	// discover it failed
	ComputeDiscoverFailed NovaComputeStatus = iota
	// ComputeDiscoverReady indicates that NovaCompute reached the Ready status and
	// it is discovered
	ComputeDiscoverReady NovaComputeStatus = iota
)

// Database -
type Database struct {
	Database *database.Database
	Status   DatabaseStatus
}

// MessageBus -
type MessageBus struct {
	TransportURL string
	Status       MessageBusStatus
}
