## IMPORTANT

Fix anything that you come across in the project while working that violates any of these guidelines as you encounter it.

**CRITICAL: When asked to update AGENTS.md:**

- ALWAYS read the ENTIRE file first before making edits
- Check for duplicate information across sections
- Condense and consolidate duplicates into a single authoritative section
- Ensure guidelines are clear and non-contradictory

Remember to always delete dead code when changing things - for example if you changed something and a function is no longer used anywhere, delete that function.

When a bug is reported, don't start by trying to fix it. Instead, start by writing a test that reproduces the bug. Then, have subagents try to fix the bug and prove it with a passing test.

- Try to keep things in one function unless composable or reusable
- Always use bun instead of npm, yarn, or pnpm
- DO NOT do unnecessary destructuring of variables
- DO NOT use `else` statements unless necessary
- DO NOT use `try`/`catch` if it can be avoided
- DO NOT make git commits
- DO NOT modify `bldr.yaml` unless explicitly asked
- DO NOT use emojis
- DO NOT use polling, always wait properly, like using a channel receive in go
- DO NOT add obvious comments like "(not persisted)" or "(ephemeral UI state)"
- DO NOT disable linter warnings unless absolutely necessary - if you need to disable a linter warning, it usually means you are doing something wrong and should rethink your approach
- DO NOT use `time.Sleep` - always use `time.After` with a select that includes `ctx.Done()` for cancellation
- DO NOT assume `bldr setup` needs to be run - it runs automatically when bldr starts for almost any operation
- DO NOT use `context.Background()` or `context.TODO()` - always use the appropriate context from the caller
- DO NOT use `context.WithoutCancel` - respect context cancellation
- AVOID `try`/`catch` where possible
- AVOID `else` statements
- AVOID using `any` type
- AVOID `let` statements
- AVOID using refs to store mutable state that affects rendering - use proper React state instead
- PREFER single word variable names where possible
- PREFER to merge multiple `useState` together into one if applicable
- PREFER `if err := ctx.Err(); err != nil` over `select { case <-ctx.Done(): ... default: }` for context cancellation checks
- PREFER one exported struct per `.go` file (file named after the struct, e.g., `state-atom-resource.go` for `StateAtomResource`)
  - Multiple unexported (internal) structs in the same file are acceptable
  - Constants and type aliases can be co-located with the struct that uses them
- PREFER to run go fix ./... after changing Go code when done with a batch of changes
- ALWAYS investigate all existing implementation details before making changes
- ALWAYS import useMemo, useCallback, etc, instead of using React.useMemo, React.useCallback, etc.
- NOTE that generated files like `*.pb*` are ignored from ripgrep (rg)
- NOTE that ripgrep does not have a built-in `tsx` type; use `-g "*.tsx"` instead of `--type tsx`
- DO NOT edit vendor/ as these changes will be clobbered by go mod vendor

## Imports

When importing from TypeScript (`.ts`) or JavaScript (`.js`) files:

- ALWAYS use the `.js` suffix in the import path
- This applies even when importing from `.ts` files
- Group imports in the following order with blank lines between groups:
  1. React and external libraries
  2. Internal modules (using blank line separator for routing, hooks, state, SDK, etc.)
  3. Local/relative imports

Example:

```tsx
// Correct - grouped and organized
import React, { useMemo, useCallback } from 'react'
import { joinPath } from '@aptre/bldr'
import { DebugInfo, useWatchStateRpc } from '@aptre/bldr-react'

import { useNavigate, useParams, useRouter } from '@web/router/router.js'
import { SharedObjectContext } from '@web/hooks/contexts.js'
import { useResource, useResourceResult } from '@web/hooks/useResource.js'
import { atomWithLocalStorage, StateNamespaceProvider } from '@web/state'
import { Space } from '@s4wave/sdk/space/space.js'
import { SpaceState, WatchStateRequest } from '@s4wave/sdk/space/space.pb.js'

import { SpaceProvider } from './SpaceContext.js'

// Incorrect - no grouping or organization
import { SpaceContainer } from '@web/space/SpaceContainer'
import { SPACE_BODY_TYPE } from '@web/space/space'
```

## Comments

When adding comments to components, functions, or files:

