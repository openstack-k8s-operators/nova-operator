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
package cyborg_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2" //revive:disable:dot-imports
	. "github.com/onsi/gomega"    //revive:disable:dot-imports

	commonservice "github.com/openstack-k8s-operators/lib-common/modules/common/service"
	cyborgv1beta1 "github.com/openstack-k8s-operators/nova-operator/api/cyborg/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

var _ = Describe("CyborgAPI validation", func() {
	var apiNames CyborgAPINames

	BeforeEach(func() {
		apiNames = GetCyborgAPINames(
			cyborgNames.CyborgName.Namespace,
			"cyborg-api-webhook-test",
		)
	})

	DescribeTable("validates public and internal API service override port names",
		func(endpoint commonservice.Endpoint, valid bool) {
			portName := "wrong"
			if valid {
				portName = string(endpoint)
			}

			spec := cyborgv1beta1.CyborgAPISpec{
				CyborgAPITemplate: cyborgv1beta1.CyborgAPITemplate{
					Replicas: ptr.To(int32(1)),
					Override: cyborgv1beta1.APIOverrideSpec{
						Service: map[commonservice.Endpoint]commonservice.RoutedOverrideSpec{
							endpoint: {
								OverrideSpec: commonservice.OverrideSpec{
									Spec: &commonservice.OverrideServiceSpec{
										Ports: []commonservice.OverrideServicePort{
											{Name: portName, Port: 6666},
										},
									},
								},
							},
						},
					},
				},
				ConfigSecret:   apiNames.ConfigSecretName.Name,
				ContainerImage: APITestImage,
				ServiceAccount: APITestSA,
				APITimeout:     ptr.To(60),
			}

			api := &cyborgv1beta1.CyborgAPI{
				ObjectMeta: metav1.ObjectMeta{
					Name:      apiNames.APIName.Name,
					Namespace: apiNames.APIName.Namespace,
				},
				Spec: spec,
			}
			err := k8sClient.Create(ctx, api)

			if valid {
				Expect(err).NotTo(HaveOccurred())
				DeferCleanup(th.DeleteInstance, api)
			} else {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(
					fmt.Sprintf("spec.override.service[%s].spec.ports[0].name: Invalid value: %q: must be %q",
						endpoint, portName, endpoint),
				))
			}
		},
		Entry("public valid", commonservice.EndpointPublic, true),
		Entry("public invalid", commonservice.EndpointPublic, false),
		Entry("internal valid", commonservice.EndpointInternal, true),
		Entry("internal invalid", commonservice.EndpointInternal, false),
	)
})
