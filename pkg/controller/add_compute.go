package controller

import (
	"github.com/nova-operator/pkg/controller/compute"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, compute.Add)
}
