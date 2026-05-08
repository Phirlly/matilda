#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/common-env.sh"

check_setup_dependencies() {
  echo
  echo "Checking local setup dependencies"
  echo "------------------------------------------------------------"

  if ! command -v ansible-playbook >/dev/null 2>&1; then
    echo "ERROR: ansible-playbook was not found in PATH." >&2
    echo >&2
    echo "Install Ansible on the machine where you run this project, then rerun setup." >&2
    exit 1
  fi

  if ! command -v ansible-doc >/dev/null 2>&1; then
    echo "ERROR: ansible-doc was not found in PATH." >&2
    echo >&2
    echo "This script uses ansible-doc to verify that the ansible.posix collection is available." >&2
    echo "Install a complete Ansible environment, then rerun setup." >&2
    exit 1
  fi

  if ! ansible-doc -t module ansible.posix.authorized_key >/dev/null 2>&1; then
    echo "ERROR: Required Ansible module not available: ansible.posix.authorized_key" >&2
    echo >&2
    echo "Why this is needed:" >&2
    echo "  The setup playbook uses ansible.posix.authorized_key to safely manage" >&2
    echo "  the Matilda SSH public key for the matilda-svc account." >&2
    echo >&2
    echo "How to fix:" >&2
    echo "  Install or enable the ansible.posix collection in the Ansible environment" >&2
    echo "  where you run this project, then rerun setup." >&2
    echo >&2
    echo "Typical install command:" >&2
    echo "  ansible-galaxy collection install ansible.posix" >&2
    exit 1
  fi

  echo "OK: ansible-playbook found."
  echo "OK: ansible.posix.authorized_key is available."
  echo
}

check_setup_dependencies
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
