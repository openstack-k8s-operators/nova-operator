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

func DeleteNovaAPI(lookupKey types.NamespacedName) {
	novaAPI := GetNovaAPI(lookupKey)
	Expect(k8sClient.Delete(ctx, novaAPI)).Should(Succeed())
	// We have to wait for the controller to fully delete the instance
	Eventually(func() bool {
		err := k8sClient.Get(ctx, lookupKey, novaAPI)
		return k8s_errors.IsNotFound(err)
	}, timeout, interval).Should(BeTrue())
}

func GetNovaAPI(lookupKey types.NamespacedName) *novav1.NovaAPI {
	instance := &novav1.NovaAPI{}
	Eventually(func() bool {
		err := k8sClient.Get(ctx, lookupKey, instance)
		return err == nil
	}, timeout, interval).Should(BeTrue())
	return instance
}

func GetConditionByType(
	lookupKey types.NamespacedName,
	conditionType condition.Type,
) *condition.Condition {
	instance := GetNovaAPI(lookupKey)

	if instance.Status.Conditions == nil {
		return nil
	}
	return instance.Status.Conditions.Get(conditionType)
}
