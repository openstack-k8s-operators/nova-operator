package main

import (
	"flag"
	"os"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"

	"github.com/blang/semver"
	util "github.com/openstack-k8s-operators/lib-common/pkg/util"
	"github.com/openstack-k8s-operators/nova-operator/tools/helper"
	csvv1 "github.com/operator-framework/operator-lifecycle-manager/pkg/api/apis/operators/v1alpha1"
	"github.com/operator-framework/operator-lifecycle-manager/pkg/lib/version"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	csvVersion         = flag.String("csv-version", "", "")
	replacesCsvVersion = flag.String("replaces-csv-version", "", "")
	namespace          = flag.String("namespace", "", "")
	pullPolicy         = flag.String("pull-policy", "Always", "")

	logoBase64 = flag.String("logo-base64", "", "")
	verbosity  = flag.String("verbosity", "1", "")

	operatorImage = flag.String("operator-image-name", "quay.io/openstack-k8s-operators/nova-operator:devel", "optional")
)

func main() {
	flag.Parse()

	data := NewClusterServiceVersionData{
		CsvVersion:         *csvVersion,
		ReplacesCsvVersion: *replacesCsvVersion,
		Namespace:          *namespace,
		ImagePullPolicy:    *pullPolicy,
		IconBase64:         *logoBase64,
		Verbosity:          *verbosity,
		OperatorImage:      *operatorImage,
	}

	csv, err := createClusterServiceVersion(&data)
	if err != nil {
		panic(err)
	}
	util.MarshallObject(csv, os.Stdout)

}

//NewClusterServiceVersionData - Data arguments used to create nova operators's CSV manifest
type NewClusterServiceVersionData struct {
	CsvVersion         string
	ReplacesCsvVersion string
	Namespace          string
	ImagePullPolicy    string
	IconBase64         string
	Verbosity          string

	DockerPrefix string
	DockerTag    string

	OperatorImage string
}

func createOperatorDeployment(repo, namespace, deployClusterResources, operatorImage, tag, verbosity, pullPolicy string) *appsv1.Deployment {
	deployment := helper.CreateOperatorDeployment("nova-operator", namespace, "name", "nova-operator", "nova-operator", int32(1))
	container := helper.CreateOperatorContainer("nova-operator", operatorImage, verbosity, corev1.PullPolicy(pullPolicy))
	container.Env = *helper.CreateOperatorEnvVar(repo, deployClusterResources, operatorImage, pullPolicy)
	deployment.Spec.Template.Spec.Containers = []corev1.Container{container}
	return deployment
}

