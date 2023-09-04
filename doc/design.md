# Goals of the Nova operator

The high-level goal is to provide a set of APIs in a form of Kubernetes Custom
Resource Definitions (CRD) implemented with Operator SDK to deploy OpenStack
Nova control plane in OpenShift.

# Decisions

1. The Nova operator provides a single top-level API for the OpenStack
operator to instantiate a Nova control plane by creating a single Custom
Resource (CR). We support this to hide any nova-specific deployment logic from
the OpenStack operator.

2. The Nova operator allows deploying every Nova service independently by
instantiating the matching Nova*ServiceType* CRD without the need to create
higher-level CRDs. We support this to limit the dependency of each
Nova*ServiceType* CRD to the minimum and by that to allow testing of each
Nova*ServiceType* CRD in isolation. As a consequence the Nova*ServiceType* CRDs
get their DB and message bus dependencies as Secret objects having user,
password, and hostname fields. The top level Nova CRD is responsible to select
the appropriate k8s Service instance to get the hostname of the service and to
register a user there. Then the Nova CRD packages this connection information
to Secret objects and pass those to lower level CRDs.

3. The Nova operator provides Nova Cells v2 aware deployment structure by
default to support scaling the deployment to more than one real cell without
the need to restructure the existing deployment. This means:

    1. The deployment always uses a set of nova-conductor services in
    superconductor mode.

    2. The deployment supports nova-metadata service deployed either globally
    for all cells or locally for each cell.

# Design details

## Configuration generation
The configuration for the podified nova services are generated into Secrets.
The top level services generate Secrets in the form of
'`nova-<service-name>-config-data` (e.g. `nova-scheduler-config-data`). The
cell level service config Secrets are named with the pattern
`nova-<cell-name>-<service-name>-config-data` (e.g.
`nova-cell0-conductor-config-data`). The conductor service has a Secret
`nova-<cell-name>-conductor-config-data` containing the db sync script.

There is an extra set of config and script Secret generated to run the cell
mapping job. They are named `nova-cell0-manage-config-data` and
`nova-cell0-manage-scripts`.

The config secrets will contain multiple keys if the user provided
`CustomServiceConfig` in the service CR. These config secrets are mounted to
the pod and kolla is used to copy the resulting config files to
`/etc/nova/nova.conf.d/`. So the key names in the Secret defines the order how
oslo.config will apply the different config snippets. The nova-operator will
generate the default configuration under the key `01-nova.conf` and copy the
user defined snippet to the Secret to key `02-nova-override.conf`. So the user
defined configuration always override the default config.

### Config generation for the services running on the external data plane node

#### nova-compute
The main nova service running on the EDP node is nova-compute. However the
nova-operator is not responsible to deploy or manage such service. It is
managed by the dataplane-operator via the generic OpenStackDataPlaneService CR.
The only responsibility of nova-operator in regards of nova-compute is to
generate the basic control plane configuration needed for the nova-compute
service to connect to the control plane properly. It is done by generating a
Secret per NovaCell with the name of
`<Nova CR name>-<NovaCell CR name>-compute-config` (e.g.
`nova-cell1-compute-config`) The human operator needs to include this Secret
in the Secrets field of the OpenStackDataPlaneService CR describing the
nova-compute service.

#### neutron-metadata-agent
The nova-metadata service is deployed by the nova-operator. For the guest VMs
to be able to access this metadata service the neutron-metadata-agent needs to
be deployed on the EDPM side and it needs to be able to talk to the
nova-metadata service running in k8s. The nova-operator generates the
necessary neutron-metadata-agent config snippet that defines how the
nova-metadata service can be accessed by the neutron-metadata-agent.

There are two possible nova-metadata deployment modes and this also means that
there are two possible configuration schemes for the neutron-metadata-agent.

1. If a single nova-metadata service is deployed on the top level (i.e. when
`Nova.Spec.MetadataServiceTemplate.Enabled` is true) then the name of the
Secret is `<Nova/name>-metadata-compute-config` (i.e.
`nova-metadata-compute-config`).

2. If the nova-metadata service is deployed per cell
(i.e. `Nova.Spec.CellTemplates[].MetadataServiceTemplate.Enabled` is true) then
the name of the Secrets are `<Nova/name>-<cell name>-metadata-compute-config`
(i.e in cell1 `nova-cell1-metadata-compute-config`)

Then the human operator needs to include the appropriate Secret into the
Secrets field of OpenStackDataPlaneService CR describing the
neutron-metadata-agent.

In both cases the Secret contains a single key `05-nova-metadata.conf` with a
value of an oslo.config snippet that provides two pieces of information to
the neutron-metadata-agent:
1. The URL of the nova-metadata service
2. A secret that is shared between the nova-metadata service and the
neutron-metadata-agent so the former can authenticate the requests from the
latter.
