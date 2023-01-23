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
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	placementv1 "github.com/openstack-k8s-operators/placement-operator/api/v1beta1"
)

const (
	// TODO(gibi): Do we want to test in a realistic namespace like "openstack"?
	TestNamespace = "default"

	SecretName = "test-secret"

	timeout  = time.Second * 2
	interval = time.Millisecond * 200
)

func DefaultPlacementAPITemplate() *placementv1.PlacementAPI {
	return &placementv1.PlacementAPI{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "placement.openstack.org/v1beta1",
			Kind:       "PlacementAPI",
		},
		ObjectMeta: metav1.ObjectMeta{},
		Spec: placementv1.PlacementAPISpec{
			DatabaseInstance: "test-db-instance",
			ContainerImage:   "test-placement-container-image",
			Secret:           SecretName,
		},
	}
}

type TestPlacementAPI struct {
	LookupKey types.NamespacedName
	// The input data for creating a PlacementAPI
	Template *placementv1.PlacementAPI
	// The current state of the PlacementAPI resource updated by Refresh()
	Instance *placementv1.PlacementAPI
}

// NewTestPlacementAPI initializes the the input for a PlacementAPI instance
// but does not create it yet. So the client can finetuned the Template data
// before calling Create()
func NewTestPlacementAPI(namespace string) TestPlacementAPI {
	name := fmt.Sprintf("placement-%s", uuid.New().String())
	template := DefaultPlacementAPITemplate()
	template.ObjectMeta.Name = name
	template.ObjectMeta.Namespace = namespace
	return TestPlacementAPI{
		LookupKey: types.NamespacedName{Name: name, Namespace: namespace},
		Template:  template,
		Instance:  &placementv1.PlacementAPI{},
	}
}

// Creates the PlacementAPI resource in k8s based on the Template. The Template
// is not updated during create. This call waits until the resource is created.
// The last known state of the resource is available via Instance.
func (t TestPlacementAPI) Create() {
	Expect(k8sClient.Create(ctx, t.Template.DeepCopy())).Should(Succeed())
	t.Refresh()
}

// Deletes the PlacementAPI resource from k8s and waits until it is deleted.
func (t TestPlacementAPI) Delete() {
	Expect(k8sClient.Delete(ctx, t.Instance)).Should(Succeed())
	// We have to wait for the PlacementAPI instance to be fully deleted
	// by the controller
	Eventually(func() bool {
		err := k8sClient.Get(ctx, t.LookupKey, t.Instance)
		return k8s_errors.IsNotFound(err)
	}, timeout, interval).Should(BeTrue())
}

// Refreshes the state of the Instance from k8s
func (t TestPlacementAPI) Refresh() *placementv1.PlacementAPI {
	Eventually(func() bool {
		err := k8sClient.Get(ctx, t.LookupKey, t.Instance)
		return err == nil
	}, timeout, interval).Should(BeTrue())
	return t.Instance
}

// Gets the condition of given type from the resource.
func (t TestPlacementAPI) GetCondition(conditionType condition.Type, reason condition.Reason) condition.Condition {
	t.Refresh()
	if t.Instance.Status.Conditions == nil {
		return condition.Condition{}
	}

	cond := t.Instance.Status.Conditions.Get(conditionType)
	if cond != nil && cond.Reason == reason {
		return *cond
	}

	return condition.Condition{}

}

type TestKeystoneAPI struct {
	LookupKey types.NamespacedName
	// The input data for creating a KeystoneAPI
	Template *keystonev1.KeystoneAPI
	Instance *keystonev1.KeystoneAPI
}

// NewTestKeystoneAPI initializes the the input for a KeystoneAPI instance
// but does not create it yet. So the client can finetuned the Template data
// before calling Create()
func NewTestKeystoneAPI(namespace string) *TestKeystoneAPI {
	name := fmt.Sprintf("keystone-%s", uuid.New().String())
	template := &keystonev1.KeystoneAPI{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "keystone.openstack.org/v1beta1",
			Kind:       "KeystoneAPI",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: TestNamespace,
		},
		Spec: keystonev1.KeystoneAPISpec{
			DatabaseUser: "foo-bar-baz",
		},
		Status: keystonev1.KeystoneAPIStatus{
			APIEndpoints: map[string]string{
				"internal": "fake-keystone-internal-endpoint",
				"public":   "fake-keystone-public-endpoint",
			},
			DatabaseHostname: "fake-database-hostname",
		},
	}
	return &TestKeystoneAPI{
		LookupKey: types.NamespacedName{Name: name, Namespace: namespace},
		Template:  template,
		Instance:  &keystonev1.KeystoneAPI{},
	}
}

