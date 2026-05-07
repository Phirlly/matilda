#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/common-env.sh"

collect_runtime_inputs
cd "${PROJECT_ROOT}"

ansible-playbook playbooks/03-validate-linux-targets.yml "${ANSIBLE_EXTRA_VARS[@]}"

echo
echo "Validated discovery IPs:"
cat reports/validated-discovery-ips.txt
