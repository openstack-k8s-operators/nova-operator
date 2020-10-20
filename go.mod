module github.com/openstack-k8s-operators/nova-operator

go 1.13

require (
	github.com/blang/semver v3.5.1+incompatible
	github.com/go-logr/logr v0.1.0
	github.com/onsi/ginkgo v1.12.1
	github.com/onsi/gomega v1.10.1
	github.com/openshift/api v3.9.0+incompatible
	github.com/openstack-k8s-operators/cinder-operator v0.0.0-20201015102724-05cd6202cb34 // indirect
	github.com/openstack-k8s-operators/keystone-operator v0.0.0-20201020115836-d8f73f3ff674
	github.com/openstack-k8s-operators/lib-common v0.0.0-20201012132655-247b83b2fafa
	github.com/openstack-k8s-operators/neutron-operator v0.0.0-20201019232507-ad9b6222b968 // indirect
	github.com/openstack-k8s-operators/placement-operator v0.0.0-20201016074241-b2b5c6eb0e9c // indirect
	github.com/operator-framework/operator-lifecycle-manager v0.0.0-20200321030439-57b580e57e88
	github.com/prometheus/common v0.7.0
	github.com/stretchr/testify v1.4.0
	k8s.io/api v0.18.6
	k8s.io/apimachinery v0.18.6
	k8s.io/client-go v0.18.6
	sigs.k8s.io/controller-runtime v0.6.2
)
