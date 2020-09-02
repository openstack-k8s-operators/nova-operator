# Build the manager binary
FROM golang:1.13 as builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY main.go main.go
COPY api/ api/
COPY controllers/ controllers/
COPY pkg/ pkg/
COPY tools/ tools/

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o manager main.go
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o csv-generator tools/csv-generator.go

FROM registry.access.redhat.com/ubi7/ubi-minimal:latest
ENV USER_UID=1001 \
    OPERATOR_TEMPLATES=/usr/share/nova-operator/templates/ \
    OPERATOR_BUNDLE=/usr/share/nova-operator/bundle/

# install our templates
RUN  mkdir -p ${OPERATOR_TEMPLATES}
COPY templates ${OPERATOR_TEMPLATES}

# install CRDs and required roles, services, etc
RUN  mkdir -p ${OPERATOR_BUNDLE}
COPY config/crd/bases/nova.openstack.org_iscsids.yaml ${OPERATOR_BUNDLE}/nova.openstack.org_iscsids_crd.yaml
COPY config/crd/bases/nova.openstack.org_libvirtds.yaml ${OPERATOR_BUNDLE}/nova.openstack.org_libvirtds_crd.yaml
COPY config/crd/bases/nova.openstack.org_novacomputes.yaml ${OPERATOR_BUNDLE}/nova.openstack.org_novacomputes_crd.yaml
COPY config/crd/bases/nova.openstack.org_novamigrationtargets.yaml ${OPERATOR_BUNDLE}/nova.openstack.org_novamigrationtargets_crd.yaml
COPY config/crd/bases/nova.openstack.org_virtlogds.yaml ${OPERATOR_BUNDLE}/nova.openstack.org_virtlogds_crd.yaml

# strip top 2 lines (this resolves parsing in opm which handles this badly)
RUN sed -i -e 1,2d ${OPERATOR_BUNDLE}/*

WORKDIR /
COPY --from=builder /workspace/manager /usr/local/bin/manager
COPY --from=builder /workspace/csv-generator /usr/local/bin/csv-generator

# user setup
COPY tools/user_setup /usr/local/bin/user_setup
RUN  /usr/local/bin/user_setup
USER ${USER_UID}

ENTRYPOINT ["/usr/local/bin/manager"]
