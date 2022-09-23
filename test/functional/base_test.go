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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
)

func CreateNamespace(name string) (*corev1.Namespace, error) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	err := k8sClient.Create(ctx, ns)
	return ns, err
}

func CreateNovaAPI(spec novav1.NovaAPISpec) (types.NamespacedName, error) {
	novaAPIName := uuid.New().String()
	novaAPI := &novav1.NovaAPI{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "nova.openstack.org/v1beta1",
			Kind:       "NovaAPI",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      novaAPIName,
			Namespace: Namespace,
		},
		Spec: spec,
	}

	err := k8sClient.Create(ctx, novaAPI)

	return types.NamespacedName{Name: novaAPIName, Namespace: Namespace}, err
}

func GetNovaAPI(lookupKey types.NamespacedName) (*novav1.NovaAPI, error) {
	instance := &novav1.NovaAPI{}
	err := k8sClient.Get(ctx, lookupKey, instance)
	return instance, err
}

func GetConditionByType(
	lookupKey types.NamespacedName,
	conditionType condition.Type,
) (*condition.Condition, error) {
	instance, err := GetNovaAPI(lookupKey)

	if err != nil || instance.Status.Conditions == nil {
		return nil, err
	}
	return instance.Status.Conditions.Get(conditionType), err
}
