#!/bin/bash
set -euo pipefail

# Regression test for scripts/release/build-helper.sh darwin arch handling.
# A stubbed swift records invocations and fakes build outputs, so the test
# proves argument routing without compiling anything. It fails on a script
# that builds an unrequested architecture or accepts an unsupported one.

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
script="$repo_dir/scripts/release/build-helper.sh"
temp_dir=$(mktemp -d)
cleanup() {
  rm -rf "$temp_dir"
}
trap cleanup EXIT HUP INT TERM

mkdir -p "$temp_dir/bin" "$temp_dir/home"
# The stub mimics modern SwiftPM: products live under <scratch>/out/Products/
# Release and --show-bin-path prints that directory without building. It never
# creates the legacy <triple>/release layout, so a script that copies from a
# hard-coded triple path fails this test.
cat > "$temp_dir/bin/swift" <<'EOF'
#!/bin/bash
set -euo pipefail
echo "swift $*" >> "${SWIFT_CALL_LOG:?}"
arch=""
scratch=""
show_bin_path=0
prev=""
for arg in "$@"; do
  case "$prev" in
    --arch) arch="$arg" ;;
    --scratch-path) scratch="$arg" ;;
  esac
  [ "$arg" = "--show-bin-path" ] && show_bin_path=1
  prev="$arg"
done
if [ -z "$arch" ] || [ -z "$scratch" ]; then
  echo "stub swift: expected --arch and --scratch-path" >&2
  exit 1
fi
product_dir="$scratch/out/Products/Release"
if [ "$show_bin_path" = 0 ]; then
  mkdir -p "$product_dir"
  printf '#!/bin/sh\n' > "$product_dir/SpacewaveHelper"
  chmod 755 "$product_dir/SpacewaveHelper"
else
  echo "$product_dir"
fi
EOF
chmod 755 "$temp_dir/bin/swift"

new_env() {
  local name="$1"
  mkdir -p "$temp_dir/$name/scripts/release" "$temp_dir/$name/desktop/macos"
  cp "$script" "$temp_dir/$name/scripts/release/build-helper.sh"
}

run_build() {
  local name="$1"
  shift
  env -i PATH="$temp_dir/bin:/usr/bin:/bin" HOME="$temp_dir/home" \
    SWIFT_CALL_LOG="$temp_dir/$name-swift-calls.log" \
    bash "$temp_dir/$name/scripts/release/build-helper.sh" "$@"
}

swift_calls() {
  cat "$temp_dir/$1-swift-calls.log"
}

# darwin arm64 builds only arm64.
new_env arm64-case
run_build arm64-case darwin arm64 > "$temp_dir/arm64-case-stdout.log"
calls=$(swift_calls arm64-case)
[ "$(grep -c '^swift ' "$temp_dir/arm64-case-swift-calls.log")" -eq 2 ]
printf '%s' "$calls" | grep -q -- '--arch arm64'
if printf '%s' "$calls" | grep -q -- '--arch x86_64'; then
  echo 'FAIL: darwin arm64 request also invoked swift for x86_64' >&2
  exit 1
fi
test -x "$temp_dir/arm64-case/dist/helper/darwin-arm64/spacewave-helper"
if [ -e "$temp_dir/arm64-case/dist/helper/darwin-amd64" ]; then
  echo 'FAIL: darwin arm64 request produced a darwin-amd64 output' >&2
  exit 1
fi

# darwin amd64 builds only x86_64.
new_env amd64-case
run_build amd64-case darwin amd64 > "$temp_dir/amd64-case-stdout.log"
calls=$(swift_calls amd64-case)
[ "$(grep -c '^swift ' "$temp_dir/amd64-case-swift-calls.log")" -eq 2 ]
printf '%s' "$calls" | grep -q -- '--arch x86_64'
test -x "$temp_dir/amd64-case/dist/helper/darwin-amd64/spacewave-helper"
if [ -e "$temp_dir/amd64-case/dist/helper/darwin-arm64" ]; then
  echo 'FAIL: darwin amd64 request produced a darwin-arm64 output' >&2
  exit 1
fi

# An unsupported darwin ARCH fails before invoking swift.
new_env badarch-case
if run_build badarch-case darwin riscv64 > "$temp_dir/badarch-case-stdout.log" 2>&1; then
  echo 'FAIL: unsupported darwin ARCH was accepted' >&2
  exit 1
fi
if [ -s "$temp_dir/badarch-case-swift-calls.log" ]; then
  echo 'FAIL: unsupported darwin ARCH invoked swift' >&2
  exit 1
fi

echo 'build-helper.test.sh: ok'
