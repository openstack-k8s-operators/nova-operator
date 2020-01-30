package controller

import (
	"github.com/nova-operator/pkg/controller/virtlogd"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, virtlogd.Add)
}
