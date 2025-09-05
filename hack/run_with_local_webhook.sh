#!/bin/bash
set -ex

# Define a cleanup function
cleanup() {
    echo "Caught signal, cleaning up local webhooks..."
    ./hack/clean_local_webhook.sh
    exit 0
}

# Set trap to catch SIGINT and SIGTERM
trap cleanup SIGINT SIGTERM

TMPDIR=${TMPDIR:-"/tmp/k8s-webhook-server/serving-certs"}
SKIP_CERT=${SKIP_CERT:-false}
CRC_IP=${CRC_IP:-$(/sbin/ip -o -4 addr list crc | awk '{print $4}' | cut -d/ -f1)}
FIREWALL_ZONE=${FIREWALL_ZONE:-"libvirt"}
SKIP_FIREWALL=${SKIP_FIREWALL:-false}
WEBHOOK_PORT=${WEBHOOK_PORT:-${WEBHOOK_PORT}}

if [ "$SKIP_FIREWALL" = false ] ; then
    #Open ${WEBHOOK_PORT}
    sudo firewall-cmd --zone=${FIREWALL_ZONE} --add-port=${WEBHOOK_PORT}/tcp
    sudo firewall-cmd --runtime-to-permanent
fi

# Generate the certs and the ca bundle
if [ "$SKIP_CERT" = false ] ; then
    mkdir -p ${TMPDIR}
    rm -rf ${TMPDIR}/* || true

    openssl req -newkey rsa:2048 -days 3650 -nodes -x509 \
    -subj "/CN=${HOSTNAME}" \
    -addext "subjectAltName = IP:${CRC_IP}" \
    -keyout ${TMPDIR}/tls.key \
    -out ${TMPDIR}/tls.crt

    cat ${TMPDIR}/tls.crt ${TMPDIR}/tls.key | base64 -w 0 > ${TMPDIR}/bundle.pem

fi

CA_BUNDLE=`cat ${TMPDIR}/bundle.pem`

# Patch the webhook(s)
cat >> ${TMPDIR}/patch_webhook_configurations.yaml <<EOF_CAT
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  name: vnova.kb.io
webhooks:
- admissionReviewVersions:
  - v1
  clientConfig:
    caBundle: ${CA_BUNDLE}
    url: https://${CRC_IP}:${WEBHOOK_PORT}/validate-nova-openstack-org-v1beta1-nova
  failurePolicy: Fail
  matchPolicy: Equivalent
  name: vnova.kb.io
  objectSelector: {}
  rules:
  - apiGroups:
    - nova.openstack.org
    apiVersions:
    - v1beta1
    operations:
    - CREATE
    - UPDATE
    resources:
    - nova
    scope: '*'
  sideEffects: None
  timeoutSeconds: 10
---
apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  name: mnova.kb.io
webhooks:
- admissionReviewVersions:
  - v1
  clientConfig:
    caBundle: ${CA_BUNDLE}
    url: https://${CRC_IP}:${WEBHOOK_PORT}/mutate-nova-openstack-org-v1beta1-nova
  failurePolicy: Fail
  matchPolicy: Equivalent
  name: mnova.kb.io
  objectSelector: {}
  rules:
  - apiGroups:
    - nova.openstack.org
    apiVersions:
    - v1beta1
    operations:
    - CREATE
    - UPDATE
    resources:
    - nova
    scope: '*'
  sideEffects: None
  timeoutSeconds: 10
---
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  name: vnovaapi.kb.io
webhooks:
- admissionReviewVersions:
  - v1
  clientConfig:
    caBundle: ${CA_BUNDLE}
    url: https://${CRC_IP}:${WEBHOOK_PORT}/validate-nova-openstack-org-v1beta1-novaapi
  failurePolicy: Fail
  matchPolicy: Equivalent
  name: vnovaapi.kb.io
  objectSelector: {}
  rules:
  - apiGroups:
    - nova.openstack.org
    apiVersions:
    - v1beta1
    operations:
    - CREATE
    - UPDATE
    resources:
    - novaapis
    scope: '*'
  sideEffects: None
  timeoutSeconds: 10
---
apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  name: mnovaapi.kb.io
webhooks:
- admissionReviewVersions:
  - v1
  clientConfig:
    caBundle: ${CA_BUNDLE}
    url: https://${CRC_IP}:${WEBHOOK_PORT}/mutate-nova-openstack-org-v1beta1-novaapi
  failurePolicy: Fail
  matchPolicy: Equivalent
  name: mnovaapi.kb.io
  objectSelector: {}
  rules:
  - apiGroups:
    - nova.openstack.org
    apiVersions:
    - v1beta1
    operations:
    - CREATE
    - UPDATE
    resources:
    - novaapis
    scope: '*'
  sideEffects: None
  timeoutSeconds: 10
---
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  name: vnovacell.kb.io
webhooks:
- admissionReviewVersions:
  - v1
  clientConfig:
    caBundle: ${CA_BUNDLE}
    url: https://${CRC_IP}:${WEBHOOK_PORT}/validate-nova-openstack-org-v1beta1-novacell
  failurePolicy: Fail
  matchPolicy: Equivalent
  name: vnovacell.kb.io
  objectSelector: {}
  rules:
  - apiGroups:
    - nova.openstack.org
    apiVersions:
    - v1beta1
    operations:
    - CREATE
    - UPDATE
    resources:
    - novacells
    scope: '*'
  sideEffects: None
  timeoutSeconds: 10
---
apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  name: mnovacell.kb.io
webhooks:
- admissionReviewVersions:
  - v1
  clientConfig:
    caBundle: ${CA_BUNDLE}
    url: https://${CRC_IP}:${WEBHOOK_PORT}/mutate-nova-openstack-org-v1beta1-novacell
  failurePolicy: Fail
  matchPolicy: Equivalent
  name: mnovacell.kb.io
  objectSelector: {}
  rules:
  - apiGroups:
    - nova.openstack.org
    apiVersions:
    - v1beta1
    operations:
    - CREATE
    - UPDATE
    resources:
    - novacells
    scope: '*'
  sideEffects: None
  timeoutSeconds: 10
---
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  name: vnovaconductor.kb.io
webhooks:
- admissionReviewVersions:
  - v1
  clientConfig:
    caBundle: ${CA_BUNDLE}
    url: https://${CRC_IP}:${WEBHOOK_PORT}/validate-nova-openstack-org-v1beta1-novaconductor
  failurePolicy: Fail
  matchPolicy: Equivalent
  name: vnovaconductor.kb.io
  objectSelector: {}
  rules:
  - apiGroups:
    - nova.openstack.org
    apiVersions:
    - v1beta1
    operations:
    - CREATE
    - UPDATE
    resources:
    - novaconductors
    scope: '*'
  sideEffects: None
  timeoutSeconds: 10
---
apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  name: mnovaconductor.kb.io
webhooks:
- admissionReviewVersions:
  - v1
  clientConfig:
    caBundle: ${CA_BUNDLE}
    url: https://${CRC_IP}:${WEBHOOK_PORT}/mutate-nova-openstack-org-v1beta1-novaconductor
  failurePolicy: Fail
  matchPolicy: Equivalent
  name: mnovaconductor.kb.io
  objectSelector: {}
  rules:
  - apiGroups:
    - nova.openstack.org
    apiVersions:
    - v1beta1
    operations:
    - CREATE
    - UPDATE
    resources:
    - novaconductors
    scope: '*'
  sideEffects: None
  timeoutSeconds: 10
---
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  name: vnovametadata.kb.io
webhooks:
- admissionReviewVersions:
  - v1
  clientConfig:
    caBundle: ${CA_BUNDLE}
    url: https://${CRC_IP}:${WEBHOOK_PORT}/validate-nova-openstack-org-v1beta1-novametadata
  failurePolicy: Fail
  matchPolicy: Equivalent
  name: vnovametadata.kb.io
  objectSelector: {}
  rules:
  - apiGroups:
    - nova.openstack.org
    apiVersions:
    - v1beta1
    operations:
    - CREATE
    - UPDATE
    resources:
    - novametadata
    scope: '*'
  sideEffects: None
  timeoutSeconds: 10
---
apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  name: mnovametadata.kb.io
webhooks:
- admissionReviewVersions:
  - v1
  clientConfig:
    caBundle: ${CA_BUNDLE}
    url: https://${CRC_IP}:${WEBHOOK_PORT}/mutate-nova-openstack-org-v1beta1-novametadata
  failurePolicy: Fail
  matchPolicy: Equivalent
  name: mnovametadata.kb.io
  objectSelector: {}
  rules:
  - apiGroups:
    - nova.openstack.org
    apiVersions:
    - v1beta1
    operations:
    - CREATE
    - UPDATE
    resources:
    - novametadata
    scope: '*'
  sideEffects: None
  timeoutSeconds: 10
---
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  name: vnovanovncproxy.kb.io
webhooks:
- admissionReviewVersions:
  - v1
  clientConfig:
    caBundle: ${CA_BUNDLE}
    url: https://${CRC_IP}:${WEBHOOK_PORT}/validate-nova-openstack-org-v1beta1-novanovncproxy
  failurePolicy: Fail
  matchPolicy: Equivalent
  name: vnovanovncproxy.kb.io
  objectSelector: {}
  rules:
  - apiGroups:
    - nova.openstack.org
    apiVersions:
    - v1beta1
    operations:
    - CREATE
    - UPDATE
    resources:
    - novanovncproxies
    scope: '*'
  sideEffects: None
  timeoutSeconds: 10
---
apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  name: mnovanovncproxy.kb.io
webhooks:
- admissionReviewVersions:
  - v1
  clientConfig:
    caBundle: ${CA_BUNDLE}
    url: https://${CRC_IP}:${WEBHOOK_PORT}/mutate-nova-openstack-org-v1beta1-novanovncproxy
  failurePolicy: Fail
  matchPolicy: Equivalent
  name: mnovanovncproxy.kb.io
  objectSelector: {}
  rules:
  - apiGroups:
    - nova.openstack.org
    apiVersions:
    - v1beta1
    operations:
    - CREATE
    - UPDATE
    resources:
    - novanovncproxies
    scope: '*'
  sideEffects: None
  timeoutSeconds: 10
---
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  name: vnovascheduler.kb.io
webhooks:
- admissionReviewVersions:
  - v1
  clientConfig:
    caBundle: ${CA_BUNDLE}
    url: https://${CRC_IP}:${WEBHOOK_PORT}/validate-nova-openstack-org-v1beta1-novascheduler
  failurePolicy: Fail
  matchPolicy: Equivalent
  name: vnovascheduler.kb.io
  objectSelector: {}
  rules:
  - apiGroups:
    - nova.openstack.org
    apiVersions:
    - v1beta1
    operations:
    - CREATE
    - UPDATE
    resources:
    - novaschedulers
    scope: '*'
  sideEffects: None
  timeoutSeconds: 10
---
apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  name: mnovascheduler.kb.io
webhooks:
- admissionReviewVersions:
  - v1
  clientConfig:
    caBundle: ${CA_BUNDLE}
    url: https://${CRC_IP}:${WEBHOOK_PORT}/mutate-nova-openstack-org-v1beta1-novascheduler
  failurePolicy: Fail
  matchPolicy: Equivalent
  name: mnovascheduler.kb.io
  objectSelector: {}
  rules:
  - apiGroups:
    - nova.openstack.org
    apiVersions:
    - v1beta1
    operations:
    - CREATE
    - UPDATE
    resources:
    - novaschedulers
    scope: '*'
  sideEffects: None
  timeoutSeconds: 10
---
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  name: vnovacompute.kb.io
webhooks:
- admissionReviewVersions:
  - v1
  clientConfig:
    caBundle: ${CA_BUNDLE}
    url: https://${CRC_IP}:${WEBHOOK_PORT}/validate-nova-openstack-org-v1beta1-novacompute
  failurePolicy: Fail
  matchPolicy: Equivalent
  name: vnovacompute.kb.io
  objectSelector: {}
  rules:
  - apiGroups:
    - nova.openstack.org
    apiVersions:
    - v1beta1
    operations:
    - CREATE
    - UPDATE
    resources:
    - novacomputes
    scope: '*'
  sideEffects: None
  timeoutSeconds: 10
---
apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  name: mnovacomputes.kb.io
webhooks:
- admissionReviewVersions:
  - v1
  clientConfig:
    caBundle: ${CA_BUNDLE}
    url: https://${CRC_IP}:${WEBHOOK_PORT}/mutate-nova-openstack-org-v1beta1-novacompute
  failurePolicy: Fail
  matchPolicy: Equivalent
  name: mnovacompute.kb.io
  objectSelector: {}
  rules:
  - apiGroups:
    - nova.openstack.org
    apiVersions:
    - v1beta1
    operations:
    - CREATE
    - UPDATE
    resources:
    - novacomputes
    scope: '*'
  sideEffects: None
  timeoutSeconds: 10
EOF_CAT

oc apply -n openstack -f ${TMPDIR}/patch_webhook_configurations.yaml

# Scale-down operator deployment replicas to zero and remove OLM webhooks
CSV_NAME="$(oc get csv -n openstack-operators -l operators.coreos.com/nova-operator.openstack-operators -o name)"

if [ -n "${CSV_NAME}" ]; then
    CUR_REPLICAS=$(oc get -n openstack-operators "${CSV_NAME}" -o=jsonpath='{.spec.install.spec.deployments[0].spec.replicas}')
    CUR_WEBHOOK_DEFS=$(oc get -n openstack-operators "${CSV_NAME}" -o=jsonpath='{.spec.webhookdefinitions}')

    # Back-up CSV if it currently uses OLM defaults for deployment replicas or webhook definitions
    if [[ "${CUR_REPLICAS}" -gt 0 || ( -n "${CUR_WEBHOOK_DEFS}" && "${CUR_WEBHOOK_DEFS}" != "[]" ) ]]; then
        CSV_FILE=$(mktemp -t "$(echo "${CSV_NAME}" | cut -d "/" -f 2).XXXXXX" --suffix .json)
        oc get -n openstack-operators "${CSV_NAME}" -o json | \
        jq -r 'del(.metadata.generation, .metadata.resourceVersion, .metadata.uid)'  > "${CSV_FILE}"

        printf \
        "\n\tNow patching operator CSV to remove its OLM deployment and associated webhooks.
        The original OLM version of the operator's CSV has been copied to %s.  To restore it, use:
        oc patch -n openstack-operators %s --type=merge --patch-file=%s\n\n" "${CSV_FILE}" "${CSV_NAME}" "${CSV_FILE}"
    fi

    oc patch "${CSV_NAME}" -n openstack-operators --type=json -p="[{'op': 'replace', 'path': '/spec/install/spec/deployments/0/spec/replicas', 'value': 0}]"
    oc patch "${CSV_NAME}" -n openstack-operators --type=json -p="[{'op': 'replace', 'path': '/spec/webhookdefinitions', 'value': []}]"
else
    # Handle operator deployed by Openstack Initialization resource
    CSV_NAME="$(oc get csv -n openstack-operators -l operators.coreos.com/openstack-operator.openstack-operators -o name)"

    printf \
    "\n\tNow patching openstack operator CSV to scale down deployment resource.
    To restore it, use:
    oc patch "${CSV_NAME}" -n openstack-operators --type=json -p=\"[{'op': 'replace', 'path': '/spec/install/spec/deployments/0/spec/replicas', 'value': 1}]\""

    oc patch "${CSV_NAME}" -n openstack-operators --type=json -p="[{'op': 'replace', 'path': '/spec/install/spec/deployments/0/spec/replicas', 'value': 0}]"
    oc scale --replicas=0 -n openstack-operators deploy/nova-operator-controller-manager
fi

go run ./main.go -metrics-bind-address ":${METRICS_PORT}" -health-probe-bind-address ":${HEALTH_PORT}" -pprof-bind-address ":${PPROF_PORT}" -webhook-bind-address "${WEBHOOK_PORT}"