func createClusterServiceVersion(data *NewClusterServiceVersionData) (*csvv1.ClusterServiceVersion, error) {

	description := `
Install and configure OpenStack Nova containers.
`
	deployment := createOperatorDeployment(
		data.DockerPrefix,
		data.Namespace,
		"true",
		data.OperatorImage,
		data.DockerTag,
		data.Verbosity,
		data.ImagePullPolicy)

	//clusterRules := getOperatorClusterRules()
	rules := getOperatorRules()
	serviceRules := getServiceRules()

	strategySpec := csvv1.StrategyDetailsDeployment{
		/*
			ClusterPermissions: []csvv1.StrategyDeploymentPermissions{
				{
					ServiceAccountName: "nova-operator",
					Rules:              *clusterRules,
				},
			},
		*/
		Permissions: []csvv1.StrategyDeploymentPermissions{
			{
				ServiceAccountName: "nova-operator",
				Rules:              *rules,
			},
			{
				ServiceAccountName: "nova",
				Rules:              *serviceRules,
			},
		},
		DeploymentSpecs: []csvv1.StrategyDeploymentSpec{
			{
				Name: "nova-operator",
				Spec: deployment.Spec,
			},
		},
	}

	csvVersion, err := semver.New(data.CsvVersion)
	if err != nil {
		return nil, err
	}

	return &csvv1.ClusterServiceVersion{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterServiceVersion",
			APIVersion: "operators.coreos.com/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nova-operator." + data.CsvVersion,
			Namespace: data.Namespace,
			Annotations: map[string]string{

				"capabilities": "Basic Install",
				"categories":   "Compute",
				"description":  "Creates and maintains Nova Compute daemonsets",
			},
		},

		Spec: csvv1.ClusterServiceVersionSpec{
			DisplayName: "Nova Operator",
			Description: description,
			Keywords:    []string{"Nova Operator", "OpenStack", "Nova", "Compute"},
			Version:     version.OperatorVersion{Version: *csvVersion},
			Maturity:    "alpha",
			Replaces:    data.ReplacesCsvVersion,
			Maintainers: []csvv1.Maintainer{{
				Name:  "OpenStack k8s Operators",
				Email: "openstack-k8s-operators@googlegroups.com",
			}},
			Provider: csvv1.AppLink{
				Name: "OpenStack K8s Operators Nova Operator project",
			},
			Links: []csvv1.AppLink{
				{
					Name: "Nova Operator",
					URL:  "https://github.com/openstack-k8s-operators/nova-operator/blob/master/README.md",
				},
				{
					Name: "Source Code",
					URL:  "https://github.com/openstack-k8s-operators/nova-operator",
				},
			},
			Icon: []csvv1.Icon{{
				Data:      data.IconBase64,
				MediaType: "image/png",
			}},
			Labels: map[string]string{
				"alm-owner-nova-operator": "nova-operator",
				"operated-by":             "nova-operator",
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"alm-owner-nova-operator": "nova-operator",
					"operated-by":             "nova-operator",
				},
			},
			InstallModes: []csvv1.InstallMode{
				{
					Type:      csvv1.InstallModeTypeOwnNamespace,
					Supported: true,
				},
				{
					Type:      csvv1.InstallModeTypeSingleNamespace,
					Supported: true,
				},
				{
					Type:      csvv1.InstallModeTypeMultiNamespace,
					Supported: true,
				},
				{
					Type:      csvv1.InstallModeTypeAllNamespaces,
					Supported: true,
				},
			},
			InstallStrategy: csvv1.NamedInstallStrategy{
				StrategyName: "deployment",
				StrategySpec: strategySpec,
			},
			CustomResourceDefinitions: csvv1.CustomResourceDefinitions{

				Owned: []csvv1.CRDDescription{
					{
						Name:        "iscsids.nova.openstack.org",
						Version:     "v1beta1",
						Kind:        "Iscsid",
						DisplayName: "Nova Iscsid",
						Description: "Iscsid is the Schema for the iscsids API",
					},
					{
						Name:        "libvirtds.nova.openstack.org",
						Version:     "v1beta1",
						Kind:        "Libvirtd",
						DisplayName: "Nova Libvirtd",
						Description: "Libvirtd is the Schema for the libvirtds API",
					},
					{
						Name:        "novacomputes.nova.openstack.org",
						Version:     "v1beta1",
						Kind:        "NovaCompute",
						DisplayName: "Nova Compute",
						Description: "NovaCompute is the Schema for the novacomputes API",
					},
					{
						Name:        "novamigrationtargets.nova.openstack.org",
						Version:     "v1beta1",
						Kind:        "NovaMigrationTarget",
						DisplayName: "Nova Migration Target",
						Description: "NovaMigrationTarget is the Schema for the novamigrationtargets API",
					},
					{
						Name:        "virtlogds.nova.openstack.org",
						Version:     "v1beta1",
						Kind:        "Virtlogd",
						DisplayName: "Virtlog Daemon",
						Description: "Virtlogd is the Schema for the virtlogds API",
					},
					{
						Name:        "nova.nova.openstack.org",
						Version:     "v1beta1",
						Kind:        "Nova",
						DisplayName: "Nova ctrplane",
						Description: "Nova is the Schema for the nova API",
					},
					{
						Name:        "novaapis.nova.openstack.org",
						Version:     "v1beta1",
						Kind:        "NovaAPI",
						DisplayName: "NovaAPI Service",
						Description: "NovaAPI is the Schema for the novaAPIs API",
					},
					{
						Name:        "novacells.nova.openstack.org",
						Version:     "v1beta1",
						Kind:        "NovaCell",
						DisplayName: "Nova Cell",
						Description: "NovaCell is the Schema for the novaCells API",
					},
					{
						Name:        "novaconductors.nova.openstack.org",
						Version:     "v1beta1",
						Kind:        "NovaConductor",
						DisplayName: "Nova Condoctor service",
						Description: "NovaConductor is the Schema for the novaConductors API",
					},
					{
						Name:        "novametadata.nova.openstack.org",
						Version:     "v1beta1",
						Kind:        "NovaMetadata",
						DisplayName: "Nova Metadata service",
						Description: "NovaMetadata is the Schema for the novaMetadatas API",
					},
					{
						Name:        "novanovncproxies.nova.openstack.org",
						Version:     "v1beta1",
						Kind:        "NovaNoVNCProxy",
						DisplayName: "Nova NoVNCProxy service",
						Description: "NovaNoVNCProxy is the Schema for the novaNoVNCProxies API",
					},
					{
						Name:        "novaschedulers.nova.openstack.org",
						Version:     "v1beta1",
						Kind:        "NovaScheduler",
						DisplayName: "Nova Scheduler service",
						Description: "NovaScheduler is the Schema for the novaSchedulers API",
					},
				},
			},
		},
	}, nil
}

