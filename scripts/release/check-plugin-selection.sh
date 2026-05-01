#!/usr/bin/env bash
set -euo pipefail

selection="${1:-everything}"

include_browser=false
include_macos=false
include_windows=false
include_linux=false
IFS=',' read -ra selection_parts <<< "${selection}"
for selection_part in "${selection_parts[@]}"; do
  selection_part="${selection_part//[[:space:]]/}"
  case "${selection_part}" in
    everything)
      include_browser=true
      include_macos=true
      include_windows=true
      include_linux=true
      ;;
    browser)
      include_browser=true
      ;;
    macos|darwin)
      include_macos=true
      ;;
    windows)
      include_windows=true
      ;;
    linux)
      include_linux=true
      ;;
    "")
      ;;
    *)
      echo "unknown plugin release selection: ${selection_part}" >&2
      exit 1
      ;;
  esac
done

printf 'browser=%s\n' "${include_browser}"
printf 'macos=%s\n' "${include_macos}"
printf 'windows=%s\n' "${include_windows}"
printf 'linux=%s\n' "${include_linux}"
