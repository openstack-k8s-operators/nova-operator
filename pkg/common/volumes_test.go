package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVolumes(t *testing.T) {
	assert := assert.New(t)
	for _, volConfig := range GetVolumes("some_name") {
		if volConfig.Name == "kolla-config" {
			assert.Equal(volConfig.VolumeSource.ConfigMap.LocalObjectReference.Name, "some_name-templates")
		}
	}
}
