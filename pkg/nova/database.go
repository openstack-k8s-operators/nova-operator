package nova

import (
	"fmt"

	util "github.com/openstack-k8s-operators/lib-common/pkg/util"

	novav1beta1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"strings"

	"k8s.io/apimachinery/pkg/util/yaml"
)

type databaseOptions struct {
	Name             string
	DatabaseHostname string
	DatabaseName     string
	Secret           string
}

// DatabaseObject func
func DatabaseObject(cr *novav1beta1.Nova, databaseName string) (unstructured.Unstructured, error) {
	opts := databaseOptions{strings.Replace(databaseName, "_", "", -1), cr.Spec.DatabaseHostname, databaseName, cr.Spec.NovaSecret}

	templatesPath := util.GetTemplatesPath()

	mariadbDatabaseTemplate := fmt.Sprintf("%s/common/internal/mariadb_database.yaml", templatesPath)
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(util.ExecuteTemplate(mariadbDatabaseTemplate, &opts)), 4096)
	u := unstructured.Unstructured{}
	err := decoder.Decode(&u)
	u.SetNamespace(cr.Namespace)
	return u, err
}
