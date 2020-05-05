# nova-operator

NOTE: 
- The current functionality is on install at the moment, no update/upgrades.
- At the moment only covers nova-compute related services (virtlogd/libvirtd/nova-compute)

## Pre Req:
- OSP16 with OVS instead of OVN deployed
- worker nodes have connection to internalapi and tenant network VLAN

#### Clone it

    mkdir openstack-k8s-operators
    cd openstack-k8s-operators
    git clone https://github.com/openstack-k8s-operators/nova-operator.git
    cd nova-operator

#### Create the operator

This is optional, a prebuild operator from quay.io/openstack-k8s-operators/nova-operator could be used, e.g. quay.io/openstack-k8s-operators/nova-operator:v0.0.1 .

Create CRDs
    
    oc create -f deploy/crds/nova_v1_virtlogd_crd.yaml
    oc create -f deploy/crds/nova_v1_libvirtd_crd.yaml
    oc create -f deploy/crds/nova_v1_novacompute_crd.yaml
    oc create -f deploy/crds/nova_v1_iscsid_crd.yaml
    oc create -f deploy/crds/nova_v1_novamigrationtarget_crd.yaml

Build the image, using your custom registry you have write access to

    operator-sdk build --image-builder buildah <image e.g quay.io/openstack-k8s-operators/nova-operator:v0.0.X>

Replace `image:` in deploy/operator.yaml with your custom registry

    sed -i 's|REPLACE_IMAGE|quay.io/openstack-k8s-operators/nova-operator:v0.0.X|g' deploy/operator.yaml
    podman push --authfile ~/mschuppe-auth.json quay.io/openstack-k8s-operators/nova-operator:v0.0.X

#### Install the operator

Create CRDs
    
    oc create -f deploy/crds/nova_v1_virtlogd_crd.yaml
    oc create -f deploy/crds/nova_v1_libvirtd_crd.yaml
    oc create -f deploy/crds/nova_v1_novacompute_crd.yaml
    oc create -f deploy/crds/nova_v1_iscsid_crd.yaml
    oc create -f deploy/crds/nova_v1_novamigrationtarget_crd.yaml

Create namespace

    oc create -f deploy/namespace.yaml

Create role, role_binding and service_account

    oc create -f deploy/role.yaml
    oc create -f deploy/role_binding.yaml
    oc create -f deploy/service_account.yaml

Create security context constraints

    oc create -f deploy/scc.yaml

Install the operator

    oc create -f deploy/operator.yaml

If necessary check logs with

    POD=`oc get pods -l name=nova-operator --field-selector=status.phase=Running -o name | head -1 -`; echo $POD
    oc logs $POD -f

Create custom resource for a compute node

`deploy/crds/nova_v1_novacompute_cr.yaml` and `deploy/crds/nova_v1_virtlogd_cr.yaml` use a patched nova-libvirt image with added cgroups tools which are required for the libvirtd.sh wrapper script. 

`deploy/crds/nova_v1_virtlogd_cr.yaml` and `deploy/crds/nova_v1_libvirtd_cr.yaml`:

    apiVersion: nova.openstack.org/v1
    kind: Virtlogd
    metadata:
      name: virtlogd
      namespace: openstack
    spec:
      novaLibvirtImage: quay.io/openstack-k8s-operators/nova-libvirt:latest
      label: compute
      serviceAccount: nova-operator

`deploy/crds/nova_v1_libvirtd_cr.yaml`:

    apiVersion: nova.openstack.org/v1
    kind: Libvirtd
    metadata:
      name: libvirtd
      namespace: openstack
    spec:
      novaLibvirtImage: quay.io/openstack-k8s-operators/nova-libvirt:latest
      label: compute
      serviceAccount: nova-operator


Update `deploy/crds/nova_v1_iscsid_cr.yaml`, `deploy/crds/nova_v1_novamigrationtarget_cr.yaml` and `deploy/crds/nova_v1_novacompute_cr.yaml` with the details of the images and the OpenStack environment.

