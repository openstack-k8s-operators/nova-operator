package common

import (
	"context"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
)

// GetAllPodsWithLabel - get all pods from namespace with a specific label
func GetAllPodsWithLabel(kclient kubernetes.Interface, log logr.Logger, labelSelectorMap map[string]string, namespace string) (*corev1.PodList, error) {
	labelSelectorString := labels.Set(labelSelectorMap).String()

	podList, err := kclient.CoreV1().Pods(namespace).List(
		context.TODO(),
		metav1.ListOptions{
			LabelSelector: labelSelectorString,
		},
	)
	if err != nil {
		return podList, err
	}

	return podList, nil
}

// DeleteNamespacedPod -
func DeleteNamespacedPod(kclient kubernetes.Interface, log logr.Logger, pod *corev1.Pod) error {

	err := kclient.CoreV1().Pods(pod.Namespace).Delete(
		context.TODO(),
		pod.Name,
		metav1.DeleteOptions{},
	)
	if err != nil && !k8s_errors.IsNotFound(err) {
		return err
	}

	return nil
}

// DeleteAllNamespacedPodsWithLabel -
func DeleteAllNamespacedPodsWithLabel(kclient kubernetes.Interface, log logr.Logger, labelSelectorMap map[string]string, namespace string) error {
	labelSelectorString := labels.Set(labelSelectorMap).String()

	err := kclient.CoreV1().Pods(namespace).DeleteCollection(
		context.TODO(),
		metav1.DeleteOptions{},
		metav1.ListOptions{
			LabelSelector: labelSelectorString,
		},
	)

	if err != nil && !k8s_errors.IsNotFound(err) {
		return err
	}

	return nil
}
