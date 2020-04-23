module github.com/openstack-k8s-operators/nova-operator

go 1.13

require (
	github.com/openshift/cluster-api v0.0.0-20191129101638-b09907ac6668
	github.com/openstack-k8s-operators/compute-node-operator v0.0.0-20200422093450-2c9f428da728
	github.com/operator-framework/operator-sdk v0.17.0
	github.com/spf13/pflag v1.0.5
	golang.org/x/tools v0.0.0-20200422205258-72e4a01eba43 // indirect
	k8s.io/api v0.17.4
	k8s.io/apimachinery v0.17.4
	k8s.io/client-go v12.0.0+incompatible
	sigs.k8s.io/controller-runtime v0.5.2
)

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v13.3.2+incompatible // Required by OLM
	k8s.io/client-go => k8s.io/client-go v0.17.4 // Required by prometheus-operator
)
