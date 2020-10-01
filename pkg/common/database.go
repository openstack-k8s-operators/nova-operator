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
	"fmt"

	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"

	util "github.com/openstack-k8s-operators/lib-common/pkg/util"
)

// Database -
type Database struct {
	DatabaseHostname string
	DatabaseName     string
	Secret           string
}

// opetions to set in the DB template
type databaseOptions struct {
	Name             string
	DatabaseHostname string
	DatabaseName     string
	Secret           string
}

// DatabaseObject func
func DatabaseObject(r ReconcilerCommon, obj metav1.Object, db Database) (unstructured.Unstructured, error) {
	opts := databaseOptions{
		strings.Replace(db.DatabaseName, "_", "", -1),
		db.DatabaseHostname,
		db.DatabaseName,
		db.Secret,
	}

	templatesPath := util.GetTemplatesPath()

	mariadbDatabaseTemplate := fmt.Sprintf("%s/common/internal/mariadb_database.yaml", templatesPath)
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(util.ExecuteTemplate(mariadbDatabaseTemplate, &opts)), 4096)
	u := unstructured.Unstructured{}
	err := decoder.Decode(&u)
	u.SetNamespace(obj.GetNamespace())

	// set owner reference
	u.SetOwnerReferences(obj.GetOwnerReferences())

	return u, err
}
