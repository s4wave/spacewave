# Bldr Build System

## Overview

Bldr is a build system that compiles manifests (plugins, distributions, web bundles) for multiple target platforms. The build process is orchestrated through a YAML configuration file (`bldr.yaml`) and executed by various compiler controllers.

## Key Concepts

### Manifests

A manifest represents a buildable unit (plugin, distribution, web bundle). Each manifest:

- Has a unique ID (e.g., `bldr-demo`, `web`)
- Is built by a specific compiler controller
- Produces artifacts for one or more target platforms
- Contains metadata including platform ID, build type, and revision number

### Platform IDs

Platform IDs identify target execution environments:

| Platform ID          | Type             | Description                                |
| -------------------- | ---------------- | ------------------------------------------ |
| `native`             | `NativePlatform` | Host OS/arch (e.g., `native/darwin/amd64`) |
| `native/linux/amd64` | `NativePlatform` | Specific OS/arch combination               |
| `native/js/wasm`     | `NativePlatform` | WebAssembly via Go compiler                |
| `js`                 | `JsPlatform`     | Pure JavaScript (esbuild/vite output)      |

Platform parsing is handled by `platform/platform.go`:

- `"native"` prefix -> `NativePlatform` (Go binaries)
- `"js"` prefix -> `JsPlatform` (JavaScript bundles)

### Build Types

Build types control optimization and debug settings:

- `development` / `dev`: Debug builds with hot reload support
- `release`: Optimized production builds

## Configuration Structure

### bldr.yaml

```yaml
id: project-name # DNS-label format

start:
  plugins: ['plugin-a', 'plugin-b'] # Plugins loaded on startup

build:
  target-name:
    manifests: ['manifest-a', 'manifest-b']
    platformIds: ['native', 'js']

manifests:
  manifest-a:
    builder:
      id: bldr/plugin/compiler/go # Compiler controller ID
      config: { ... }
    rev: 1 # Minimum revision number

remotes:
  remote-name:
    engineId: engine-id
    objectKey: dist/path
```

### Key Data Structures

**ProjectConfig** (`project/project.proto`):

- `id`: Project identifier
- `manifests`: Map of manifest ID -> ManifestConfig
- `build`: Map of build target name -> BuildConfig
- `remotes`: Map of remote name -> RemoteConfig
- `publish`: Map of publish target name -> PublishConfig

**BuildConfig** (`project/project.proto`):

- `manifests`: List of manifest IDs to build
- `platform_ids`: List of platform IDs to target

**ManifestMeta** (`manifest/manifest.proto`):

- `manifest_id`: Unique identifier
- `build_type`: "development" or "release"
- `platform_id`: Target platform
- `rev`: Revision number (higher takes priority)

## Build Flow

### 1. Configuration Parsing

Build targets are parsed from `bldr.yaml`. Each build target specifies:

- Which manifests to build
- Which platform IDs to target

### 2. Manifest Selection

`ForManifestSelector()` in `project/controller/manifest-selector.go` creates all combinations:

```
manifests × platformIds -> [(manifest, platform), ...]
```

**Important**: Currently, ALL manifests are built for ALL specified platform IDs. There is no per-manifest platform filtering.

### 3. Builder Execution

For each (manifest, platform) combination:

1. `ManifestBuilderConfig` is created with manifest ID, build type, platform ID
2. Platform ID is resolved to fully-qualified form (e.g., `native` -> `native/darwin/amd64`)
3. Appropriate compiler controller is loaded based on manifest config
4. Compiler builds the manifest for the target platform

### 4. Platform Resolution

`ManifestMeta.Resolve()` in `manifest/manifest-meta.go`:

```go
func (m *ManifestMeta) Resolve() (*ManifestMeta, Platform, error) {
    buildPlatform, err := ParsePlatform(meta.GetPlatformId())
    meta.PlatformId = buildPlatform.GetPlatformID()  // Fully qualified
    return meta, buildPlatform, nil
}
```

## Compiler Controllers

### Go Plugin Compiler (`bldr/plugin/compiler/go`)

Location: `plugin/compiler/go/compiler.go`

Builds Go-based plugins:

