# Spacewave Architecture Overview

This document provides a high-level explanation of the Spacewave architecture, including its plugin system, resource management, testing infrastructure, and end-to-end data flow.

## Table of Contents

1. [Overview](#overview)
2. [Core Concepts](#core-concepts)
3. [Plugin System Architecture](#plugin-system-architecture)
4. [Resources SDK Pattern](#resources-sdk-pattern)
5. [World State System](#world-state-system)
6. [Testing Infrastructure](#testing-infrastructure)
7. [End-to-End Data Flow](#end-to-end-data-flow)

## Overview

Spacewave is a distributed application built on a plugin-based architecture with bidirectional TypeScript/Go RPC communication. The system enables:

- **Cross-language communication**: Go backend services exposed to TypeScript/JavaScript frontends
- **Resource lifecycle management**: Automatic cleanup of resources with parent-child relationships
- **Transactional world state**: A graph-based data structure with ACID properties
- **Plugin isolation**: Each plugin runs in its own process/sandbox with controlled communication
- **End-to-end testing**: TypeScript tests that verify the complete stack from Go to JavaScript

## Core Concepts

### Plugins

Spacewave uses a plugin-based architecture managed by `bldr` (the build system). There are three main plugins defined in `bldr.yaml`:

1. **web**: Provides bindings to Electron/Tauri/Browser environments
2. **spacewave-core**: The Go-based backend that implements core logic (world, space, accounts, sessions, resources)
3. **spacewave-app**: The TypeScript/React frontend application

Each plugin is built, loaded, and managed by the `bldr` plugin system, which handles:

- Compilation (Go binaries, JavaScript bundles)
- Plugin host processes (native Go processes, QuickJS/WASM sandboxes)
- Inter-plugin RPC communication via SRPC (StarRPC)
- Plugin lifecycle and configuration

### ControllerBus

The ControllerBus is Spacewave's dependency injection and service orchestration system. Controllers:

- Implement specific functionality (sessions, providers, storage, etc.)
- Resolve directives (queries/commands) using a resolver pattern
- Run in the background with lifecycle management
- Can depend on other controllers via directives

Example: A session controller resolves `LookupSession` directives by providing session instances.

### Resources

Resources represent RPC-accessible objects with automatic lifecycle management:

- Each resource has a unique ID assigned by the server
- Client references are tracked with reference counting
- Resources can create child resources (forming a resource tree)
- When a parent resource is released, all children are automatically cleaned up
- Both Go and TypeScript have resource abstractions that implement disposable patterns

### World State

The World State is a transactional graph database built on Hydra's storage layer:

- **Objects**: Nodes with unique keys and metadata (type, revision, root references)
- **Quads**: RDF-style subject-predicate-object-graph relationships
- **Transactions**: Read/write transactions with MVCC semantics
- **Sequence numbers**: Monotonic counters tracking state changes
- **Reactive watching**: Server-side change tracking with automatic client notifications

## Plugin System Architecture

### Build and Compilation Flow

```
bldr.yaml
    ↓
Project Controller (Go)
    ↓
    ├─→ Plugin Compiler (Go)      → Compiles Go packages into plugin binaries
    ├─→ Plugin Compiler (JS)      → Bundles TypeScript/React into JS modules
    └─→ Web Bundler (Vite)        → Produces optimized web assets
         ↓
Plugin Host Processes
    ├─→ Process Host              → Runs native Go plugins
    └─→ QuickJS/WASM Host         → Runs sandboxed JavaScript plugins
         ↓
RPC Communication (SRPC/StarRPC)
```

### Runtime Communication

Plugins communicate via SRPC (StarRPC), a bidirectional streaming RPC protocol:

```
TypeScript Plugin                Go Plugin
    (spacewave-app)                 (spacewave-core)
         ↓                               ↓
    SRPC Client  ←──────RPC──────→  SRPC Server
         ↓                               ↓
  Resource Client                 Resource Server
         ↓                               ↓
   SDK Wrappers                    Go Implementations
  (Root, Session,                  (Controllers,
   Space, World)                    Directives)
```

The web plugin can invoke Go backend services, and Go can push updates back to TypeScript clients.

## Resources SDK Pattern

The Resources SDK bridges Go interfaces to TypeScript clients over RPC with automatic lifecycle tracking.

### Four-Step Pattern

1. **Define Protocol Buffers**: Create `.proto` files mirroring Go interfaces
2. **Implement SDK Wrappers**: Create Go/TypeScript clients in `./sdk/` wrapping protobuf services
3. **Conform to Go Interfaces**: Make Go SDK clients implement original interfaces for drop-in compatibility
4. **Write Tests**: Test using resources testbed in `core/resource/testbed/`

### Resource Lifecycle

```
Client (TypeScript)
    ↓
1. client.accessRootResource()
    ↓
2. ResourceClient RPC stream established
    ↓
3. Server assigns clientHandleId and rootResourceId
    ↓
4. Client creates ClientResourceRef wrapper
    ↓
5. Client calls root.createSpace()
    ↓
6. Server creates Space resource, returns resource_id
    ↓
7. Client receives resource_id, wraps in Space instance
    ↓
8. Client calls space.release() (or `using` scope ends)
    ↓
9. Reference count decrements to zero
    ↓
10. Client sends ResourceRefRelease RPC
    ↓
11. Server invokes cleanup callback, releases controller
    ↓
12. Child resources automatically released (cascade)
```

### Resource Server Architecture

**Server-side** (`core/resource/server/server.go`):

- `ResourceServer`: Manages client connections and resource tracking
- `RemoteResourceClient`: Tracks resources per client session
- `trackedResource`: Individual resource with mux (service router) and release callback
- `ResourceRpc`: Component-based RPC routing (resource_id → service methods)
- Automatic cleanup when clients disconnect

**Client-side** (`sdk/resource/client.ts`):

- `Client`: Manages connection lifecycle and resource references
- `ClientResourceRef`: Reference to a remote resource with disposal support
- Reference counting: Multiple refs to same resource_id only notify server once all are released
- Automatic reconnection with retry logic
- TypeScript `using` statement support for automatic cleanup

### Example: Creating a World Engine

**Protocol Buffer** (`sdk/testbed/testbed.proto`):

```protobuf
service TestbedResourceService {
  rpc CreateWorld(CreateWorldRequest) returns (CreateWorldResponse);
}

message CreateWorldResponse {
  uint32 resource_id = 1;  // ID of the created Engine resource
}
```

**Go Implementation** (`core/resource/testbed/root.go`):

```go
func (s *TestbedResourceServer) CreateWorld(ctx context.Context, req *CreateWorldRequest) (*CreateWorldResponse, error) {
    resourceCtx, _ := resource_server.MustGetResourceClientContext(ctx)

    // Create world engine
    busEngine := world.NewBusEngine(s.ctx, s.bus, engineID)
    engineResource := resource_world.NewEngineResource(le, bus, busEngine, nil, engineInfo)

    // Add to client's resource tree
    id, _ := resourceCtx.AddResource(engineResource.GetMux(), releaseFunc)

    return &CreateWorldResponse{ResourceId: id}, nil
}
```

**TypeScript SDK** (`sdk/testbed/testbed.ts`):

```typescript
export class TestbedRoot extends Resource {
  public async createWorld(
    engineId?: string,
    abortSignal?: AbortSignal,
  ): Promise<Engine> {
    const resp = await this.service.CreateWorld({ engineId }, abortSignal)
    // Convert resource_id to Engine instance with automatic cleanup
    return this.resourceRef.createResource(resp.resourceId ?? 0, Engine)
  }
}
```

**TypeScript Usage**:

```typescript
using testbedRoot = new TestbedRoot(rootRef)
using engine = await testbedRoot.createWorld('test-engine')
using tx = await engine.newTransaction(true)
// All resources automatically released when scope exits
```

## World State System

The World State is a transactional graph database with reactive tracking capabilities.

### Architecture

```
Engine
  ↓
Transaction (Tx)
  ↓
WorldState (read/write operations)
  ↓
ObjectState (individual objects)
  ↓
Block Storage (Hydra)
```

### Key Components

**Engine** (`sdk/world/engine.ts`, `hydra/world/block/engine`):

- Top-level resource for a world database instance
- Creates transactions (read-only or read-write)
- Tracks sequence numbers (seqno) for change detection
- Provides storage cursor access
- Implements reactive watching via `WatchWorldState`

**Transaction** (`sdk/world/tx.ts`):

- Short-lived read or write transaction
- Implements `WorldState` interface (create/get/delete objects)
- Read transactions can see current committed state
- Write transactions accumulate changes, commit atomically
- Must be explicitly committed (write) or can be discarded (read)

**WorldState** (`sdk/world/world-state.go`, `sdk/world/world-state.ts`):

- Interface for accessing and modifying world objects
- Operations: `CreateObject`, `GetObject`, `DeleteObject`
- Graph operations: `SetGraphQuad`, `GetGraphQuad`, `DeleteGraphQuad`
- Implemented by both `Tx` and `EngineWorldState` (wrapper for read-only access)

**ObjectState** (`sdk/world/object-state.ts`):

- Represents a single object in the world
- Has a key, revision number, and root reference (content pointer)
- Operations: `GetKey`, `GetRootRef`, `IncrementRev`, `SetRootRef`
- Resource that can be tracked and cleaned up

### Transaction Pattern

```typescript
import { keyToIRI, predToIRI } from '@s4wave/sdk/world/graph-utils.js'

// Create a write transaction
using tx = await engine.newTransaction(true)

// Create an object
const objectKey = 'my-object-key'
using obj = await tx.createObject(objectKey, {})

// Modify the object
await obj.incrementRev()
const [rootRef, rev] = await obj.getRootRef()

// Set graph relationships (RDF quads)
// Note: Use keyToIRI() to wrap keys in IRI format: <key>
await tx.setGraphQuad(
  keyToIRI(objectKey),
  predToIRI('rdf:type'),
  keyToIRI('MyType'),
  '',
)

// Commit the transaction (all changes atomic)
await tx.commit()

// Get the new sequence number
const newSeqno = await engine.getSeqno()

// Wait for seqno to be reached (useful for synchronization)
await engine.waitSeqno(newSeqno)
```

### Reactive Watching

The `WatchWorldState` feature enables automatic re-execution when tracked resources change:

```typescript
const stopWatching = engine.watchWorldState(
  async (worldState, abortSignal, cleanup) => {
    // Access world state - server tracks which resources are accessed
    const obj = cleanup(await worldState.getObject('watched-key'))

    // When 'watched-key' changes, this callback re-executes automatically
    console.log('Object changed:', obj)
  },
)

// Stop watching
stopWatching()
```

**How it works**:

1. Client calls `WatchWorldState` RPC (streaming)
2. Server creates tracked `WorldState` resource, returns `resource_id`
3. Client callback executes, accesses resources via `WorldState`
4. Server records which objects/quads were accessed
5. When tracked resources change, server detects and sends new `resource_id`
6. Client re-executes callback with fresh `WorldState` snapshot
7. Loop continues until client cancels watch

## Testing Infrastructure

Spacewave has a end to end test infrastructure that validates the stack.

### Testbed Architecture

The testbed is a minimal environment for testing the Resources SDK and plugin system:

```
Go Test Process
    ↓
Testbed Setup
    ├─→ ControllerBus (dependency injection)
    ├─→ Volume/Storage (Hydra block storage)
    ├─→ Plugin Hosts (Go process, QuickJS/WASM)
    ├─→ TestbedResourceServer (root resource for tests)
    └─→ ResourceServer (wraps testbed root)
         ↓
Plugin Loading
    ├─→ Project Controller (loads bldr.yaml config)
    ├─→ Manifest Builders (compile plugins)
    └─→ Plugin Execution (starts spacewave-core and spacewave-app)
         ↓
TypeScript Test Execution
    ↓
Test Result Reporting (via TestbedRoot.markTestResult)
    ↓
Go Test Verification
```

### Key Components

**TestbedResourceServer** (`core/resource/testbed/root.go`):

- Root resource for creating test world engines
- `CreateWorld`: Creates isolated world engine instances for testing
- `MarkTestResult`: Allows TypeScript tests to report success/failure
- `WaitForTestResult`: Blocks Go test until TypeScript test completes
- Uses broadcast/signaling to coordinate between Go and TypeScript

**TestbedRoot** (`sdk/testbed/testbed.ts`):

- TypeScript SDK wrapper for testbed operations
- `createWorld`: Creates Engine resources for testing
- `markTestResult`: Reports test completion to Go test harness

**Setup Helpers** (`core/resource/testbed/testutil.go`):

- `SetupTestbedWithClient`: One-liner setup for resource tests
- Creates testbed, starts resource server, returns client
- Provides cleanup function for automatic teardown

### Test Flow

#### 1. Go Unit Tests (`core/resource/testbed/testbed_test.go`)

These tests verify the Resources SDK directly from Go:

```go
func TestTestbedResourceServerViaSDK(t *testing.T) {
    ctx := context.Background()
    _, resClient, cleanup := resource_testbed.SetupTestbedWithClient(ctx, t)
    defer cleanup()

    // Access root resource
    rootRef := resClient.AccessRootResource()
    defer rootRef.Release()

    // Create world engine
    testbedClient := s4wave_testbed.NewSRPCTestbedResourceServiceClient(...)
    resp, _ := testbedClient.CreateWorld(ctx, &CreateWorldRequest{})

    // Wrap in SDK
    engineRef := resClient.CreateResourceReference(resp.ResourceId)
    engine, _ := s4wave_world.NewEngine(resClient, engineRef)
    defer engine.Release()

    // Perform operations
    tx, err := engine.NewTransaction(ctx, true)
    defer tx.Discard() // ensure cleanup

    obj, err := tx.CreateObject(ctx, "test-obj", nil)
    err := tx.Commit(ctx)
}
```

#### 2. End-to-End Tests (`core/e2e/e2e_test.go`)

These tests start the entire plugin system and run TypeScript tests:

```go
func TestSpacewaveCoreE2E(t *testing.T) {
    // Setup testbed with plugin system
    tb, _ := testbed.BuildTestbed(ctx, le)

    // Add controllers
    sr.AddFactory(plugin_host_process.NewFactory(b))
    sr.AddFactory(bldr_project_controller.NewFactory(b))
    // ... other factories

    // Create testbed resource server
    testbedResourceServer := resource_testbed.NewTestbedResourceServer(...)

    // Register with mux
    rootResourceMux := srpc.NewMux()
    testbedResourceServer.Register(rootResourceMux)

    // Start project controller (compiles and loads plugins)
    projCtrl, _, projCtrlRef, _ := loader.WaitExecControllerRunningTyped[...](
        ctx, tb.GetBus(), projCtrlConf, nil,
    )

    // Wait for TypeScript test to complete
    success, errorMsg, _ := testbedResourceServer.WaitForTestResult(ctx)
    if !success {
        t.Fatalf("test failed: %s", errorMsg)
    }
}
```

#### 3. TypeScript E2E Tests (`core/e2e/e2e.ts`)

The TypeScript test runs inside the plugin system:

```typescript
export default async function main(backendAPI: BackendAPI, abortSignal: AbortSignal) {
    let testbedRoot: TestbedRoot | undefined

    try {
        // Connect to testbed resource server
        const testbedResourcesClient = new ResourcesClient(...)
        using testbedRootRef = await testbedResourcesClient.accessRootResource()
        testbedRoot = new TestbedRoot(testbedRootRef)

        // Connect to spacewave-core plugin
        const corePluginClient = new SRPCClient(...)
        const resourcesClient = new ResourcesClient(...)
        using rootResource = new Root(await resourcesClient.accessRootResource())

        // Run test cases
        await testProvider(rootResource)
        await testHashFunctions(rootResource, abortSignal)
        await testQuickstartFlow(rootResource, abortSignal)

        // Mark test as successful
        await testbedRoot.markTestResult(true)
    } catch (error) {
        // Mark test as failed
        if (testbedRoot) {
            await testbedRoot.markTestResult(false, errorMessage)
        }
        throw error
    }
}
```

#### 4. Testbed-specific E2E Tests (`core/resource/testbed/testbed-e2e.ts`)

Simpler tests that verify Resources SDK from TypeScript:

```typescript
export default async function main(
  backendAPI: BackendAPI,
  abortSignal: AbortSignal,
  testbedRoot: TestbedRoot,
) {
  // Create a world engine
  using engine = await testbedRoot.createWorld('test-engine-ts')

  // Wrap in EngineWorldState
  const worldState = new EngineWorldState(engine, true)

  // Create an object to signal completion
  using _objState = await worldState.createObject('e2e-test-completed', {})

  // Verify we can get it back
  using objState2 = await worldState.getObject('e2e-test-completed')
  if (!objState2) {
    throw new Error('failed to retrieve completion marker object')
  }
}
```

### Test Patterns

**Resource Cleanup Testing**:

```typescript
// Verify resources are cleaned up when released
using engineRef = await rootRef.createWorld()
const stream = await engineClient.WatchWorldState(...)
const msg = await stream.Recv()

// Release the engine
engineRef.release()

// Next recv should fail (resource cleaned up)
const msg2 = await stream.Recv()  // Throws error
```

**Parent-Child Resource Relationships**:

```typescript
// Child resources automatically released when parent is released
using engine = await testbedRoot.createWorld()
using tx = await engine.newTransaction(true)
using obj = await tx.createObject('key', {})

// Releasing engine releases tx and obj automatically
```

**SDK Wrapper vs Raw RPC**:

- Tests verify both raw protobuf RPC calls and SDK wrapper methods
- Ensures SDK correctly converts `resource_id` to Resource instances
- Validates that metadata is properly passed through

## End-to-End Data Flow

Let's trace a complete example: Creating a space with a demo drive.

### 1. Plugin Startup

```
User starts app
    ↓
Web plugin loads (Electron/Browser)
    ↓
Web plugin requests spacewave-core plugin
    ↓
bldr compiles Go packages into spacewave-core binary
    ↓
Plugin host spawns spacewave-core process
    ↓
spacewave-core controllers start (root resource, session, provider, space, etc.)
    ↓
Web plugin connects to spacewave-core via SRPC
```

### 2. Resource Access

```typescript
// Frontend: web/app/App.tsx
const rootResource = useResource(async (signal, cleanup) => {
  const ref = await client.accessRootResource()
  return cleanup(new Root(ref)) // Register and return for cleanup
})

// Root resource established, all other resources chain from this
```

### 3. Session Creation

```typescript
// Create local provider account
const accountResp = await rootResource.createLocalProviderAccount({}, signal)

// Mount session
const session = await rootResource.mountSession(
  { sessionRef: accountResp.sessionListEntry?.sessionRef },
  signal,
)
```

**What happens**:

1. TypeScript calls `rootResource.createLocalProviderAccount()`
2. SRPC sends request to spacewave-core `RootResourceService.CreateLocalProviderAccount`
3. Go resolves `provider/local` directive on ControllerBus
4. Local provider controller creates account, stores private key in volume
5. Returns session reference
6. TypeScript calls `mountSession` with session reference
7. Go resolves `session` directive, creates Session resource
8. Returns `resource_id` for Session
9. TypeScript wraps in `Session` SDK instance

### 4. Space Creation

```typescript
// Create space
const spaceResp = await session.createSpace({ spaceName: 'My Space' }, signal)

// Mount space
const space = await session.mountSpace(
  { spaceRef: spaceResp.spaceListEntry?.spaceRef },
  signal,
)

// Access world state
const spaceWorld = await space.accessWorldState(true, signal)
```

**What happens**:

1. Session creates space (shared object in Hydra storage)
2. Returns space reference (object key + metadata)
3. TypeScript mounts space, gets `Space` resource
4. TypeScript accesses world state, gets `WorldState` resource
5. All resources tracked in resource tree

### 5. World Operations

```typescript
import { keyToIRI, predToIRI } from '@s4wave/sdk/world/graph-utils.js'

// Create transaction
using tx = await engine.newTransaction(true)

// Create settings object
const settingsKey = 'space-settings'
using settingsObj = await tx.createObject(settingsKey, {})

// Set type quad
// Note: Use keyToIRI() and predToIRI() to wrap strings in IRI format
await tx.setGraphQuad(
  keyToIRI(settingsKey),
  predToIRI('rdf:type'),
  keyToIRI('SpaceSettings'),
  '',
)

// Set property (value can be a literal string or another IRI)
await tx.setGraphQuad(
  keyToIRI(settingsKey),
  predToIRI('settings:name'),
  `"${spaceName}"`, // Literal value (quoted string)
  '',
)

// Link to another object
await tx.setGraphQuad(
  keyToIRI(settingsKey),
  predToIRI('settings:owner'),
  keyToIRI(ownerKey), // Reference to another object
  '',
)

// Commit transaction
await tx.commit()
```

**What happens**:

1. Transaction created on world engine
2. TypeScript calls `createObject` → SRPC → Go creates object in transaction
3. TypeScript sets graph quads → Go validates and adds to transaction
4. TypeScript commits → Go atomically writes to block storage
5. Sequence number increments
6. Watchers notified of changes
7. Transaction resource released

### 6. Reactive Updates

```typescript
// In React component
const spaceResource = useResource(
  sharedObjectBodyResource,
  async (parentSharedObjectBody) =>
    parentSharedObjectBody ?
      new Space(parentSharedObjectBody.resourceRef)
    : null,
  [], // deps array is required
)

// Automatically re-renders when space changes
```

**What happens**:

1. Component subscribes to space shared object
2. Backend watches for changes to space object
3. When space object changes (e.g., new metadata), backend notifies client
4. Client invalidates cache, re-fetches
5. React re-renders with new data

### 7. Cleanup

```typescript
// Component unmounts or user navigates away
// All resources disposed via useResource cleanup

// Or explicitly:
space.release() // Releases Space resource
session.release() // Releases Session resource
root.release() // Releases Root resource (on app exit)
```

**What happens**:

1. TypeScript calls `release()` on resources
2. Reference count decrements
3. When count reaches zero, sends `ResourceRefRelease` RPC to server
4. Server calls release callback
5. Go controllers release references
6. Child resources automatically cascade release
7. Storage connections closed, memory freed

## Summary

Spacewave's architecture demonstrates several key patterns:

1. **Plugin-based modularity**: Clear separation between web, core, and application layers
2. **Cross-language RPC**: Seamless Go ↔ TypeScript communication via SRPC
3. **Resource lifecycle management**: Automatic cleanup with parent-child relationships
4. **Transactional graph database**: ACID properties with reactive change tracking
5. **Testability**: End-to-end tests that validate the full stack
6. **Type safety**: Protocol buffers ensure type safety across language boundaries

The system is designed for:

- **Extensibility**: New plugins can be added without modifying core
- **Reliability**: Automatic resource cleanup prevents leaks
- **Performance**: Streaming RPC, MVCC transactions, efficient storage
- **Developer experience**: Strong typing, clear patterns, comprehensive testing

This architecture enables building complex distributed applications with the simplicity of a monolithic system, while maintaining the flexibility and performance of a microservices architecture.