`deploy/crds/nova_v1_iscsid_cr.yaml`:

    apiVersion: nova.openstack.org/v1
    kind: Iscsid
    metadata:
      name: iscsid
      namespace: openstack
    spec:
      iscsidImage: docker.io/tripleotrain/rhel-binary-iscsid:current-tripleo
      label: compute
      serviceAccount: nova-operator

`deploy/crds/nova_v1_novamigrationtarget_cr.yaml`:

    apiVersion: nova.openstack.org/v1
    kind: NovaMigrationTarget
    metadata:
      name: nova-migration-target
      namespace: openstack
    spec:
      sshdPort: 2022
      novaComputeImage: docker.io/tripleotrain/rhel-binary-nova-compute:current-tripleo
      label: compute
      serviceAccount: nova-operator

`deploy/crds/nova_v1_novacompute_cr.yaml`:

    apiVersion: nova.openstack.org/v1
    kind: NovaCompute
    metadata:
      name: nova-compute
      namespace: openstack
    spec:
      # Public and internal VIP of the OSP controllers
      publicVip: 10.0.0.143
      internalAPIVip: 172.17.1.29
      # Memcached server list
      memcacheServers: 172.17.1.83:11211
      # Rabbit transport url
      rabbitTransportURL: rabbit://guest:eJNAlgHTTN8A6mclF6q6dBdL1@controller-0.internalapi.redhat.local:5672/?ssl=0
      # User Passwords
      cinderPassword: kxtKYHKLdbEGzfGN6OTESU66t
      novaPassword: ytalxxwY2ovYx0FcQpjbfFeK1
      neutronPassword: HKCe8oWszT6brfYlJUPHH3moh
      placementPassword: 3e2CahSMAw8xMA576ORh0yaWc
      # Optional parameters to configure cores to be used for pinned/non pinned instances
      #novaComputeCPUDedicatedSet: 4-7
      #novaComputeCPUSharedSet: 0-3

      novaComputeImage: docker.io/tripleotrain/rhel-binary-nova-compute:current-tripleo
      label: compute
      serviceAccount: nova-operator

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

    oc apply -f deploy/crds/nova_v1_virtlogd_cr.yaml
    oc apply -f deploy/crds/nova_v1_libvirtd_cr.yaml
    oc apply -f deploy/crds/nova_v1_iscsid_cr.yaml
    oc apply -f deploy/crds/nova_v1_novamigrationtarget_cr.yaml
    oc apply -f deploy/crds/nova_v1_novacompute_cr.yaml

    oc get pods
    NAME                           READY   STATUS    RESTARTS   AGE
    nova-operator-ffd64796-vshg6   1/1     Running   0          119s

    oc get ds
    NAME           DESIRED   CURRENT   READY   UP-TO-DATE   AVAILABLE   NODE SELECTOR    AGE
    nova-compute   0         0         0       0            0           daemon=compute   118s

### Create required common-config configMap

Get the following config from a compute node in the OSP env:
- /etc/hosts

Place it in a config dir like:
- common-conf

Add OSP environment controller-0 short hostname in common-conf/osp_controller_hostname

    echo "SHORT OSP CTRL-0 HOSTNAME"> /root/common-conf/osp_controller_hostname

Create the configMap

    oc create configmap common-config --from-file=/root/common-conf/

Note: if a later update is needed do e.g.

    oc create configmap common-config --from-file=./common-conf/ --dry-run -o yaml | oc apply -f -

Note: Right now the operator does not handle config updates to the common-config configMap. The pod needs to be recreated that the hosts file entries get updated

!! Make sure we have the OSP needed network configs on the worker nodes. The workers need to be able to reach the internalapi, storage and tenant network !!

    $ oc get nodes
    NAME       STATUS   ROLES    AGE   VERSION
    master-0   Ready    master   8d    v1.14.6+8e46c0036
    worker-0   Ready    worker   8d    v1.14.6+8e46c0036
    worker-1   Ready    worker   8d    v1.14.6+8e46c0036

