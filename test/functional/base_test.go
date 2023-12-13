/*
Copyright 2022.

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
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	api "github.com/openstack-k8s-operators/lib-common/modules/test/apis"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
)

const (
	SecretName     = "external-secret"
	ContainerImage = "test://nova"
	timeout        = 10 * time.Second
	// have maximum 100 retries before the timeout hits
	interval = timeout / 100
	// consistencyTimeout is the amount of time we use to repeatedly check
	// that a condition is still valid. This is intended to be used in
	// asserts using `Consistently`.
	consistencyTimeout = timeout
	ironicComputeName  = "ironic-compute"
	//PublicCertSecretName   = "public-tls-certs"
	//InternalCertSecretName = "internal-tls-certs"
	//CABundleSecretName     = "combined-ca-bundle"
)

type NovaAPIFixture struct {
	api.APIFixture
	APIRequests []map[string]string
}

func AddNovaAPIFixture(log logr.Logger, server *api.FakeAPIServer) *NovaAPIFixture {
	fixture := &NovaAPIFixture{
		APIFixture: api.APIFixture{
			Server:     server,
			Log:        log,
			URLBase:    "/compute",
			OwnsServer: false,
		},
		APIRequests: []map[string]string{},
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
	f.APIRequests = append(f.APIRequests, map[string]string{
		"method": r.Method,
		"path":   r.URL.Path,
		"query":  r.URL.RawQuery,
	})
}

func (f *NovaAPIFixture) FindRequest(method string, path string, query string) bool {
	for _, request := range f.APIRequests {
		if request["method"] == method && request["path"] == path {
			if request["query"] == query || query == "" {
				return true
			}
		}
	}
	return false
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

func GetDefaultNovaAPISpec(novaNames NovaNames) map[string]interface{} {
	return map[string]interface{}{
		"secret":                novaNames.InternalTopLevelSecretName.Name,
		"apiDatabaseHostname":   "nova-api-db-hostname",
		"cell0DatabaseHostname": "nova-cell0-db-hostname",
		"keystoneAuthURL":       "keystone-internal-auth-url",
		"keystonePublicAuthURL": "keystone-public-auth-url",
		"containerImage":        ContainerImage,
		"serviceAccount":        "nova-sa",
		"registeredCells":       map[string]string{},
	}
}

func CreateNovaAPI(name types.NamespacedName, spec map[string]interface{}) client.Object {
	raw := map[string]interface{}{
		"apiVersion": "nova.openstack.org/v1beta1",
		"kind":       "NovaAPI",
		"metadata": map[string]interface{}{
			"name":      name.Name,
			"namespace": name.Namespace,
		},
		"spec": spec,
	}
	return th.CreateUnstructured(raw)

}

func GetNovaAPI(name types.NamespacedName) *novav1.NovaAPI {
	instance := &novav1.NovaAPI{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}

func NovaAPINotExists(name types.NamespacedName) {
	Consistently(func(g Gomega) {
		instance := &novav1.NovaAPI{}
		err := k8sClient.Get(ctx, name, instance)
		g.Expect(k8s_errors.IsNotFound(err)).To(BeTrue())
	}, consistencyTimeout, interval).Should(Succeed())
}

func NovaAPIConditionGetter(name types.NamespacedName) condition.Conditions {
	instance := GetNovaAPI(name)
	return instance.Status.Conditions
}

func NovaSchedulerConditionGetter(name types.NamespacedName) condition.Conditions {
	instance := GetNovaScheduler(name)
	return instance.Status.Conditions
}

func GetDefaultNovaSpec() map[string]interface{} {
	return map[string]interface{}{
		"secret":                SecretName,
		"cellTemplates":         map[string]interface{}{},
		"apiMessageBusInstance": cell0.TransportURLName.Name,
	}
}

func GetDefaultNovaCellTemplate() map[string]interface{} {
	return map[string]interface{}{
		"cellDatabaseUser": "nova_cell0",
		"hasAPIAccess":     true,
	}
}

func CreateNova(name types.NamespacedName, spec map[string]interface{}) client.Object {
	raw := map[string]interface{}{
		"apiVersion": "nova.openstack.org/v1beta1",
		"kind":       "Nova",
		"metadata": map[string]interface{}{
			"name":      name.Name,
			"namespace": name.Namespace,
		},
		"spec": spec,
	}
	return th.CreateUnstructured(raw)
}

func CreateNovaWithCell0(name types.NamespacedName) client.Object {
	rawNova := map[string]interface{}{
		"apiVersion": "nova.openstack.org/v1beta1",
		"kind":       "Nova",
		"metadata": map[string]interface{}{
			"name":      name.Name,
			"namespace": name.Namespace,
		},
		"spec": map[string]interface{}{
			"secret": SecretName,
			"cellTemplates": map[string]interface{}{
				"cell0": map[string]interface{}{
					"cellDatabaseUser": "nova_cell0",
					"hasAPIAccess":     true,
				},
			},
			"apiMessageBusInstance": cell0.TransportURLName.Name,
		},
	}

	return th.CreateUnstructured(rawNova)
}

func GetNova(name types.NamespacedName) *novav1.Nova {
	instance := &novav1.Nova{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}

func NovaConditionGetter(name types.NamespacedName) condition.Conditions {
	instance := GetNova(name)
	return instance.Status.Conditions
}

func GetDefaultNovaConductorSpec(cell CellNames) map[string]interface{} {
	return map[string]interface{}{
		"cellName":            cell.CellName,
		"secret":              cell.InternalCellSecretName.Name,
		"containerImage":      ContainerImage,
		"keystoneAuthURL":     "keystone-auth-url",
		"serviceAccount":      "nova-sa",
		"customServiceConfig": "foo=bar",
	}
}

func CreateNovaConductor(name types.NamespacedName, spec map[string]interface{}) client.Object {
	raw := map[string]interface{}{
		"apiVersion": "nova.openstack.org/v1beta1",
		"kind":       "NovaConductor",
		"metadata": map[string]interface{}{
			"name":      name.Name,
			"namespace": name.Namespace,
		},
		"spec": spec,
	}
	return th.CreateUnstructured(raw)
}

func GetNovaConductor(name types.NamespacedName) *novav1.NovaConductor {
	instance := &novav1.NovaConductor{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}

func NovaConductorConditionGetter(name types.NamespacedName) condition.Conditions {
	instance := GetNovaConductor(name)
	return instance.Status.Conditions
}

func CreateNovaMessageBusSecret(cell CellNames) *corev1.Secret {
	s := th.CreateSecret(
		types.NamespacedName{Namespace: cell.CellCRName.Namespace, Name: fmt.Sprintf("%s-secret", cell.TransportURLName.Name)},
		map[string][]byte{
			"transport_url": []byte(fmt.Sprintf("rabbit://%s/fake", cell.CellName)),
		},
	)
	logger.Info("Secret created", "name", s.Name)
	return s
}

func GetDefaultNovaCellSpec(cell CellNames) map[string]interface{} {
	return map[string]interface{}{
		"cellName":             cell.CellName,
		"secret":               cell.InternalCellSecretName.Name,
		"cellDatabaseHostname": "cell-database-hostname",
		"keystoneAuthURL":      "keystone-auth-url",
		"serviceAccount":       "nova",
	}
}

func CreateNovaCell(name types.NamespacedName, spec map[string]interface{}) client.Object {

	raw := map[string]interface{}{
		"apiVersion": "nova.openstack.org/v1beta1",
		"kind":       "NovaCell",
		"metadata": map[string]interface{}{
			"name":      name.Name,
			"namespace": name.Namespace,
		},
		"spec": spec,
	}
	return th.CreateUnstructured(raw)
}

func GetNovaCell(name types.NamespacedName) *novav1.NovaCell {
	instance := &novav1.NovaCell{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}

func NovaCellNotExists(name types.NamespacedName) {
	Consistently(func(g Gomega) {
		instance := &novav1.NovaCell{}
		err := k8sClient.Get(ctx, name, instance)
		g.Expect(k8s_errors.IsNotFound(err)).To(BeTrue())
	}, consistencyTimeout, interval).Should(Succeed())
}

func NovaCellConditionGetter(name types.NamespacedName) condition.Conditions {
	instance := GetNovaCell(name)
	return instance.Status.Conditions
}

func CreateNovaSecret(namespace string, name string) *corev1.Secret {
	return th.CreateSecret(
		types.NamespacedName{Namespace: namespace, Name: name},
		map[string][]byte{
			"NovaPassword":              []byte("service-password"),
			"NovaAPIDatabasePassword":   []byte("api-database-password"),
			"MetadataSecret":            []byte("metadata-secret"),
			"NovaCell0DatabasePassword": []byte("cell0-database-password"),
		},
	)
}

func CreateNovaSecretFor3Cells(namespace string, name string) *corev1.Secret {
	return th.CreateSecret(
		types.NamespacedName{Namespace: namespace, Name: name},
		map[string][]byte{
			"NovaPassword":              []byte("service-password"),
			"NovaAPIDatabasePassword":   []byte("api-database-password"),
			"MetadataSecret":            []byte("metadata-secret"),
			"NovaCell0DatabasePassword": []byte("cell0-database-password"),
			"NovaCell1DatabasePassword": []byte("cell1-database-password"),
			"NovaCell2DatabasePassword": []byte("cell2-database-password"),
		},
	)
}

func GetDefaultNovaSchedulerSpec(novaNames NovaNames) map[string]interface{} {
	return map[string]interface{}{
		"secret":                novaNames.InternalTopLevelSecretName.Name,
		"apiDatabaseHostname":   "nova-api-db-hostname",
		"cell0DatabaseHostname": "nova-cell0-db-hostname",
		"keystoneAuthURL":       "keystone-auth-url",
		"containerImage":        ContainerImage,
		"serviceAccount":        "nova-sa",
		"registeredCells":       map[string]string{},
	}
}

func CreateNovaScheduler(name types.NamespacedName, spec map[string]interface{}) client.Object {
	raw := map[string]interface{}{
		"apiVersion": "nova.openstack.org/v1beta1",
		"kind":       "NovaScheduler",
		"metadata": map[string]interface{}{
			"name":      name.Name,
			"namespace": name.Namespace,
		},
		"spec": spec,
	}
	return th.CreateUnstructured(raw)
}

func GetNovaScheduler(name types.NamespacedName) *novav1.NovaScheduler {
	instance := &novav1.NovaScheduler{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}

func NovaSchedulerNotExists(name types.NamespacedName) {
	Consistently(func(g Gomega) {
		instance := &novav1.NovaScheduler{}
		err := k8sClient.Get(ctx, name, instance)
		g.Expect(k8s_errors.IsNotFound(err)).To(BeTrue())
	}, consistencyTimeout, interval).Should(Succeed())
}

type CellNames struct {
	CellName                         string
	CellCRName                       types.NamespacedName
	MariaDBDatabaseName              types.NamespacedName
	ConductorName                    types.NamespacedName
	DBSyncJobName                    types.NamespacedName
	ConductorConfigDataName          types.NamespacedName
	ConductorScriptDataName          types.NamespacedName
	ConductorStatefulSetName         types.NamespacedName
	TransportURLName                 types.NamespacedName
	CellMappingJobName               types.NamespacedName
	MetadataName                     types.NamespacedName
	MetadataStatefulSetName          types.NamespacedName
	MetadataConfigDataName           types.NamespacedName
	MetadataNeutronConfigDataName    types.NamespacedName
	NoVNCProxyName                   types.NamespacedName
	NoVNCProxyStatefulSetName        types.NamespacedName
	CellNoVNCProxyNameConfigDataName types.NamespacedName
	InternalCellSecretName           types.NamespacedName
	InternalAPINetworkNADName        types.NamespacedName
	ComputeConfigSecretName          types.NamespacedName
	NovaComputeName                  types.NamespacedName
	NovaComputeStatefulSetName       types.NamespacedName
	NovaComputeConfigDataName        types.NamespacedName
	HostDiscoveryJobName             types.NamespacedName
}

func GetCellNames(novaName types.NamespacedName, cell string) CellNames {
	cellName := types.NamespacedName{
		Namespace: novaName.Namespace,
		Name:      novaName.Name + "-" + cell,
	}
	cellConductor := types.NamespacedName{
		Namespace: novaName.Namespace,
		Name:      cellName.Name + "-conductor",
	}
	metadataName := types.NamespacedName{
		Namespace: novaName.Namespace,
		Name:      cellName.Name + "-metadata",
	}
	novncproxyName := types.NamespacedName{
		Namespace: novaName.Namespace,
		Name:      cellName.Name + "-novncproxy",
	}
	novaCompute := types.NamespacedName{
		Namespace: novaName.Namespace,
		Name:      cellName.Name + "-" + ironicComputeName + "-compute",
	}

	c := CellNames{
		CellName:   cell,
		CellCRName: cellName,
		MariaDBDatabaseName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      "nova-" + cell,
		},
		ConductorName: cellConductor,
		DBSyncJobName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      cellConductor.Name + "-db-sync",
		},
		ConductorStatefulSetName: cellConductor,
		TransportURLName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      cellName.Name + "-transport",
		},
		CellMappingJobName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      cellName.Name + "-cell-mapping",
		},
		ConductorConfigDataName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      cellConductor.Name + "-config-data",
		},
		ConductorScriptDataName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      cellConductor.Name + "-scripts",
		},
		MetadataName:            metadataName,
		MetadataStatefulSetName: metadataName,
		MetadataConfigDataName: types.NamespacedName{
			Namespace: metadataName.Namespace,
			Name:      metadataName.Name + "-config-data",
		},
		MetadataNeutronConfigDataName: types.NamespacedName{
			Namespace: metadataName.Namespace,
			Name:      metadataName.Name + "-neutron-config",
		},
		NoVNCProxyName:            novncproxyName,
		NoVNCProxyStatefulSetName: novncproxyName,
		CellNoVNCProxyNameConfigDataName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      cellName.Name + "-novncproxy" + "-config-data",
		},
		NovaComputeName:            novaCompute,
		NovaComputeStatefulSetName: novaCompute,
		NovaComputeConfigDataName: types.NamespacedName{
			Namespace: novaCompute.Namespace,
			Name:      cellName.Name + "-" + ironicComputeName + "-compute" + "-config-data",
		},
		HostDiscoveryJobName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      cellName.Name + "-host-discover",
		},
		InternalCellSecretName: cellName,
		InternalAPINetworkNADName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      "internalapi",
		},
		ComputeConfigSecretName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      cellName.Name + "-compute-config",
		},
	}

	if cell == "cell0" {
		c.TransportURLName = types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      novaName.Name + "-api-transport",
		}
	}

	return c
}

type NovaNames struct {
	Namespace               string
	NovaName                types.NamespacedName
	InternalNovaServiceName types.NamespacedName
	PublicNovaServiceName   types.NamespacedName
	AdminNovaServiceName    types.NamespacedName
	KeystoneServiceName     types.NamespacedName
	APIName                 types.NamespacedName
	APIMariaDBDatabaseName  types.NamespacedName
	APIDeploymentName       types.NamespacedName
	APIKeystoneEndpointName types.NamespacedName
	APIStatefulSetName      types.NamespacedName
	APIConfigDataName       types.NamespacedName
	InternalCertSecretName  types.NamespacedName
	PublicCertSecretName    types.NamespacedName
	CaBundleSecretName      types.NamespacedName
	// refers internal API network for all Nova services (not just nova API)
	InternalAPINetworkNADName       types.NamespacedName
	SchedulerName                   types.NamespacedName
	SchedulerStatefulSetName        types.NamespacedName
	SchedulerConfigDataName         types.NamespacedName
	MetadataName                    types.NamespacedName
	MetadataStatefulSetName         types.NamespacedName
	MetadataNeutronConfigDataName   types.NamespacedName
	ServiceAccountName              types.NamespacedName
	RoleName                        types.NamespacedName
	RoleBindingName                 types.NamespacedName
	MetadataConfigDataName          types.NamespacedName
	InternalNovaMetadataServiceName types.NamespacedName
	InternalTopLevelSecretName      types.NamespacedName
	Cells                           map[string]CellNames
}

func GetNovaNames(novaName types.NamespacedName, cellNames []string) NovaNames {
	// NOTE(bogdando): use random UUIDs instead of static "nova" part of names.
	// These **must** replicate existing Nova*/Dataplane controllers suffixing/prefixing logic.
	// While dynamic UUIDs also provide enhanced testing coverage for "synthetic" cases,
	// which could not be caught for normal names with static "nova" prefixes.
	novaAPI := types.NamespacedName{
		Namespace: novaName.Namespace,
		Name:      fmt.Sprintf("%s-api", novaName.Name),
	}
	novaScheduler := types.NamespacedName{
		Namespace: novaName.Namespace,
		Name:      fmt.Sprintf("%s-scheduler", novaName.Name),
	}
	novaMetadata := types.NamespacedName{
		Namespace: novaName.Namespace,
		Name:      fmt.Sprintf("%s-metadata", novaName.Name),
	}

	cells := map[string]CellNames{}
	for _, cellName := range cellNames {
		cells[cellName] = GetCellNames(novaName, cellName)
	}

	return NovaNames{
		Namespace: novaName.Namespace,
		NovaName:  novaName,
		InternalNovaServiceName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      "nova-internal",
		},
		PublicNovaServiceName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      "nova-public",
		},
		KeystoneServiceName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      "nova", // static value hardcoded in controller code
		},
		APIName: novaAPI,
		APIMariaDBDatabaseName: types.NamespacedName{
			Namespace: novaAPI.Namespace,
			Name:      "nova-api", // a static DB name for nova
		},
		APIDeploymentName: novaAPI,
		APIKeystoneEndpointName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      "nova", // a static keystone endpoint name for nova
		},
		APIStatefulSetName: novaAPI,
		APIConfigDataName: types.NamespacedName{
			Namespace: novaAPI.Namespace,
			Name:      novaAPI.Name + "-config-data",
		},
		InternalCertSecretName: types.NamespacedName{
			Namespace: novaAPI.Namespace,
			Name:      "internal-tls-certs"},
		PublicCertSecretName: types.NamespacedName{
			Namespace: novaAPI.Namespace,
			Name:      "public-tls-certs"},
		CaBundleSecretName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      "combined-ca-bundle"},

		InternalAPINetworkNADName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      "internalapi",
		},
		SchedulerName:            novaScheduler,
		SchedulerStatefulSetName: novaScheduler,
		SchedulerConfigDataName: types.NamespacedName{
			Namespace: novaScheduler.Namespace,
			Name:      novaScheduler.Name + "-config-data",
		},
		MetadataName:            novaMetadata,
		MetadataStatefulSetName: novaMetadata,
		ServiceAccountName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      "nova-" + novaName.Name,
		},
		RoleName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      "nova-" + novaName.Name + "-role",
		},
		RoleBindingName: types.NamespacedName{
			Namespace: novaName.Namespace,
			Name:      "nova-" + novaName.Name + "-rolebinding",
		},
		MetadataConfigDataName: types.NamespacedName{
			Namespace: novaMetadata.Namespace,
			Name:      novaMetadata.Name + "-config-data",
		},
		MetadataNeutronConfigDataName: types.NamespacedName{
			Namespace: novaMetadata.Namespace,
			Name:      novaMetadata.Name + "-neutron-config",
		},
		InternalNovaMetadataServiceName: types.NamespacedName{
			Namespace: novaMetadata.Namespace,
			Name:      "nova-metadata-internal",
		},
		InternalTopLevelSecretName: novaName,

		Cells: cells,
	}
}

