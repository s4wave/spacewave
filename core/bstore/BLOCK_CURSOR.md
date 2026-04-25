# Block Cursor Pattern

This document explains how to use the Hydra block storage pattern (`AccessWorldObject` => `SetBlock`) for managing world state.

## Overview

Hydra uses a content-addressed Merkle DAG (Directed Acyclic Graph) for storage. The `block.Cursor` pattern provides a high-level API to read and modify blocks within this DAG structure.

**Key concepts:**

- **Blocks**: Content-addressed storage units (identified by hash of contents)
- **Cursors**: Position trackers within the DAG that provide read/write access
- **Transactions**: Batch multiple block writes, handle dependencies via topological sort
- **Dirty tracking**: Only writes blocks that have changed

## Problem: TypeScript SetBlock vs Go SetBlock

**Current Issue:**

The `SetBlock` RPC currently accepts raw `bytes` data, but Go's `block.Cursor.SetBlock()` expects a `block.Block` interface implementation, not raw bytes. This creates a type mismatch.

From `core/resource/block/cursor/cursor.go:48-52`:

```go
func (r *BlockCursorResource) SetBlock(ctx context.Context, req *s4wave_block_cursor.SetBlockRequest) (*s4wave_block_cursor.SetBlockResponse, error) {
	r.cursor.SetBlock(req.GetData(), req.GetMarkDirty())  // Wrong: passes []byte, expects block.Block
	return &s4wave_block_cursor.SetBlockResponse{}, nil
}
```

From `vendor/github.com/s4wave/spacewave/db/block/cursor.go`:

```go
func (c *Cursor) SetBlock(b any, dirty bool)  // expects block.Block, not []byte
```

**Why This Matters:**

The `block.Block` interface requires implementation of `MarshalBlock()` and `UnmarshalBlock()` methods. When Go code calls `SetBlock` with a typed block, the cursor can:

1. Validate the block structure
2. Handle block references correctly
3. Apply transformations during marshaling
4. Maintain type safety throughout the DAG

## Solution: Directive-Based Block Type Resolution

We use the controller/directive pattern to resolve block types dynamically without global registries.

### Architecture

**1. BlockType Interface**

Defines how to construct and identify block types:

```go
// BlockType provides construction and identification for a block.Block type.
type BlockType interface {
	// Constructor builds a new zero-value block.Block instance.
	Constructor() block.Block

	// GetBlockTypeID returns the unique identifier for this block type.
	// Format: "github.com/s4wave/spacewave/db/block/mock.Root"
	GetBlockTypeID() string

	// MatchesBlockType checks if a block.Block is of this type.
	MatchesBlockType(b block.Block) bool
}
```

**2. LookupBlockType Directive**

Used to look up a BlockType by ID:

```go
// LookupBlockType directive value format: "block-type={id-of-block-type}"
// Example: "block-type=github.com/s4wave/spacewave/db/block/mock.Root"
type LookupBlockType interface {
	directive.Directive

	// GetBlockTypeID returns the block type ID to look up.
	GetBlockTypeID() string
}
```

**3. BlockType Controller**

Resolves LookupBlockType directives with a lookup function:

```go
// LookupBlockTypeFunc looks up a BlockType by its type ID.
// Returns the BlockType or nil if not found.
type LookupBlockTypeFunc = func(ctx context.Context, typeID string) (BlockType, error)

// BlockTypeController resolves LookupBlockType directives.
type BlockTypeController struct {
	lookupFunc LookupBlockTypeFunc
}

// NewBlockTypeController creates a controller with a lookup function.
func NewBlockTypeController(lookupFunc LookupBlockTypeFunc) *BlockTypeController {
	return &BlockTypeController{lookupFunc: lookupFunc}
}

// Resolve handles LookupBlockType directives.
func (c *BlockTypeController) Resolve(ctx context.Context, handler directive.Handler) error {
	// Match LookupBlockType directives and return BlockType values
}
```

**4. Usage in SetBlock RPC**

The SetBlock RPC uses the directive to resolve block types:

```go
func (r *BlockCursorResource) SetBlock(ctx context.Context, req *s4wave_block_cursor.SetBlockRequest) (*s4wave_block_cursor.SetBlockResponse, error) {
	blockTypeID := req.GetBlockType()
	if blockTypeID == "" {
		return nil, errors.New("block_type is required")
	}

	// Use directive to lookup BlockType
	dir := &lookupBlockTypeDirective{blockTypeID: blockTypeID}
	blockType, _, err := bus.ExecOneOff(ctx, r.bus, dir, nil)
	if err != nil {
		return nil, fmt.Errorf("unknown block type %s: %w", blockTypeID, err)
	}

	// Construct new block instance
	blk := blockType.Constructor()

	// Unmarshal data into the typed block
	if err := blk.UnmarshalBlock(req.GetData()); err != nil {
		return nil, err
	}

	// Pass typed block to cursor
	r.cursor.SetBlock(blk, req.GetMarkDirty())
	return &s4wave_block_cursor.SetBlockResponse{}, nil
}
```

