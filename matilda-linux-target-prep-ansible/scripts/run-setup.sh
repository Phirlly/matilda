#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/common-env.sh"

collect_runtime_inputs
cd "${PROJECT_ROOT}"

echo "============================================================"
echo "Matilda Linux Target Prep - SETUP"
echo "Mode: modifies target VMs"
echo "Inventory: ${PROJECT_ROOT}/inventory.yml"
echo "============================================================"
echo
echo "This step will configure target VMs by:"
echo "  - creating/updating matilda-svc"
echo "  - installing the Matilda public key"
echo "  - writing /etc/sudoers.d/matilda-discovery"
echo "  - validating sudoers syntax"
echo

read -r -p "Continue with setup? [y/N]: " CONFIRM
case "${CONFIRM}" in
  y|Y|yes|YES)
    ;;
  *)
    echo "Setup cancelled."
    exit 0
    ;;
esac

ansible-playbook playbooks/02-setup-linux-targets.yml "${ANSIBLE_EXTRA_VARS[@]}"

echo
echo "Setup complete. Next run:"
echo "  ./scripts/run-validate.sh"
