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
package functional_test

import (
	"os"
	"path/filepath"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/types"
)

const SamplesDir = "../../config/samples/"

func ReadSample(sampleFileName string) map[string]interface{} {
	rawSample := make(map[string]interface{})

	bytes, err := os.ReadFile(filepath.Join(SamplesDir, sampleFileName))
	Expect(err).ShouldNot(HaveOccurred())
	Expect(yaml.Unmarshal(bytes, rawSample)).Should(Succeed())

	return rawSample
}

func CreateNovaFromSample(sampleFileName string, namespace string) types.NamespacedName {
	novaName := types.NamespacedName{
		Namespace: namespace,
		Name:      uuid.New().String(),
	}

	raw := ReadSample(sampleFileName)
	CreateNova(novaName, raw["spec"].(map[string]interface{}))
	DeferCleanup(DeleteNova, novaName)
	return novaName
}

func CreateNovaAPIFromSample(sampleFileName string, namespace string) types.NamespacedName {
	raw := ReadSample(sampleFileName)
	name := CreateNovaAPI(namespace, raw["spec"].(map[string]interface{}))
	DeferCleanup(DeleteNova, name)
	return name
}

func CreateNovaCellFromSample(sampleFileName string, namespace string) types.NamespacedName {
	raw := ReadSample(sampleFileName)
	name := CreateNovaCell(namespace, raw["spec"].(map[string]interface{}))
	DeferCleanup(DeleteNova, name)
	return name
}

func CreateNovaConductorFromSample(sampleFileName string, namespace string) types.NamespacedName {
	raw := ReadSample(sampleFileName)
	name := CreateNovaConductor(namespace, raw["spec"].(map[string]interface{}))
	DeferCleanup(DeleteNova, name)
	return name
}

// This is a set of test for our samples. It only validates that the sample
// file has all the required field with proper types. But it does not
// validate that using a sample file will result in a working deployment.
// TODO(gibi): By building up all the prerequisits (e.g. MariaDBDatabase) in
// the test and by simulating Job and Deploymnet success we could assert
// that each sample creates a CR in Ready state.
var _ = Describe("Samples", func() {
	var namespace string
	BeforeEach(func() {
		// NOTE(gibi): We need to create a unique namespace for each test run
		// as namespaces cannot be deleted in a locally running envtest. See
		// https://book.kubebuilder.io/reference/envtest.html#namespace-usage-limitation
		namespace = uuid.New().String()
		CreateNamespace(namespace)
		// We still request the delete of the Namespace to properly cleanup if
		// we run the test in an existing cluster.
		DeferCleanup(DeleteNamespace, namespace)
		// NOTE(gibi): ConfigMap generation looks up the local templates
		// directory via ENV, so provide it
		DeferCleanup(os.Setenv, "OPERATOR_TEMPLATES", os.Getenv("OPERATOR_TEMPLATES"))
		os.Setenv("OPERATOR_TEMPLATES", "../../templates")

		// Uncomment this if you need the full output in the logs from gomega
		// matchers
		// format.MaxLength = 0

	})

	When("nova_v1beta1_nova.yaml sample is applied", func() {
		It("Nova is created", func() {
			name := CreateNovaFromSample("nova_v1beta1_nova.yaml", namespace)
			GetNova(name)
		})
	})
	When("nova_v1beta1_nova_only_cell0.yaml sample is applied", func() {
		It("Nova is created", func() {
			name := CreateNovaFromSample("nova_v1beta1_nova_only_cell0.yaml", namespace)
			GetNova(name)
		})
	})
	When("nova_v1beta1_nova-multi-cell.yaml sample is applied", func() {
		It("Nova is created", func() {
			name := CreateNovaFromSample("nova_v1beta1_nova-multi-cell.yaml", namespace)
			GetNova(name)
		})
	})
	When("nova_v1beta1_novaapi.yaml sample is applied", func() {
		It("NovaAPI is created", func() {
			name := CreateNovaAPIFromSample("nova_v1beta1_novaapi.yaml", namespace)
			GetNovaAPI(name)
		})
	})
	When("nova_v1beta1_novacell0.yaml sample is applied", func() {
		It("NovaCell is created", func() {
			name := CreateNovaCellFromSample("nova_v1beta1_novacell0.yaml", namespace)
			GetNovaCell(name)
		})
	})
	When("nova_v1beta1_novacell1-upcall.yaml sample is applied", func() {
		It("NovaCell is created", func() {
			name := CreateNovaCellFromSample("nova_v1beta1_novacell1-upcall.yaml", namespace)
			GetNovaCell(name)
		})
	})
	When("nova_v1beta1_novacell2-without-upcall.yaml sample is applied", func() {
		It("NovaCell is created", func() {
			name := CreateNovaCellFromSample("nova_v1beta1_novacell2-without-upcall.yaml", namespace)
			GetNovaCell(name)
		})
	})
	When("nova_v1beta1_novaconductor-super.yaml sample is applied", func() {
		It("NovaConductor is created", func() {
			name := CreateNovaConductorFromSample("nova_v1beta1_novaconductor-super.yaml", namespace)
			GetNovaConductor(name)
		})
	})
	When("nova_v1beta1_novaconductor-cell.yaml sample is applied", func() {
		It("NovaConductor is created", func() {
			name := CreateNovaConductorFromSample("nova_v1beta1_novaconductor-cell.yaml", namespace)
			GetNovaConductor(name)
		})
	})
})
