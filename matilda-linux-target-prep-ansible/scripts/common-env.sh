#!/usr/bin/env bash
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE_LOADED="false"

# Optional convenience file.
# Users do NOT need this file. If present, it pre-populates runtime values.
if [[ -f "${PROJECT_ROOT}/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "${PROJECT_ROOT}/.env"
  set +a
  ENV_FILE_LOADED="true"
fi

expand_path() {
  local input_path="$1"

  if [[ "${input_path}" == "~/"* ]]; then
    printf '%s/%s' "${HOME}" "${input_path#~/}"
  else
    printf '%s' "${input_path}"
  fi
}

prompt_default() {
  local var_name="$1"
  local prompt_text="$2"
  local default_value="$3"
  local current_value="${!var_name-}"
  local answer=""

  # If already provided by .env or environment, do not prompt.
  if [[ -n "${current_value}" ]]; then
    return 0
  fi

  read -r -p "${prompt_text} [${default_value}]: " answer
  printf -v "${var_name}" '%s' "${answer:-$default_value}"
}

prompt_required() {
  local var_name="$1"
  local prompt_text="$2"
  local answer=""

  # If already provided by .env or environment, do not prompt.
  if [[ -n "${!var_name-}" ]]; then
    return 0
  fi

  while true; do
    read -r -p "${prompt_text}: " answer
    printf -v "${var_name}" '%s' "${answer}"

    if [[ -n "${!var_name}" ]]; then
      break
    fi

    echo "ERROR: ${prompt_text} is required."
  done
}

require_local_file() {
  local label="$1"
  local file_path="$2"

  if [[ ! -f "${file_path}" ]]; then
    echo "ERROR: ${label} not found: ${file_path}" >&2
    exit 1
  fi
}

collect_runtime_inputs() {
  echo
  echo "Matilda Linux Target Prep - Runtime Inputs"
  echo "------------------------------------------------------------"

  if [[ "${ENV_FILE_LOADED}" == "true" ]]; then
    echo "Loaded optional .env file from: ${PROJECT_ROOT}/.env"
    echo "Values present in .env will not be prompted again."
  else
    echo "No .env file found. You will be prompted for required values."
    echo "Tip: To avoid prompts in future runs, copy .env.example to .env"
    echo "and fill in your environment-specific values."
  fi

  echo
  echo "Required runtime values:"
  echo "  1. Target VM admin SSH access"
  echo "  2. MatildaProbeVM admin SSH access"
  echo "  3. Matilda discovery public key on this machine"
  echo "  4. Matilda discovery private key path on MatildaProbeVM"
  echo
  echo "These keys may be the same in simple labs, but they are separate inputs."
  echo

  prompt_default TARGET_ADMIN_USER "Target admin SSH user" "opc"

  prompt_required TARGET_ADMIN_PRIVATE_KEY_FILE "Target admin private key path"
  TARGET_ADMIN_PRIVATE_KEY_FILE="$(expand_path "${TARGET_ADMIN_PRIVATE_KEY_FILE}")"
  require_local_file "Target admin private key" "${TARGET_ADMIN_PRIVATE_KEY_FILE}"

  prompt_required MATILDA_PROBE_ANSIBLE_HOST "MatildaProbeVM SSH host/IP"

  prompt_default MATILDA_PROBE_USER "MatildaProbeVM SSH user" "opc"

  prompt_required MATILDA_PROBE_ADMIN_PRIVATE_KEY_FILE "MatildaProbeVM admin private key path"
  MATILDA_PROBE_ADMIN_PRIVATE_KEY_FILE="$(expand_path "${MATILDA_PROBE_ADMIN_PRIVATE_KEY_FILE}")"
  require_local_file "MatildaProbeVM admin private key" "${MATILDA_PROBE_ADMIN_PRIVATE_KEY_FILE}"

  prompt_required MATILDA_PUBLIC_KEY_FILE "Matilda discovery public key path on this machine"
  MATILDA_PUBLIC_KEY_FILE="$(expand_path "${MATILDA_PUBLIC_KEY_FILE}")"
  require_local_file "Matilda discovery public key" "${MATILDA_PUBLIC_KEY_FILE}"

  local default_probe_discovery_key="/home/${MATILDA_PROBE_USER}/.ssh/MatildaProbeKey.pem"
  prompt_default MATILDA_PROBE_PRIVATE_KEY_ON_PROBE "Matilda discovery private key path on Probe" "${default_probe_discovery_key}"

  ANSIBLE_EXTRA_VARS=(
    --extra-vars "target_admin_user=${TARGET_ADMIN_USER}"
    --extra-vars "target_admin_private_key_file=${TARGET_ADMIN_PRIVATE_KEY_FILE}"
    --extra-vars "matilda_probe_ansible_host=${MATILDA_PROBE_ANSIBLE_HOST}"
    --extra-vars "matilda_probe_user=${MATILDA_PROBE_USER}"
    --extra-vars "matilda_probe_admin_private_key_file=${MATILDA_PROBE_ADMIN_PRIVATE_KEY_FILE}"
    --extra-vars "matilda_public_key_file=${MATILDA_PUBLIC_KEY_FILE}"
    --extra-vars "matilda_probe_private_key_on_probe=${MATILDA_PROBE_PRIVATE_KEY_ON_PROBE}"
  )

  echo
  echo "Runtime input summary"
  echo "------------------------------------------------------------"
  echo "Target admin user:                  ${TARGET_ADMIN_USER}"
  echo "Target admin private key:           ${TARGET_ADMIN_PRIVATE_KEY_FILE}"
  echo "MatildaProbeVM host/IP:             ${MATILDA_PROBE_ANSIBLE_HOST}"
  echo "MatildaProbeVM SSH user:            ${MATILDA_PROBE_USER}"
  echo "MatildaProbeVM admin private key:   ${MATILDA_PROBE_ADMIN_PRIVATE_KEY_FILE}"
  echo "Matilda public key local path:      ${MATILDA_PUBLIC_KEY_FILE}"
  echo "Discovery private key on Probe:     ${MATILDA_PROBE_PRIVATE_KEY_ON_PROBE}"
  echo
}
