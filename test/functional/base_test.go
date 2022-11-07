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
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	mariadbv1 "github.com/openstack-k8s-operators/mariadb-operator/api/v1beta1"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
)

const (
	SecretName     = "test-secret"
	ContainerImage = "test://nova"

	interval = time.Millisecond * 10
	timeout  = interval * 200
	// consistencyTimeout is the amount of time we use to repeatedly check
	// that a condition is still valid. This is intendet to be used in
	// asserts using `Consistently`.
	consistencyTimeout = interval * 200
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
		g.Expect(err).Should(BeNil())

		g.Expect(k8sClient.Delete(ctx, novaAPI)).Should(Succeed())

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

func NovaAPINotExists(name types.NamespacedName) {
	Consistently(func(g Gomega) {
		instance := &novav1.NovaAPI{}
		err := k8sClient.Get(ctx, name, instance)
		g.Expect(k8s_errors.IsNotFound(err)).To(BeTrue())
	}, consistencyTimeout, interval).Should(Succeed())
}

type conditionsGetter interface {
	GetConditions(name types.NamespacedName) condition.Conditions
}

type conditionGetterFunc func(name types.NamespacedName) condition.Conditions

func (f conditionGetterFunc) GetConditions(name types.NamespacedName) condition.Conditions {
	return f(name)
}

func NovaAPIConditionGetter(name types.NamespacedName) condition.Conditions {
	instance := GetNovaAPI(name)
	return instance.Status.Conditions
}

func ExpectCondition(
	name types.NamespacedName,
	getter conditionsGetter,
	conditionType condition.Type,
	expectedStatus corev1.ConditionStatus,
) {
	logger.Info("ExpectCondition", "type", conditionType, "expected status", expectedStatus, "on", name)
	Eventually(func(g Gomega) {
		conditions := getter.GetConditions(name)
		g.Expect(conditions).NotTo(
			BeNil(), "Conditions in nil")
		g.Expect(conditions.Has(conditionType)).To(
			BeTrue(), "Does not have condition type %s", conditionType)
		actual := conditions.Get(conditionType).Status
		g.Expect(actual).To(
			Equal(expectedStatus),
			"%s condition is in an unexpected state. Expected: %s, Actual: %s, instance name: %s, Conditions: %v",
			conditionType, expectedStatus, actual, name, conditions)
	}, timeout, interval).Should(Succeed())
	logger.Info("ExpectCondition succeeded", "type", conditionType, "expected status", expectedStatus, "on", name)
}

func ExpectConditionWithDetails(
	name types.NamespacedName,
	getter conditionsGetter,
	conditionType condition.Type,
	expectedStatus corev1.ConditionStatus,
	expectedReason condition.Reason,
	expecteMessage string,
) {
	logger.Info("ExpectConditionWithDetails", "type", conditionType, "expected status", expectedStatus, "on", name)
	Eventually(func(g Gomega) {
		conditions := getter.GetConditions(name)
		g.Expect(conditions).NotTo(
			BeNil(), "Status.Conditions in nil")
		g.Expect(conditions.Has(conditionType)).To(
			BeTrue(), "Condition type is not in Status.Conditions %s", conditionType)
		actualCondition := conditions.Get(conditionType)
		g.Expect(actualCondition.Status).To(
			Equal(expectedStatus),
			"%s condition is in an unexpected state. Expected: %s, Actual: %s",
			conditionType, expectedStatus, actualCondition.Status)
		g.Expect(actualCondition.Reason).To(
			Equal(expectedReason),
			"%s condition has a different reason. Actual condition: %v", conditionType, actualCondition)
		g.Expect(actualCondition.Message).To(
			Equal(expecteMessage),
			"%s condition has a different message. Actual condition: %v", conditionType, actualCondition)
	}, timeout, interval).Should(Succeed())

	logger.Info("ExpectConditionWithDetails succeeded", "type", conditionType, "expected status", expectedStatus, "on", name)
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
	Eventually(func(g Gomega) {
		job := GetJob(name)

		// NOTE(gibi) when run against a real env we need to find a
		// better way to make the job fail. This works but it is unreal.

		// Simulate that the job is failed
		job.Status.Failed = 1
		job.Status.Active = 0
		// This can return conflict so we need the Eventually block to retry
		g.Expect(k8sClient.Status().Update(ctx, job)).To(Succeed())
	}, timeout, interval).Should(Succeed())
	logger.Info("Simulated job failure", "on", name)
}

func SimulateJobSuccess(name types.NamespacedName) {
	Eventually(func(g Gomega) {

		job := GetJob(name)
		// NOTE(gibi): We don't need to do this when run against a real
		// env as there the job could run successfully automatically if the
		// database user is registered manually in the DB service. But for that
		// we would need another set of test setup, i.e. deploying the
		// mariadb-operator.

		// Simulate that the job is succeeded
		job.Status.Succeeded = 1
		job.Status.Active = 0
		// This can return conflict so we need the Eventually block to retry
		g.Expect(k8sClient.Status().Update(ctx, job)).To(Succeed())
	}, timeout, interval).Should(Succeed())
	logger.Info("Simulated job success", "on", name)
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
	Eventually(func(g Gomega) {
		deployment := GetDeployment(name)
		// NOTE(gibi): We don't need to do this when run against a real
		// env as there the deployment could reach the ready state automatically.
		// But for that  we would need another set of test setup, i.e. deploying
		// the mariadb-operator.

		deployment.Status.Replicas = 1
		deployment.Status.ReadyReplicas = 1
		g.Expect(k8sClient.Status().Update(ctx, deployment)).To(Succeed())

	}, timeout, interval).Should(Succeed())
	logger.Info("Simulated deployment success", "on", name)
}

func CreateNova(name types.NamespacedName, spec novav1.NovaSpec) {
	nova := &novav1.Nova{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "nova.openstack.org/v1beta1",
			Kind:       "Nova",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.Name,
			Namespace: name.Namespace,
		},
		Spec: spec,
	}

	Expect(k8sClient.Create(ctx, nova)).Should(Succeed())
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
		g.Expect(err).Should(BeNil())

		g.Expect(k8sClient.Delete(ctx, nova)).Should(Succeed())

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

func NovaConditionGetter(name types.NamespacedName) condition.Conditions {
	instance := GetNova(name)
	return instance.Status.Conditions
}

// CreateDBService creates a k8s Service object that matches with the
// expectations of lib-common database module as a Service for the MariaDB
func CreateDBService(namespace string, mariadbCRName string, spec corev1.ServiceSpec) types.NamespacedName {
	// The Name is used as the hostname to access the service. So
	// we generate something unique for the MariaDB CR it represents
	// so we can assert that the correct Service is selected.
	serviceName := "hostname-for-" + mariadbCRName
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: namespace,
			// NOTE(gibi): The lib-common databvase module looks up the
			// Service exposed by MariaDB via these labels.
			Labels: map[string]string{
				"app": "mariadb",
				"cr":  "mariadb-" + mariadbCRName,
			},
		},
		Spec: spec,
	}
	Expect(k8sClient.Create(ctx, service)).Should(Succeed())

	return types.NamespacedName{Name: serviceName, Namespace: namespace}
}

