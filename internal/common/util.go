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

import (
	"sort"

	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	util "github.com/openstack-k8s-operators/lib-common/modules/common/util"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ConditionUpdater updates conditions on a reconciled object.
type ConditionUpdater interface {
	Set(c *condition.Condition)
	MarkTrue(t condition.Type, messageFormat string, messageArgs ...any)
}

// ConditionsGetter provides access to an object's conditions.
type ConditionsGetter interface {
	GetConditions() condition.Conditions
}

// GetSecret defines an interface for objects that can provide a secret name.
type GetSecret interface {
	GetSecret() string
	client.Object
}

// AllSubConditionIsTrue reports whether all non-Ready conditions are True.
func AllSubConditionIsTrue(conditionsGetter ConditionsGetter) bool {
	for _, c := range conditionsGetter.GetConditions() {
		if c.Type == condition.ReadyCondition {
			continue
		}
		if c.Status != corev1.ConditionTrue {
			return false
		}
	}
	return true
}

// OwnedBy returns true if the owner has an OwnerReference on the owned object.
func OwnedBy(owned client.Object, owner client.Object) bool {
	for _, ref := range owned.GetOwnerReferences() {
		if owner.GetUID() == ref.UID {
			return true
		}
	}
	return false
}

// HashOfStringMap returns a stable hash of a string map.
func HashOfStringMap(input map[string]string) (string, error) {
	keys := make([]string, 0, len(input))
	for k := range input {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	keyValues := []string{}
	for _, key := range keys {
		value := input[key]
		keyValues = append(keyValues, key+value)
	}
	return util.ObjectHash(keyValues)
}
