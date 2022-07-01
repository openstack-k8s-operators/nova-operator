# OpenShift CRDs
In our EnvTest we need to define all the CRDs our operator depends on so we
need the CRD for ``github.com/openshift/api/route/v1``.

Based on [issue-1191](https://github.com/kubernetes-sigs/controller-runtime/issues/1191#issuecomment-833058115)
OpenShift does not publish CRDs for every resources it provides. For example
``route/v1 `` cannot be even generated with ``make update-codegen-crds`` in the
[openshift/api](https://github.com/openshift/api/) repository without the
following trick.

1. Create a stub ``route_crd.yaml`` file under ``route/v1`` with the content:
```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  # name must be in the form: <plural>.<group>
  # <group> has to match the correspondig .spec field
  name: routes.route.openshift.io
spec:
  # group name to use for REST API: /apis/<group>/<version>
  group: route.openshift.io
  versions:
    - name: v1
      served: true
      storage: true
  scope: Namespaced
  subresources:
    # enable spec/status
    status: {}
  names:
    plural: routes
    singular: route
    kind: Route
```
2. Add the following to the Makfile
```
$(call add-crd-gen,route,./route/v1,./route/v1,./route/v1)
```
3. Run ``make update-codegen-crds``. This will update the ``route_crd.yaml``
with the full CRD definition.

We store such generated CRDs in our repo under ``openshift_crds`` for now to
avoid the need for regenerating them for every run.
