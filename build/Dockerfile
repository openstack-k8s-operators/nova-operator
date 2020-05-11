FROM registry.access.redhat.com/ubi7/ubi-minimal:latest

ENV OPERATOR=/usr/local/bin/nova-operator \
    USER_UID=1001 \
    USER_NAME=nova-operator \
    OPERATOR_TEMPLATES=/usr/share/nova-operator/templates/ \
    OPERATOR_BUNDLE=/usr/share/nova-operator/bundle/

# install operator binary
COPY build/_output/bin/nova-operator ${OPERATOR}
COPY build/_output/csv-generator /usr/local/bin/csv-generator

COPY build/bin /usr/local/bin
RUN  /usr/local/bin/user_setup

# install our templates
RUN  mkdir -p ${OPERATOR_TEMPLATES}
COPY templates ${OPERATOR_TEMPLATES}

# install CRDs and required roles, services, etc
RUN  mkdir -p ${OPERATOR_BUNDLE}
COPY deploy/crds/*crd.yaml ${OPERATOR_BUNDLE}

ENTRYPOINT ["/usr/local/bin/entrypoint"]

USER ${USER_UID}
