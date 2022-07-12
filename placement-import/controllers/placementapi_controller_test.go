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
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	placementv1 "github.com/openstack-k8s-operators/placement-operator/api/v1beta1"
)

const (
	// TODO(gibi): Do we want to test in a realistic namespace like "openstack"?
	PlacementAPINamespace = "default"

	SecretName = "test-secret"

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

func GetCondition(
	lookupKey types.NamespacedName,
	conditionType condition.Type,
	reason condition.Reason,
) condition.Condition {
	instance := GetPlacementAPIInstance(lookupKey)

	if instance.Status.Conditions == nil {
		return condition.Condition{}
	}

	cond := instance.Status.Conditions.Get(conditionType)
	if cond != nil && cond.Reason == reason {
		return *cond
	}

	return condition.Condition{}
}

func NewKeystoneAPI() *keystonev1.KeystoneAPI {
	ctx := context.Background()

	keystoneAPIName := uuid.New().String()
	keystoneAPI := &keystonev1.KeystoneAPI{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "keystone.openstack.org/v1beta1",
			Kind:       "KeystoneAPI",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      keystoneAPIName,
			Namespace: PlacementAPINamespace,
		},
		Spec: keystonev1.KeystoneAPISpec{
			DatabaseUser: "foo-bar-baz",
		},
		Status: keystonev1.KeystoneAPIStatus{
			APIEndpoints:     map[string]string{"public": "fake-keystone-public-endpoint"},
			DatabaseHostname: "fake-database-hostname",
		},
	}
	// NOTE(gibi): the Create call will update keysteneAPI so we pass a copy
	// to preserve our input
	Expect(k8sClient.Create(ctx, keystoneAPI.DeepCopy())).Should(Succeed())

	keystoneAPILookupKey := types.NamespacedName{Name: keystoneAPIName, Namespace: PlacementAPINamespace}
	keystoneAPIInstance := GetKeystoneAPIInstance(keystoneAPILookupKey)
	// the Status needs to be written via a separate client
	keystoneAPIInstance.Status = keystoneAPI.Status
	k8sClient.Status().Update(ctx, keystoneAPIInstance)

	keystoneAPIInstance = GetKeystoneAPIInstance(keystoneAPILookupKey)

	return keystoneAPIInstance
}

func GetKeystoneAPIInstance(lookupKey types.NamespacedName) *keystonev1.KeystoneAPI {
	instance := &keystonev1.KeystoneAPI{}
	Eventually(func() bool {
		err := k8sClient.Get(ctx, lookupKey, instance)
		return err == nil
	}, timeout, interval).Should(BeTrue())
	return instance
}

func DeleteKeystoneAPI(lookupKey types.NamespacedName) {
	keystoneAPI := GetKeystoneAPIInstance(lookupKey)
	Expect(k8sClient.Delete(ctx, keystoneAPI)).Should(Succeed())
	Eventually(func() bool {
		err := k8sClient.Get(ctx, lookupKey, keystoneAPI)
		return k8s_errors.IsNotFound(err)
	}, timeout, interval).Should(BeTrue())
}

func GetConfigMap(namespace string, configMapName string) *corev1.ConfigMap {
	configList := &corev1.ConfigMapList{}
	listOpts := []client.ListOption{
		client.InNamespace(namespace),
	}

	err := k8sClient.List(ctx, configList, listOpts...)
	Expect(err).NotTo(HaveOccurred())

	for _, c := range configList.Items {
		if c.ObjectMeta.Name == configMapName {
			return &c
		}
	}
	return nil
}

func GetConfigDataConfigMap(ownerLookupKey types.NamespacedName) *corev1.ConfigMap {
	name := fmt.Sprintf("%s-%s", ownerLookupKey.Name, "config-data")
	return GetConfigMap(ownerLookupKey.Namespace, name)
}

func GetScriptsConfigMap(ownerLookupKey types.NamespacedName) *corev1.ConfigMap {
	name := fmt.Sprintf("%s-%s", ownerLookupKey.Name, "scritps")
	return GetConfigMap(ownerLookupKey.Namespace, name)
}

var _ = Describe("PlacementAPI controller", func() {

	var placementAPILookupKey types.NamespacedName

	BeforeEach(func() {

		// lib-common uses OPERATOR_TEMPLATES env var to locate the "templates"
		// directory of the operator. We need to set them othervise lib-common
		// will fail to generate the ConfigMap as it does not find common.sh
		err := os.Setenv("OPERATOR_TEMPLATES", "../templates")
		Expect(err).NotTo(HaveOccurred())

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
				Secret:           SecretName,
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

		It("should be in a state of waiting for the secret as it is not create yet", func() {
			Eventually(func() condition.Condition {
				// TODO (mschuppert) change conditon package to be able to use haveSameStateOf Matcher here
				return GetCondition(placementAPILookupKey, condition.InputReadyCondition, condition.RequestedReason)
			}, timeout, interval).Should(HaveField("Status", corev1.ConditionFalse))
		})
	})

	When("an unrelated secret is provided", func() {
		It("should remain in a state of waiting for the proper secret", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "an-unrelated-secret",
					Namespace: PlacementAPINamespace,
				},
			}
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
			Eventually(func() condition.Condition {
				return GetCondition(placementAPILookupKey, condition.InputReadyCondition, condition.RequestedReason)
			}, timeout, interval).Should(HaveField("Status", corev1.ConditionFalse))
		})
	})

	When("the proper secret is provided", func() {
		It("should not be in a state of waiting for the secret", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      SecretName,
					Namespace: PlacementAPINamespace,
				},
			}
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
			Eventually(func() condition.Condition {
				return GetCondition(placementAPILookupKey, condition.InputReadyCondition, condition.ReadyReason)
			}, timeout, interval).Should(HaveField("Status", corev1.ConditionTrue))
		})
	})

	When("keystoneAPI instance is available", func() {
		It("should create a ConfigMap for placement.conf with the auth_url config option set based on the KeystoneAPI", func() {
			// TODO(gibi): make sure that the KeystoneAPI instance is deleted AfterSuite
			NewKeystoneAPI()
			Eventually(func() *corev1.ConfigMap {
				return GetConfigDataConfigMap(placementAPILookupKey)
			}, timeout, interval).ShouldNot(BeNil())
			configData := GetConfigDataConfigMap(placementAPILookupKey)
			Expect(configData.Data["placement.conf"]).Should(ContainSubstring("auth_url = fake-keystone-public-endpoint"))
		})
	})
})
