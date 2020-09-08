package placement

import (
	"fmt"

	util "github.com/openstack-k8s-operators/lib-common/pkg/util"

	placementv1beta1 "github.com/openstack-k8s-operators/placement-operator/api/v1beta1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"strings"

	"k8s.io/apimachinery/pkg/util/yaml"
)

type schemaOptions struct {
	DatabaseHostname string
	SchemaName       string
	Secret           string
}

// SchemaObject func
func SchemaObject(cr *placementv1beta1.PlacementAPI) (unstructured.Unstructured, error) {
	opts := schemaOptions{cr.Spec.DatabaseHostname, cr.Name, cr.Spec.Secret}

	templatesPath := util.GetTemplatesPath()

	mariadbSchemaTemplate := fmt.Sprintf("%s/%s/internal/mariadb_schema.yaml", templatesPath, strings.ToLower(cr.Kind))
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(util.ExecuteTemplate(mariadbSchemaTemplate, &opts)), 4096)
	u := unstructured.Unstructured{}
	err := decoder.Decode(&u)
	u.SetNamespace(cr.Namespace)
	return u, err
}
