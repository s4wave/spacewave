#!/bin/sh
set -eu

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
script="${repo_dir}/scripts/spacewave.sh"
temp_dir=$(mktemp -d)
cleanup() {
  rm -rf "${temp_dir}"
}
trap cleanup EXIT HUP INT TERM

mkdir -p "${temp_dir}/archive" "${temp_dir}/bin" "${temp_dir}/home" \
  "${temp_dir}/install-tmp"
cat > "${temp_dir}/archive/spacewave" <<'EOF'
#!/bin/sh
if [ -n "${SPACEWAVE_TEST_TMP_ROOT:-}" ]; then
  for path in "${SPACEWAVE_TEST_TMP_ROOT}"/*; do
    if [ -d "${path}" ]; then
      echo "bootstrap temp directory remained before CLI exec" >&2
      exit 1
    fi
  done
fi
printf '%s\n' "$@" > "${SPACEWAVE_TEST_ARGS:?}"
EOF
chmod 755 "${temp_dir}/archive/spacewave"
tar -czf "${temp_dir}/spacewave-cli-linux-amd64.tar.gz" \
  -C "${temp_dir}/archive" spacewave
if command -v sha256sum >/dev/null 2>&1; then
  checksum=$(sha256sum "${temp_dir}/spacewave-cli-linux-amd64.tar.gz" | awk '{ print $1 }')
elif command -v shasum >/dev/null 2>&1; then
  checksum=$(shasum -a 256 "${temp_dir}/spacewave-cli-linux-amd64.tar.gz" | awk '{ print $1 }')
else
  echo "sha256sum or shasum is required for this test." >&2
  exit 1
fi

cat > "${temp_dir}/bin/curl" <<'EOF'
#!/bin/sh
set -eu
output=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    --output)
      output="$2"
      shift 2
      ;;
    *)
      url="$1"
      shift
      ;;
  esac
done
case "${url}" in
  */checksums.txt)
    printf '%s  spacewave-cli-linux-amd64.tar.gz\n' "${SPACEWAVE_TEST_CHECKSUM:?}" > "${output:?}"
    ;;
  */spacewave-cli-linux-amd64.tar.gz)
    cp "${SPACEWAVE_TEST_ARCHIVE:?}" "${output:?}"
    ;;
  *)
    echo "unexpected curl URL: ${url}" >&2
    exit 1
    ;;
esac
EOF
chmod 755 "${temp_dir}/bin/curl"

cat > "${temp_dir}/bin/uname" <<'EOF'
#!/bin/sh
case "${1:-}" in
  -s) printf '%s\n' Linux ;;
  -m) printf '%s\n' x86_64 ;;
  *) printf '%s\n' Linux ;;
esac
EOF
chmod 755 "${temp_dir}/bin/uname"

PATH="${temp_dir}/bin:/usr/bin:/bin" \
HOME="${temp_dir}/home" \
TMPDIR="${temp_dir}/install-tmp" \
SPACEWAVE_TEST_TMP_ROOT="${temp_dir}/install-tmp" \
SPACEWAVE_TEST_ARCHIVE="${temp_dir}/spacewave-cli-linux-amd64.tar.gz" \
SPACEWAVE_TEST_CHECKSUM="${checksum}" \
SPACEWAVE_TEST_ARGS="${temp_dir}/args" \
"${script}" device setup --label "Build Host" --target-hint "space-1" \
  2> "${temp_dir}/stderr"

test -x "${temp_dir}/home/.local/bin/spacewave"
expected_args="$(printf '%s\n' device setup --label "Build Host" --target-hint space-1)"
test "$(cat "${temp_dir}/args")" = "${expected_args}"
grep -F "Add it to PATH for future shells." "${temp_dir}/stderr" >/dev/null

if PATH="${temp_dir}/bin:/usr/bin:/bin" \
  HOME="${temp_dir}/bad-home" \
  SPACEWAVE_TEST_ARCHIVE="${temp_dir}/spacewave-cli-linux-amd64.tar.gz" \
  SPACEWAVE_TEST_CHECKSUM="not-a-checksum" \
  "${script}" version 2> "${temp_dir}/bad-stderr"; then
  echo "expected checksum mismatch to fail" >&2
  exit 1
fi
test ! -e "${temp_dir}/bad-home/.local/bin/spacewave"
grep -F "checksum verification failed" "${temp_dir}/bad-stderr" >/dev/null

printf '%s\n' "spacewave.sh test: PASS"
