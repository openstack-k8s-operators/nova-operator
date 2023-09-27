#!/bin/bash
set -ex

oc delete validatingwebhookconfiguration/vnova.kb.io --ignore-not-found
oc delete mutatingwebhookconfiguration/mnova.kb.io --ignore-not-found
oc delete validatingwebhookconfiguration/vnovaapi.kb.io --ignore-not-found
oc delete mutatingwebhookconfiguration/mnovaapi.kb.io --ignore-not-found
oc delete validatingwebhookconfiguration/vnovacell.kb.io --ignore-not-found
oc delete mutatingwebhookconfiguration/mnovacell.kb.io --ignore-not-found
oc delete validatingwebhookconfiguration/vnovaconductor.kb.io --ignore-not-found
oc delete mutatingwebhookconfiguration/mnovaconductor.kb.io --ignore-not-found
oc delete validatingwebhookconfiguration/vnovametadata.kb.io --ignore-not-found
oc delete mutatingwebhookconfiguration/mnovametadata.kb.io --ignore-not-found
oc delete validatingwebhookconfiguration/vnovanovncproxy.kb.io --ignore-not-found
oc delete mutatingwebhookconfiguration/mnovanovncproxy.kb.io --ignore-not-found
oc delete validatingwebhookconfiguration/vnovascheduler.kb.io --ignore-not-found
oc delete mutatingwebhookconfiguration/mnovascheduler.kb.io --ignore-not-found
oc delete validatingwebhookconfiguration/vnovacompute.kb.io --ignore-not-found
oc delete mutatingwebhookconfiguration/mnovacompute.kb.io --ignore-not-found
