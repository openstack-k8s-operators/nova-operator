/*
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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
)

var _ = Describe("NovaNoVNCProxy controller", func() {
	When("NovaNoVNCProxy CR is created without container image defined", func() {
		BeforeEach(func() {
			spec := GetDefaultNovaNoVNCProxySpec()
			spec["containerImage"] = ""
			novncproxy := CreateNovaNoVNCProxy(novaNames.NoVNCProxyName, spec)
			DeferCleanup(th.DeleteInstance, novncproxy)
		})
		It("has the expected container image default", func() {
			novaNoVNCProxyDefault := GetNovaNoVNCProxy(novaNames.NoVNCProxyName)
			Expect(novaNoVNCProxyDefault.Spec.ContainerImage).To(Equal(util.GetEnvVar("NOVA_NOVNC_IMAGE_URL_DEFAULT", novav1.NovaNoVNCContainerImage)))
		})
	})
})
