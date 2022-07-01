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

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	placementv1 "github.com/openstack-k8s-operators/placement-operator/api/v1beta1"
)

const (
	// TODO(gibi): Do we want to test in a realistic namespace like "openstack"?
	PlacementAPINamespace = "default"

	timeout  = time.Second * 2
	interval = time.Millisecond * 200
)

func GetPlacementAPIInstance(lookupKey types.NamespacedName) *placementv1.PlacementAPI {
	instance := &placementv1.PlacementAPI{}
	Eventually(func() bool {
		err := k8sClient.Get(ctx, lookupKey, instance)
		return err == nil
	}, timeout, interval).Should(BeTrue())
	return instance
}

var _ = Describe("PlacementAPI controller", func() {

	var placementAPILookupKey types.NamespacedName

	BeforeEach(func() {
		ctx := context.Background()

		// If we want to support parallel spec execution we have to make
		// sure that the placementAPI instance are unique for each test
		placementAPIName := uuid.New().String()
		placementAPI := &placementv1.PlacementAPI{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "placement.openstack.org/v1beta1",
				Kind:       "PlacementAPI",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      placementAPIName,
				Namespace: PlacementAPINamespace,
			},
			Spec: placementv1.PlacementAPISpec{
				DatabaseInstance: "test-db-instance",
				ContainerImage:   "test-placement-container-image",
				Secret:           "test-secret",
			},
		}
		Expect(k8sClient.Create(ctx, placementAPI)).Should(Succeed())

		placementAPILookupKey = types.NamespacedName{Name: placementAPIName, Namespace: PlacementAPINamespace}
		GetPlacementAPIInstance(placementAPILookupKey)
	})

	AfterEach(func() {
		placementAPIInstance := GetPlacementAPIInstance(placementAPILookupKey)
		// Delete the PlacementAPI instance
		Expect(k8sClient.Delete(ctx, placementAPIInstance)).Should(Succeed())
		// We have to wait for the PlacementAPI instance to be fully deleted
		// by the controller
		Eventually(func() bool {
			err := k8sClient.Get(ctx, placementAPILookupKey, placementAPIInstance)
			return k8s_errors.IsNotFound(err)
		}, timeout, interval).Should(BeTrue())
	})

	When("A PlacementAPI instance is created", func() {

		It("should have the Spec and Status fields initialized", func() {
			placementAPIInstance := GetPlacementAPIInstance(placementAPILookupKey)
			Expect(placementAPIInstance.Spec.DatabaseInstance).Should(Equal("test-db-instance"))
			// TODO(gibi): Why defaulting does not work?
			// Expect(createdPlacementAPI.Spec.ServiceUser).Should(Equal("placement"))
		})

		It("should have a finalizer", func() {
			// the reconciler loop adds the finalizer so we have to wait for
			// it to run
			Eventually(func() []string {
				return GetPlacementAPIInstance(placementAPILookupKey).ObjectMeta.Finalizers
			}, timeout, interval).Should(ContainElement("PlacementAPI"))
		})
	})

})
