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

package v1beta1

import "github.com/openstack-k8s-operators/lib-common/modules/common/condition"

const (
	// DbSyncHash hash
	DbSyncHash = "dbsync"
)

const (
	// CyborgRabbitMQTransportURLReadyCondition indicates whether the Cyborg RabbitMQ TransportURL is ready
	CyborgRabbitMQTransportURLReadyCondition condition.Type = "CyborgRabbitMQTransportURLReady"

	// CyborgAPIReadyCondition indicates whether the CyborgAPI is ready
	CyborgAPIReadyCondition condition.Type = "CyborgAPIReady"

	// CyborgConductorReadyCondition indicates whether the CyborgConductor is ready
	CyborgConductorReadyCondition condition.Type = "CyborgConductorReady"
)

const (
	// CyborgRabbitMQTransportURLReadyRunningMessage -
	CyborgRabbitMQTransportURLReadyRunningMessage = "CyborgRabbitMQTransportURL creation in progress"

	// CyborgRabbitMQTransportURLReadyMessage -
	CyborgRabbitMQTransportURLReadyMessage = "CyborgRabbitMQTransportURL successfully created"

	// CyborgRabbitMQTransportURLReadyErrorMessage -
	CyborgRabbitMQTransportURLReadyErrorMessage = "CyborgRabbitMQTransportURL error occurred %s"

	// CyborgAPIReadyInitMessage -
	CyborgAPIReadyInitMessage = "CyborgAPI not started"

	// CyborgConductorReadyInitMessage -
	CyborgConductorReadyInitMessage = "CyborgConductor not started"

	// CyborgApplicationCredentialSecretErrorMessage -
	CyborgApplicationCredentialSecretErrorMessage = "Error with application credential secret"
)
