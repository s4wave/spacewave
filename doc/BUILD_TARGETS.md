# Build Targets Design Document

## Overview

This document describes the build target system for bldr, which enables building plugins for different deployment environments (browser, desktop, etc.) while automatically selecting the best platform for each compiler.

## Goals

1. **Simplify configuration**: Users specify a deployment target (e.g., `browser`) instead of raw platform IDs
2. **Automatic platform selection**: The build system selects the highest-priority platform each compiler supports
3. **Multi-platform bundling**: Dist bundles can include manifests from multiple compatible platforms
4. **Extensibility**: Support future targets (cloudflare-worker, docker, etc.) without architectural changes
5. **Backward compatibility**: Existing `platform_ids` configuration continues to work

## UX Goals

- `bldr build --target browser` builds all plugins for browser deployment
- `bldr build --target desktop` builds all plugins for native desktop deployment
- Users don't need to understand platform ID internals
- Clear error messages when a plugin can't be built for a target

## Current Architecture

### Platform IDs

Platform IDs identify build targets at a low level:

| Platform ID          | Description                                    |
| -------------------- | ---------------------------------------------- |
| `native/{os}/{arch}` | Native Go binary (e.g., `native/darwin/arm64`) |
| `native/js/wasm`     | Go compiled to WebAssembly for browser         |
| `native/wasi/wasm`   | Go compiled to WASI WebAssembly                |
| `js`                 | Pure JavaScript/TypeScript                     |
| `none`               | Static files only                              |

### Compilers

Two plugin compilers exist:

1. **Go compiler** (`bldr/plugin/compiler/go`)
   - Builds Go code to native binaries or WebAssembly
   - Supports: `native/*` platforms
   - Location: `plugin/compiler/go/compiler.go`

2. **JS compiler** (`bldr/plugin/compiler/js`)
   - Builds TypeScript/JavaScript plugins
   - Supports: `js` platform only
   - Location: `plugin/compiler/js/compiler.go`

### Current Build Flow

1. `BuildConfig` in `project.proto` specifies `manifests` and `platform_ids`
2. For each (manifest, platform) pair, the manifest builder controller is invoked
3. The compiler receives the platform via `ManifestMeta.platform_id`
4. Compiler checks if it supports the platform:
   - **Go compiler** (`compiler.go:177-181`): `if _, ok := buildPlatform.(*bldr_platform.NativePlatform); !ok { return nil, nil }`
   - **JS compiler** (`compiler.go:146-149`): `if buildPlatform.GetBasePlatformID() != bldr_platform.PlatformID_JS { return nil, nil }`
5. If unsupported, compiler returns `(nil, nil)` meaning "skip me for this platform"

### Current Problem

When building a dist bundle for `native/js/wasm` (browser WebAssembly):

1. The dist compiler in `dist/compiler/compiler.go:192` filters `CollectManifests` to only the single platform ID
2. This excludes `js` platform manifests even though they're compatible with the browser environment
3. Results in "waiting for not-found manifests" errors for JS plugins

## Proposed Architecture

### Build Targets

A **Target** is a named deployment environment that defines:

1. An ordered list of platform IDs (priority order)
2. Used for both **build selection** AND **runtime compatibility**

### Built-in Targets

| Target                | Platform IDs (priority order)        | Description                          |
| --------------------- | ------------------------------------ | ------------------------------------ |
| `browser`             | `native/js/wasm`, `js`               | Web browser environment              |
| `desktop`             | `native/{host-os}/{host-arch}`, `js` | Native desktop with QuickJS fallback |
| `desktop/{os}/{arch}` | `native/{os}/{arch}`, `js`           | Cross-compile for specific OS/arch   |
| `desktop/cross`       | All native platforms + `js`          | Build for all architectures          |

Future targets:

- `cloudflare-worker`: `js` (maybe `wasi` in future)
- `docker`: `native/linux/amd64`, `native/linux/arm64`

### Compiler Platform Support Interface

Add `GetSupportedPlatforms()` to the manifest builder controller interface:

