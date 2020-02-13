package controller

import (
	"github.com/openstack-k8s-operators/nova-operator/pkg/controller/novamigrationtarget"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, novamigrationtarget.Add)
}