- Use the format: `// ComponentName does something specific.`
- Start with the component/function/class name followed by a verb
- End with a period
- Keep it concise and descriptive

Example:

```tsx
// SharedObjectBodyContainer renders the type-specific body of a SharedObject.
export function SharedObjectBodyContainer(props: { ... }) {
  ...
}
```

### Go Type Assertions

When using type assertion syntax in Go, add a comment line just before:

```go
// _ is a type assertion
var _ SomeInterface = (*SomeStruct)(nil)
```

This verifies at compile-time that `SomeStruct` implements `SomeInterface`.

## Broadcast Wait Pattern

Use `broadcast.Broadcast` as the single mutex guarding shared state rather than a separate `sync.Mutex`. Name the field `bcast`. When waiting for a condition, always check state and get the wait channel inside the same `HoldLock` call to prevent missed-wakeup race conditions.

```go
// Correct: check state and get wait channel atomically
for {
	var ch <-chan struct{}
	var val SomeType
	s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		ch = getWaitCh()
		val = s.state
	})

	if val != nil {
		return val
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-ch:
	}
}

// Wrong: separate lock and broadcast (missed wakeup possible)
for {
	s.mtx.Lock()
	val := s.state
	s.mtx.Unlock()
	if val != nil {
		return val
	}

	var ch <-chan struct{}
	s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		ch = getWaitCh()
	})
	// BUG: broadcast can fire between mtx.Unlock and getWaitCh,
	// closing the old channel before we get the new one.
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-ch:
	}
}
```

When broadcasting a state change, update state and broadcast inside the same `HoldLock`:

```go
s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
	s.state = newValue
	broadcast()
})
```

## File Logging

Bldr supports file-based logging via the `--log-file` flag and `BLDR_LOG_FILE`
environment variable. Implementation is in `util/logfile/`.

```bash
# Explicit file logging
bldr --log-file 'level=DEBUG;format=json;path=.bldr/logs/{ts}.log' start web

# Via environment variable
BLDR_LOG_FILE='level=WARN;path=/tmp/bldr-warn.log' bldr start web

# Short form (path only, defaults to level=DEBUG;format=text)
bldr --log-file '.bldr/logs/{ts}.log' start web

# Disable auto-logging in dev mode
BLDR_LOG_FILE=none bldr start web
```

In dev mode (`--build-type dev`), file logging is auto-enabled with
`level=DEBUG;path=.bldr/logs/{ts}.log`. Log files are created under
`.bldr/logs/` with session-stamped filenames. No auto-cleanup or rotation.

## Controller Patterns

### Execute() Method

For controllers that don't need to perform any background work, use a simple implementation:

**✅ Do this:**

```go
// Execute executes the controller.
func (c *Controller) Execute(ctx context.Context) error {
	return nil
}
```

**❌ Don't do this:**

```go
// Execute executes the controller.
func (c *Controller) Execute(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}
```

The simpler version is preferred when the controller doesn't need to block or perform background operations.

### Directive Resolver Patterns

When implementing directive resolvers in controllers:

**Resolver naming:**

```go
// ✅ Do this - descriptive name indicating what directive it resolves
type lookupBlockTypeResolver struct {
	ctx        context.Context
	di         directive.Instance
	dir        blocktype.LookupBlockType
	lookupFunc LookupBlockTypeFunc
}

// ❌ Don't do this - generic name
type resolver struct {
	...
}
```

**Store the directive, not extracted fields:**

```go
// ✅ Do this - store the directive itself
type lookupBlockTypeResolver struct {
	dir blocktype.LookupBlockType
}

func (r *lookupBlockTypeResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	typeID := r.dir.LookupBlockTypeID()
	...
}

// ❌ Don't do this - extract and store fields
type resolver struct {
	typeID string
}
```

**Use pointer receivers:**

```go
// ✅ Do this - pointer receiver
func (r *lookupBlockTypeResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	...
}

// ❌ Don't do this - value receiver
func (r lookupBlockTypeResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	...
}
```

**Return early for nil checks:**

```go
// ✅ Do this - positive check with early return
if blockType != nil {
	_, _ = handler.AddValue(blockType)
}
return nil

// ❌ Don't do this - negative check with early return
if blockType == nil {
	return nil
}
handler.AddValue(blockType)
return nil
```

