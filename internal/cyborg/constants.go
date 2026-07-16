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

// Package cyborg provides constants and utilities for OpenStack Cyborg service functionality
package cyborg

const (
	// ServiceName identifies the cyborg service
	ServiceName = "cyborg"

	// ServiceType is the Keystone service type for Cyborg
	ServiceType = "accelerator"

	// DatabaseName is the name of the database used in CREATE DATABASE
	DatabaseName = "cyborg"

	// DatabaseCRName is the CR name for the MariaDBDatabase
	DatabaseCRName = "cyborg"

	// DatabaseUsernamePrefix is used by EnsureMariaDBAccount for new usernames
	DatabaseUsernamePrefix = "cyborg"

	// CyborgPublicPort is the default public port for the cyborg-api service
	CyborgPublicPort int32 = 6666

	// CyborgInternalPort is the default internal port for the cyborg-api service
	CyborgInternalPort int32 = 6666

	// ConfigVolume is the default volume name used to mount service config
	ConfigVolume = "config-data"

	// DefaultsConfigFileName is the file name with default configuration
	DefaultsConfigFileName = "00-default.conf"

	// DBSyncCommand is the kolla command to run the dbsync job
	DBSyncCommand = "/usr/local/bin/kolla_set_configs && /usr/local/bin/kolla_start"

	// CyborgLogPath is the default path for the cyborg service logs
	CyborgLogPath = "/var/log/cyborg/"
)
