#!/bin/bash
set -ex

TMPDIR=${TMPDIR:-"/tmp/k8s-webhook-server/serving-certs"}
SKIP_CERT=${SKIP_CERT:-false}
CRC_IP=${CRC_IP:-$(/sbin/ip -o -4 addr list crc | awk '{print $4}' | cut -d/ -f1)}
FIREWALL_ZONE=${FIREWALL_ZONE:-"libvirt"}
SKIP_FIREWALL=${SKIP_FIREWALL:-false}

if [ "$SKIP_FIREWALL" = false ] ; then
    #Open 9443
    sudo firewall-cmd --zone=${FIREWALL_ZONE} --add-port=9443/tcp
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
    url: https://${CRC_IP}:9443/validate-nova-openstack-org-v1beta1-nova
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
    url: https://${CRC_IP}:9443/mutate-nova-openstack-org-v1beta1-nova
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
    url: https://${CRC_IP}:9443/validate-nova-openstack-org-v1beta1-novaapi
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
    url: https://${CRC_IP}:9443/mutate-nova-openstack-org-v1beta1-novaapi
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
    url: https://${CRC_IP}:9443/validate-nova-openstack-org-v1beta1-novacell
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
    url: https://${CRC_IP}:9443/mutate-nova-openstack-org-v1beta1-novacell
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
    url: https://${CRC_IP}:9443/validate-nova-openstack-org-v1beta1-novaconductor
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
    url: https://${CRC_IP}:9443/mutate-nova-openstack-org-v1beta1-novaconductor
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
    url: https://${CRC_IP}:9443/validate-nova-openstack-org-v1beta1-novametadata
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
    url: https://${CRC_IP}:9443/mutate-nova-openstack-org-v1beta1-novametadata
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
    url: https://${CRC_IP}:9443/validate-nova-openstack-org-v1beta1-novanovncproxy
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
    url: https://${CRC_IP}:9443/mutate-nova-openstack-org-v1beta1-novanovncproxy
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
    url: https://${CRC_IP}:9443/validate-nova-openstack-org-v1beta1-novascheduler
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
    url: https://${CRC_IP}:9443/mutate-nova-openstack-org-v1beta1-novascheduler
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
    url: https://${CRC_IP}:9443/validate-nova-openstack-org-v1beta1-novacompute
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
    url: https://${CRC_IP}:9443/mutate-nova-openstack-org-v1beta1-novacompute
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
