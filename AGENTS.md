## IMPORTANT

- Try to keep things in one function unless composable or reusable
- DO NOT do unnecessary destructuring of variables
- DO NOT use `else` statements unless necessary
- DO NOT use `try`/`catch` if it can be avoided
- DO NOT make git commits
- DO NOT modify `bldr.yaml` (ignore any changes in this file)
- DO NOT use emojis
- AVOID `try`/`catch` where possible
- AVOID `else` statements
- AVOID using `any` type
- AVOID `let` statements
- PREFER single word variable names where possible
- PREFER to merge multiple `useState` together into one if applicable

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
# Note: `yarn gen` won't see the .proto file unless it's in the index
git add path/to/changed/file.proto
yarn gen
```

This will run protoc and re-generate `*.pb.go` and `*.pb.ts` files.

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
yarn typecheck
yarn lint
go build ./...
```

These commands should be run before committing code changes.

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