```go
// In manifest/builder/builder.go
type Controller interface {
    controller.Controller

    // BuildManifest attempts to compile the manifest once.
    BuildManifest(
        ctx context.Context,
        args *BuildManifestArgs,
        host BuildManifestHost,
    ) (*BuilderResult, error)

    // GetSupportedPlatforms returns the base platform IDs this compiler supports.
    // Used by the build system to select the appropriate platform for a target.
    // Returns values like "native" or "js".
    GetSupportedPlatforms() []string
}
```

Compiler implementations:

```go
// Go compiler
func (c *Controller) GetSupportedPlatforms() []string {
    return []string{bldr_platform.PlatformID_NATIVE}
}

// JS compiler
func (c *Controller) GetSupportedPlatforms() []string {
    return []string{bldr_platform.PlatformID_JS}
}
```

### Target Resolution Flow

When building with `--target browser`:

1. Parse target to get platform list: `["native/js/wasm", "js"]`
2. For each manifest to build:
   a. Get the compiler from `ManifestConfig.builder`
   b. Call `compiler.GetSupportedPlatforms()` to get supported base platforms
   c. Iterate through target's platform list in priority order
   d. Select first platform where `GetBasePlatformID()` matches a supported platform
   e. Build manifest for that platform
3. Result: Go plugins build for `native/js/wasm`, JS plugins build for `js`

### Dist Compiler Changes

When bundling a dist for a target:

1. Get ALL platform IDs from the target (not just one)
2. Pass the full list to `CollectManifests`
3. This allows embedding both `native/js/wasm` and `js` manifests

```go
// In dist/compiler/compiler.go, line ~192
// Before:
collectedManifests, manifestErrs, err := bldr_manifest_world.CollectManifests(ctx, ws, []string{platformID}, searchKeys...)

// After:
targetPlatformIDs := target.GetPlatformIDs()
collectedManifests, manifestErrs, err := bldr_manifest_world.CollectManifests(ctx, ws, targetPlatformIDs, searchKeys...)
```

### Configuration Schema

Update `project.proto`:

```protobuf
// BuildConfig configures a build target.
message BuildConfig {
  // Manifests is the list of manifest IDs to build.
  repeated string manifests = 1;
  // PlatformIds is the list of platforms to target.
  // If targets is set, platform_ids are merged with the targets' platform lists.
  repeated string platform_ids = 2;
  // Targets is the list of deployment targets (e.g., "browser", "desktop").
  // Multiple targets can be specified to build for multiple environments.
  // Built-in targets: "browser", "desktop", "desktop/{os}/{arch}".
  repeated string targets = 3;
}
```

### Target Definition Location

Targets are defined in two places:

1. **Built-in targets**: Hardcoded in `platform/target.go`
2. **Custom targets**: Configurable in `ProjectConfig` (future enhancement)

```go
// platform/target.go

// Target represents a deployment target with prioritized platform support.
type Target struct {
    // ID is the target identifier (e.g., "browser", "desktop").
    ID string
    // PlatformIDs is the ordered list of platform IDs (highest priority first).
    PlatformIDs []string
    // Description describes the target.
    Description string
}

// BuiltinTargets contains the predefined targets.
var BuiltinTargets = map[string]*Target{
    "browser": {
        ID:          "browser",
        PlatformIDs: []string{"native/js/wasm", "js"},
        Description: "Web browser environment (WebAssembly + JavaScript)",
    },
    "desktop": {
        ID:          "desktop",
        PlatformIDs: []string{GetHostPlatformID(), "js"},
        Description: "Native desktop application with QuickJS fallback",
    },
}

// GetHostPlatformID returns the platform ID for the current host.
func GetHostPlatformID() string {
    return fmt.Sprintf("native/%s/%s", runtime.GOOS, runtime.GOARCH)
}

// ParseTarget parses a target string, supporting built-in and parameterized targets.
// Examples: "browser", "desktop", "desktop/darwin/arm64"
func ParseTarget(id string) (*Target, error) {
    // Check built-in targets first
    if target, ok := BuiltinTargets[id]; ok {
        return target, nil
    }

    // Handle parameterized targets like "desktop/darwin/arm64"
    if strings.HasPrefix(id, "desktop/") {
        suffix := strings.TrimPrefix(id, "desktop/")
        if suffix == "cross" {
            return &Target{
                ID:          id,
                PlatformIDs: GetAllNativePlatformIDs(),
                Description: "Cross-compile for all architectures",
            }, nil
        }
        // Parse as specific OS/arch
        platformID := "native/" + suffix
        if _, err := ParsePlatform(platformID); err != nil {
            return nil, err
        }
        return &Target{
            ID:          id,
            PlatformIDs: []string{platformID, "js"},
            Description: fmt.Sprintf("Desktop for %s", suffix),
        }, nil
    }

    return nil, errors.Errorf("unknown target: %s", id)
}
```

