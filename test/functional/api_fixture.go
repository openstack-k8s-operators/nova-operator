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
	"fmt"
	"net/http"

	"github.com/go-logr/logr"

	api "github.com/openstack-k8s-operators/lib-common/modules/test/apis"
)

type NovaAPIFixture struct {
	api.APIFixture
	APIRequests []http.Request
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
	f.registerHandler(api.Handler{Pattern: "/os-services/", Func: f.ServicesList})
}

func (f *NovaAPIFixture) ServicesList(w http.ResponseWriter, r *http.Request) {
	f.LogRequest(r)
	f.RecordRequest(r)
	switch r.Method {
	case "GET":
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(200)
		fmt.Fprintf(w,
			`
			{
				"services": [
					{
						"id": 1,
						"binary": "nova-scheduler",
						"disabled_reason": "test1",
						"host": "host1",
						"state": "up",
						"status": "disabled",
						"updated_at": "2012-10-29T13:42:02.000000",
						"forced_down": false,
						"zone": "internal"
					},
					{
						"id": 2,
						"binary": "nova-compute",
						"disabled_reason": "test2",
						"host": "host1",
						"state": "up",
						"status": "disabled",
						"updated_at": "2012-10-29T13:42:05.000000",
						"forced_down": false,
						"zone": "nova"
					},
					{
						"id": 3,
						"binary": "nova-scheduler",
						"disabled_reason": null,
						"host": "host2",
						"state": "down",
						"status": "enabled",
						"updated_at": "2012-09-19T06:55:34.000000",
						"forced_down": false,
						"zone": "internal"
					},
					{
						"id": 4,
						"binary": "nova-compute",
						"disabled_reason": "test4",
						"host": "host2",
						"state": "down",
						"status": "disabled",
						"updated_at": "2012-09-18T08:03:38.000000",
						"forced_down": false,
						"zone": "nova"
					},
					{
						"id": 4,
						"binary": "nova-conductor",
						"disabled_reason": "test4",
						"host": "host2",
						"state": "down",
						"status": "disabled",
						"updated_at": "2012-09-18T08:03:38.000000",
						"forced_down": false,
						"zone": "nova"
					}
				]
			}
			`)
	case "DELETE":
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(204)
	default:
		f.UnexpectedRequest(w, r)
		return
	}
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
