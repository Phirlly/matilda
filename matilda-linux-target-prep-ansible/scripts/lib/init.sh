#!/usr/bin/env bash

# Init workflow for matilda-prep.
# This file is intended to be sourced, not executed directly.

create_env_guided() {
  local dest_path="${SCRIPT_DIR}/.env"
  local action=""
  local target_admin_user=""
  local target_admin_private_key_file=""
  local probe_host=""
  local probe_user=""
  local probe_admin_private_key_file=""
  local matilda_public_key_file=""
  local default_probe_key=""
  local probe_private_key_on_probe=""

  action="$(prepare_destination "${dest_path}")"
  if [[ "${action}" == "skip" ]]; then
    echo "Kept existing ${dest_path}" >&2
    return 0
  fi

  echo >&2
  echo "Configure runtime values for .env" >&2
  echo "------------------------------------------------------------" >&2
  echo "Only paths and hostnames are stored. Private key contents are not copied into .env." >&2
  echo >&2

  target_admin_user="$(prompt_default "Target admin SSH user" "opc")"
  target_admin_private_key_file="$(prompt_local_file "Target admin private key path")"
  probe_host="$(prompt_required "MatildaProbeVM SSH host/IP")"
  probe_user="$(prompt_default "MatildaProbeVM SSH user" "opc")"
  probe_admin_private_key_file="$(prompt_local_file "MatildaProbeVM admin private key path")"
  matilda_public_key_file="$(prompt_local_file "Matilda discovery public key path on this machine")"
  default_probe_key="/home/${probe_user}/.ssh/MatildaProbeKey.pem"
  probe_private_key_on_probe="$(prompt_default "Matilda discovery private key path on MatildaProbeVM" "${default_probe_key}")"

  {
    echo "# Local runtime values for Matilda Linux Target Prep."
    echo "# Do not commit this file."
    echo
    write_env_line "TARGET_ADMIN_USER" "${target_admin_user}"
    write_env_line "TARGET_ADMIN_PRIVATE_KEY_FILE" "${target_admin_private_key_file}"
    echo
    write_env_line "MATILDA_PROBE_ANSIBLE_HOST" "${probe_host}"
    write_env_line "MATILDA_PROBE_USER" "${probe_user}"
    write_env_line "MATILDA_PROBE_ADMIN_PRIVATE_KEY_FILE" "${probe_admin_private_key_file}"
    echo
    write_env_line "MATILDA_PUBLIC_KEY_FILE" "${matilda_public_key_file}"
    write_env_line "MATILDA_PROBE_PRIVATE_KEY_ON_PROBE" "${probe_private_key_on_probe}"
  } > "${dest_path}"

  echo "Created ${dest_path}" >&2
}

create_env_from_template() {
  local source_path="${SCRIPT_DIR}/.env.example"
  local dest_path="${SCRIPT_DIR}/.env"
  local action=""

  if [[ ! -f "${source_path}" ]]; then
    echo "ERROR: .env template not found: ${source_path}" >&2
    return 1
  fi

  action="$(prepare_destination "${dest_path}")"
  if [[ "${action}" == "skip" ]]; then
    echo "Kept existing ${dest_path}" >&2
    return 0
  fi

  cp "${source_path}" "${dest_path}"
  echo "Created ${dest_path} from ${source_path}" >&2
}

append_public_target() {
  local output_file="$1"
  local hostname="$2"
  local public_ip="$3"
  local private_ip="$4"
  local discovery_ip="$5"

  cat >> "${output_file}" <<EOF
        ${hostname}:
          ansible_host: ${public_ip}
          public_ip: ${public_ip}
          private_ip: ${private_ip}
          discovery_ip: ${discovery_ip}
EOF
}

append_private_target() {
  local output_file="$1"
  local hostname="$2"
  local private_ip="$3"
  local discovery_ip="$4"

  cat >> "${output_file}" <<EOF
        ${hostname}:
          ansible_host: ${private_ip}
          private_ip: ${private_ip}
          discovery_ip: ${discovery_ip}
EOF
}

