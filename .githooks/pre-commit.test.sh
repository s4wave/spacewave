#!/bin/sh
# Tests for .githooks/pre-commit.
#
# Each test installs the hook under test into a disposable git repository,
# stubs the external formatters (bun, oxfmt, oxlint, gofmt, go) with
# deterministic recorders, and drives the hook through ordinary `git commit`.
# No test passes --no-verify; a commit that fails here fails because the real
# hook rejected it.
#
# HOOK_UNDER_TEST overrides the hook file (default .githooks/pre-commit).
#
# Run: ./.githooks/pre-commit.test.sh

set -u

repo_root=$(cd "$(dirname "$0")/.." && pwd)
hook_under_test=${HOOK_UNDER_TEST:-"$repo_root/.githooks/pre-commit"}

pass_count=0
fail_count=0
test_dir=
commit_out=

die() {
	printf '%s\n' "$*" >&2
	exit 1
}

[ -f "$hook_under_test" ] || die "hook not found: $hook_under_test"

note_result() {
	if [ "$1" -eq 0 ]; then
		pass_count=$((pass_count + 1))
		printf 'ok   %s\n' "$2"
	else
		fail_count=$((fail_count + 1))
		printf 'FAIL %s\n' "$2"
	fi
}

new_repo() {
	test_dir=$(mktemp -d "${TMPDIR:-/tmp}/pre-commit-test.XXXXXX")
	mkdir -p "$test_dir/repo"
	git init -q "$test_dir/repo"
	git -C "$test_dir/repo" config user.email hook-test@example.invalid
	git -C "$test_dir/repo" config user.name "Hook Test"
	git -C "$test_dir/repo" config core.hooksPath .git/hooks

	# Formatter stubs. bun records its arguments and, when
	# HOOKTEST_REWRITE_TARGETS is set, rewrites those files during
	# `bun run format` to simulate a formatter that would change the tree.
	cat >"$test_dir/bun" <<'STUB'
#!/bin/sh
printf 'bun %s\n' "$*" >>"${HOOKTEST_LOG:?}"
if [ "${1:-}" = run ] && [ "${2:-}" = format ]; then
	for target in ${HOOKTEST_REWRITE_TARGETS:-}; do
		printf 'formatted by bun run format\n' >>"$target"
	done
fi
exit 0
STUB

	cat >"$test_dir/go" <<'STUB'
#!/bin/sh
printf 'go %s\n' "$*" >>"${HOOKTEST_LOG:?}"
exit 0
STUB

	for tool in gofmt oxfmt oxlint; do
		cat >"$test_dir/$tool" <<STUB
#!/bin/sh
i=0
for arg in "\$@"; do
	i=\$((i + 1))
	printf '$tool %s %s\n' "\$i" "\$arg" >>"\${HOOKTEST_LOG:?}"
done
exit 0
STUB
	done
	chmod +x "$test_dir/bun" "$test_dir/go" "$test_dir/gofmt" \
		"$test_dir/oxfmt" "$test_dir/oxlint"
	mkdir -p "$test_dir/repo/node_modules/.bin"
	ln -sf "$test_dir/oxfmt" "$test_dir/repo/node_modules/.bin/oxfmt"
	ln -sf "$test_dir/oxlint" "$test_dir/repo/node_modules/.bin/oxlint"

	cp "$hook_under_test" "$test_dir/repo/.git/hooks/pre-commit"
	chmod +x "$test_dir/repo/.git/hooks/pre-commit"
}

seed_commit() {
	git -C "$test_dir/repo" add -A
	HOOKTEST_LOG=/dev/null PATH="$test_dir:$PATH" \
		git -C "$test_dir/repo" commit -qm seed
}

try_commit() {
	# try_commit <expect: ok|fail>
	log="$test_dir/hook.log"
	commit_out="$test_dir/commit.out"
	: >"$log"
	(cd "$test_dir/repo" && PATH="$test_dir:$PATH" HOOKTEST_LOG="$log" \
		git commit -qm "test commit" >"$commit_out" 2>&1)
	rc=$?
	if [ "$1" = fail ] && [ "$rc" -eq 0 ]; then
		sed 's/^/    unexpected success: /' "$commit_out"
		return 1
	fi
	if [ "$1" = ok ] && [ "$rc" -ne 0 ]; then
		sed 's/^/    unexpected failure: /' "$commit_out"
		return 1
	fi
	return 0
}

log_has() {
	grep -Fqx "$1" "$test_dir/hook.log"
}

log_lacks() {
	! grep -Fqx "$1" "$test_dir/hook.log"
}

out_says() {
	grep -Fq "$1" "$commit_out"
}

