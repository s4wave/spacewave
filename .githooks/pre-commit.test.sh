#!/bin/sh
# Tests for .githooks/pre-commit.
#
# Each test installs the hook under test into a disposable git repository,
# stubs the external formatters (bun, oxfmt, oxlint, gofmt, go) with
# deterministic recorders, and drives the hook through ordinary `git commit`.
# No test passes --no-verify; a commit that fails here fails because the real
# hook rejected it.
#
# HOOK_UNDER_TEST overrides the hook file (default .githooks/pre-commit), and
# PUSH_HOOK_UNDER_TEST the pre-push hook (default .githooks/pre-push).
#
# Run: ./.githooks/pre-commit.test.sh

set -u

repo_root=$(cd "$(dirname "$0")/.." && pwd)
hook_under_test=${HOOK_UNDER_TEST:-"$repo_root/.githooks/pre-commit"}
push_hook_under_test=${PUSH_HOOK_UNDER_TEST:-"$repo_root/.githooks/pre-push"}

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

	# Tool stubs. bun records its arguments.
	cat >"$test_dir/bun" <<'STUB'
#!/bin/sh
printf 'bun %s\n' "$*" >>"${HOOKTEST_LOG:?}"
case " $* " in
*" lint "*) [ -z "${HOOKTEST_LINT_FAIL:-}" ] || exit 1 ;;
esac
exit 0
STUB

	cat >"$test_dir/go" <<'STUB'
#!/bin/sh
printf 'go %s\n' "$*" >>"${HOOKTEST_LOG:?}"
printf 'go GIT_DIR=%s\n' "${GIT_DIR:-}" >>"$HOOKTEST_LOG"
printf 'go GIT_INDEX_FILE=%s\n' "${GIT_INDEX_FILE:-}" >>"$HOOKTEST_LOG"
printf 'go GO_PARAGRAPHS_INDEX=%s\n' "${GO_PARAGRAPHS_INDEX:-}" >>"$HOOKTEST_LOG"
if [ "${1:-}" = run ] && [ "${2:-}" = ./.githooks/go-paragraphs ] && [ -n "${HOOKTEST_PARAGRAPHS_FAIL:-}" ]; then
	exit 1
fi
if [ "${1:-}" = env ] && [ "${2:-}" = GOMOD ]; then
	prefix=$(git rev-parse --show-prefix)
	printf '%s/go.mod\n' "${PWD%/${prefix%/}}"
fi
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
	cp "$push_hook_under_test" "$test_dir/repo/.git/hooks/pre-push"
	chmod +x "$test_dir/repo/.git/hooks/pre-commit" "$test_dir/repo/.git/hooks/pre-push"
	mkdir -p "$test_dir/repo/.githooks"
	cp "$(dirname "$hook_under_test")/check-excuse-comments" "$test_dir/repo/.githooks/"
}

seed_commit() {
	git -C "$test_dir/repo" add -A
	HOOKTEST_LOG=/dev/null PATH="$test_dir:$PATH" \
		git -C "$test_dir/repo" commit -qm seed
}