// Creates the KeystoneAPI resource in k8s based on the Template. The Template
// is not updated with the result of the create.
func (t TestKeystoneAPI) Create() {
	t.Instance = t.Template.DeepCopy()
	Expect(k8sClient.Create(ctx, t.Instance)).Should(Succeed())

	// the Status field needs to be written via a separate client
	t.Instance.Status = t.Template.Status
	Expect(k8sClient.Status().Update(ctx, t.Instance)).Should(Succeed())
}

// Deletes the KeystoneAPI resource from k8s and waits until it is deleted.
func (t TestKeystoneAPI) Delete() {
	Expect(k8sClient.Delete(ctx, t.Template.DeepCopy())).Should(Succeed())
	// We have to wait for the instance to be fully deleted
	// by the controller
	Eventually(func() bool {
		err := k8sClient.Get(ctx, t.LookupKey, t.Instance)
		return k8s_errors.IsNotFound(err)
	}, timeout, interval).Should(BeTrue())
}

var _ = Describe("PlacementAPI controller", func() {

	var placementAPI TestPlacementAPI
	var secret *corev1.Secret
	var keystoneAPI *TestKeystoneAPI

	BeforeEach(func() {
		// lib-common uses OPERATOR_TEMPLATES env var to locate the "templates"
		// directory of the operator. We need to set them othervise lib-common
		// will fail to generate the ConfigMap as it does not find common.sh
		err := os.Setenv("OPERATOR_TEMPLATES", "../../templates")
		Expect(err).NotTo(HaveOccurred())

		placementAPI = NewTestPlacementAPI(TestNamespace)
		placementAPI.Create()

	})

	AfterEach(func() {
		placementAPI.Delete()
		if secret != nil {
			Expect(k8sClient.Delete(ctx, secret)).Should(Succeed())
		}
		if keystoneAPI != nil {
			keystoneAPI.Delete()
		}
	})

	When("A PlacementAPI instance is created", func() {

		It("should have the Spec and Status fields initialized", func() {
			Expect(placementAPI.Instance.Spec.DatabaseInstance).Should(Equal("test-db-instance"))
			// TODO(gibi): Why defaulting does not work?
			// Expect(placementAPI.Instance.Spec.ServiceUser).Should(Equal("placement"))
		})

		It("should have a finalizer", func() {
			// the reconciler loop adds the finalizer so we have to wait for
			// it to run
			Eventually(func() []string {
				return placementAPI.Refresh().ObjectMeta.Finalizers
			}, timeout, interval).Should(ContainElement("PlacementAPI"))
		})

		It("should be in a state of not having the input ready as the secrete is not create yet", func() {
			Eventually(func() condition.Condition {
				// TODO (mschuppert) change conditon package to be able to use haveSameStateOf Matcher here
				return placementAPI.GetCondition(condition.InputReadyCondition, condition.RequestedReason)
			}, timeout, interval).Should(HaveField("Status", corev1.ConditionFalse))
		})
	})

	When("an unrelated secret is provided", func() {
		It("should remain in a state of waiting for the proper secret", func() {
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "an-unrelated-secret",
					Namespace: TestNamespace,
				},
			}
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())

			Eventually(func() condition.Condition {
				return placementAPI.GetCondition(condition.InputReadyCondition, condition.RequestedReason)
			}, timeout, interval).Should(HaveField("Status", corev1.ConditionFalse))
		})
	})

	When("the proper secret is provided", func() {
		It("should not be in a state of having the input ready", func() {
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      SecretName,
					Namespace: TestNamespace,
				},
			}
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
			Eventually(func() condition.Condition {
				return placementAPI.GetCondition(condition.InputReadyCondition, condition.ReadyReason)
			}, timeout, interval).Should(HaveField("Status", corev1.ConditionTrue))
		})
	})

	When("keystoneAPI instance is available", func() {
		It("should create a ConfigMap for placement.conf with some config options set based on the KeystoneAPI", func() {
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      SecretName,
					Namespace: TestNamespace,
				},
			}
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())

			keystoneAPI = NewTestKeystoneAPI(TestNamespace)
			keystoneAPI.Create()

			configData := th.GetConfigMap(
				types.NamespacedName{
					Namespace: placementAPI.LookupKey.Namespace,
					Name:      fmt.Sprintf("%s-%s", placementAPI.LookupKey.Name, "config-data"),
				},
			)

			Eventually(configData).ShouldNot(BeNil())
			Expect(configData.Data["placement.conf"]).Should(
				ContainSubstring("auth_url = %s", keystoneAPI.Template.Status.APIEndpoints["internal"]))
			Expect(configData.Data["placement.conf"]).Should(
				ContainSubstring("www_authenticate_uri = %s", keystoneAPI.Template.Status.APIEndpoints["public"]))
		})
	})
})
