module github.com/openstack-k8s-operators/nova-operator

go 1.13

require (
	github.com/go-logr/logr v0.1.0
	github.com/openshift/cluster-api v0.0.0-20191129101638-b09907ac6668
	github.com/openstack-k8s-operators/compute-node-operator v0.0.0-20200422093450-2c9f428da728
	github.com/openstack-k8s-operators/lib-common v0.0.0-20200429140354-7adb5090d08d
	github.com/operator-framework/operator-sdk v0.17.0
	github.com/spf13/pflag v1.0.5
	golang.org/x/crypto v0.0.0-20200220183623-bac4c82f6975
	golang.org/x/tools v0.0.0-20200504022951-6b6965ac5dd1 // indirect
	k8s.io/api v0.17.4
	k8s.io/apimachinery v0.17.4
	k8s.io/client-go v12.0.0+incompatible
	sigs.k8s.io/controller-runtime v0.5.2
)

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v13.3.2+incompatible // Required by OLM
	k8s.io/client-go => k8s.io/client-go v0.17.4 // Required by prometheus-operator
)