try_commit() {
	# try_commit <expect: ok|fail> [checkout, default repo]
	log="$test_dir/hook.log"
	commit_out="$test_dir/commit.out"
	: >"$log"
	(cd "$test_dir/${2:-repo}" && PATH="$test_dir:$PATH" HOOKTEST_LOG="$log" \
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

# no_formatter_ran succeeds when neither oxfmt nor gofmt received a file.
no_formatter_ran() {
	! grep -Eq '^(oxfmt|gofmt) ' "$test_dir/hook.log"
}

out_says() {
	grep -Fq "$1" "$commit_out"
}

cleanup() {
	rm -rf "$test_dir"
	test_dir=
}

# --- 1. fast path ---------------------------------------------------------

test_docs_commit_runs_no_formatter() {
	label='fast path: a docs-only commit runs no formatter'
	new_repo
	printf 'readme\n' >"$test_dir/repo/README.md"
	seed_commit
	printf 'more\n' >>"$test_dir/repo/README.md"
	git -C "$test_dir/repo" add README.md
	try_commit ok &&
		no_formatter_ran
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
	try_commit fail &&
		log_has 'oxfmt 1 --stdin-filepath=web/new.ts' &&
		grep -Fqx 'web/new.ts' "$test_dir/commit.out"
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
		! grep -Fq 'go fix' "$test_dir/hook.log"
	note_result $? "$label"
	cleanup
}

# --- 6. package boundaries ---------------------------------------------------

test_go_pkg_dirs_unique_and_rooted() {
	label='packages: unique rooted dirs incl root-level file; no go fix'
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
		! grep -Fq 'go fix' "$test_dir/hook.log"
	note_result $? "$label"
	cleanup
}

# --- 7. exactness of the candidate set ---------------------------------------

test_deleted_file_not_candidate() {
	label='gate: a staged deletion is not handed to a formatter'
	new_repo
	mkdir -p "$test_dir/repo/web"
	printf 'let gone = 1;\n' >"$test_dir/repo/web/gone.ts"
	seed_commit
	git -C "$test_dir/repo" rm -q web/gone.ts
	try_commit ok &&
		no_formatter_ran
	note_result $? "$label"
	cleanup
}

test_go_paragraphs_gate() {
	label='paragraphs: a paragraph failure fails the commit after the lint still runs'
	new_repo
	printf 'package a\n' >"$test_dir/repo/a.go"
	seed_commit
	printf 'package a\n\nfunc F() {}\n' >"$test_dir/repo/a.go"
	git -C "$test_dir/repo" add a.go
	HOOKTEST_PARAGRAPHS_FAIL=1
	export HOOKTEST_PARAGRAPHS_FAIL
	try_commit fail &&
		log_has 'go run ./.githooks/go-paragraphs' &&
		log_has 'bun run go:aptre -- lint ./.' &&
		grep -Fq 'pre-commit: go-paragraphs failed' "$test_dir/commit.out"
	rc=$?
	unset HOOKTEST_PARAGRAPHS_FAIL
	note_result "$rc" "$label"
	cleanup
}

test_docs_skip_go_paragraphs() {
	label='paragraphs: a commit without Go files skips the paragraph check'
	new_repo
	printf 'readme\n' >"$test_dir/repo/README.md"
	seed_commit
	printf 'more\n' >>"$test_dir/repo/README.md"
	git -C "$test_dir/repo" add README.md
	try_commit ok &&
		log_lacks 'go run ./.githooks/go-paragraphs'
	note_result $? "$label"
	cleanup
}

test_worktree_go_without_git_dir() {
	label='worktree: go runs without GIT_DIR or GIT_INDEX_FILE and the main checkout tools resolve'
	new_repo
	git -C "$test_dir/repo" config core.hooksPath "$test_dir/repo/.git/hooks"
	printf 'package main\n' >"$test_dir/repo/root.go"
	seed_commit
	git -C "$test_dir/repo" worktree add -q "$test_dir/wt"
	printf 'package main\nvar A = 1\n' >"$test_dir/wt/root.go"
	git -C "$test_dir/wt" add root.go
	try_commit ok wt &&
		log_has 'go list ./.' &&
		! grep -q '^go GIT_DIR=.' "$test_dir/hook.log" &&
		! grep -q '^go GIT_INDEX_FILE=.' "$test_dir/hook.log"
	note_result $? "$label"
	cleanup
}

test_worktree_links_main_node_modules() {
	label='worktree: a JS commit links the main checkout node_modules and lints with it'
	new_repo
	git -C "$test_dir/repo" config core.hooksPath "$test_dir/repo/.git/hooks"
	printf 'node_modules\n' >"$test_dir/repo/.gitignore"
	mkdir -p "$test_dir/repo/web"
	printf 'export const a = 1\n' >"$test_dir/repo/web/a.ts"
	seed_commit
	git -C "$test_dir/repo" worktree add -q "$test_dir/wt"
	printf 'export const a = 2\n' >"$test_dir/wt/web/a.ts"
	git -C "$test_dir/wt" add web/a.ts
	try_commit ok wt &&
		log_has 'oxlint 2 web/a.ts' &&
		[ -L "$test_dir/wt/node_modules" ] &&
		[ "$(cd -P "$test_dir/wt/node_modules" && pwd)" = "$(cd -P "$test_dir/repo/node_modules" && pwd)" ] &&
		[ -z "$(git -C "$test_dir/wt" status --porcelain)" ]
	note_result $? "$label"
	cleanup
}

test_commit_all_reads_temporary_index() {
	label='commit -a: the hook reads the temporary index git commits'
	new_repo
	printf 'package main\n' >"$test_dir/repo/root.go"
	seed_commit
	printf 'package main\nvar A = 1\n' >"$test_dir/repo/root.go"
	log="$test_dir/hook.log"
	: >"$log"
	(cd "$test_dir/repo" && PATH="$test_dir:$PATH" HOOKTEST_LOG="$log" \
		git commit -qam "test commit" >"$test_dir/commit.out" 2>&1) &&
		log_has 'go list ./.' &&
		! grep -q '^go GIT_INDEX_FILE=.' "$log" &&
		grep '^go GO_PARAGRAPHS_INDEX=.' "$log" | grep -Fvq "=$test_dir/repo/.git/index"
	note_result $? "$label"
	cleanup
}

# try_push pushes HEAD to the bare remote's main and expects ok or fail.
try_push() {
	: >"$test_dir/hook.log"
	commit_out="$test_dir/push.out"
	(cd "$test_dir/repo" && PATH="$test_dir:$PATH" HOOKTEST_LOG="$test_dir/hook.log" \
		git push -q "$test_dir/remote.git" HEAD:refs/heads/main >"$commit_out" 2>&1)
	rc=$?
	if [ "$1" = ok ] && [ "$rc" -ne 0 ]; then
		sed 's/^/    unexpected push failure: /' "$commit_out"
		return 1
	fi
	if [ "$1" = fail ] && [ "$rc" -eq 0 ]; then
		sed 's/^/    unexpected push: /' "$commit_out"
		return 1
	fi
	return 0
}

# pick_from_side commits $1 on a side branch from the seed, through the commit
# hook, and cherry-picks it onto main, which runs no hook.
pick_from_side() {
	git -C "$test_dir/repo" switch -q -c "side-$1" seed
	mkdir -p "$test_dir/repo/$1"
	printf 'package %s\n' "$1" >"$test_dir/repo/$1/$1.go"
	git -C "$test_dir/repo" add "$1"
	try_commit ok || return 1
	git -C "$test_dir/repo" switch -q main
	git -C "$test_dir/repo" cherry-pick "side-$1" >/dev/null
}

test_push_checks_only_unseen_trees() {
	label='pre-push: a committed tree pushes unchecked; a picked one is checked; a finding refuses'
	new_repo
	git -C "$test_dir/repo" switch -q -c main
	printf 'package main\n' >"$test_dir/repo/root.go"
	seed_commit
	git -C "$test_dir/repo" tag seed
	git init -q --bare "$test_dir/remote.git"
	printf 'package main\nvar A = 1\n' >"$test_dir/repo/root.go"
	git -C "$test_dir/repo" add root.go
	try_commit ok &&
		log_has 'bun run go:aptre -- lint ./.' &&
		try_push ok &&
		! grep -q 'aptre' "$test_dir/hook.log" &&
		pick_from_side b &&
		try_push ok &&
		out_says 'no commit hook saw tree' &&
		log_has 'bun run go:aptre -- lint ./b' &&
		log_lacks 'bun run go:aptre -- lint ./.' &&
		try_push ok &&
		! grep -q 'aptre' "$test_dir/hook.log" &&
		pick_from_side c &&
		HOOKTEST_LINT_FAIL=1 try_push fail &&
		out_says 'failed the commit checks' &&
		[ "$(git -C "$test_dir/remote.git" rev-parse main)" = "$(git -C "$test_dir/repo" rev-parse main^)" ]
	note_result $? "$label"
	cleanup
}

# --- 8. excuse comments ------------------------------------------------------

test_excuse_comment_refused() {
	label='excuse: an added comment that excuses a workaround fails the commit'
	new_repo
	mkdir -p "$test_dir/repo/web"
	printf 'export const a = 1\n' >"$test_dir/repo/web/a.ts"
	seed_commit
	printf 'export const a = 1\n// TODO: hack for now\nexport const b = 2\n' >"$test_dir/repo/web/a.ts"
	git -C "$test_dir/repo" add web/a.ts
	try_commit fail &&
		out_says 'web/a.ts:2: hack: // TODO: hack for now' &&
		out_says 'pre-commit: excuse-comments failed'
	note_result $? "$label"
	cleanup
}

test_excuse_comment_cited_passes() {
	label='excuse: a comment that cites its outside constraint passes'
	new_repo
	printf 'package a\n' >"$test_dir/repo/a.go"
	seed_commit
	printf 'package a\n\n// The parser rejects tabs; work around it until it is fixed:\n// https://github.com/example/parser/issues/12\nvar A = 1\n' >"$test_dir/repo/a.go"
	git -C "$test_dir/repo" add a.go
	try_commit ok
	note_result $? "$label"
	cleanup
}

test_excuse_comment_context_passes() {
	label='excuse: an existing excuse beside an added line does not fail the commit'
	new_repo
	printf 'package a\n\n// HACK: keep\nvar A = 1\n' >"$test_dir/repo/a.go"
	# The seed stands for code that predates the check, so it skips the hook.
	git -C "$test_dir/repo" add -A
	git -C "$test_dir/repo" -c core.hooksPath=/dev/null commit -qm seed
	printf 'package a\n\n// HACK: keep\nvar A = 2\n' >"$test_dir/repo/a.go"
	git -C "$test_dir/repo" add a.go
	try_commit ok
	note_result $? "$label"
	cleanup
}

# --- runner ------------------------------------------------------------------

for t in \
	test_docs_commit_runs_no_formatter \
	test_rename_dirty_destination \
	test_rename_clean_commits_and_formats_destination \
	test_spaces_and_quotes_in_names \
	test_space_in_go_name_package_dirs \
	test_go_pkg_dirs_unique_and_rooted \
	test_deleted_file_not_candidate \
	test_go_paragraphs_gate \
	test_docs_skip_go_paragraphs \
	test_worktree_go_without_git_dir \
	test_worktree_links_main_node_modules \
	test_commit_all_reads_temporary_index \
	test_push_checks_only_unseen_trees \
	test_excuse_comment_refused \
	test_excuse_comment_cited_passes \
	test_excuse_comment_context_passes
do
	"$t"
done

printf '\n%d passed, %d failed\n' "$pass_count" "$fail_count"
[ "$fail_count" -eq 0 ]