### Implementation Steps

**1. Update SetBlockRequest proto** (`sdk/block/cursor/cursor.proto`):

```protobuf
message SetBlockRequest {
  // Data is the marshaled block data.
  bytes data = 1;
  // MarkDirty marks the block as dirty for persistence.
  bool mark_dirty = 2;
  // BlockType is the block type identifier.
  // Format: "github.com/s4wave/spacewave/db/block/mock.Root"
  string block_type = 3;
}
```

**2. Implement BlockType for SpaceSettings** (`core/space/world/space-settings-blocktype.go`):

```go
package block_mock

import "github.com/s4wave/spacewave/db/block"

// MockRootBlockType implements BlockType for Root.
type MockRootBlockType struct{}

func (t *MockRootBlockType) Constructor() block.Block {
	return &Root{}
}

func (t *MockRootBlockType) GetBlockTypeID() string {
	return "github.com/s4wave/spacewave/db/block/mock.Root"
}

func (t *MockRootBlockType) MatchesBlockType(b block.Block) bool {
	_, ok := b.(*Root)
	return ok
}

var _ BlockType = (*MockRootBlockType)(nil)
```

**3. Implement block.Block interface** (`vendor/github.com/s4wave/spacewave/db/block/mock/mock.go`):

```go
// MarshalBlock marshals the block to binary.
func (r *Root) MarshalBlock() ([]byte, error) {
	return r.MarshalVT()
}

// UnmarshalBlock unmarshals the block from binary.
func (r *Root) UnmarshalBlock(data []byte) error {
	return r.UnmarshalVT(data)
}

var _ block.Block = (*Root)(nil)
```

**4. Register BlockTypes at startup** (in appropriate controller initialization):

```go
// Create a lookup function that resolves block types
lookupFunc := func(ctx context.Context, typeID string) (blocktype.BlockType, error) {
	switch typeID {
	case "github.com/s4wave/spacewave/db/block/mock.Root":
		return block_mock.MockRootBlockType, nil
	case "github.com/s4wave/spacewave/db/block/mock.SubBlock":
		return block_mock.MockSubBlockType, nil
	default:
		return nil, fmt.Errorf("unknown block type: %s", typeID)
	}
}

blockTypeController := blocktype.NewBlockTypeController(lookupFunc)
bus.AddController(ctx, blockTypeController, nil)
```

**5. Update TypeScript SDK** (`sdk/block/cursor/cursor.ts`):

```typescript
async setBlock(
  req: { data: Uint8Array; markDirty: boolean; blockType: string },
  abortSignal?: AbortSignal,
): Promise<void> {
  await this.service.SetBlock(
    {
      data: req.data,
      markDirty: req.markDirty,
      blockType: req.blockType,
    },
    abortSignal,
  )
}
```

**6. Update Calling Code** (`web/app/quickstart/create.ts`):

```typescript
await cursor.setBlock(
  {
    data: MockRoot.toBinary(settings),
    markDirty: true,
    blockType: 'github.com/s4wave/spacewave/db/block/mock.Root',
  },
  abortSignal,
)
```

### Benefits

- **No global registries**: Block types registered via controller pattern
- **Modular**: Different controllers can handle different block type namespaces
- **Testable**: Mock BlockType implementations for testing
- **Type-safe**: Full type information preserved from TypeScript to Go
- **Extensible**: New block types added by registering with controller
- **Consistent**: Uses existing directive/controller patterns in the codebase

## The Pattern

```go
// AccessWorldObject => callback with cursor => UnmarshalBlock => SetBlock
_, _, err = world.AccessWorldObject(ctx, worldHandle, objKey, create, func(bcs *block.Cursor) error {
    // 1. Read existing data (returns zero value if not found)
    existing, err := UnmarshalBlock(ctx, bcs)
    if err != nil {
        return err
    }

    // 2. Compare and conditionally write
    if !existing.EqualVT(newData) {
        bcs.SetBlock(newData.CloneVT(), markDirty)
    }

    return nil
})
```

## Real Example: UpdateBlockStoreStateOp

From `core/bstore/world/update-state.go:62-71`:

```go
_, _, err = world.AccessWorldObject(ctx, worldHandle, objKey, true, func(bcs *block.Cursor) error {
    storedInfo, err := UnmarshalBlockStoreState(ctx, bcs)
    if err != nil {
        return err
    }
    if !storedInfo.EqualVT(o.GetUpdatedState()) {
        bcs.SetBlock(o.GetUpdatedState().CloneVT(), true)
    }
    return nil
})
```

**What happens:**