cleanup() {
	rm -rf "$test_dir"
	test_dir=
}

# --- 1. fast path ---------------------------------------------------------

test_fast_path_skips_full_format() {
	label='fast path: nothing dirty-formattable skips bun run format'
	new_repo
	printf 'readme\n' >"$test_dir/repo/README.md"
	seed_commit
	printf 'more\n' >>"$test_dir/repo/README.md"
	git -C "$test_dir/repo" add README.md
	try_commit ok &&
		log_lacks 'bun run format'
	note_result $? "$label"
	cleanup
}

# --- 2. whitespace-only dirty file ----------------------------------------

test_whitespace_dirty_failure() {
	label='gate: whitespace-only dirty ts triggers full check, format change fails commit'
	new_repo
	mkdir -p "$test_dir/repo/web"
	printf 'let x = 1;\n' >"$test_dir/repo/web/a.ts"
	seed_commit
	# Whitespace-only unstaged edit.
	printf 'let x = 1;\n \t\n' >"$test_dir/repo/web/a.ts"
	printf 'doc\n' >"$test_dir/repo/README.md"
	git -C "$test_dir/repo" add README.md
	HOOKTEST_REWRITE_TARGETS=web/a.ts
	export HOOKTEST_REWRITE_TARGETS
	try_commit fail &&
		out_says 'bun run format changed the working tree' &&
		log_has 'bun run format'
	unset HOOKTEST_REWRITE_TARGETS
		log_has 'bun run format'
	note_result $? "$label"
	cleanup
}

test_whitespace_dirty_clean_format_passes() {
	label='gate: whitespace-only dirty ts with idempotent format passes'
	new_repo
	mkdir -p "$test_dir/repo/web"
	printf 'let x = 1;\n' >"$test_dir/repo/web/a.ts"
	seed_commit
	printf 'let x = 1;\n \t\n' >"$test_dir/repo/web/a.ts"
	printf 'doc\n' >"$test_dir/repo/README.md"
	git -C "$test_dir/repo" add README.md
	try_commit ok &&
		log_has 'bun run format'
	note_result $? "$label"
	cleanup
}

# --- 3. nested dirty Go source ---------------------------------------------

test_nested_go_dirty_detected() {
	label='gate: nested dirty Go source triggers check, go fix edit fails commit'
	new_repo
	mkdir -p "$test_dir/repo/net/deep"
	printf 'package deep\n' >"$test_dir/repo/net/deep/b.go"
	printf 'package net\n' >"$test_dir/repo/net/a.go"
	seed_commit
	# Unstaged, unformatted edit in the same package tree as the staged file.
	printf 'package deep\nfunc F( ) {}\n' >"$test_dir/repo/net/deep/b.go"
	printf 'doc\n' >"$test_dir/repo/README.md"
	git -C "$test_dir/repo" add README.md net/a.go
	HOOKTEST_REWRITE_TARGETS=net/deep/b.go
	export HOOKTEST_REWRITE_TARGETS
	try_commit fail &&
		out_says 'bun run format changed the working tree' &&
		log_has 'bun run format'
	unset HOOKTEST_REWRITE_TARGETS
		out_says 'bun run format changed the working tree' &&
		log_has 'bun run format'
	note_result $? "$label"
	cleanup
}

# --- 4. renamed files -------------------------------------------------------

test_rename_dirty_destination() {
	label='rename: unstaged edit at rename destination triggers the gate'
	new_repo
	mkdir -p "$test_dir/repo/web"
	printf 'let old = 1;\n' >"$test_dir/repo/web/old.ts"
	seed_commit
	git -C "$test_dir/repo" mv web/old.ts web/new.ts
	printf 'let old = 1;\n \n' >"$test_dir/repo/web/new.ts"
	printf 'doc\n' >"$test_dir/repo/README.md"
	git -C "$test_dir/repo" add README.md
	HOOKTEST_REWRITE_TARGETS=web/new.ts
	export HOOKTEST_REWRITE_TARGETS
	try_commit fail &&
		log_has 'bun run format'
	unset HOOKTEST_REWRITE_TARGETS
	note_result $? "$label"
	cleanup
}

test_rename_clean_commits_and_formats_destination() {
	label='rename: clean staged rename formats the destination path only'
	new_repo
	mkdir -p "$test_dir/repo/web"
	printf 'let old = 1;\n' >"$test_dir/repo/web/old.ts"
	seed_commit
	git -C "$test_dir/repo" mv web/old.ts web/new.ts
	try_commit ok &&
		log_has 'oxfmt 1 web/new.ts'
	note_result $? "$label"
	cleanup
}

# --- 5. argv edge cases ------------------------------------------------------

