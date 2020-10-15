/*
Copyright 2020 Red Hat

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

package common

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	util "github.com/openstack-k8s-operators/lib-common/pkg/util"
	novav1beta1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// BITSIZE
const (
	BITSIZE int = 4096
)

// GetSecretsFromCR - get all parameters ending with "Secret" from Spec to verify they exist, add the hash to env and status
func GetSecretsFromCR(r ReconcilerCommon, obj runtime.Object, namespace string, spec interface{}, envVars *map[string]util.EnvSetter) ([]novav1beta1.Hash, error) {
	hashes := []novav1beta1.Hash{}
	specParameters := make(map[string]interface{})
	inrec, _ := json.Marshal(spec)
	json.Unmarshal(inrec, &specParameters)

	for param, value := range specParameters {
		if strings.HasSuffix(param, "Secret") {
			_, hash, err := GetSecret(r.GetClient(), fmt.Sprintf("%v", value), namespace)
			if err != nil {
				return nil, err
			}

			// add hash to envVars
			(*envVars)[param] = util.EnvValue(hash)
			hashes = append(hashes, novav1beta1.Hash{Name: param, Hash: hash})
		}
	}

	return hashes, nil
}

// GetSecret -
func GetSecret(c client.Client, secretName string, secretNamespace string) (*corev1.Secret, string, error) {
	secret := &corev1.Secret{}

	err := c.Get(context.TODO(), types.NamespacedName{Name: secretName, Namespace: secretNamespace}, secret)
	if err != nil {
		return nil, "", err
	}

	secretHash, err := util.ObjectHash(secret)
	if err != nil {
		return nil, "", fmt.Errorf("error calculating configuration hash: %v", err)
	}
	return secret, secretHash, nil
}

// CreateOrUpdateSecret -
func CreateOrUpdateSecret(r ReconcilerCommon, obj metav1.Object, secret *corev1.Secret) (string, controllerutil.OperationResult, error) {

	op, err := controllerutil.CreateOrUpdate(context.TODO(), r.GetClient(), secret, func() error {

		err := controllerutil.SetControllerReference(obj, secret, r.GetScheme())
		if err != nil {
			return err
		}

		return nil
	})

	secretHash, err := util.ObjectHash(secret)
	if err != nil {
		return "", "", fmt.Errorf("error calculating configuration hash: %v", err)
	}

	return secretHash, op, err
}

// SSHKeySecret - func
func SSHKeySecret(name string, namespace string, labels map[string]string) (*corev1.Secret, error) {

	privateKey, err := util.GeneratePrivateKey(BITSIZE)
	if err != nil {
		return nil, err
	}

	publicKey, err := util.GeneratePublicKey(&privateKey.PublicKey)
	if err != nil {
		return nil, err
	}

	privateKeyPem := util.EncodePrivateKeyToPEM(privateKey)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Type: "Opaque",
		StringData: map[string]string{
			"identity":        privateKeyPem,
			"authorized_keys": publicKey,
		},
	}
	return secret, nil
}