## Design Decisions

### Why Targets Instead of Just Platform Lists?

1. **User experience**: `--target browser` is clearer than `--platform-ids native/js/wasm,js`
2. **Abstraction**: Users don't need to know internal platform ID formats
3. **Maintainability**: If platform IDs change, only target definitions need updating
4. **Semantics**: Targets express intent (where to deploy), platforms express mechanics (how to build)

### Why Priority-Ordered Platform Lists?

1. **Optimal selection**: Build each plugin for the best available platform
2. **Fallback support**: If a compiler doesn't support the preferred platform, use the next best
3. **Runtime behavior**: Plugin host can use the same priority when loading

### Why Add GetSupportedPlatforms() Interface?

1. **Query without building**: Can determine support before invoking a build
2. **Target resolution**: Select the right platform before calling `BuildManifest`
3. **Error messages**: Can report "no compatible platform" before attempting build
4. **Backward compatible**: Existing `nil, nil` return behavior still works as fallback

### Why Keep platform_ids in BuildConfig?

1. **Backward compatibility**: Existing configurations continue to work
2. **Flexibility**: Power users can specify exact platforms when needed
3. **Combination**: Can use target + additional platform_ids together

## Implementation Plan

### Phase 1: Core Infrastructure - COMPLETE

1. ~~Add `GetSupportedPlatforms()` to `bldr_manifest_builder.Controller` interface~~
2. ~~Implement in Go compiler (return `["native"]`)~~
3. ~~Implement in JS compiler (return `["js"]`)~~
4. ~~Add `Target` type and built-in targets to `platform/target.go`~~

**Files changed:**

- `manifest/builder/builder.go` - Added `GetSupportedPlatforms()` to `Controller` interface
- `plugin/compiler/go/compiler.go` - Implemented `GetSupportedPlatforms()`
- `plugin/compiler/js/compiler.go` - Implemented `GetSupportedPlatforms()`
- `dist/compiler/compiler.go` - Implemented `GetSupportedPlatforms()`
- `web/pkg/compiler/compiler.go` - Implemented `GetSupportedPlatforms()`
- `web/plugin/compiler/compiler.go` - Implemented `GetSupportedPlatforms()`
- `web/bundler/esbuild/compiler/compiler.go` - Implemented `GetSupportedPlatforms()` (returns nil, sub-builder)
- `web/bundler/vite/compiler/compiler.go` - Implemented `GetSupportedPlatforms()` (returns nil, sub-builder)
- `platform/target.go` - NEW: `Target` type, built-in targets, `ParseTarget()`, `SelectPlatformForCompiler()`

### Phase 2: Build System Integration - COMPLETE

5. ~~Update `project.proto` with `target` field in `BuildConfig`~~
6. ~~Update build logic in `project/controller/build.go` to resolve targets~~
7. ~~Update dist compiler to use target's platform list for manifest collection~~

**Files changed:**