test_spaces_and_quotes_in_names() {
	label='argv: spaces and quotes in staged names reach oxfmt intact'
	new_repo
	mkdir -p "$test_dir/repo/web"
	printf 'let a = 1;\n' >"$test_dir/repo/web/my file.ts"
	printf "let b = 2;\n" >"$test_dir/repo/web/it's.ts"
	seed_commit # seed includes them; restage a tweak so they are ACMR
	printf 'let a = 2;\n' >"$test_dir/repo/web/my file.ts"
	printf "let b = 3;\n" >"$test_dir/repo/web/it's.ts"
	git -C "$test_dir/repo" add web
	try_commit ok &&
		grep -Eq '^oxfmt [0-9]+ web/my file\.ts$' "$test_dir/hook.log" &&
		grep -Fq "web/it's.ts" "$test_dir/hook.log"
	note_result $? "$label"
	cleanup
}

test_space_in_go_name_package_dirs() {
	label='argv: Go path with space keeps package dir intact through go list'
	new_repo
	mkdir -p "$test_dir/repo/net/my pkg"
	printf 'package pkg\n' >"$test_dir/repo/net/my pkg/a.go"
	seed_commit
	printf 'package pkg\nvar X = 1\n' >"$test_dir/repo/net/my pkg/a.go"
	git -C "$test_dir/repo" add 'net/my pkg/a.go'
	try_commit ok &&
		log_has 'go list ./net/my pkg' &&
		log_has 'go fix ./net/my pkg'
	note_result $? "$label"
	cleanup
}

# --- 6. package boundaries ---------------------------------------------------

test_go_pkg_dirs_unique_and_rooted() {
	label='packages: unique rooted dirs incl root-level file; one go fix per dir set'
	new_repo
	mkdir -p "$test_dir/repo/net" "$test_dir/repo/svc/sub"
	printf 'package main\n' >"$test_dir/repo/root.go"
	printf 'package net\n' >"$test_dir/repo/net/a.go"
	printf 'package sub\n' >"$test_dir/repo/svc/sub/b.go"
	seed_commit
	printf 'package main\nvar A = 1\n' >"$test_dir/repo/root.go"
	printf 'package net\nvar B = 1\n' >"$test_dir/repo/net/a.go"
	printf 'package sub\nvar C = 1\n' >"$test_dir/repo/svc/sub/b.go"
	git -C "$test_dir/repo" add root.go net/a.go svc/sub/b.go
	try_commit ok &&
		log_has 'go list ./.' &&
		log_has 'go list ./net' &&
		log_has 'go list ./svc/sub' &&
		log_has 'go fix ./. ./net ./svc/sub'
	note_result $? "$label"
	cleanup
}

# --- 7. exactness of the candidate set ---------------------------------------

test_deleted_file_not_candidate() {
	label='gate: deleted formattable file is not a format-check candidate'
	new_repo
	mkdir -p "$test_dir/repo/web"
	printf 'let gone = 1;\n' >"$test_dir/repo/web/gone.ts"
	seed_commit
	rm "$test_dir/repo/web/gone.ts"
	printf 'doc\n' >"$test_dir/repo/README.md"
	git -C "$test_dir/repo" add README.md
	try_commit ok &&
		log_lacks 'bun run format'
	note_result $? "$label"
	cleanup
}

test_vendor_dirty_not_candidate() {
	label='gate: vendored dirty Go source is not a format-check candidate'
	new_repo
	mkdir -p "$test_dir/repo/vendor/example.com/pkg"
	printf 'package pkg\n' >"$test_dir/repo/vendor/example.com/pkg/v.go"
	seed_commit
	printf 'package pkg\nvar V = 1\n' >"$test_dir/repo/vendor/example.com/pkg/v.go"
	printf 'doc\n' >"$test_dir/repo/README.md"
	git -C "$test_dir/repo" add README.md
	try_commit ok &&
		log_lacks 'bun run format'
	note_result $? "$label"
	cleanup
}

# --- runner ------------------------------------------------------------------

for t in \
	test_fast_path_skips_full_format \
	test_whitespace_dirty_failure \
	test_whitespace_dirty_clean_format_passes \
	test_nested_go_dirty_detected \
	test_rename_dirty_destination \
	test_rename_clean_commits_and_formats_destination \
	test_spaces_and_quotes_in_names \
	test_space_in_go_name_package_dirs \
	test_go_pkg_dirs_unique_and_rooted \
	test_deleted_file_not_candidate \
	test_vendor_dirty_not_candidate
do
	"$t"
done

printf '\n%d passed, %d failed\n' "$pass_count" "$fail_count"
[ "$fail_count" -eq 0 ]