**Prefer FuncResolver for simple resolvers:**

When the resolver logic is simple and doesn't require maintaining state across multiple calls, use `directive.NewFuncResolver` instead of creating a custom resolver type:

```go
// ✅ Do this - use FuncResolver for simple logic
func (c *Controller) resolveBlockType(
	ctx context.Context,
	di directive.Instance,
	dir blocktype.LookupBlockType,
) (directive.Resolver, error) {
	typeID := dir.LookupBlockTypeID()
	if typeID == "" {
		return nil, nil
	}

	return directive.NewFuncResolver(func(ctx context.Context, handler directive.ResolverHandler) error {
		blockType, err := c.lookupFunc(ctx, typeID)
		if err != nil {
			return err
		}

		if blockType != nil {
			_, _ = handler.AddValue(blockType)
		}

		return nil
	}), nil
}

// ❌ Don't do this - create custom resolver type for simple logic
type lookupBlockTypeResolver struct {
	ctx        context.Context
	di         directive.Instance
	dir        blocktype.LookupBlockType
	lookupFunc LookupBlockTypeFunc
}

func (r *lookupBlockTypeResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	blockType, err := r.lookupFunc(ctx, r.dir.LookupBlockTypeID())
	if err != nil {
		return err
	}
	if blockType != nil {
		_, _ = handler.AddValue(blockType)
	}
	return nil
}

func (c *Controller) resolveBlockType(...) (directive.Resolver, error) {
	return &lookupBlockTypeResolver{...}, nil
}
```

Only create custom resolver types when you need:

- Complex state management across multiple Resolve calls
- Methods beyond just Resolve()
- Resolver logic that benefits from being a separate testable unit

**Return resolver as pointer:**

```go
// ✅ Do this - return pointer to resolver
return &lookupBlockTypeResolver{
	ctx:        ctx,
	di:         di,
	dir:        dir,
	lookupFunc: c.lookupFunc,
}, nil

// ❌ Don't do this - return resolver value
return lookupBlockTypeResolver{
	ctx:        ctx,
	di:         di,
	dir:        dir,
	lookupFunc: c.lookupFunc,
}, nil
```

## Generating protobufs

After changing .proto files you must re-generate the protobufs:

```
# Note: `bun gen` won't see the .proto file unless it's in the index
git add path/to/changed/file.proto
bun gen
```

This will run protoc and re-generate `*.pb.go` and `*.pb.ts` files.

### Dist Sources (Embedded TypeScript Files)

When adding new TypeScript files that need to be bundled for the Electron or browser entrypoints, you must add them to the `//go:embed` directives in `dist.go`.

The `DistSources` embed.FS contains TypeScript sources used by esbuild during the build process. If a new `.pb.ts` file or other TypeScript module is imported by files in `web/electron/` or `web/entrypoint/`, it must be explicitly embedded.

Example - if you add a new protobuf that needs to be imported:

```go
// In dist.go, add the embed directive:
//go:embed web/plugin/electron/electron.pb.ts
```

Without this, esbuild will fail with "Could not resolve" errors when building the Electron or browser bundles.

### Proto comments

Always add comments to .proto files:

- Use the format: `// FieldName is the description of the field.`
- Start with the field/message/service/rpc name followed by `is` or a verb
- End with a period
- Keep it concise and descriptive
- Follow the Go comments of the relevant resource you are wrapping

Example:

```protobuf
// SessionResourceService provides access to session functionality.
service SessionResourceService {
  // GetSessionInfo returns information about this session.
  rpc GetSessionInfo(GetSessionInfoRequest) returns (GetSessionInfoResponse);
}

// GetSessionInfoResponse is the response type for GetSessionInfo.
message GetSessionInfoResponse {
  // SessionRef is the session reference.
  .session.SessionRef session_ref = 1;
  // PeerId is the session peer id.
  string peer_id = 2;
}
```

### Proto imports

Proto files use Go-style import paths based on Go module names (from `go.mod`).

**Within this project:**

This project's module is `github.com/aperturerobotics/alpha`. Local proto files reference each other using the full Go module path:

```protobuf
// In sdk/session/session.proto
import "github.com/aperturerobotics/alpha/core/session/session.proto";
import "github.com/aperturerobotics/alpha/core/sobject/sobject.proto";
```

**From external Go modules:**

You can import proto files from dependencies listed in `go.mod`:

```protobuf
// Importing from github.com/aperturerobotics/controllerbus
import "github.com/aperturerobotics/controllerbus/bus/bus.proto";

// Importing from github.com/aperturerobotics/starpc
import "github.com/aperturerobotics/starpc/srpc/srpc.proto";
```

The protobuf compiler resolves these paths by looking through Go module dependencies.

**Package naming conventions:**

- `sdk/` files use the full `s4wave.` prefix (e.g., `package s4wave.space;`)
- `core/` files use shortened package names without the prefix (e.g., `package space.world;`)
- When referencing types from `core/` packages in `sdk/` files, use a leading `.` for fully-qualified references (e.g., `.space.world.WorldContents`)

## Linting and typechecking

After making code changes, verify they compile correctly:

```
bun run typecheck
bun run lint
go build ./...
```

These commands should be run before committing code changes.

### Rebuilding .bldr

If you encounter issues with `.bldr` (stale exports, module resolution errors, etc.), rebuild it with:

```
bun run setup
```

This regenerates the `.bldr/src` directory from source.

### Running Commands with Large Output

For commands that produce large output or may timeout, redirect to a log file and show the last few lines:

```bash
# Run command, show last 5 lines
bun run test > .tmp/test.log 2>&1; tail -5 .tmp/test.log

# With exit code visibility
bun run test > .tmp/test.log 2>&1; echo "exit: $?"; tail -5 .tmp/test.log

# Show errors on failure, then tail
cmd > .tmp/log 2>&1; EC=$?; [ $EC -ne 0 ] && grep -i "error\|fail" .tmp/log; tail -5 .tmp/log
```

After running, you can inspect more of the log:

```bash
grep -A 5 "FAIL" .tmp/test.log   # Context around failures
tail -50 .tmp/test.log            # More lines
```

Why this matters:

- Piping long-running commands (`cmd | tail`) may timeout before output appears
- Log files allow multiple reads without re-running expensive commands
- The `.tmp/` directory is gitignored and safe for temporary files

## Testing

### Time-Based Tests

**NEVER use time-based synchronization in tests:**

- DO NOT use `context.WithTimeout` for test synchronization
- DO NOT use `time.Sleep` to wait for operations
- DO NOT use `time.After` for delays
- DO NOT use any time-based polling or waiting

Time-based tests are:

- Flaky and unreliable (may fail on slower CI machines)
- Slow (artificially increase test runtime)
- Non-deterministic (race conditions)

**✅ Do this instead:**

Use the controllerbus idle callback pattern to wait for operations to complete:

```go
// Wait for directive to become idle (no values found)
dir := blocktype.NewLookupBlockType("nonexistent.type")
_, _, _, err := bus.ExecOneOffTyped[blocktype.BlockType](
	ctx,
	b,
	dir,
	bus.ReturnWhenIdle(), // Returns when directive becomes idle
	nil,
)
```

**Common patterns:**

- `bus.ReturnWhenIdle()` - Returns immediately when the directive becomes idle
- `bus.WaitWhenIdle(ignoreErrors bool)` - Continues waiting when idle
- `bus.ReturnIfIdle(returnIfIdle bool)` - Conditionally returns based on parameter

**For operations that should succeed:**

```go
// This will wait until a value is found or an error occurs
result, ref, err := blocktype.ExLookupBlockType(ctx, b, typeID)
if err != nil {
	t.Fatalf("failed: %v", err)
}
defer ref.Release()
```

**For operations that should find no results:**

```go
// Use ReturnWhenIdle to return nil when no resolvers provide values
dir := blocktype.NewLookupBlockType("nonexistent")
val, _, ref, err := bus.ExecOneOffTyped[blocktype.BlockType](
	ctx,
	b,
	dir,
	bus.ReturnWhenIdle(),
	nil,
)
if val != nil && ref != nil {
	ref.Release()
}
// val will be nil if no values found
```
