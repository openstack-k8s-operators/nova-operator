#!/bin/bash
set -ex

oc delete validatingwebhookconfiguration/vnova.kb.io --ignore-not-found
oc delete mutatingwebhookconfiguration/mnova.kb.io --ignore-not-found
oc delete validatingwebhookconfiguration/vnovacell.kb.io --ignore-not-found
oc delete mutatingwebhookconfiguration/mnovacell.kb.io --ignore-not-found
oc delete validatingwebhookconfiguration/vnovaexternalcompute.kb.io --ignore-not-found
oc delete mutatingwebhookconfiguration/mnovaexternalcompute.kb.io --ignore-not-found
