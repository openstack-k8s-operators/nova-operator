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

package v1beta1

import (
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
)

// Nova Condition Types used by API objects.
const (
	// NovaAPIReadyCondition indicates if the NovaAPI is operational
	NovaAPIReadyCondition condition.Type = "NovaAPIReady"
	// NovaConductorReadyCondition indicates if the NovaConductor is ready
	// in a given cell.
	NovaConductorReadyCondition condition.Type = "NovaConductorReady"
	// NovaAPIDBReadyCondition indicates if the nova_api DB is created
	NovaAPIDBReadyCondition condition.Type = "NovaAPIDBReady"
	// NovaAllCellsDBReadyCondition indicates that the DB for each configured
	// Cell is created successfully
	NovaAllCellsDBReadyCondition condition.Type = "NovaAllCellDBReady"
	// NovaAllCellsReadyCondition indicates that every defined Cell is ready
	NovaAllCellsReadyCondition condition.Type = "NovaAllCellReady"
	// NovaAPIMQReadyCondition indicated that the top level message bus is ready
	NovaAPIMQReadyCondition condition.Type = "NovaAPIMQReady"
	// NovaAllCellsMQReadyCondition indicates that the message bus for each
	// configured Cell is created successfully
	NovaAllCellsMQReadyCondition condition.Type = "NovaAllCellsMQReady"
	// NovaSchedulerReadyCondition indicates if the NovaScheduler is operational
	NovaSchedulerReadyCondition condition.Type = "NovaSchedulerReady"
	// NovaCellReadyCondition indicates when the given NovaCell instance is Ready
	NovaCellReadyCondition condition.Type = "NovaCellReady"
	// NovaMetadataReadyCondition indicates when the given NovaMetadata instance is Ready
	NovaMetadataReadyCondition condition.Type = "NovaMetadataReady"
	// NovaNoVNCProxyReadyCondition indicates when the given NoVNCProxy instance is Ready
	NovaNoVNCProxyReadyCondition condition.Type = "NovaNoVNCProxyReady"
	// NovaComputeServiceConfigReady indicates when the compute service config
	// is ready for the given NovaCell
	NovaComputeServiceConfigReady condition.Type = "NovaComputeServiceConfigReady"
	// NovaComputeReadyCondition indicates when the given NovaCompute instance is Ready
	NovaComputeReadyCondition condition.Type = "NovaComputeReady"
)

// Common Messages used by API objects.
const (
	// NovaAPIReadyInitMessage
	NovaAPIReadyInitMessage = "NovaAPI not started"

	// NovaAPIReadyErrorMessage
	NovaAPIReadyErrorMessage = "NovaAPI error occurred %s"

	// NovaConductorReadyInitMessage
	NovaConductorReadyInitMessage = "NovaConductor not started"

	// NovaConductorReadyErrorMessage
	NovaConductorReadyErrorMessage = "NovaConductor error occurred %s"

	// NovaAllCellsDBReadyInitMessage
	NovaAllCellsDBReadyInitMessage = "DB creation not started"

	// NovaAllCellsDBReadyCreatingMessage
	NovaAllCellsDBReadyCreatingMessage = "DB creation ongoing for %s"

	// NovaAllCellsDBReadyErrorMessage
	NovaAllCellsDBReadyErrorMessage = "DB creation failed for %s"

	// NovaAllCellsReadyMessage
	NovaAllCellsDBReadyMessage = "All DBs created successfully"

	// NovaAllCellsReadyInitMessage
	NovaAllCellsReadyInitMessage = "NovaCells are not started"

	// NovaAllCellsReadyCreatingMessage
	NovaAllCellsReadyNotReadyMessage = "NovaCell %s is not Ready"

	// NovaAllCellsReadyWaitingMessage
	NovaAllCellsReadyWaitingMessage = "NovaCell creation waits for DB creation for %s"

	// NovaAllCellsReadyErrorMessage
	NovaAllCellsReadyErrorMessage = "NovaCell creation failed for %s"

	// NovaAllCellsReadyMessage
	NovaAllCellsReadyMessage = "All NovaCells are ready"

	// NovaAPIMQReadyInitMessage
	NovaAPIMQReadyInitMessage = "API message bus not started"

	// NovaAPIMQReadyErrorMessage
	NovaAPIMQReadyErrorMessage = "API message bus creation failed: %s"

	// NovaAPIMQReadyMessage
	NovaAPIMQReadyMessage = "API message bus creation successfully"

	// NovaAPIMQReadyCreatingMessage
	NovaAPIMQReadyCreatingMessage = "API message bus creation ongoing"

	// NovaAllCellsMQReadyInitMessage
	NovaAllCellsMQReadyInitMessage = "Message bus creation not started"

	// NovaAllCellsMQReadyErrorMessage
	NovaAllCellsMQReadyCreatingMessage = "Message bus creation ongoing for %s"

	// NovaAllCellsMQReadyErrorMessage
	NovaAllCellsMQReadyErrorMessage = "Message bus creation failed for %s"

	// NovaAllCellsMQReadyMessage
	NovaAllCellsMQReadyMessage = "All message busses created successfully"

	// NovaSchedulerReadyInitMessage
	NovaSchedulerReadyInitMessage = "NovaScheduler not started"

	// NovaSchedulerReadyErrorMessage
	NovaSchedulerReadyErrorMessage = "NovaScheduler error occurred %s"

	// InputReadyWaitingMessage
	InputReadyWaitingMessage = "Input data resources missing: %s"

	// NovaCellReadyInitMessage
	NovaCellReadyInitMessage = "The status of NovaCell %s is unknown"

	// NovaCellReadyNotExistsMessage
	NovaCellReadyNotExistsMessage = "Waiting for NovaCell %s to exists"

	// NovaCellReadyNotReadyMessage
	NovaCellReadyNotReadyMessage = "Waiting for NovaCell %s to become Ready"

	//NovaCellReadyErrorMessage
	NovaCellReadyErrorMessage = "Error occurred while querying NovaCell %s: %s"

	//NovaCellReadyMessage
	NovaCellReadyMessage = "NovaCell %s is Ready"

	//NovaMetadataReadyInitMessage
	NovaMetadataReadyInitMessage = "NovaMetadata not started"

	//NovaMetadataReadyErrorMessage
	NovaMetadataReadyErrorMessage = "NovaMetadata error occurred %s"

	//NovaNoVNCProxyReadyInitMessage
	NovaNoVNCProxyReadyInitMessage = "NovaNoVNCProxy not started"

	//NovaNoVNCProxyReadyErrorMessage
	NovaNoVNCProxyReadyErrorMessage = "NovaNoVNCProxy error occurred %s"

	//NovaComputeServiceConfigInitMessage
	NovaComputeServiceConfigInitMessage = "Compute service config generation is not started"

	//NovaComputeServiceConfigErrorMessage
	NovaComputeServiceConfigErrorMessage = "Compute service config generation error occurred %s"

	//NovaComputeReadyInitMessage
	NovaComputeReadyInitMessage = "NovaCompute not started"

	//NovaComputeReadyErrorMessage
	NovaComputeReadyErrorMessage = "NovaCompute error occurred %s"
)
