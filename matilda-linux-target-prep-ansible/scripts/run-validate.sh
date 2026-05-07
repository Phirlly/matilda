#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/common-env.sh"

collect_runtime_inputs
cd "${PROJECT_ROOT}"

echo "============================================================"
echo "Matilda Linux Target Prep - VALIDATE"
echo "Mode: validates target readiness for Matilda Discovery"
echo "Inventory: ${PROJECT_ROOT}/inventory.yml"
echo "============================================================"
echo
echo "This step validates:"
echo "  - local sudo as matilda-svc"
echo "  - Probe-to-target SSH as matilda-svc"
echo "  - Probe-to-target sudo discovery command"
echo

ansible-playbook playbooks/03-validate-linux-targets.yml "${ANSIBLE_EXTRA_VARS[@]}"

echo
echo "Validated discovery IPs:"
cat reports/validated-discovery-ips.txt

echo
echo "Validation summary:"
cat reports/validation-summary.txt

echo
echo "Use reports/validated-discovery-ips.txt in Matilda Network Discovery."
