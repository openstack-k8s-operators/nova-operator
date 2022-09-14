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
