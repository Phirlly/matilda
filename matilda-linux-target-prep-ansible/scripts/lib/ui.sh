#!/usr/bin/env bash

# Shared UI and file-safety helpers for matilda-prep.
# This file is intended to be sourced, not executed directly.

expand_path() {
  local input_path="$1"

  if [[ "${input_path}" == "~/"* ]]; then
    printf '%s/%s' "${HOME}" "${input_path#~/}"
  else
    printf '%s' "${input_path}"
  fi
}

prompt_default() {
  local prompt_text="$1"
  local default_value="$2"
  local answer=""

  read -r -p "${prompt_text} [${default_value}]: " answer >&2
  printf '%s' "${answer:-$default_value}"
}

prompt_required() {
  local prompt_text="$1"
  local answer=""

  while true; do
    read -r -p "${prompt_text}: " answer >&2
    if [[ -n "${answer}" ]]; then
      printf '%s' "${answer}"
      return 0
    fi
    echo "ERROR: ${prompt_text} is required." >&2
  done
}

prompt_local_file() {
  local label="$1"
  local answer=""
  local expanded=""
  local confirm=""

  while true; do
    answer="$(prompt_required "${label}")"
    expanded="$(expand_path "${answer}")"

    if [[ -f "${expanded}" ]]; then
      printf '%s' "${expanded}"
      return 0
    fi

    echo "WARNING: ${label} was not found: ${expanded}" >&2
    read -r -p "Use this path anyway? [y/N]: " confirm >&2
    case "${confirm}" in
      y|Y|yes|YES)
        printf '%s' "${expanded}"
        return 0
        ;;
      *)
        echo "Please enter a valid path." >&2
        ;;
    esac
  done
}

prompt_hostname() {
  local prompt_text="$1"
  local value=""

  while true; do
    value="$(prompt_required "${prompt_text}")"
    if [[ "${value}" =~ ^[A-Za-z0-9._-]+$ ]]; then
      printf '%s' "${value}"
      return 0
    fi
    echo "ERROR: Use only letters, numbers, dots, underscores, and hyphens." >&2
  done
}

choose_file_action() {
  local dest_path="$1"
  local answer=""

  if [[ ! -f "${dest_path}" ]]; then
    printf 'write'
    return 0
  fi

  echo >&2
  echo "${dest_path} already exists. Choose an action:" >&2
  echo "  1) Keep existing file" >&2
  echo "  2) Back up existing file and create a new one" >&2
  echo "  3) Overwrite existing file without backup" >&2
  read -r -p "Select [1]: " answer >&2

  case "${answer:-1}" in
    1)
      printf 'skip'
      ;;
    2)
      printf 'backup_write'
      ;;
    3)
      printf 'overwrite'
      ;;
    *)
      echo "Invalid choice. Keeping existing ${dest_path}." >&2
      printf 'skip'
      ;;
  esac
}

prepare_destination() {
  local dest_path="$1"
  local action=""
  local backup_path=""

  action="$(choose_file_action "${dest_path}")"

  case "${action}" in
    write)
      printf 'write'
      ;;
    skip)
      printf 'skip'
      ;;
    backup_write)
      backup_path="${dest_path}.backup-$(date +%Y%m%d-%H%M%S)"
      cp "${dest_path}" "${backup_path}"
      echo "Backed up ${dest_path} to ${backup_path}" >&2
      printf 'write'
      ;;
    overwrite)
      echo "Overwriting ${dest_path} without backup." >&2
      printf 'write'
      ;;
    *)
      echo "Invalid file action for ${dest_path}. Keeping existing file." >&2
      printf 'skip'
      ;;
  esac
}

write_env_line() {
  local key="$1"
  local value="$2"
  printf '%s=%q\n' "${key}" "${value}"
}