Label a worker node as compute

    # oc label nodes worker-0 daemon=compute --overwrite
    node/worker-0 labeled

    oc get daemonset
    NAME                    DESIRED   CURRENT   READY   UP-TO-DATE   AVAILABLE   NODE SELECTOR    AGE
    iscsid                  1         1         1       1            1           daemon=compute   69m
    libvirtd                1         1         1       1            1           daemon=compute   43m
    neutron-ovsagent        1         1         1       1            1           daemon=compute   5d23h
    nova-compute            1         1         1       1            1           daemon=compute   54m
    nova-migration-target   1         1         1       1            1           daemon=compute   54m
    virtlogd                1         1         1       1            1           daemon=compute   68m

    oc get pods
    NAME                                READY   STATUS    RESTARTS   AGE
    iscsid-hq2h4                        1/1     Running   1          70m
    libvirtd-6wtsb                      1/1     Running   0          44m
    nova-compute-dlxdx                  1/1     Running   1          54m
    nova-migration-target-kt4vq         1/1     Running   1          54m
    nova-operator-57465b7dbf-rqrfn      1/1     Running   1          45m
    virtlogd-5cz7f                      1/1     Running   1          69m

    oc get pods nova-compute-dlxdx -o yaml | grep nodeName
      nodeName: worker-0

Label 2nd worker node

    oc label nodes worker-1 daemon=compute --overwrite
    node/worker-1 labeled

    oc get daemonset
    NAME                    DESIRED   CURRENT   READY   UP-TO-DATE   AVAILABLE   NODE SELECTOR    AGE
    iscsid                  2         2         2       2            2           daemon=compute   69m
    libvirtd                2         2         2       2            2           daemon=compute   43m
    neutron-ovsagent        2         2         2       2            2           daemon=compute   5d23h
    nova-compute            2         2         2       2            2           daemon=compute   54m
    nova-migration-target   2         2         2       2            2           daemon=compute   54m
    virtlogd                2         2         2       2            2           daemon=compute   68m

    oc get pods
    NAME                                READY   STATUS    RESTARTS   AGE
    iscsid-hq2h4                        1/1     Running   1          70m
    iscsid-ltqxl                        1/1     Running   1          66m
    libvirtd-6wtsb                      1/1     Running   0          44m
    libvirtd-ddrr2                      1/1     Running   0          44m
    neutron-operator-855c5b58bf-vhxvn   1/1     Running   4          5d2h
    neutron-ovsagent-hs4ck              1/1     Running   8          5d23h
    neutron-ovsagent-tqrfl              1/1     Running   9          5d23h
    nova-compute-dlxdx                  1/1     Running   1          54m
    nova-compute-zcczn                  1/1     Running   1          54m
    nova-migration-target-kt4vq         1/1     Running   1          54m
    nova-migration-target-tb9xt         1/1     Running   1          54m
    nova-operator-57465b7dbf-rqrfn      1/1     Running   1          45m
    virtlogd-5cz7f                      1/1     Running   1          69m
    virtlogd-mpm6g                      1/1     Running   1          69m

    oc get pods -o custom-columns='NAME:metadata.name,NODE:spec.nodeName'
    NAME                   NODE
    nova-compute-dlxdx     worker-0
    nova-compute-zcczn     worker-1
    ...

If need get into nova-compute container of daemonset via:

    oc exec nova-compute-dr6j5 -i -t -- bash -il

## POST steps to add compute workers to the cell

#### Map the computes to the default cell

    (undercloud) $ source stackrc
    (undercloud) $ CTRL=controller-0
    (undercloud) $ CTRL_IP=$(openstack server list -f value -c Networks --name $CTRL | sed 's/ctlplane=//')
    (undercloud) $ export CONTAINERCLI='podman'
    (undercloud) $ ssh heat-admin@${CTRL_IP} sudo ${CONTAINERCLI} exec -i -u root nova_api  nova-manage cell_v2 discover_hosts --by-service --verbose
    Warning: Permanently added '192.168.24.44' (ECDSA) to the list of known hosts.
    Found 2 cell mappings.
    Skipping cell0 since it does not contain hosts.
    Getting computes from cell 'default': ba9981ae-1e79-4b20-a6ff-0416f986af3b
    Creating host mapping for service worker-0
    Creating host mapping for service worker-1
    Found 2 unmapped computes in cell: ba9981ae-1e79-4b20-a6ff-0416f986af3b

    (undercloud) $ ssh heat-admin@${CTRL_IP} sudo ${CONTAINERCLI} exec -i -u root nova_api  nova-manage cell_v2 list_hosts
    Warning: Permanently added '192.168.24.44' (ECDSA) to the list of known hosts.
    +-----------+--------------------------------------+------------------------+
    | Cell Name |              Cell UUID               |        Hostname        |
    +-----------+--------------------------------------+------------------------+
    |  default  | ba9981ae-1e79-4b20-a6ff-0416f986af3b | compute-0.redhat.local |
    |  default  | ba9981ae-1e79-4b20-a6ff-0416f986af3b | compute-1.redhat.local |
    |  default  | ba9981ae-1e79-4b20-a6ff-0416f986af3b |        worker-0        |
    |  default  | ba9981ae-1e79-4b20-a6ff-0416f986af3b |        worker-1        |
    +-----------+--------------------------------------+------------------------+

