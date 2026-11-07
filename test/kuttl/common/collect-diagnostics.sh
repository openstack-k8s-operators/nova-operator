#!/usr/bin/env bash
# KUTTL failure collector for nova/placement suites.
# Triggered only when a TestAssert with this collector fails.
#
# Writes under artifactsDir/<test-name>/ where test-name is basename of cwd
# (kuttl runs collectors with cwd = test case dir; no TEST_NAME env is set):
#   nova:      ../output/<test>/
#   placement: ../../output/<test>/
#
# Env from kuttl: NAMESPACE, KUBECONFIG, PATH
# Optional: OPERATOR_NAMESPACE (default openstack-operators)

set -u

namespace="${NAMESPACE:-}"
if [[ -z "${namespace}" ]]; then
    echo "NAMESPACE not set; skipping collection" >&2
    exit 0
fi

operator_ns="${OPERATOR_NAMESPACE:-openstack-operators}"
test_name="$(basename "${PWD}")"

if [[ -d ../output ]]; then
    outdir="../output/${test_name}"
elif [[ -d ../../output ]]; then
    outdir="../../output/${test_name}"
else
    echo "Could not find suite artifactsDir (../output or ../../output)" >&2
    exit 0
fi

mkdir -p "${outdir}"

run_to() {
    local file="$1"
    shift
    echo "===== $* -> ${outdir}/${file} ====="
    "$@" > "${outdir}/${file}" 2>&1 || true
}

safe_name() {
    echo "$1" | tr '/:' '__'
}

echo "$(date '+%Y-%m-%dT%H:%M:%S%z') Collecting diagnostics from namespace ${namespace} into ${outdir}"

run_to events.txt oc get events -n "${namespace}" --sort-by=.lastTimestamp
run_to pods.txt oc get pods -n "${namespace}" -o wide
run_to operator.log oc logs -n "${operator_ns}" -l openstack.org/operator-name=nova --all-containers --tail=-1

while IFS= read -r pod; do
    [[ -z "${pod}" ]] && continue
    while IFS= read -r container; do
        [[ -z "${container}" ]] && continue
        base="$(safe_name "${pod}")_${container}"
        run_to "${base}.log" oc logs -n "${namespace}" "${pod}" -c "${container}" --tail=-1
        run_to "${base}.previous.log" oc logs -n "${namespace}" "${pod}" -c "${container}" --previous --tail=-1
    done < <(
        oc get "${pod}" -n "${namespace}" \
            -o jsonpath='{range .spec.initContainers[*]}{.name}{"\n"}{end}{range .spec.containers[*]}{.name}{"\n"}{end}{range .spec.ephemeralContainers[*]}{.name}{"\n"}{end}' \
            2>/dev/null || true
    )
done < <(oc get pods -n "${namespace}" -o name 2>/dev/null || true)

echo "Done. Files in ${outdir}:"
ls -la "${outdir}" || true
