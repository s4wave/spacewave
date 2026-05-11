#!/usr/bin/env bash
set -euo pipefail

selection="${1:-everything}"

include_browser=false
include_macos=false
include_windows=false
include_linux=false
seen_everything=false
seen_specific=false
IFS=',' read -ra selection_parts <<< "${selection}"
for selection_part in "${selection_parts[@]}"; do
  selection_part="${selection_part//[[:space:]]/}"
  case "${selection_part}" in
    everything)
      seen_everything=true
      include_browser=true
      include_macos=true
      include_windows=true
      include_linux=true
      ;;
    browser)
      seen_specific=true
      include_browser=true
      ;;
    macos|darwin)
      seen_specific=true
      include_macos=true
      ;;
    windows)
      seen_specific=true
      include_windows=true
      ;;
    linux)
      seen_specific=true
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
if [[ "${seen_everything}" == "true" && "${seen_specific}" == "true" ]]; then
  echo "everything cannot be combined with narrower plugin release selections" >&2
  exit 1
fi
if [[ "${include_browser}" == "false" && "${include_macos}" == "false" && "${include_windows}" == "false" && "${include_linux}" == "false" ]]; then
  echo "plugin release selection produced no surfaces" >&2
  exit 1
fi

printf 'include_browser=%s\n' "${include_browser}"
printf 'include_macos=%s\n' "${include_macos}"
printf 'include_windows=%s\n' "${include_windows}"
printf 'include_linux=%s\n' "${include_linux}"