#### Create an AZ and add the OCP workers at it

    (undercloud) $ source overcloudrc
    (overcloud) $ openstack aggregate create --zone ocp ocp
    (overcloud) $ openstack aggregate add host ocp worker-0
    (overcloud) $ openstack aggregate add host ocp worker-1
    (overcloud) $ openstack availability zone list --compute --long
    +-----------+-------------+---------------+---------------------------+----------------+----------------------------------------+
    | Zone Name | Zone Status | Zone Resource | Host Name                 | Service Name   | Service Status                         |
    +-----------+-------------+---------------+---------------------------+----------------+----------------------------------------+
    | internal  | available   |               | controller-0.redhat.local | nova-conductor | enabled :-) 2020-01-20T13:54:35.000000 |
    | internal  | available   |               | controller-0.redhat.local | nova-scheduler | enabled :-) 2020-01-20T13:54:34.000000 |
    | nova      | available   |               | compute-1.redhat.local    | nova-compute   | enabled :-) 2020-01-20T13:54:40.000000 |
    | nova      | available   |               | compute-0.redhat.local    | nova-compute   | enabled :-) 2020-01-20T13:54:42.000000 |
    | ocp       | available   |               | worker-0                  | nova-compute   | enabled :-) 2020-01-20T13:54:32.000000 |
    | ocp       | available   |               | worker-1                  | nova-compute   | enabled :-) 2020-01-20T13:54:35.000000 |
    +-----------+-------------+---------------+---------------------------+----------------+----------------------------------------+

#### Check nova compute service shows as up on the worker nodes

    (overcloud) $ openstack compute service list -c Id -c Host -c Zone -c State
    +---------------------------+----------+-------+
    | Host                      | Zone     | State |
    +---------------------------+----------+-------+
    | controller-0.redhat.local | internal | up    |
    | controller-0.redhat.local | internal | up    |
    | compute-0.redhat.local    | nova     | up    |
    | compute-1.redhat.local    | nova     | up    |
    | worker-0                  | ocp      | up    |
    | worker-1                  | ocp      | up    |
    +---------------------------+----------+-------+

## Start an instance and verify network connectivity works

NOTE: install the ovs agent operator before start an instance!
NOTE: selinux needs to be disabled on the compute worker nodes to start an instance

    2020-01-21 10:28:12.280 164015 ERROR nova.compute.manager [instance: fd1cf110-3921-4a65-b45d-807709fe5008] libvirt.libvirtError: internal error: process exited while connecting to monitor: libvirt:  error : cannot execute binary /usr/libexec/qemu-kvm: Permission denied

### Create two instances

    (overcloud) $ openstack server create --flavor m1.small --image cirros --nic net-id=$(openstack network list --name private -f value -c ID) --availability-zone ocp test
    (overcloud) $ openstack server create --flavor m1.small --image cirros --nic net-id=$(openstack network list --name private -f value -c ID) --availability-zone ocp test2
    (overcloud) $ openstack server list --long -c ID -c Name -c Status -c "Power State" -c Networks -c Host
    +--------------------------------------+-------+--------+-------------+-----------------------+----------+
    | ID                                   | Name  | Status | Power State | Networks              | Host     |
    +--------------------------------------+-------+--------+-------------+-----------------------+----------+
    | 55eb5cef-2580-48b8-a3ee-d27e96979fac | test2 | ACTIVE | Running     | private=192.168.0.58  | worker-0 |
    | 516a6b9c-a88d-4718-96bc-83d4315249fc | test  | ACTIVE | Running     | private=192.168.0.117 | worker-1 |
    +--------------------------------------+-------+--------+-------------+-----------------------+----------+

