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

// Package novncproxy contains Nova noVNC proxy service configuration and deployment functionality.
package novncproxy

const (
	//NoVNCProxyPort - The port the nova-noVNCProxyPort service is exposed on
	NoVNCProxyPort = 6080
	// ServiceName - The name of the service exposed to k8s
	ServiceName = "nova-novncproxy"
	// Host is the default host address for noVNC proxy to bind to
	Host = "::0"
	// VencryptName is the name used for VeNCrypt encryption
	VencryptName = "vencrypt"
)
