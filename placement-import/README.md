# placement-operator

A Kubernetes Operator built using the [Operator Framework](https://github.com/operator-framework) for Go.
The Operator provides a way to easily install and manage an OpenStack Placement installation on Kubernetes.
This Operator was developed using [RDO](https://www.rdoproject.org/) containers for openStack.

# Deployment

The operator is intended to be deployed via OLM [Operator Lifecycle Manager](https://github.com/operator-framework/operator-lifecycle-manager)

# API Example

The Operator creates a custom PlacementAPI resource that can be used to create Placement API
instances within the cluster. Example CR to create a Placement API in your cluster:

```yaml
apiVersion: placement.openstack.org/v1beta1
kind: PlacementAPI
metadata:
  name: placement
spec:
  containerImage: quay.io/tripleotraincentos8/centos-binary-placement-api:current-tripleo
  databaseInstance: openstack
  secret: placement-secret
```

# Design
*TBD*
