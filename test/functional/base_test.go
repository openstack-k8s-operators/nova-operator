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
	"github.com/google/uuid"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
)

func CreateNamespace(name string) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	Expect(k8sClient.Create(ctx, ns)).Should(Succeed())
}

func DeleteNamespace(name string) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	Expect(k8sClient.Delete(ctx, ns)).Should(Succeed())
}

func CreateNovaAPI(namespace string, spec novav1.NovaAPISpec) types.NamespacedName {
	novaAPIName := uuid.New().String()
	novaAPI := &novav1.NovaAPI{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "nova.openstack.org/v1beta1",
			Kind:       "NovaAPI",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      novaAPIName,
			Namespace: namespace,
		},
		Spec: spec,
	}

	Expect(k8sClient.Create(ctx, novaAPI)).Should(Succeed())

	return types.NamespacedName{Name: novaAPIName, Namespace: namespace}
}

func DeleteNovaAPI(name types.NamespacedName) {
	// We have to wait for the controller to fully delete the instance
	Eventually(func(g Gomega) {
		novaAPI := &novav1.NovaAPI{}
		err := k8sClient.Get(ctx, name, novaAPI)
		// if it is already gone that is OK
		if k8s_errors.IsNotFound(err) {
			return
		}
		Expect(err).Should(BeNil())

		Expect(k8sClient.Delete(ctx, novaAPI)).Should(Succeed())

		err = k8sClient.Get(ctx, name, novaAPI)
		g.Expect(k8s_errors.IsNotFound(err)).To(BeTrue())
	}, timeout, interval).Should(Succeed())
}

func GetNovaAPI(name types.NamespacedName) *novav1.NovaAPI {
	instance := &novav1.NovaAPI{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}

func ExpectNovaAPICondition(
	name types.NamespacedName,
	conditionType condition.Type,
	expectedStatus corev1.ConditionStatus,
) {
	Eventually(func(g Gomega) {
		instance := GetNovaAPI(name)
		g.Expect(instance.Status.Conditions).NotTo(
			BeNil(), "NovaAPI.Status.Conditions in nil")
		g.Expect(instance.Status.Conditions.Has(conditionType)).To(
			BeTrue(), "NovaAPI does not have condition type %s", conditionType)
		actual := instance.Status.Conditions.Get(conditionType).Status
		g.Expect(actual).To(
			Equal(expectedStatus),
			"NovaAPI %s condition is in an unexpected state. Expected: %s, Actual: %s",
			conditionType, expectedStatus, actual)
	}, timeout, interval).Should(Succeed())
}

func GetConfigMap(name types.NamespacedName) corev1.ConfigMap {
	cm := &corev1.ConfigMap{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, cm)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return *cm
}

func ListConfigMaps(namespace string) corev1.ConfigMapList {
	cms := &corev1.ConfigMapList{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.List(ctx, cms, client.InNamespace(namespace))).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return *cms

}