create_inventory_guided() {
  local dest_path="${SCRIPT_DIR}/inventory.yml"
  local action=""
  local target_count=""
  local index=1
  local hostname=""
  local access_choice=""
  local public_ip=""
  local private_ip=""
  local discovery_ip=""
  local public_tmp=""
  local private_tmp=""
  local public_count=0
  local private_count=0

  action="$(prepare_destination "${dest_path}")"
  if [[ "${action}" == "skip" ]]; then
    echo "Kept existing ${dest_path}" >&2
    return 0
  fi

  echo >&2
  echo "Configure inventory.yml" >&2
  echo "------------------------------------------------------------" >&2
  echo "This wizard currently supports Linux public/direct and private/via-Probe targets." >&2
  echo >&2

  while true; do
    read -r -p "How many targets do you want to add? " target_count >&2
    if [[ "${target_count}" =~ ^[0-9]+$ ]] && [[ "${target_count}" -gt 0 ]]; then
      break
    fi
    echo "ERROR: Enter a positive number." >&2
  done

  public_tmp="$(mktemp)"
  private_tmp="$(mktemp)"

  while [[ "${index}" -le "${target_count}" ]]; do
    echo >&2
    echo "Target ${index} of ${target_count}" >&2
    echo "------------------------------------------------------------" >&2
    hostname="$(prompt_hostname "Inventory hostname")"

    echo "Access type:" >&2
    echo "  1) public/direct from this machine" >&2
    echo "  2) private/via MatildaProbeVM" >&2
    read -r -p "Select [1]: " access_choice >&2
    access_choice="${access_choice:-1}"

    case "${access_choice}" in
      1)
        public_ip="$(prompt_required "Target public IP for Ansible access")"
        private_ip="$(prompt_required "Target private IP")"
        discovery_ip="$(prompt_default "Discovery IP used by MatildaProbeVM" "${private_ip}")"
        append_public_target "${public_tmp}" "${hostname}" "${public_ip}" "${private_ip}" "${discovery_ip}"
        public_count=$((public_count + 1))
        ;;
      2)
        private_ip="$(prompt_required "Target private IP")"
        discovery_ip="$(prompt_default "Discovery IP used by MatildaProbeVM" "${private_ip}")"
        append_private_target "${private_tmp}" "${hostname}" "${private_ip}" "${discovery_ip}"
        private_count=$((private_count + 1))
        ;;
      *)
        echo "Invalid choice. Please re-enter target ${index}." >&2
        continue
        ;;
    esac

    index=$((index + 1))
  done

  {
    echo "all:"
    echo "  children:"
    echo "    public_targets:"
    if [[ "${public_count}" -gt 0 ]]; then
      echo "      hosts:"
      cat "${public_tmp}"
    else
      echo "      hosts: {}"
    fi
    echo
    echo "    private_targets:"
    if [[ "${private_count}" -gt 0 ]]; then
      echo "      hosts:"
      cat "${private_tmp}"
    else
      echo "      hosts: {}"
    fi
  } > "${dest_path}"

  rm -f "${public_tmp}" "${private_tmp}"

  echo "Created ${dest_path}" >&2
}

create_inventory_from_template() {
  local source_path="${SCRIPT_DIR}/inventory.example.yml"
  local dest_path="${SCRIPT_DIR}/inventory.yml"
  local action=""

  if [[ ! -f "${source_path}" ]]; then
    echo "ERROR: inventory template not found: ${source_path}" >&2
    return 1
  fi

  action="$(prepare_destination "${dest_path}")"
  if [[ "${action}" == "skip" ]]; then
    echo "Kept existing ${dest_path}" >&2
    return 0
  fi

  cp "${source_path}" "${dest_path}"
  echo "Created ${dest_path} from ${source_path}" >&2
}

run_init() {
  local mode=""

  echo "============================================================"
  echo "Matilda Linux Target Prep - INIT"
  echo "Mode: local file setup only, no target changes"
  echo "============================================================"
  echo
  echo "This command can create local starter files:"
  echo "  - .env"
  echo "  - inventory.yml"
  echo
  echo "It does not run Ansible and does not modify target VMs."
  echo

  echo "Choose init mode:"
  echo "  1) Guided wizard (recommended)"
  echo "  2) Copy example templates only"
  read -r -p "Select [1]: " mode >&2
  mode="${mode:-1}"

  case "${mode}" in
    1)
      create_env_guided
      create_inventory_guided
      ;;
    2)
      create_env_from_template
      create_inventory_from_template
      ;;
    *)
      echo "ERROR: Invalid init mode: ${mode}" >&2
      exit 1
      ;;
  esac

  echo
  echo "Init complete. Next steps:"
  echo "  1. Review .env and inventory.yml."
  echo "  2. Run: ./matilda-prep preflight"
}
