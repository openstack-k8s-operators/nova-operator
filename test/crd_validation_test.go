package test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RHsyseng/operator-utils/pkg/validation"
	v1 "github.com/openstack-k8s-operators/nova-operator/pkg/apis/nova/v1"

	"github.com/ghodss/yaml"

	"github.com/stretchr/testify/assert"
)

func TestSampleCustomResources(t *testing.T) {
	root := "./../deploy/crds"
	crdCrMap := map[string]string{
		"nova_v1_iscsid_crd.yaml":              "nova_v1_iscsid_cr",
		"nova_v1_libvirtd_crd.yaml":            "nova_v1_libvirtd_cr",
		"nova_v1_novacompute_crd.yaml":         "nova_v1_novacompute_cr",
		"nova_v1_novamigrationtarget_crd.yaml": "nova_v1_novamigrationtarget_cr",
		"nova_v1_virtlogd_crd.yaml":            "nova_v1_virtlogd_cr",
	}
	for crd, prefix := range crdCrMap {
		validateCustomResources(t, root, crd, prefix)
	}
}

func validateCustomResources(t *testing.T, root string, crd string, prefix string) {
	schema := getSchema(t, fmt.Sprintf("%s/%s", root, crd))
	assert.NotNil(t, schema)
	walkFunc := func(path string, info os.FileInfo, err error) error {
		if strings.HasSuffix(info.Name(), "crd.yaml") {
			//Ignore CRD
			return nil
		}
		if strings.HasPrefix(info.Name(), prefix) {
			bytes, err := ioutil.ReadFile(path)
			assert.NoError(t, err, "Error reading CR yaml from %v", path)
			var input map[string]interface{}
			assert.NoError(t, yaml.Unmarshal(bytes, &input))
			assert.NoError(t, schema.Validate(input), "File %v does not validate against the %s CRD schema", info.Name(), crd)
		}
		return nil
	}
	err := filepath.Walk(root, walkFunc)
	assert.NoError(t, err, "Error reading CR yaml files from ", root)
}

func TestCompleteCRD(t *testing.T) {
	root := "./../deploy/crds"
	crdStructMap := map[string]interface{}{
		"nova_v1_iscsid_crd.yaml":              &v1.Iscsid{},
		"nova_v1_libvirtd_crd.yaml":            &v1.Libvirtd{},
		"nova_v1_novacompute_crd.yaml":         &v1.NovaCompute{},
		"nova_v1_novamigrationtarget_crd.yaml": &v1.NovaMigrationTarget{},
		"nova_v1_virtlogd_crd.yaml":            &v1.Virtlogd{},
	}
	for crd, obj := range crdStructMap {
		schema := getSchema(t, fmt.Sprintf("%s/%s", root, crd))
		missingEntries := schema.GetMissingEntries(obj)
		// Here we can add nested spec
		nestedOmissions := [...]string{}
		for _, missing := range missingEntries {
			skipAsOmission := false
			for _, omit := range nestedOmissions {
				if strings.HasPrefix(missing.Path, omit) {
					skipAsOmission = true
					break
				}
			}
			if !skipAsOmission {
				assert.Fail(t, "Discrepancy between CRD and Struct", "CRD: %s: Missing or incorrect schema validation at %s, expected type %s", crd, missing.Path, missing.Type)
			}
		}
	}
}

func getSchema(t *testing.T, crd string) validation.Schema {
	bytes, err := ioutil.ReadFile(crd)
	assert.NoError(t, err, "Error reading CRD yaml from %v", crd)
	schema, err := validation.New(bytes)
	assert.NoError(t, err)
	return schema
}
