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

//
// Nova Condition Types used by API objects.
//
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
)

//
// Common Messages used by API objects.
//
const (
	// NovaAPIReadyInitMessage
	NovaAPIReadyInitMessage = "NovaAPI not started"

	// NovaAPIReadyErrorMessage
	NovaAPIReadyErrorMessage = "NovaAPI error occured %s"

	// NovaConductorReadyInitMessage
	NovaConductorReadyInitMessage = "NovaConductor not started"

	// NovaConductorReadyErrorMessage
	NovaConductorReadyErrorMessage = "NovaConductor error occured %s"

	// NovaAllCellsDBReadyInitMessage
	NovaAllCellsDBReadyInitMessage = "DB creation not started"

	// NovaAllCellsDBReadyErrorMessage
	NovaAllCellsDBReadyCreatingMessage = "DB creation ongoing for %s"

	// NovaAllCellsDBReadyErrorMessage
	NovaAllCellsDBReadyErrorMessage = "DB creation failed for %s"

	// NovaAllCellsReadyMessage
	NovaAllCellsDBReadyMessage = "All DBs created succcessfully"

	// NovaAllCellsReadyInitMessage
	NovaAllCellsReadyInitMessage = "NovaCells are not started"

	// NovaAllCellsReadyCreatingMessage
	NovaAllCellsReadyCreatingMessage = "NovaCell creation ongoing for %s"

	// NovaAllCellsReadyWaitingMessage
	NovaAllCellsReadyWaitingMessage = "NovaCell creation waits for DB creation for %s"

	// NovaAllCellsReadyErrorMessage
	NovaAllCellsReadyErrorMessage = "NovaCell creation failed for %s"

	// NovaAllCellsReadyMessage
	NovaAllCellsReadyMessage = "All NovaCells are ready"
)