### Check tenant network connectivity from inside the dhcp namespace 

    (undercloud) [stack@undercloud-0 ~]$ ssh heat-admin@192.168.24.44
    [heat-admin@controller-0 ~]$ sudo -i

#### Ping instances from inside the namespace

Note: If it fails it might be that you need to apply OpenStack security rules!

    [root@controller-0 ~]# ip netns exec qdhcp-3821b285-fcc4-485b-89c1-6a5d242e7742 sh
    sh-4.4# ping -c 1 192.168.0.58
    PING 192.168.0.58 (192.168.0.58) 56(84) bytes of data.
    64 bytes from 192.168.0.58: icmp_seq=1 ttl=64 time=1.07 ms

    --- 192.168.0.58 ping statistics ---
    1 packets transmitted, 1 received, 0% packet loss, time 0ms
    rtt min/avg/max/mdev = 1.074/1.074/1.074/0.000 ms

    sh-4.4# ping -c 1 192.168.0.117
    PING 192.168.0.117 (192.168.0.117) 56(84) bytes of data.
    64 bytes from 192.168.0.117: icmp_seq=1 ttl=64 time=5.05 ms

    --- 192.168.0.117 ping statistics ---
    1 packets transmitted, 1 received, 0% packet loss, time 0ms
    rtt min/avg/max/mdev = 5.051/5.051/5.051/0.000 ms

#### Login to one of the instances via ssh 

    sh-4.4# ssh cirros@192.168.0.58
    The authenticity of host '192.168.0.58 (192.168.0.58)' can't be established.
    RSA key fingerprint is SHA256:TYk+lqGQ3tqWgmCe7CnmoJXZ15MYXcF2ANnv0vWpNn0.
    Are you sure you want to continue connecting (yes/no/[fingerprint])? yes

    Warning: Permanently added '192.168.0.58' (RSA) to the list of known hosts.
    cirros@192.168.0.58's password:

#### From the instance ping the other one

    $ ping -c 1 192.168.0.117
    PING 192.168.0.117 (192.168.0.117): 56 data bytes
    64 bytes from 192.168.0.117: seq=0 ttl=64 time=4.434 ms

    --- 192.168.0.117 ping statistics ---
    1 packets transmitted, 1 packets received, 0% packet loss
    round-trip min/avg/max = 4.434/4.434/4.434 ms

## Cleanup

First delete all instances running on the OCP worker AZ

    oc delete -f deploy/crds/nova_v1_novacompute_cr.yaml
    oc delete -f deploy/crds/nova_v1_libvirtd_cr.yaml
    oc delete -f deploy/crds/nova_v1_virtlogd_cr.yaml
    oc delete -f deploy/crds/nova_v1_iscsid_cr.yaml
    oc delete -f deploy/crds/nova_v1_novamigrationtarget_cr.yaml
    oc delete -f deploy/operator.yaml
    oc delete -f deploy/role.yaml
    oc delete -f deploy/role_binding.yaml
    oc delete -f deploy/service_account.yaml
    oc delete -f deploy/scc.yaml
    oc delete -f deploy/namespace.yaml
    oc delete -f deploy/crds/nova_v1_novacompute_crd.yaml
    oc delete -f deploy/crds/nova_v1_libvirtd_crd.yaml
    oc delete -f deploy/crds/nova_v1_virtlogd_crd.yaml
    oc delete -f deploy/crds/nova_v1_iscsid_crd.yaml
    oc delete -f deploy/crds/nova_v1_novamigrationtarget_crd.yaml

## Formatting

For code formatting we are using goimports. It based on go fmt but also adding missing imports and removing unreferenced ones.

    go get golang.org/x/tools/cmd/goimports
    export PATH=$PATH:$GOPATH/bin
    goimports -w -v ./