func DeleteDBService(name types.NamespacedName) {
	Eventually(func(g Gomega) {
		service := &corev1.Service{}
		err := k8sClient.Get(ctx, name, service)
		// if it is already gone that is OK
		if k8s_errors.IsNotFound(err) {
			return
		}
		g.Expect(err).Should(BeNil())

		g.Expect(k8sClient.Delete(ctx, service)).Should(Succeed())

		err = k8sClient.Get(ctx, name, service)
		g.Expect(k8s_errors.IsNotFound(err)).To(BeTrue())
	}, timeout, interval).Should(Succeed())
}

func GetMariaDBDatabase(name types.NamespacedName) *mariadbv1.MariaDBDatabase {
	instance := &mariadbv1.MariaDBDatabase{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}

func ListMariaDBDatabase(namespace string) *mariadbv1.MariaDBDatabaseList {
	mariaDBDatabases := &mariadbv1.MariaDBDatabaseList{}
	Expect(k8sClient.List(ctx, mariaDBDatabases, client.InNamespace(namespace))).Should(Succeed())
	return mariaDBDatabases
}

func SimulateMariaDBDatabaseCompleted(name types.NamespacedName) {
	Eventually(func(g Gomega) {
		db := GetMariaDBDatabase(name)
		db.Status.Completed = true
		// This can return conflict so we have the Eventually block to retry
		g.Expect(k8sClient.Status().Update(ctx, db)).To(Succeed())

	}, timeout, interval).Should(Succeed())

	logger.Info("Simulated DB completed", "on", name)
}

func CreateNovaConductor(namespace string, spec novav1.NovaConductorSpec) types.NamespacedName {
	novaConductorName := uuid.New().String()
	novaConductor := &novav1.NovaConductor{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "nova.openstack.org/v1beta1",
			Kind:       "NovaConductor",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      novaConductorName,
			Namespace: namespace,
		},
		Spec: spec,
	}

	Expect(k8sClient.Create(ctx, novaConductor)).Should(Succeed())

	return types.NamespacedName{Name: novaConductorName, Namespace: namespace}
}

