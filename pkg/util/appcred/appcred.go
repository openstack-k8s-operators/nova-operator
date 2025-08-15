package appcred

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	SecretKeyID     = "AC_ID"
	SecretKeySecret = "AC_SECRET"
)

var (
	// ErrMissingSecretKey is returned when a required key is missing from the secret
	ErrMissingSecretKey = errors.New("secret missing required key")
)

// LoadSecret fetches the Secret and returns the two required fields.
func LoadSecret(ctx context.Context, c client.Client, nn types.NamespacedName) (id string, secret string, secretObj *corev1.Secret, err error) {
	s := &corev1.Secret{}
	if err := c.Get(ctx, nn, s); err != nil {
		return "", "", nil, fmt.Errorf("get secret %s/%s: %w", nn.Namespace, nn.Name, err)
	}
	idBytes, ok := s.Data[SecretKeyID]
	if !ok {
		return "", "", s, fmt.Errorf("secret %s/%s missing key %q: %w", nn.Namespace, nn.Name, SecretKeyID, ErrMissingSecretKey)
	}
	secretBytes, ok := s.Data[SecretKeySecret]
	if !ok {
		return "", "", s, fmt.Errorf("secret %s/%s missing key %q: %w", nn.Namespace, nn.Name, SecretKeySecret, ErrMissingSecretKey)
	}
	return string(idBytes), string(secretBytes), s, nil
}

// HashSecret returns a short stable hash of the two values that drive Deployment rollouts.
func HashSecret(id, secret string) string {
	h := sha256.Sum256([]byte(id + "\x00" + secret))
	return hex.EncodeToString(h[:])
}

// LogACInUse prints a uniform log line across reconcilers.
func LogACInUse(logger logr.Logger, service string) {
	logger.Info("Using ApplicationCredentials auth", "service", service)
}
