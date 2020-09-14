package common

import (
	"context"
	"fmt"
	"reflect"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/go-logr/logr"
	util "github.com/openstack-k8s-operators/lib-common/pkg/util"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
)

// CreateOrUpdateConfigMap -
func CreateOrUpdateConfigMap(c client.Client, log logr.Logger, configMap *corev1.ConfigMap) error {
	// Check if this configMap already exists
	foundConfigMap := &corev1.ConfigMap{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: configMap.Name, Namespace: configMap.Namespace}, foundConfigMap)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating a new ConfigMap", "ConfigMap.Namespace", configMap.Namespace, "ConfigMap.Name", configMap.Name)
		err = c.Create(context.TODO(), configMap)
		if err != nil {
			return err
		}
	} else if !reflect.DeepEqual(configMap.Data, foundConfigMap.Data) {
		log.Info(fmt.Sprintf("Updating ConfigMap: %s", configMap.Name))
		foundConfigMap.Data = configMap.Data
		err = c.Update(context.TODO(), foundConfigMap)
		if err != nil {
			return err
		}
	}
	return nil
}

// CreateOrGetCustomConfigMap -
func CreateOrGetCustomConfigMap(c client.Client, log logr.Logger, configMap *corev1.ConfigMap) error {
	// Check if this configMap already exists
	foundConfigMap := &corev1.ConfigMap{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: configMap.Name, Namespace: configMap.Namespace}, foundConfigMap)
	if err != nil && k8s_errors.IsNotFound(err) {
		log.Info("Creating a new ConfigMap", "ConfigMap.Namespace", configMap.Namespace, "ConfigMap.Name", configMap.Name)
		err = c.Create(context.TODO(), configMap)
		if err != nil {
			return err
		}
	} else {
		// use data from already existing custom configmap
		configMap.Data = foundConfigMap.Data
	}
	return nil
}

// GetConfigMapAndHashWithName -
func GetConfigMapAndHashWithName(c client.Client, log logr.Logger, configMapName string, namespace string) (*corev1.ConfigMap, string, error) {

	configMap := &corev1.ConfigMap{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: configMapName, Namespace: namespace}, configMap)
	if err != nil && errors.IsNotFound(err) {
		log.Error(err, configMapName+" ConfigMap not found!", "Instance.Namespace", namespace, "ConfigMap.Name", configMapName)
		return configMap, "", err
	}
	configMapHash, err := util.ObjectHash(configMap)
	if err != nil {
		return configMap, "", fmt.Errorf("error calculating configuration hash: %v", err)
	}
	return configMap, configMapHash, nil
}
