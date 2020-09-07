# nova-operator

NOTE: 
- The current functionality is on install at the moment, no update/upgrades.
- At the moment only covers nova-compute related services

## Pre Req:
- OCP 4 installed
- operator-sdk 1.x

## Clone it

    git clone https://github.com/openstack-k8s-operators/nova-operator.git
    cd nova-operator

## Create the operator

This is optional, a prebuild operator from quay.io/ltomasbo/compute-operator could be used, e.g. quay.io/mschuppe/nova-operator:v0.0.1 .

Build the image, using your custom registry you have write access to
 
    WATCH_NAMESPACE="openstack" make docker-build IMG=<image e.g quay.io/mschuppe/nova-operator:v0.0.X>

Push image to your registry:

    podman push --authfile quay.io/mschuppe/nova-operator:v0.0.X

## Run the operator local for dev

    WATCH_NAMESPACE="openstack" make run

## Pre Req to run the operator

Create required common-config configMap and osp-secrets secret

### common-config configMap

    mkdir common-conf

Add `keystoneAPI` file which holds the keystone endpoint url of the control plane:

    echo -n "http://keystone.openstack.svc:5000/" > common-conf/keystoneAPI

Add `glanceAPI` file which holds the glance endpoint url of the control plane:

    echo -n "http://glanceapi.openstack.svc:9292/" > common-conf/glanceAPI

Add `memcacheServers` file which holds the memcache server string:

    echo -n "192.168.25.20:11211" > common-conf/memcacheServers

Create the configMap

    oc create -n openstack configmap common-config --from-file=/root/common-conf/

Note: if a later update is needed do e.g.

    oc create -n openstack configmap common-config --from-file=./common-conf/ --dry-run -o yaml | oc apply -f -

Note: Right now the operator does not handle config updates to the common-config configMap.

### osp-secrets secret

Create a secrets yaml file like the following and add the base64 encoded information for the relevant parameters:

    apiVersion: v1
    kind: Secret
    metadata:
      name: osp-secrets
      namespace: openstack
    data:
      RabbitTransportURL: cmFiYml0Oi8vZ3Vlc3Q6cnBjLXBhc3N3b3JkQGNvbnRyb2xsZXItMC5pbnRlcm5hbGFwaS5vc3Rlc3Q6NTY3Mi8/c3NsPTA=
      NovaPassword: bm92YS1wYXNzd29yZA==
      CinderPassword: Y2luZGVyLXBhc3N3b3Jk
      NeutronPassword: bmV1dHJvbi1wYXNzd29yZA==
      PlacementPassword: cGxhY2VtZW50LXBhc3N3b3Jk

**NOTE** The strings to encode must not have an ending line break. Use e.g. the following command to get the encoded string:

    echo -n "nova-password" | base64

## Install the operator to an OCP env