func CreateNovaMetadata(name types.NamespacedName, spec map[string]interface{}) client.Object {
	raw := map[string]interface{}{
		"apiVersion": "nova.openstack.org/v1beta1",
		"kind":       "NovaMetadata",
		"metadata": map[string]interface{}{
			"name":      name.Name,
			"namespace": name.Namespace,
		},
		"spec": spec,
	}
	return th.CreateUnstructured(raw)
}

func GetNovaMetadata(name types.NamespacedName) *novav1.NovaMetadata {
	instance := &novav1.NovaMetadata{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}

func NovaMetadataConditionGetter(name types.NamespacedName) condition.Conditions {
	instance := GetNovaMetadata(name)
	return instance.Status.Conditions
}

func CreateInternalTopLevelSecret(novaNames NovaNames) *corev1.Secret {
	return th.CreateSecret(
		novaNames.InternalTopLevelSecretName,
		map[string][]byte{
			"ServicePassword":      []byte("service-password"),
			"APIDatabasePassword":  []byte("api-database-password"),
			"CellDatabasePassword": []byte("cell-database-password"),
			"MetadataSecret":       []byte("metadata-secret"),
			"transport_url":        []byte("rabbit://api/fake"),
		},
	)
}

func GetDefaultNovaMetadataSpec(secretName types.NamespacedName) map[string]interface{} {
	return map[string]interface{}{
		"secret":               secretName.Name,
		"apiDatabaseHostname":  "nova-api-db-hostname",
		"cellDatabaseHostname": "nova-cell-db-hostname",
		"containerImage":       ContainerImage,
		"keystoneAuthURL":      "keystone-auth-url",
		"serviceAccount":       "nova-sa",
	}
}

func AssertMetadataDoesNotExist(name types.NamespacedName) {
	instance := &novav1.NovaMetadata{}
	Eventually(func(g Gomega) {
		err := k8sClient.Get(ctx, name, instance)
		g.Expect(k8s_errors.IsNotFound(err)).To(BeTrue())
	}, timeout, interval).Should(Succeed())
}

func CreateNovaNoVNCProxy(name types.NamespacedName, spec map[string]interface{}) client.Object {
	raw := map[string]interface{}{
		"apiVersion": "nova.openstack.org/v1beta1",
		"kind":       "NovaNoVNCProxy",
		"metadata": map[string]interface{}{
			"name":      name.Name,
			"namespace": name.Namespace,
		},
		"spec": spec,
	}
	return th.CreateUnstructured(raw)
}

func NoVNCProxyConditionGetter(name types.NamespacedName) condition.Conditions {
	instance := GetNovaNoVNCProxy(name)
	return instance.Status.Conditions
}

func GetNovaNoVNCProxy(name types.NamespacedName) *novav1.NovaNoVNCProxy {
	instance := &novav1.NovaNoVNCProxy{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}

func GetDefaultNovaNoVNCProxySpec(cell CellNames) map[string]interface{} {
	return map[string]interface{}{
		"secret":               cell.InternalCellSecretName.Name,
		"cellDatabaseHostname": "nova-cell-db-hostname",
		"containerImage":       ContainerImage,
		"keystoneAuthURL":      "keystone-auth-url",
		"serviceAccount":       "nova-sa",
		"cellName":             cell.CellName,
	}
}

func CreateCellInternalSecret(cell CellNames) *corev1.Secret {
	return th.CreateSecret(
		cell.InternalCellSecretName,
		map[string][]byte{
			"ServicePassword":      []byte("service-password"),
			"CellDatabasePassword": []byte("cell-database-password"),
			// TODO(gibi): we only need this for cells with metadata
			"MetadataSecret": []byte("metadata-secret"),
			"transport_url":  []byte(fmt.Sprintf("rabbit://%s/fake", cell.CellName)),
		},
	)
}

func AssertNoVNCProxyDoesNotExist(name types.NamespacedName) {
	instance := &novav1.NovaNoVNCProxy{}
	Eventually(func(g Gomega) {
		err := k8sClient.Get(ctx, name, instance)
		g.Expect(k8s_errors.IsNotFound(err)).To(BeTrue())
	}, timeout, interval).Should(Succeed())
}

func CreateNovaCompute(name types.NamespacedName, spec map[string]interface{}) client.Object {
	raw := map[string]interface{}{
		"apiVersion": "nova.openstack.org/v1beta1",
		"kind":       "NovaCompute",
		"metadata": map[string]interface{}{
			"name":      name.Name,
			"namespace": name.Namespace,
		},
		"spec": spec,
	}
	return th.CreateUnstructured(raw)
}

func GetNovaCompute(name types.NamespacedName) *novav1.NovaCompute {
	instance := &novav1.NovaCompute{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}

func NovaComputeConditionGetter(name types.NamespacedName) condition.Conditions {
	instance := GetNovaCompute(name)
	return instance.Status.Conditions
}

func GetDefaultNovaComputeTemplate() map[string]interface{} {
	return map[string]interface{}{
		"computeDriver": novav1.IronicDriver,
		"name":          ironicComputeName,
	}
}

func GetDefaultNovaComputeSpec(cell CellNames) map[string]interface{} {
	return map[string]interface{}{
		"secret":               cell.InternalCellSecretName.Name,
		"computeName":          "compute1",
		"cellDatabaseHostname": "nova-cell-db-hostname",
		"containerImage":       ContainerImage,
		"keystoneAuthURL":      "keystone-auth-url",
		"serviceAccount":       "nova",
		"cellName":             cell.CellName,
		"computeDriver":        novav1.IronicDriver,
	}
}
