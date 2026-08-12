#!/bin/sh
# Install the Spacewave CLI for the current user when it is not already
# available, then execute the command supplied by the caller.
set -eu

repo_url="https://github.com/s4wave/spacewave"
release_url="${repo_url}/releases/latest/download"

if command -v spacewave >/dev/null 2>&1; then
  exec spacewave "$@"
fi

if [ -z "${HOME:-}" ]; then
  echo "HOME is required to install the Spacewave CLI." >&2
  exit 1
fi
install_dir="${HOME}/.local/bin"
target="${install_dir}/spacewave"

case ":${PATH:-}:" in
  *":${install_dir}:"*) install_dir_on_path=true ;;
  *) install_dir_on_path=false ;;
esac

exec_spacewave() {
  if [ "${install_dir_on_path}" = false ]; then
    printf 'Spacewave is installed in %s. Add it to PATH for future shells.\n' \
      "${install_dir}" >&2
  fi
  PATH="${install_dir}:${PATH:-}"
  export PATH
  if [ -n "${temp_dir:-}" ]; then
    cleanup
  fi
  exec spacewave "$@"
}

if [ -x "${target}" ]; then
  exec_spacewave "$@"
fi

case "$(uname -s)" in
  Darwin)
    os="macos"
    archive_ext="zip"
    ;;
  Linux)
    os="linux"
    archive_ext="tar.gz"
    ;;
  *)
    echo "Spacewave CLI is not available for $(uname -s)." >&2
    exit 1
    ;;
esac

case "$(uname -m)" in
  x86_64 | amd64)
    arch="amd64"
    ;;
  aarch64 | arm64)
    arch="arm64"
    ;;
  *)
    echo "Spacewave CLI is not available for $(uname -m)." >&2
    exit 1
    ;;
esac

if ! command -v curl >/dev/null 2>&1; then
  echo "curl is required to install the Spacewave CLI." >&2
  exit 1
fi

archive_name="spacewave-cli-${os}-${arch}.${archive_ext}"
temp_dir="$(mktemp -d)"
cleanup() {
  rm -rf "${temp_dir}"
}
trap cleanup EXIT HUP INT TERM

archive_path="${temp_dir}/${archive_name}"
checksums_path="${temp_dir}/checksums.txt"
extract_dir="${temp_dir}/extract"
curl --fail --location --silent --show-error \
  --output "${checksums_path}" \
  "${release_url}/checksums.txt"
curl --fail --location --silent --show-error \
  --output "${archive_path}" \
  "${release_url}/${archive_name}"

expected_checksum="$(awk -v name="${archive_name}" '$2 == name { print $1; exit }' "${checksums_path}")"
if [ -z "${expected_checksum}" ]; then
  echo "Release checksums do not include ${archive_name}." >&2
  exit 1
fi

if command -v sha256sum >/dev/null 2>&1; then
  actual_checksum="$(sha256sum "${archive_path}" | awk '{ print $1 }')"
elif command -v shasum >/dev/null 2>&1; then
  actual_checksum="$(shasum -a 256 "${archive_path}" | awk '{ print $1 }')"
else
  echo "sha256sum or shasum is required to verify the Spacewave CLI." >&2
  exit 1
fi
if [ "${actual_checksum}" != "${expected_checksum}" ]; then
  echo "Spacewave CLI checksum verification failed." >&2
  exit 1
fi

mkdir -p "${extract_dir}" "${install_dir}"
case "${archive_ext}" in
  tar.gz)
    tar -xzf "${archive_path}" -C "${extract_dir}"
    ;;
  zip)
    if ! command -v unzip >/dev/null 2>&1; then
      echo "unzip is required to install the Spacewave CLI on macOS." >&2
      exit 1
    fi
    unzip -q "${archive_path}" -d "${extract_dir}"
    ;;
esac

binary_path="${extract_dir}/spacewave"
if [ ! -f "${binary_path}" ]; then
  echo "Spacewave CLI archive did not contain spacewave." >&2
  exit 1
fi
chmod 755 "${binary_path}"
mv "${binary_path}" "${target}"

exec_spacewave "$@"
