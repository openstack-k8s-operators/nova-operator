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

package controllers

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	placementv1beta1 "github.com/openstack-k8s-operators/placement-operator/api/v1beta1"
)

var _ = Describe("PlacementAPI controller", func() {

	const (
		PlacementAPIName = "test-placementapi"
		// TODO(gibi): Do we want to test in a realistic namespace like "openstack"?
		PlacementAPINamespace = "default"
		JobName               = "test-job"

		timeout  = time.Second * 1
		interval = time.Millisecond * 200
	)

	Context("When PlacementAPI is created", func() {
		It("Should have all the spec fields initialized", func() {
			By("By creating a new PlacementAPI")
			ctx := context.Background()
			placementAPI := &placementv1beta1.PlacementAPI{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "placement.openstack.org/v1beta1",
					Kind:       "PlacementAPI",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      PlacementAPIName,
					Namespace: PlacementAPINamespace,
				},
				Spec: placementv1beta1.PlacementAPISpec{
					DatabaseInstance: "test-db-instance",
					ContainerImage:   "test-placement-container-image",
					Secret:           "test-secret",
				},
			}
			Expect(k8sClient.Create(ctx, placementAPI)).Should(Succeed())

			placementAPILookupKey := types.NamespacedName{Name: PlacementAPIName, Namespace: PlacementAPINamespace}
			createdPlacementAPI := &placementv1beta1.PlacementAPI{}

			// We'll need to retry getting this newly created placementAPI, given that creation may not immediately happen.
			Eventually(func() bool {
				err := k8sClient.Get(ctx, placementAPILookupKey, createdPlacementAPI)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			// Let's make sure our placementAPI spec has proper default values
			// TODO(gibi): match the rest of the fields
			Expect(createdPlacementAPI.Spec.DatabaseInstance).Should(Equal("test-db-instance"))
			// TODO(gibi): Why defaulting does not work?
			// Expect(createdPlacementAPI.Spec.ServiceUser).Should(Equal("placement"))
		})
	})

})
