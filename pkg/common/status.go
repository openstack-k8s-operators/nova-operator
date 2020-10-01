/*
Copyright 2020 Red Hat

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

package common

import (
	"context"
	"fmt"

	novav1beta1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
)

// UpdateStatusHash - update/set hashes in CR status
func UpdateStatusHash(r ReconcilerCommon, obj runtime.Object, hashes *[]novav1beta1.Hash, newHashes []novav1beta1.Hash) error {
	update := false

	for _, newHash := range newHashes {
		exists := false

		// is there already an existing hash entry for the resource
		for i := 0; i < len(*hashes); i++ {
			if (*hashes)[i].Name == newHash.Name {
				if (*hashes)[i].Hash != newHash.Hash {
					r.GetLogger().Info(fmt.Sprintf("Hash of resource %s changed - old: %s new: %s", (*hashes)[i].Name, (*hashes)[i].Hash, newHash.Hash))
					(*hashes)[i] = novav1beta1.Hash{
						Name: newHash.Name,
						Hash: newHash.Hash,
					}
					update = true
				}
				exists = true
				break
			}
		}
		// add hash entry for new resource
		if !exists {
			r.GetLogger().Info(fmt.Sprintf("New hash of resource %s added to status - %s", newHash.Name, newHash.Hash))
			*hashes = append(*hashes, newHash)
			update = true
		}
	}

	// update status if required
	if update {
		if err := r.GetClient().Status().Update(context.TODO(), obj); err != nil {
			return err
		}
	}
	return nil
}
