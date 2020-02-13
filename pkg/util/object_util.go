package util

import (
        "fmt"
        "k8s.io/apimachinery/pkg/util/rand"
        "crypto/sha256"
	"encoding/json"
)

// create a deep object hash and return it as a safe encoded string
func ObjectHash(i interface{}) (string, error) {
        // Convert the hashSource to a byte slice so that it can be hashed
        hashBytes, err := json.Marshal(i)
	if err != nil {
		return "", fmt.Errorf("unable to convert to JSON: %v", err)
	}
        hash := sha256.Sum256(hashBytes)
        return rand.SafeEncodeString(fmt.Sprint(hash)), nil
}