1. `AccessWorldObject` gets or creates the object at `objKey`
2. Provides a cursor (`bcs`) positioned at that object's block
3. `UnmarshalBlockStoreState` reads current data (or returns empty if new)
4. Compare with `EqualVT` to avoid unnecessary writes
5. `SetBlock` writes new data and marks dirty
6. On return, transaction handles merkle DAG updates

## Core Functions

### AccessWorldObject

```go
func AccessWorldObject(
    ctx context.Context,
    worldHandle WorldState,
    objKey string,
    create bool,
    cb func(*block.Cursor) error,
) (ObjectState, bool, error)
```

**Parameters:**

- `objKey`: Object identifier (e.g., `"bstore/provider-id/account-id/bstore-id"`)
- `create`: Create object if it doesn't exist
- `cb`: Callback receiving cursor at object's root block

**Returns:**

- `ObjectState`: Handle to the object
- `bool`: True if object was created
- `error`: Any error during access or callback

### block.Cursor Methods

```go
// Read current block data
func UnmarshalBlock[T Block](ctx context.Context, bcs *block.Cursor, factory func() Block) (T, error)

// Write block data
func (bcs *block.Cursor) SetBlock(b Block, markDirty bool)
```

**SetBlock parameters:**

- `b`: New block data (must implement `block.Block` interface)
- `markDirty`: Mark for persistence (typically `true` for state changes)

## Implementing block.Block

Your data structure must implement:

```go
type Block interface {
    MarshalBlock() ([]byte, error)   // Serialize to bytes
    UnmarshalBlock([]byte) error     // Deserialize from bytes
}
```

**Example** (`core/bstore/world/bstore.go:24-34`):

```go
func (i *BlockStoreState) MarshalBlock() ([]byte, error) {
    return i.MarshalVT()  // Uses vtprotobuf
}

func (i *BlockStoreState) UnmarshalBlock(data []byte) error {
    return i.UnmarshalVT(data)
}

var _ block.Block = ((*BlockStoreState)(nil))  // Type assertion
```

## Why This Pattern?

### Content-Addressed Storage

- Blocks identified by hash → automatic deduplication
- Immutable → safe concurrent access
- Verifiable → cryptographic integrity

### Efficient Updates

- Dirty tracking → only persist changed blocks
- Structural sharing → unchanged sub-trees reference old blocks
- Transaction batching → resolve dependencies once

### Merkle DAG Benefits

- Parent blocks reference child blocks by hash
- Topological sorting ensures children written before parents
- Sub-blocks can be loaded independently

## Common Operations

### Create or Update

```go
world.AccessWorldObject(ctx, w, key, true, func(bcs *block.Cursor) error {
    data, _ := UnmarshalData(ctx, bcs)
    data.Field = newValue
    bcs.SetBlock(data, true)
    return nil
})
```

### Read Only

```go
obj, err := w.GetObject(ctx, key)
data, err := UnmarshalData(ctx, obj.GetRootCursor())
```

### Conditional Update

```go
world.AccessWorldObject(ctx, w, key, false, func(bcs *block.Cursor) error {
    data, _ := UnmarshalData(ctx, bcs)
    if shouldUpdate(data) {
        bcs.SetBlock(modified, true)
    }
    return nil
})
```

## Key Takeaways

1. **Always compare before SetBlock** → avoid unnecessary merkle updates
2. **Clone data structures** → prevent aliasing issues (`CloneVT()`)
3. **Use UnmarshalBlock helpers** → handles missing blocks gracefully
4. **Mark dirty when state changes** → ensures persistence
5. **Transactions handle complexity** → focus on your update logic
6. **Pass typed blocks to SetBlock** → not raw bytes (requires refactoring)

## Current Limitation

**TypeScript → Go Type Boundary Issue:**

The current RPC implementation passes `[]byte` to `cursor.SetBlock()`, but Go's implementation expects a typed `block.Block` interface. This works accidentally in some cases because Go's `any` type accepts anything, but breaks proper block handling.

**Impact:**

- Block validation is skipped
- Block references cannot be processed
- Transformations may not apply correctly
- Type information is lost

**Solution:** Implement the Type Registry Approach above to properly unmarshal blocks on the Go side before calling `cursor.SetBlock()`.

## Related Files

- `core/bstore/world/update-state.go` - Full example implementation
- `core/bstore/world/bstore.go` - Block interface implementation
- `core/bstore/world/world.go` - Helper functions and constants
- `core/resource/block/cursor/cursor.go` - SetBlock RPC handler (needs refactoring)
- `sdk/block/cursor/cursor.proto` - SetBlock RPC definition (needs typeUrl field)
- `vendor/github.com/s4wave/spacewave/db/world/util.go` - AccessWorldObject
- `vendor/github.com/s4wave/spacewave/db/block/cursor.go` - Cursor implementation
- `vendor/github.com/s4wave/spacewave/db/block/block.go` - Block interface definition
