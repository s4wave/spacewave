---
name: post-work-review
description: Use after completing implementation work to catch AGENTS.md violations, dead code, and issues missed during development. Run before showing results to the user.
---

# Post-Work Review

Review your diff against AGENTS.md rules and fix violations before presenting work.

## Process

1. Run `git diff HEAD` (or appropriate base) to see all changes
2. Read `AGENTS.md` fully -- the rules section is authoritative
3. For each changed file, check every rule that applies
4. Fix violations directly -- do not just report them
5. Re-run type check (`npx tsc --noEmit` or `go build ./...`)

## High-Signal Checks

These are the violations most commonly introduced under time pressure:

| Check | Rule | How to spot |
|-------|------|-------------|
| Type casts | AVOID `as` statements | grep for ` as ` in diff |
| Any types | AVOID `any` type | grep for `: any` or `as any` |
| Let bindings | AVOID `let` statements | grep for `let ` in diff |
| Else branches | DO NOT use `else` unless necessary | grep for `} else` |
| Dead code | Delete unused code when changing things | check if old code is still referenced |
| String interpolation | Use `cn()` not template literals for className | grep for `` className={` `` |
| Obvious comments | DO NOT use obvious comments | read comments -- are they saying what the code already says? |
| Disabled linters | DO NOT disable linter warnings unless necessary | grep for `eslint-disable` in new lines |

## Quick Grep

```bash
# Run against staged/unstaged changes
git diff HEAD -U0 | grep -E '^\+.*\bas\b' | grep -v '^+++' | grep -v 'import.*as'
git diff HEAD -U0 | grep -E '^\+.*: any' | grep -v '^+++'
git diff HEAD -U0 | grep -E '^\+.*\blet\b ' | grep -v '^+++'
git diff HEAD -U0 | grep -E '^\+.*\} else' | grep -v '^+++'
git diff HEAD -U0 | grep -E '^\+.*eslint-disable' | grep -v '^+++'
```

## Common Misses

- **Casts introduced to satisfy a generic** -- rethink the generic or type parameter instead
- **`undefined as void`** -- `undefined` is assignable to `void` directly
- **`let` for a variable reassigned once** -- restructure to use `const`
- **Dead imports** -- after deleting code, check if its imports are still needed
- **Comments describing the obvious** -- `// Release all resources` above `releaseAllResources()` adds nothing
