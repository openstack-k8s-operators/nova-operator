#!/bin/bash
set -ex

oc delete validatingwebhookconfiguration/vplacementapi.kb.io --ignore-not-found
oc delete mutatingwebhookconfiguration/mplacementapi.kb.io --ignore-not-found
