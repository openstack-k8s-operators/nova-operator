/*
Copyright 2024.

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
package functional_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-logr/logr"

	. "github.com/onsi/ginkgo/v2" //revive:disable:dot-imports

	keystone_helper "github.com/openstack-k8s-operators/keystone-operator/api/test/helpers"
	api "github.com/openstack-k8s-operators/lib-common/modules/test/apis"
)

// Service represents a Compute service in the OpenStack cloud.
// NOTE(gibi): When we are on golang 1.22 and gophercloud v2 is released we can
// use the "github.com/gophercloud/gophercloud/openstack/compute/v2/services"
// structs directly.
type Service struct {
	// The binary name of the service.
	Binary string `json:"binary"`

	// The reason for disabling a service.
	DisabledReason string `json:"disabled_reason"`

	// Whether or not service was forced down manually.
	ForcedDown bool `json:"forced_down"`

	// The name of the host.
	Host string `json:"host"`

	// The id of the service.
	ID string `json:"id"`

	// The state of the service. One of up or down.
	State string `json:"state"`

	// The status of the service. One of enabled or disabled.
	Status string `json:"status"`

	// The date and time when the resource was updated.
	UpdatedAt time.Time `json:"-"`

	// The availability zone name.
	Zone string `json:"zone"`
}

type NovaAPIFixture struct {
	api.APIFixture
	APIRequests []http.Request
	Services    []Service
}

func AddNovaAPIFixture(log logr.Logger, server *api.FakeAPIServer) *NovaAPIFixture {
	fixture := &NovaAPIFixture{
		APIFixture: api.APIFixture{
			Server:     server,
			Log:        log,
			URLBase:    "/compute",
			OwnsServer: false,
		},
		APIRequests: []http.Request{},
		Services: []Service{
			{
				ID:         "1",
				Binary:     "nova-scheduler",
				Host:       "nova-scheduler-0",
				State:      "up",
				Status:     "disabled",
				ForcedDown: false,
			},
			{
				ID:         "2",
				Binary:     "nova-compute",
				Host:       "nova-compute-0",
				State:      "up",
				Status:     "disabled",
				ForcedDown: false,
			},
			{
				ID:         "3",
				Binary:     "nova-scheduler",
				Host:       "nova-scheduler-1",
				State:      "down",
				Status:     "enabled",
				ForcedDown: false,
			},
			{
				ID:         "4",
				Binary:     "nova-compute",
				Host:       "nova-compute-1",
				State:      "down",
				Status:     "disabled",
				ForcedDown: false,
			},
			{
				ID:         "5",
				Binary:     "nova-conductor",
				Host:       "nova-cell0-conductor-0",
				State:      "up",
				Status:     "disabled",
				ForcedDown: false,
			},
			{
				ID:         "6",
				Binary:     "nova-conductor",
				Host:       "nova-cell1-conductor-0",
				State:      "down",
				Status:     "disabled",
				ForcedDown: false,
			},
			{
				ID:         "7",
				Binary:     "nova-conductor",
				Host:       "nova-cell1-conductor-1",
				State:      "up",
				Status:     "disabled",
				ForcedDown: false,
			},
			{
				ID:         "8",
				Binary:     "nova-conductor",
				Host:       "nova-cell0-conductor-1",
				State:      "down",
				Status:     "disabled",
				ForcedDown: false,
			},
		},
	}
	return fixture
}

// NewNovaAPIFixtureWithServer set up a nova-api simulator with an
// embedded http server
func NewNovaAPIFixtureWithServer(log logr.Logger) *NovaAPIFixture {
	server := &api.FakeAPIServer{}
	server.Setup(log)
	fixture := AddNovaAPIFixture(log, server)
	fixture.OwnsServer = true
	return fixture
}

func (f *NovaAPIFixture) RecordRequest(r *http.Request) {
	f.APIRequests = append(f.APIRequests, *r)
}

func (f *NovaAPIFixture) FindRequest(method string, path string, query string) *http.Request {
	for _, request := range f.APIRequests {
		if request.Method == method && request.URL.Path == path {
			if request.URL.RawQuery == query || query == "" {
				return &request
			}
		}
	}
	return nil
}

func (f *NovaAPIFixture) HasRequest(method string, path string, query string) bool {
	return f.FindRequest(method, path, query) != nil
}

// Setup adds the API request handlers to the fixture. If no handlers is passed
// then a basic set of well behaving handlers are added that will simulate the
// happy path.
// If you need to customize the behavior of the fixture, e.g. to inject faults,
// then you can pass a list of handlers to register instead.
func (f *NovaAPIFixture) Setup(handlers ...api.Handler) {
	if len(handlers) == 0 {
		f.registerNormalHandlers()
	}
	for _, handler := range handlers {
		f.registerHandler(handler)
	}
}

func (f *NovaAPIFixture) registerHandler(handler api.Handler) {
	f.Server.AddHandler(f.URLBase+handler.Pattern, handler.Func)
}

func (f *NovaAPIFixture) registerNormalHandlers() {
	f.registerHandler(api.Handler{Pattern: "/os-services/", Func: f.ServicesHandler})
}

func (f *NovaAPIFixture) ServicesHandler(w http.ResponseWriter, r *http.Request) {
	f.LogRequest(r)
	f.RecordRequest(r)
	switch r.Method {
	case "GET":
		f.getServices(w, r)
	case "DELETE":
		f.deleteService(w, r)
	default:
		f.UnexpectedRequest(w, r)
		return
	}
}

func (f *NovaAPIFixture) getServices(w http.ResponseWriter, r *http.Request) {
	services := []Service{}
	for _, service := range f.Services {
		binaryFilter := r.URL.Query().Get("binary")
		if binaryFilter == "" || service.Binary == binaryFilter {
			services = append(services, service)
		}
	}

	var s struct {
		Services []Service `json:"services"`
	}
	s.Services = services

	bytes, err := json.Marshal(&s)
	if err != nil {
		f.InternalError(err, "Error during marshalling response", w, r)
		return
	}
	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(200)
	fmt.Fprint(w, string(bytes))
}

func (f *NovaAPIFixture) deleteService(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "application/json")

	items := strings.Split(r.URL.Path, "/")
	id := items[len(items)-1]

	for i, service := range f.Services {
		if service.ID == id {
			w.WriteHeader(204)
			// remove the deleted item from the fixtures internal state
			f.Services = append(f.Services[:i], f.Services[i+1:]...)
			return
		}
	}
	w.WriteHeader(404)
}

// ResponseHandleToken responds with a valid keystone token and the computeURL in the catalog
func ResponseHandleToken(keystoneURL string, computeURL string) string {
	return fmt.Sprintf(
		`
			{
				"token":{
				   "catalog":[
					  {
						 "endpoints":[
							{
							   "id":"e6ec29ecce164c3084ef308478080127",
							   "interface":"public",
							   "region_id":"RegionOne",
							   "url":"%s",
							   "region":"RegionOne"
							}
						 ],
						 "id":"edad7277e52a47b3bfb2b7004f77110f",
						 "type":"identity",
						 "name":"keystone"
					  },
					  {
						"endpoints":[
							{
								"name":"nova",
								"id":"501f5ea604e443239fc81cfe7740eb52",
								"interface":"internal",
								"region_id":"regionOne",
								"url":"%s",
								"region":"regionOne"
							},
							{
								"name":"nova",
								"id":"cf5ea593463147ceabaa23d905ebc96d",
								"interface":"public",
								"region_id":"regionOne",
								"url":"%s",
								"region":"regionOne"
							}
						],
						"id":"76086b1494bd497dbe7d45c53bd0cc70",
						"type":"compute",
						"name":"nova"
					}
				   	]
				}
			 }
			`, keystoneURL, computeURL, computeURL)
}

// SetupAPIFixture creates both keystone and nova API server simulators
func SetupAPIFixtures(logger logr.Logger) (*keystone_helper.KeystoneAPIFixture, *NovaAPIFixture) {
	novaAPIServer := NewNovaAPIFixtureWithServer(logger)
	novaAPIServer.Setup()
	DeferCleanup(novaAPIServer.Cleanup)

	keystone := keystone_helper.NewKeystoneAPIFixtureWithServer(logger)
	keystone.Setup(
		api.Handler{Pattern: "/", Func: keystone.HandleVersion},
		api.Handler{Pattern: "/v3/users", Func: keystone.HandleUsers},
		api.Handler{Pattern: "/v3/domains", Func: keystone.HandleDomains},
		api.Handler{Pattern: "/v3/auth/tokens", Func: func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case "POST":
				w.Header().Add("Content-Type", "application/json")
				w.WriteHeader(202)
				// ensure keystone returns the simulator endpoints in its catalog
				fmt.Fprint(w, ResponseHandleToken(keystone.Endpoint(), novaAPIServer.Endpoint()))
			}
		}})
	DeferCleanup(keystone.Cleanup)

	return keystone, novaAPIServer
}
