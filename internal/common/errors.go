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

package common //nolint:revive // common is the established package name for multi-group shared code

import "errors"

var (
	// ErrACSecretNotFound indicates that the ApplicationCredential secret was not found.
	ErrACSecretNotFound = errors.New("ApplicationCredential secret not found")
	// ErrACSecretMissingKeys indicates that the ApplicationCredential secret is missing required keys.
	ErrACSecretMissingKeys = errors.New("ApplicationCredential secret missing required keys")
	// ErrTemplateDirUnset indicates that no template directory was provided.
	ErrTemplateDirUnset = errors.New("templateDir must be set")
)