func DeleteNovaConductor(name types.NamespacedName) {
	// We have to wait for the controller to fully delete the instance
	Eventually(func(g Gomega) {
		novaConductor := &novav1.NovaConductor{}
		err := k8sClient.Get(ctx, name, novaConductor)
		// if it is already gone that is OK
		if k8s_errors.IsNotFound(err) {
			return
		}
		g.Expect(err).Should(BeNil())

		g.Expect(k8sClient.Delete(ctx, novaConductor)).Should(Succeed())

		err = k8sClient.Get(ctx, name, novaConductor)
		g.Expect(k8s_errors.IsNotFound(err)).To(BeTrue())
	}, timeout, interval).Should(Succeed())
}

func GetNovaConductor(name types.NamespacedName) *novav1.NovaConductor {
	instance := &novav1.NovaConductor{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}

func NovaConductorConditionGetter(name types.NamespacedName) condition.Conditions {
	instance := GetNovaConductor(name)
	return instance.Status.Conditions
}

// CreateNovaConductorSecret creates a secret that has all the information
// NovaConductor needs
func CreateNovaConductorSecret(namespace string, name string) *corev1.Secret {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"NovaCell0DatabasePassword": []byte("12345678"),
		},
	}
	Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
	return secret
}

func CreateNovaCell(namespace string, spec novav1.NovaCellSpec) types.NamespacedName {
	novaCellName := uuid.New().String()
	novaCell := &novav1.NovaCell{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "nova.openstack.org/v1beta1",
			Kind:       "NovaCell",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      novaCellName,
			Namespace: namespace,
		},
		Spec: spec,
	}

	Expect(k8sClient.Create(ctx, novaCell)).Should(Succeed())

	return types.NamespacedName{Name: novaCellName, Namespace: namespace}
}

func DeleteNovaCell(name types.NamespacedName) {
	// We have to wait for the controller to fully delete the instance
	Eventually(func(g Gomega) {
		novaCell := &novav1.NovaCell{}
		err := k8sClient.Get(ctx, name, novaCell)
		// if it is already gone that is OK
		if k8s_errors.IsNotFound(err) {
			return
		}
		g.Expect(err).Should(BeNil())

		g.Expect(k8sClient.Delete(ctx, novaCell)).Should(Succeed())

		err = k8sClient.Get(ctx, name, novaCell)
		g.Expect(k8s_errors.IsNotFound(err)).To(BeTrue())
	}, timeout, interval).Should(Succeed())
}

func GetNovaCell(name types.NamespacedName) *novav1.NovaCell {
	instance := &novav1.NovaCell{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}

func NovaCellNotExists(name types.NamespacedName) {
	Consistently(func(g Gomega) {
		instance := &novav1.NovaCell{}
		err := k8sClient.Get(ctx, name, instance)
		g.Expect(k8s_errors.IsNotFound(err)).To(BeTrue())
	}, consistencyTimeout, interval).Should(Succeed())
}

func NovaCellConditionGetter(name types.NamespacedName) condition.Conditions {
	instance := GetNovaCell(name)
	return instance.Status.Conditions
}

func CreateNovaSecret(namespace string, name string) *corev1.Secret {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"NovaPassword":              []byte("12345678"),
			"NovaAPIDatabasePassword":   []byte("12345678"),
			"NovaAPIMessageBusPassword": []byte("12345678"),
			"NovaCell0DatabasePassword": []byte("12345678"),
		},
	}
	Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
	return secret
}

func GetStatefulSet(name types.NamespacedName) *appsv1.StatefulSet {
	ss := &appsv1.StatefulSet{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, ss)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return ss
}

func ListStatefulSets(namespace string) *appsv1.StatefulSetList {
	sss := &appsv1.StatefulSetList{}
	Expect(k8sClient.List(ctx, sss, client.InNamespace(namespace))).Should(Succeed())
	return sss

}

func SimulateStatefulSetReplicaReady(name types.NamespacedName) {
	Eventually(func(g Gomega) {
		ss := GetStatefulSet(name)
		ss.Status.Replicas = 1
		ss.Status.ReadyReplicas = 1
		g.Expect(k8sClient.Status().Update(ctx, ss)).To(Succeed())

	}, timeout, interval).Should(Succeed())
	logger.Info("Simulated statefulset success", "on", name)
}
