module github.com/openstack-k8s-operators/placement-operator

go 1.13

require (
	github.com/blang/semver v3.5.1+incompatible
	github.com/go-logr/logr v0.1.0
	github.com/onsi/ginkgo v1.12.1
	github.com/onsi/gomega v1.10.1
	github.com/openshift/api v3.9.0+incompatible
	github.com/openstack-k8s-operators/keystone-operator v0.0.0-20201012214326-7b0b20e9777b
	github.com/openstack-k8s-operators/lib-common v0.0.0-20200910130010-129482aabaf9
	github.com/operator-framework/operator-lifecycle-manager v0.0.0-20200321030439-57b580e57e88
	github.com/prometheus/common v0.7.0
	k8s.io/api v0.18.6
	k8s.io/apimachinery v0.18.6
	k8s.io/client-go v0.18.6
	sigs.k8s.io/controller-runtime v0.6.2
)
