# Goals of the nova-operator

The high-level goal is to provide a set of APIs in a form of Kubernetes Custom
Resource Definitions (CRD) implemented with Operator SDK to deploy OpenStack
Nova and Placement control plane services in OpenShift.

The Placement service code that previously lived in placement-operator is now
managed by the same nova-operator binary and OLM bundle. There is no separate
placement-operator deployment.

# Decisions

1. The nova-operator provides a single top-level Nova API for the OpenStack
operator to instantiate a Nova control plane by creating a single Custom
Resource (CR). We support this to hide any nova-specific deployment logic from
the OpenStack operator. Placement has no equivalent top-level CR; only the
PlacementAPI service CR exists.

2. The nova-operator allows deploying control plane services independently by
instantiating a service-level CRD without the matching top-level CR. We support
this to limit the dependency of each service CRD to the minimum and by that to
allow testing of each service CRD in isolation. This applies to the
Nova*ServiceType* CRDs and the PlacementAPI CRD.

    1. For the Nova*ServiceType* CRDs, they get their DB and message bus
    dependencies as Secret objects having user, password, and hostname fields.
    The top level Nova CRD is responsible to select the
    appropriate k8s Service instance to get the hostname of the service and to
    register a user there. Then the Nova CRD packages this connection
    information to Secret objects and pass those to lower level CRDs.

    2. For the PlacementAPI CRD, it gets its DB and service password
    dependencies from the Secret referenced in
    `PlacementAPI.Spec.Secret` and from the MariaDB instance referenced in
    `PlacementAPI.Spec.DatabaseInstance`. The openstack-operator is responsible
    to create the PlacementAPI CR and the supporting Secrets in the deployment
    namespace. Nova services such as nova-scheduler consume the placement API
    via a registered keystone endpoint; they do not deploy PlacementAPI as part
    of the Nova CR reconciliation.

3. The nova-operator provides Nova Cells v2 aware deployment structure by
default to support scaling the deployment to more than one real cell without
the need to restructure the existing deployment. This means:

    1. The deployment always uses a set of nova-conductor services in
    superconductor mode.

    2. The deployment supports nova-metadata service deployed either globally
    for all cells or locally for each cell.

# Design details

## Nova configuration generation

The configuration for the podified Nova services are generated into Secrets.
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
managed by the dataplane controller that runs in the openstack-operator. Users
interact with this via the generic OpenStackDataPlaneService CR.
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
Secret is `<Nova/name>-metadata-neutron-config` (i.e.
`nova-metadata-neutron-config`).

2. If the nova-metadata service is deployed per cell
(i.e. `Nova.Spec.CellTemplates[].MetadataServiceTemplate.Enabled` is true) then
the name of the Secrets are `<Nova/name>-<cell name>-metadata-neutron-config`
(i.e in cell1 `nova-cell1-metadata-neutron-config`)

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

## PlacementAPI configuration generation

The configuration for the podified placement-api service is generated into
Secrets named after the PlacementAPI CR. For a PlacementAPI CR named
`placement` the operator generates `<PlacementAPI name>-config-data` (e.g.
`placement-config-data`) and `<PlacementAPI name>-scripts` (e.g.
`placement-scripts`). Before the placement-api Deployment is created, the
operator runs a db sync Job named `<PlacementAPI name>-db-sync`.

The config secrets will contain multiple keys if the user provided
`CustomServiceConfig` in the service CR. These config secrets are mounted to
the pod and kolla is used to copy the resulting config files to
`/etc/placement/placement.conf.d/`. The nova-operator will generate the default
configuration from templates under `templates/placement/api/` and copy the user
defined snippet to the Secret to key `custom.conf`. So the user defined
configuration always override the default config.
