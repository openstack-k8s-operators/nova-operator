package common

import (
	"context"
	"fmt"

	routev1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// AppLabels - List of all app: labels corresponding to an OSP ctlplane service to
// create the endpoint map
var AppLabels = []string{
	"glance-api",
	"keystone-api",
	"neutron-api",
	"nova-api",
	"placement",
}

// App labels for all ctlplane service which can be used to access the endpoint
// details of the map, like e.g. OSPEndpoints[common.NovaAPIAppLabel].IP
const (
	GlanceAPIAppLabel    = "glance-api"
	KeystoneAPIAppLabel  = "keystone-api"
	NeutronAPIAppLabel   = "neutron-api"
	NovaAPIAppLabel      = "nova-api"
	PlacementAPIAppLabel = "placement"
)

// OSPEndpoint -  struct which describes an OSP endpoint
type OSPEndpoint struct {
	Name        string
	IP          string
	Port        string
	InternalURL string
	PublicURL   string
}

//GetAllOspEndpoints - get endpoint map of all endpoints identified by applabel from route/svc
func GetAllOspEndpoints(c client.Client, namespace string) (map[string]OSPEndpoint, error) {
	var endpoint OSPEndpoint
	var ospEndpoints = make(map[string]OSPEndpoint)

	for _, label := range AppLabels {
		err := endpoint.InitEndpoint(c, label, namespace)
		if err != nil {
			return nil, err
		}
		ospEndpoints[label] = endpoint
	}
	return ospEndpoints, nil
}

// InitEndpoint - initilize osp endpoint from service/route information identified by label, namespace
// requires import of openshift route scheme, e.g. in main.go:
//       if err := routev1.AddToScheme(mgr.GetScheme()); err != nil {
//               log.Error(err, "")
//               os.Exit(1)
//       }
func (e *OSPEndpoint) InitEndpoint(c client.Client, label string, namespace string) error {
	err := e.setEndpointName(c, label, namespace)
	if err != nil {
		return err
	}

	err = e.setEndpointClusterIP(c, label, namespace)
	if err != nil {
		return err
	}

	err = e.setEndpointPort(c, label, namespace)
	if err != nil {
		return err
	}

	// TODO: mschuppert handle https
	e.InternalURL = fmt.Sprintf("http://%s.%s.svc:%s", e.Name, namespace, e.Port)

	err = e.setPublicEndpoint(c, label, namespace)
	if err != nil {
		return err
	}
	return nil
}

// setServiceName - set service name from service with label, namespace
func (e *OSPEndpoint) setEndpointName(c client.Client, label string, namespace string) error {
	service, err := getService(c, label, namespace)
	if err != nil {
		return err
	}

	e.Name = service.ObjectMeta.Name
	return nil
}

// setServicePort - set service port from service with label, namespace
func (e *OSPEndpoint) setEndpointPort(c client.Client, label string, namespace string) error {
	service, err := getService(c, label, namespace)
	if err != nil {
		return err
	}

	port := ""
	for _, p := range service.Spec.Ports {
		port = fmt.Sprintf("%d", p.Port)
	}

	e.Port = port
	return nil
}

// setServiceClusterIP - set clusterIP from service with label, namespace
func (e *OSPEndpoint) setEndpointClusterIP(c client.Client, label string, namespace string) error {
	service, err := getService(c, label, namespace)
	if err != nil {
		return err
	}

	e.IP = service.Spec.ClusterIP
	return nil
}

// setPublicEndpoint - set public endpoint url from route with label, namespace
func (e *OSPEndpoint) setPublicEndpoint(c client.Client, label string, namespace string) error {
	route, err := getRoute(c, label, namespace)
	if err != nil {
		return err
	}

	// TODO: mschuppert handle https
	e.PublicURL = fmt.Sprintf("http://%s", route.Spec.Host)
	return nil
}

// getService - from namespace with app label
func getService(c client.Client, label string, namespace string) (corev1.Service, error) {
	service := corev1.Service{}
	listOpts := []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingLabels{"app": label},
	}

	serviceList := &corev1.ServiceList{}
	err := c.List(context.TODO(), serviceList, listOpts...)
	if err != nil {
		return service, err
	}
	if len(serviceList.Items) != 1 {
		return service, err
	}

	return serviceList.Items[0], nil
}

// getRoute - from namespace with app label
func getRoute(c client.Client, label string, namespace string) (routev1.Route, error) {
	route := routev1.Route{}
	listOpts := []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingLabels{"app": label},
	}

	routeList := &routev1.RouteList{}
	err := c.List(context.TODO(), routeList, listOpts...)
	if err != nil {
		return route, err
	}
	if len(routeList.Items) != 1 {
		return route, err
	}

	return routeList.Items[0], nil
}
