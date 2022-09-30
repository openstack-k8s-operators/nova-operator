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
	"strconv"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
)

const (
	SecretName     = "test-secret"
	ContainerImage = "test://nova-api"

	timeout  = time.Second * 10
	interval = time.Millisecond * 200
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

func ExpectNovaAPIConditionWithDetails(
	name types.NamespacedName,
	conditionType condition.Type,
	expectedStatus corev1.ConditionStatus,
	expectedReason condition.Reason,
	expecteMessage string,
) {
	Eventually(func(g Gomega) {
		instance := GetNovaAPI(name)
		g.Expect(instance.Status.Conditions).NotTo(
			BeNil(), "NovaAPI.Status.Conditions in nil")
		g.Expect(instance.Status.Conditions.Has(conditionType)).To(
			BeTrue(), "NovaAPI does not have condition type %s", conditionType)
		actual_condition := instance.Status.Conditions.Get(conditionType)
		g.Expect(actual_condition.Status).To(
			Equal(expectedStatus),
			"NovaAPI %s condition is in an unexpected state. Expected: %s, Actual: %s",
			conditionType, expectedStatus, actual_condition.Status)
		g.Expect(actual_condition.Reason).To(Equal(expectedReason))
		g.Expect(actual_condition.Message).To(Equal(expecteMessage))
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

// CreateSecret creates a secret that has all the information NovaAPI needs
func CreateNovaAPISecret(namespace string, name string) *corev1.Secret {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"NovaPassword":              []byte("12345678"),
			"NovaAPIDatabasePassword":   []byte("12345678"),
			"NovaAPIMessageBusPassword": []byte("12345678"),
		},
	}
	Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
	return secret
}

func GetJob(name types.NamespacedName) *batchv1.Job {
	job := &batchv1.Job{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, job)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return job
}

func ListJobs(namespace string) *batchv1.JobList {
	jobs := &batchv1.JobList{}
	Expect(k8sClient.List(ctx, jobs, client.InNamespace(namespace))).Should(Succeed())
	return jobs

}

func SimulateJobFailure(name types.NamespacedName) {
	job := GetJob(name)

	// NOTE(gibi) when run against a real env we need to find a
	// better way to make the job fail. This works but it is unreal.

	// Simulate that the job is failed
	job.Status.Failed = 1
	job.Status.Active = 0
	Expect(k8sClient.Status().Update(ctx, job)).To(Succeed())
}

func SimulateJobSuccess(name types.NamespacedName) {
	job := GetJob(name)
	// NOTE(gibi): We don't need to do this when run against a real
	// env as there the job could run successfully automatically if the
	// database user is registered manually in the DB service. But for that
	// we would need another set of test setup, i.e. deploying the
	// mariadb-operator.

	// Simulate that the job is succeeded
	job.Status.Succeeded = 1
	job.Status.Active = 0
	Expect(k8sClient.Status().Update(ctx, job)).To(Succeed())
}

func GetDeployment(name types.NamespacedName) *appsv1.Deployment {
	deployment := &appsv1.Deployment{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, deployment)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return deployment
}

func ListDeployments(namespace string) *appsv1.DeploymentList {
	deployments := &appsv1.DeploymentList{}
	Expect(k8sClient.List(ctx, deployments, client.InNamespace(namespace))).Should(Succeed())
	return deployments

}

func SimulateDeploymentReplicaReady(name types.NamespacedName) {
	deployment := GetDeployment(name)
	// NOTE(gibi): We don't need to do this when run against a real
	// env as there the deployment could reach the ready state automatically.
	// But for that  we would need another set of test setup, i.e. deploying
	// the mariadb-operator.

	deployment.Status.Replicas = 1
	deployment.Status.ReadyReplicas = 1
	Expect(k8sClient.Status().Update(ctx, deployment)).To(Succeed())
}

func SkipInExistingCluster(message string) {
	s := os.Getenv("USE_EXISTING_CLUSTER")
	v, err := strconv.ParseBool(s)

	if err == nil && v {
		Skip("Skipped running against existing cluster. " + message)
	}

}

func CreateNova(namespace string, spec novav1.NovaSpec) types.NamespacedName {
	novaName := uuid.New().String()
	nova := &novav1.Nova{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "nova.openstack.org/v1beta1",
			Kind:       "Nova",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      novaName,
			Namespace: namespace,
		},
		Spec: spec,
	}

	Expect(k8sClient.Create(ctx, nova)).Should(Succeed())

	return types.NamespacedName{Name: novaName, Namespace: namespace}
}

func DeleteNova(name types.NamespacedName) {
	// We have to wait for the controller to fully delete the instance
	Eventually(func(g Gomega) {
		nova := &novav1.Nova{}
		err := k8sClient.Get(ctx, name, nova)
		// if it is already gone that is OK
		if k8s_errors.IsNotFound(err) {
			return
		}
		Expect(err).Should(BeNil())

		Expect(k8sClient.Delete(ctx, nova)).Should(Succeed())

		err = k8sClient.Get(ctx, name, nova)
		g.Expect(k8s_errors.IsNotFound(err)).To(BeTrue())
	}, timeout, interval).Should(Succeed())
}

func GetNova(name types.NamespacedName) *novav1.Nova {
	instance := &novav1.Nova{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}

type NovaUnderTest struct {
}

func (n NovaUnderTest) GetConditions(name types.NamespacedName) condition.Conditions {
	return GetNova(name).Status.Conditions
}

type conditionsGetter interface {
	GetConditions(name types.NamespacedName) condition.Conditions
}

type conditionGetterFunc func(name types.NamespacedName) condition.Conditions

func (f conditionGetterFunc) GetConditions(name types.NamespacedName) condition.Conditions {
	return f(name)
}
func NovaConditionGetter(name types.NamespacedName) condition.Conditions {
	instance := GetNova(name)
	return instance.Status.Conditions
}

func ExpectCondition(
	name types.NamespacedName,
	getter conditionsGetter,
	conditionType condition.Type,
	expectedStatus corev1.ConditionStatus,
) {
	Eventually(func(g Gomega) {
		conditions := getter.GetConditions(name)
		g.Expect(conditions).NotTo(
			BeNil(), "Conditions in nil")
		g.Expect(conditions.Has(conditionType)).To(
			BeTrue(), "Does not have condition type %s", conditionType)
		actual := conditions.Get(conditionType).Status
		g.Expect(actual).To(
			Equal(expectedStatus),
			"%s condition is in an unexpected state. Expected: %s, Actual: %s",
			conditionType, expectedStatus, actual)
	}, timeout, interval).Should(Succeed())
}
