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
	"fmt"

	"github.com/go-logr/logr"
	routev1 "github.com/openshift/api/route/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	intstr "k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// RouteDetails -
type RouteDetails struct {
	Name      string
	Namespace string
	AppLabel  string
	Port      string
}

// Route func
func Route(routeInfo RouteDetails) *routev1.Route {

	serviceRef := routev1.RouteTargetReference{
		Kind: "Service",
		Name: routeInfo.Name,
	}
	routePort := &routev1.RoutePort{
		TargetPort: intstr.FromString(routeInfo.Port),
	}
	route := &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      routeInfo.Name,
			Namespace: routeInfo.Namespace,
			Labels:    GetLabels(routeInfo.Name, routeInfo.AppLabel),
		},
		Spec: routev1.RouteSpec{
			To:   serviceRef,
			Port: routePort,
		},
	}
	return route
}

// CreateOrUpdateRoute -
func CreateOrUpdateRoute(c client.Client, log logr.Logger, route *routev1.Route) error {
	// Check if this Route already exists
	foundRoute := &routev1.Route{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: route.Name, Namespace: route.Namespace}, foundRoute)
	if err != nil && !k8s_errors.IsNotFound(err) {
		return fmt.Errorf("error getting route object: %v", err)
	}

	if k8s_errors.IsNotFound(err) {
		log.Info("Creating a new Route", "Route.Namespace", route.Namespace, "Route.Name", route.Name)
		err = c.Create(context.TODO(), route)
		if err != nil {
			return fmt.Errorf("error creating route object: %v", err)
		}
	} else {
		route.ResourceVersion = foundRoute.ResourceVersion
		err = c.Update(context.TODO(), route)
		if err != nil && !k8s_errors.IsNotFound(err) {
			return fmt.Errorf("error updating route object: %v", err)
		}
		return err
	}

	return nil
}
