
# Testing with local nova


Nova is included in several services in the control plane: nova-api, nova-cellX-conductor, nova-cellX-novncproxy, nova-metadata, and nova-scheduler container image. The nova-operator doesn't support extra volume mounts for any of the mentioned pods. However, thanks to oc debug, it is possible to develop and test local changes to nova without having to build and deploy new Nova OpenStack changes.

## Provide NFS access to your nova-openstack directory


The technique described here uses NFS to access the nova opensctack directory on
your development system, so you'll need to install an NFS server and create
an appropriate export on your development system. Of course, this implies
your OpenShift deployment that runs the openstack-operator has access to
the NFS server, including any required firewall rules.

When using OpenShift Local (aka CRC), your export will be something like this:

```
    % echo "${HOME}/nova/nova 192.168.130.0/24(rw,sync,no_root_squash)" > /etc/exports

    % exportfs -r
```

Make sure nfs-server and firewalld are started:


```    % systemctl start firewalld
    % systemctl start nfs-server
```
### tip::

   CRC installs its own firewall rules, which likely will need to be adjusted
   depending on the location of your NFS server. If your nova-openstack
   directory is on the same system that hosts your CRC, then the simplest
   thing to do is insert a rule that essentially circumvents the other rules:

   `% nft add rule inet firewalld filter_IN_libvirt_pre accept`

## Create nova openstack PV and PVC


Create an NFS PV, and a PVC that can be mounted on the ansibleee pods.

### note::

   While it's possible to add an NFS volume directly to a pod, the default k8s
   Security Context Constraint (SCC) for non-privileged pods does not permit
   NFS volume mounts. The approach of using an NFS PV and PVC works just as
   well, and avoids the need to fiddle with SCC policies.

```
    NFS_SHARE=<Path to your nova directory>
    NFS_SERVER=<IP of your NFS server>
    cat <<EOF >nova-openstack-storage.yaml
    apiVersion: v1
    kind: PersistentVolume
    metadata:
      # Do not use just "nova" for the metadata name!
      name: nova-openstack-dev
    spec:
      capacity:
        storage: 1Gi
      volumeMode: Filesystem
      accessModes:
        - ReadOnlyMany
      # IMPORTANT! The persistentVolumeReclaimPolicy must be "Retain" or else
      # your code will be deleted when the volume is reclaimed!
      persistentVolumeReclaimPolicy: Retain
      storageClassName: nova-openstack
      mountOptions:
        - nfsvers=4.1
      nfs:
        path: ${NFS_SHARE}
        server: ${NFS_SERVER}
    ---
    apiVersion: v1
    kind: PersistentVolumeClaim
    metadata:
      name: nova-openstack-dev
    spec:
      storageClassName: nova-openstack
      accessModes:
        - ReadOnlyMany
      resources:
        requests:
          storage: 1Gi
    EOF

    oc apply -f nova-openstacke-storage.yaml
```

## Add additional mounts to your  CR


First, we need to get the correct pod definition to debug. From oc get pod, find the pod in which you want to replace Nova and then run:
```
oc debug --keep-labels=true pod/nova-<pod_name> -o yaml
```

Next, edit the file to add the Nova OpenStack PVC to the pod definition:

```
spec:
  containers:
    volumeMounts:
      - mountPath: /usr/lib/python3.9/site-packages/nova
        name: nova-openstack
      name: config-data
        volumes:
        - name: nova-openstack
          persistentVolumeClaim:
            claimName: nova-openstack
            readOnly: true
```

Now we can spawn the container using: `oc debug -f <file_with_pod_definition>`
Finally, scale your pod to zero by using `oc edit OpenStackControlPlane` and editing the replicas of the selected service to 0.
