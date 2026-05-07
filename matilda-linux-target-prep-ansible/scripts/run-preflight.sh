#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/common-env.sh"

collect_runtime_inputs
cd "${PROJECT_ROOT}"

echo "============================================================"
echo "Matilda Linux Target Prep - PREFLIGHT"
echo "Mode: read-only validation, no target changes"
echo "Inventory: ${PROJECT_ROOT}/inventory.yml"
echo "============================================================"
echo
echo "Ansible will run preflight checks against hosts in inventory.yml."
echo "No target VM changes will be made."
echo

ansible-playbook playbooks/01-preflight.yml "${ANSIBLE_EXTRA_VARS[@]}"

echo
echo "Preflight complete. If all hosts passed, run:"
echo "  ./scripts/run-setup.sh"
