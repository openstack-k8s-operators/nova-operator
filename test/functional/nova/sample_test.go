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

	. "github.com/onsi/ginkgo/v2" //revive:disable:dot-imports
	. "github.com/onsi/gomega"    //revive:disable:dot-imports
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/types"
)

const SamplesDir = "../../../config/samples/"

func ReadSample(sampleFileName string) map[string]interface{} {
	rawSample := make(map[string]interface{})

	bytes, err := os.ReadFile(filepath.Join(SamplesDir, sampleFileName))
	Expect(err).ShouldNot(HaveOccurred())
	Expect(yaml.Unmarshal(bytes, rawSample)).Should(Succeed())

	return rawSample
}

func CreateNovaFromSample(sampleFileName string, name types.NamespacedName) types.NamespacedName {
	raw := ReadSample(sampleFileName)
	instance := CreateNova(name, raw["spec"].(map[string]interface{}))
	DeferCleanup(th.DeleteInstance, instance)
	return types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
}

func CreateNovaAPIFromSample(sampleFileName string, name types.NamespacedName) types.NamespacedName {
	raw := ReadSample(sampleFileName)
	instance := CreateNovaAPI(name, raw["spec"].(map[string]interface{}))
	DeferCleanup(th.DeleteInstance, instance)
	return types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
}

func CreateNovaCellFromSample(sampleFileName string, name types.NamespacedName) types.NamespacedName {
	raw := ReadSample(sampleFileName)
	instance := CreateNovaCell(
		name,
		raw["spec"].(map[string]interface{}),
	)
	DeferCleanup(th.DeleteInstance, instance)
	return types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
}

func CreateNovaConductorFromSample(sampleFileName string, name types.NamespacedName) types.NamespacedName {
	raw := ReadSample(sampleFileName)
	instance := CreateNovaConductor(name, raw["spec"].(map[string]interface{}))
	DeferCleanup(th.DeleteInstance, instance)
	return types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
}

// This is a set of test for our samples. It only validates that the sample
// file has all the required field with proper types. But it does not
// validate that using a sample file will result in a working deployment.
// TODO(gibi): By building up all the prerequisites (e.g. MariaDBDatabase) in
// the test and by simulating Job and Deployment success we could assert
// that each sample creates a CR in Ready state.
var _ = Describe("Samples", func() {

	When("nova_v1beta1_nova.yaml sample is applied", func() {
		It("Nova is created", func() {
			name := CreateNovaFromSample("nova_v1beta1_nova.yaml", novaNames.NovaName)
			GetNova(name)
		})
	})
	When("nova_v1beta1_nova-multi-cell.yaml sample is applied", func() {
		It("Nova is created", func() {
			name := CreateNovaFromSample("nova_v1beta1_nova-multi-cell.yaml", novaNames.NovaName)
			GetNova(name)
		})
	})
	When("nova_v1beta1_nova_collapsed_cell.yaml sample is applied", func() {
		It("Nova is created", func() {
			name := CreateNovaFromSample("nova_v1beta1_nova_collapsed_cell.yaml", novaNames.NovaName)
			GetNova(name)
		})
	})
	When("nova_v1beta1_nova-compute-ironic.yaml sample is applied", func() {
		It("Nova is created", func() {
			name := CreateNovaFromSample("nova_v1beta1_nova-compute-ironic.yaml", novaNames.NovaName)
			GetNova(name)
		})
	})
	When("nova_v1beta1_nova-compute-fake.yaml sample is applied", func() {
		It("Nova is created", func() {
			name := CreateNovaFromSample("nova_v1beta1_nova-compute-fake.yaml", novaNames.NovaName)
			GetNova(name)
		})
	})
	When("nova_v1beta1_novaapi.yaml sample is applied", func() {
		It("NovaAPI is created", func() {
			name := CreateNovaAPIFromSample("nova_v1beta1_novaapi.yaml", novaNames.APIName)
			GetNovaAPI(name)
		})
	})
	When("nova_v1beta1_novacell0.yaml sample is applied", func() {
		It("NovaCell is created", func() {
			name := CreateNovaCellFromSample("nova_v1beta1_novacell0.yaml", cell0.CellCRName)
			GetNovaCell(name)
		})
	})
	When("nova_v1beta1_novacell1-upcall.yaml sample is applied", func() {
		It("NovaCell is created", func() {
			name := CreateNovaCellFromSample("nova_v1beta1_novacell1-upcall.yaml", cell1.CellCRName)
			GetNovaCell(name)
		})
	})
	When("nova_v1beta1_novacell2-without-upcall.yaml sample is applied", func() {
		It("NovaCell is created", func() {
			name := CreateNovaCellFromSample("nova_v1beta1_novacell2-without-upcall.yaml", cell2.CellCRName)
			GetNovaCell(name)
		})
	})
	When("nova_v1beta1_novaconductor-super.yaml sample is applied", func() {
		It("NovaConductor is created", func() {
			name := CreateNovaConductorFromSample("nova_v1beta1_novaconductor-super.yaml", cell0.ConductorName)
			GetNovaConductor(name)
		})
	})
	When("nova_v1beta1_novaconductor-cell.yaml sample is applied", func() {
		It("NovaConductor is created", func() {
			name := CreateNovaConductorFromSample("nova_v1beta1_novaconductor-cell.yaml", cell1.ConductorName)
			GetNovaConductor(name)
		})
	})
})