func getOperatorRules() *[]rbacv1.PolicyRule {
	return &[]rbacv1.PolicyRule{
		{
			APIGroups: []string{
				"",
			},
			Resources: []string{
				"pods",
				"services",
				"services/finalizers",
				"endpoints",
				"persistentvolumeclaims",
				"events",
				"configmaps",
				"secrets",
			},
			Verbs: []string{
				"*",
			},
		},
		{
			APIGroups: []string{
				"apps",
			},
			Resources: []string{
				"deployments",
				"daemonsets",
				"replicasets",
				"statefulsets",
			},
			Verbs: []string{
				"*",
			},
		},
		{
			APIGroups: []string{
				"batch",
			},
			Resources: []string{
				"jobs",
			},
			Verbs: []string{
				"*",
			},
		},
		{
			APIGroups: []string{
				"apps",
			},
			Resources: []string{
				"daemonsets",
			},
			ResourceNames: []string{
				"nova-operator",
			},
			Verbs: []string{
				"delete",
				"update",
			},
		},
		{
			APIGroups: []string{
				"monitoring.coreos.com",
			},
			Resources: []string{
				"servicemonitors",
			},
			Verbs: []string{
				"get",
				"create",
			},
		},
		{
			APIGroups: []string{
				"route.openshift.io",
			},
			Resources: []string{
				"routes",
			},
			Verbs: []string{
				"list",
				"watch",
				"create",
				"patch",
				"update",
			},
		},
		{
			APIGroups: []string{
				"apps",
			},
			Resources: []string{
				"deployments/finalizers",
			},
			ResourceNames: []string{
				"nova-operator",
			},
			Verbs: []string{
				"update",
			},
		},
		{
			APIGroups: []string{
				"nova.openstack.org",
			},
			Resources: []string{
				"*",
				"virtlogds",
				"libvirtds",
				"iscsids",
				"novamigrationtargets",
				"nova",
				"novaapis",
				"novacells",
				"novaconductors",
				"novamedata",
				"novanovncproxies",
				"novaschedulers",
			},
			Verbs: []string{
				"*",
			},
		},
		{
			APIGroups: []string{
				"database.openstack.org",
			},
			Resources: []string{
				"mariadbdatabases",
			},
			Verbs: []string{
				"get",
				"create",
			},
		},
	}
}

func getServiceRules() *[]rbacv1.PolicyRule {
	return &[]rbacv1.PolicyRule{
		{
			APIGroups: []string{
				"",
			},
			Resources: []string{
				"pods",
			},
			Verbs: []string{
				"*",
			},
		},
		{
			APIGroups: []string{
				"security.openshift.io",
			},
			Resources: []string{
				"securitycontextconstraints",
			},
			ResourceNames: []string{
				"privileged",
				"anyuid",
			},
			Verbs: []string{
				"use",
			},
		},
	}
}