- Analyzes Go packages for assets, esbuild, vite directives
- Compiles Go code to native binary or WebAssembly
- Bundles JavaScript/CSS via sub-manifests
- Generates variable definitions for asset paths

Skips non-Go platforms:

```go
if _, ok := buildPlatform.(*bldr_platform.NativePlatform); !ok {
    le.Warnf("skipping build for non-go platform: %v", buildPlatform.GetInputPlatformID())
    return nil, nil  // Returns nil result, not an error
}
```

### JavaScript Plugin Compiler (`bldr/plugin/compiler/js`)

Location: `plugin/compiler/js/compiler.go`

Builds pure JavaScript plugins via esbuild.

Only builds for JS platform:

```go
if buildPlatform.GetBasePlatformID() != bldr_platform.PlatformID_JS {
    return nil, nil  // Skip non-JS platforms
}
```

### Distribution Compiler (`bldr/dist/compiler`)

Location: `dist/compiler/compiler.go`

Builds standalone distribution bundles:

- Embeds specified manifests
- Creates static block store
- Builds entrypoint binary

### Web Plugin Compiler (`bldr/web/plugin/compiler`)

Location: `web/plugin/compiler/compiler.go`

Builds the web runtime plugin:

- Only targets native platforms (for WASM)
- Bundles web runtime JavaScript

## Waiting for Manifests

The dist compiler waits for dependent manifests to be built before proceeding.

Location: `dist/compiler/compiler.go` lines 182-226

```go
handler := world_control.NewWaitForStateHandler(func(...) (bool, error) {
    collectedManifests, _, err := bldr_manifest_world.CollectManifests(
        ctx, ws, []string{platformID}, searchKeys...)

    var notFoundManifestIDs []string
    for i, embedManifestID := range embedManifestIDs {
        if len(collectedManifests[embedManifestID]) == 0 {
            notFoundManifestIDs = append(notFoundManifestIDs, embedManifestID)
        }
    }

    if len(notFoundManifestIDs) != 0 {
        le.Infof("waiting for %d not-found manifests: %v",
            len(notFoundManifestIDs), notFoundManifestIDs)
        return true, nil  // Continue waiting
    }
    return false, nil  // Done waiting
})
```

## Platform-Specific Behavior

### Executable Extensions

Determined by `Platform.GetExecutableExt()`:

| Platform     | Extension |
| ------------ | --------- |
| Windows      | `.exe`    |
| JS/WASM      | `.mjs`    |
| WASM         | `.wasm`   |
| Other native | (none)    |

### Compiler Skipping

Compilers return `nil, nil` (no error, no result) when they cannot build for a platform:

- Go compiler skips non-native platforms
- JS compiler skips non-JS platforms
- This is by design - not all manifests support all platforms

## DistSources Embedding

TypeScript source files for the web entrypoints and SDKs are embedded in Go via `dist.go`:

```go
//go:embed web/bldr/*.ts web/bldr/*.tsx
//go:embed web/wasi-shim/*.ts
// ... other embed directives
var DistSources embed.FS
```

**Important:** When adding new TypeScript directories that need to be bundled (like `web/wasi-shim/`), you must add a corresponding `//go:embed` directive in `dist.go`. Otherwise, the esbuild bundler won't be able to resolve imports to those files.

Files are checked out to `.bldr/src/` so TypeScript and IDEs can see them during development.

## Current Limitations

### No Per-Manifest Platform Filtering

Currently, if you specify:

```yaml
build:
  web:
    manifests: ['web', 'go-plugin']
    platformIds: ['js', 'native/js/wasm']
```

The system will attempt to build both manifests for both platforms. The Go compiler will skip `js` platform builds, but this is handled at the compiler level, not configuration level.

To support per-manifest platforms, the configuration would need to change to something like:

```yaml
manifests:
  web:
    platformIds: ['js'] # Only build for JS
    builder: { ... }
  go-plugin:
    platformIds: ['native/js/wasm'] # Only build for WASM
    builder: { ... }
```

This would require modifications to:

1. `ManifestConfig` proto to add `platform_ids` field
2. `ForManifestSelector()` to filter by manifest-specific platforms
3. Build target resolution logic