The operator is intended to be deployed via OLM [Operator Lifecycle Manager](https://github.com/operator-framework/operator-lifecycle-manager)

If necessary check logs with

    POD=`oc get pods -l name=nova-operator --field-selector=status.phase=Running -o name | head -1 -`; echo $POD
    oc logs $POD -f


## Install compute-node-operator which manages nova-compute related CRs

Install the compute-node-operator from and create a machineset for the worker-osp role. When deployed the OSP worker is installed with additional worker-osp role:

    $ oc get nodes
    NAME       STATUS   ROLES               AGE     VERSION
    master-0   Ready    master,worker       4h19m   v1.17.1
    master-1   Ready    master,worker       4h19m   v1.17.1
    master-2   Ready    master,worker       4h19m   v1.17.1
    worker-0   Ready    worker              4h3m    v1.17.1
    worker-1   Ready    worker              4h4m    v1.17.1
    worker-2   Ready    worker,worker-osp   76m     v1.17.1


    oc get daemonset -n openstack
    NAME                    DESIRED   CURRENT   READY   UP-TO-DATE   AVAILABLE   NODE SELECTOR                         AGE
    iscsid                  1         1         1       1            1           node-role.kubernetes.io/worker-osp=   88m
    libvirtd                1         1         1       1            1           node-role.kubernetes.io/worker-osp=   12m
    nova-compute            1         1         1       1            1           node-role.kubernetes.io/worker-osp=   12m
    nova-migration-target   1         1         1       1            1           node-role.kubernetes.io/worker-osp=   12m
    virtlogd                1         1         1       1            1           node-role.kubernetes.io/worker-osp=   12m

    oc get pods -n openstack
    NAME                                READY   STATUS    RESTARTS   AGE
    iscsid-hq2h4                        1/1     Running   0          70m
    libvirtd-6wtsb                      1/1     Running   0          44m
    nova-compute-dlxdx                  1/1     Running   0          54m
    nova-migration-target-kt4vq         1/1     Running   0          54m
    nova-operator-57465b7dbf-rqrfn      1/1     Running   0          45m
    virtlogd-5cz7f                      1/1     Running   0          69m

    oc get pods -n openstack nova-compute-dlxdx -o yaml | grep nodeName
      nodeName: worker-2

Now the compute number can be scaled up using the compute-node-operator CR!

If need get into nova-compute container of daemonset via:

    oc rsh nova-compute-dlxdx


## (Optional) Manually create custom resource for a compute node

Usually compute-node resources are managed via the compute-node-operator. In case manual CR create is needed:

`config/samples/nova_v1beta1_libvirtd.yaml` and `config/samples/nova_v1beta1_virtlogd.yaml` use a patched nova-libvirt image with added cgroups tools which are required for the libvirtd.sh wrapper script. 

`config/samples/nova_v1beta1_virtlogd.yaml`:

    apiVersion: nova.openstack.org/v1beta1
    kind: Virtlogd
    metadata:
      name: virtlogd-worker-osp
      namespace: openstack
    spec:
      novaLibvirtImage: quay.io/openstack-k8s-operators/nova-libvirt:latest
      serviceAccount: nova
      roleName: worker-osp

`deploy/crds/nova.openstack.org_libvirtd_cr.yaml`:

    apiVersion: nova.openstack.org/v1beta1
    kind: Libvirtd
    metadata:
      name: libvirtd-worker-osp
      namespace: openstack
    spec:
      novaLibvirtImage: quay.io/openstack-k8s-operators/nova-libvirt:latest
      serviceAccount: nova
      roleName: worker-osp

Update `config/samples/nova_v1beta1_iscsid.yaml`, `config/samples/nova_v1beta1_novamigrationtarget.yaml` and `config/samples/nova_v1beta1_novacompute.yaml` with the details of the images and the OpenStack ctrl plane.

`config/samples/nova_v1beta1_iscsid.yaml`:

    apiVersion: nova.openstack.org/v1beta1
    kind: Iscsid
    metadata:
      name: iscsid-worker-osp
      namespace: openstack
    spec:
      iscsidImage: docker.io/tripleotrain/rhel-binary-iscsid:current-tripleo
      serviceAccount: nova
      roleName: worker-osp

`config/samples/nova_v1beta1_novamigrationtarget.yaml`:


    apiVersion: nova.openstack.org/v1beta1
    kind: NovaMigrationTarget
    metadata:
      name: nova-migration-target
      namespace: openstack
    spec:
      sshdPort: 2022
      novaComputeImage: docker.io/tripleotrain/rhel-binary-nova-compute:current-tripleo
      serviceAccount: nova-operator
      roleName: worker-osp

`deploy/crds/nova.openstack.org_novacompute_cr.yaml`:

    apiVersion: nova.openstack.org/v1beta1
    kind: NovaCompute
    metadata:
      name: nova-compute-worker-osp
      namespace: openstack
    spec:
      commonConfigMap: common-config
      ospSecrets: osp-secrets
      novaComputeCPUDedicatedSet: 4-7
      novaComputeCPUSharedSet: 0-3
      novaComputeImage: docker.io/tripleotrain/rhel-binary-nova-compute:current-tripleo
      serviceAccount: nova
      roleName: worker-osp


If instances with CPU pinning are used, the cores which are set for novaComputeCPUDedicatedSet should be excluded from
the kernel scheduler. With this it is sure that the core is exclusive for the pinned instances.

Using the machine configuartion operator additinal kernel parameters can be set like with the following yaml.
In this example core range 4-7 are isolated using `isolcpus`:

    apiVersion: machineconfiguration.openshift.io/v1
    kind: MachineConfig
    metadata:
      labels:
        machineconfiguration.openshift.io/role: worker
      name: 05-compute-cpu-pinning
    spec:
      config:
        ignition:
          version: 2.2.0
      kernelArguments:
        - isolcpus=4-7

Apply the CRs:

    oc apply -f config/samples/nova_v1beta1_novamigrationtarget.yaml
    oc apply -f config/samples/nova_v1beta1_virtlogd.yaml
    oc apply -f config/samples/nova_v1beta1_iscsid.yaml
    oc apply -f config/samples/nova_v1beta1_novamigrationtarget.yaml
    oc apply -f deploy/crds/nova.openstack.org_novacompute_cr.yaml

    oc get pods -n openstack
    NAME                           READY   STATUS    RESTARTS   AGE
    nova-operator-ffd64796-vshg6   1/1     Running   0          119s

    NAME                    DESIRED   CURRENT   READY   UP-TO-DATE   AVAILABLE   NODE SELECTOR                         AGE
    iscsid                  0         0         0       0            0           node-role.kubernetes.io/worker-osp=   88m
    libvirtd                0         0         0       0            0           node-role.kubernetes.io/worker-osp=   12m
    nova-compute            0         0         0       0            0           node-role.kubernetes.io/worker-osp=   12m
    nova-migration-target   0         0         0       0            0           node-role.kubernetes.io/worker-osp=   12m
    virtlogd                0         0         0       0            0           node-role.kubernetes.io/worker-osp=   12m


## Cleanup

* First delete all instances running on the OCP worker
* Scale down all compute workers
* Delete the operator using OLM

## Formatting

For code formatting we are using goimports. It based on go fmt but also adding missing imports and removing unreferenced ones.

    go get golang.org/x/tools/cmd/goimports
    export PATH=$PATH:$GOPATH/bin
    goimports -w -v ./