- `project/project.proto` - Added `target` field to `BuildConfig`
- `manifest/builder/builder.proto` - Added `target_platform_ids` field to `BuilderConfig`
- `project/controller/config.proto` - Added `target_platform_ids` field to `ManifestBuilderConfig`
- `project/controller/build.go` - Added `ResolveBuildConfigPlatformIDs()`, `GetBuildConfigTarget()`, `MergePlatformIDs()`, `FilterPlatformIDsByBase()`; updated `BuildTargets()` to pass target platform IDs
- `project/controller/manifest-builder.go` - Added `NewManifestBuilderConfigWithTargetPlatforms()`; updated to pass `TargetPlatformIds` to `BuilderConfig`
- `dist/compiler/compiler.go` - Updated `CollectManifests` call to use `builderConf.GetTargetPlatformIds()` instead of single platform ID

### Phase 3: CLI and UX - COMPLETE

8. ~~Add `--target` flag to build commands~~
9. ~~Add `bldr targets` command to list available targets~~
10. Update error messages for target resolution failures (not needed, ParseTarget already provides clear errors)

**Files changed:**

- `devtool/args.go` - Added `Target` field to `DevtoolArgs`, `--target` flag to build command, `BuildTargetsCommand()`
- `devtool/build.go` - Pass `Target` to `BuildTargets()`
- `devtool/targets.go` - NEW: `ListTargets()` implementation
- `project/controller/build.go` - Added `targetOverride` parameter to `BuildTargets()` and `ResolveBuildConfigPlatformIDs()`

### Phase 4: Cleanup - NOT STARTED

11. Remove `GetCompatiblePlatformIDs()` from `Platform` interface (does not exist, no action needed)
12. Update documentation
13. Add tests for target resolution

## File Changes Summary

| File                                       | Status | Change                                                       |
| ------------------------------------------ | ------ | ------------------------------------------------------------ |
| `manifest/builder/builder.go`              | DONE   | Add `GetSupportedPlatforms()` to `Controller` interface      |
| `manifest/builder/builder.proto`           | DONE   | Add `target_platform_ids` field to `BuilderConfig`           |
| `plugin/compiler/go/compiler.go`           | DONE   | Implement `GetSupportedPlatforms()` returning `["native"]`   |
| `plugin/compiler/js/compiler.go`           | DONE   | Implement `GetSupportedPlatforms()` returning `["js"]`       |
| `dist/compiler/compiler.go`                | DONE   | Implement `GetSupportedPlatforms()`; use target platforms    |
| `web/pkg/compiler/compiler.go`             | DONE   | Implement `GetSupportedPlatforms()` returning `["native"]`   |
| `web/plugin/compiler/compiler.go`          | DONE   | Implement `GetSupportedPlatforms()` returning `["native"]`   |
| `web/bundler/esbuild/compiler/compiler.go` | DONE   | Implement `GetSupportedPlatforms()` returning `nil`          |
| `web/bundler/vite/compiler/compiler.go`    | DONE   | Implement `GetSupportedPlatforms()` returning `nil`          |
| `platform/target.go`                       | DONE   | New file: `Target` type and built-in targets                 |
| `project/project.proto`                    | DONE   | Add `target` field to `BuildConfig`                          |
| `project/controller/config.proto`          | DONE   | Add `target_platform_ids` field to `ManifestBuilderConfig`   |
| `project/controller/build.go`              | DONE   | Add target resolution functions; `targetOverride` parameter  |
| `project/controller/manifest-builder.go`   | DONE   | Pass `TargetPlatformIds` to `BuilderConfig`                  |
| `devtool/args.go`                          | DONE   | Add `--target` flag, `Target` field, `BuildTargetsCommand()` |
| `devtool/build.go`                         | DONE   | Pass `Target` to `BuildTargets()`                            |
| `devtool/targets.go`                       | DONE   | New file: `ListTargets()` implementation                     |

## Testing Strategy

1. **Unit tests**: Target parsing, platform matching logic
2. **Integration tests**: Build Go plugin for browser target, build JS plugin for browser target
3. **End-to-end**: Build complete dist with mixed Go/JS plugins for browser target

## Migration Path

1. Existing `platform_ids` configurations continue to work unchanged
2. Users can gradually migrate to `target` syntax
3. No breaking changes to existing behavior
