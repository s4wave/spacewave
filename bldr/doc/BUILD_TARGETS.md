# Build Targets

Build targets let you specify a deployment environment (e.g., `browser`, `desktop`) instead of raw platform IDs. The build system automatically selects the best platform for each compiler based on the target's priority list.

## Quick Start

```bash
# Build all plugins for browser deployment
bldr build -b myapp -t browser

# Build for the host desktop platform
bldr build -b myapp -t desktop

# Build for a specific OS/arch
bldr build -b myapp -t desktop/linux/amd64

# Cross-compile for all desktop architectures
bldr build -b myapp -t desktop/cross

# List available targets
bldr targets
```

Or configure targets in `bldr.yaml`:

```yaml
build:
  myapp:
    manifests:
      - my-go-plugin
      - my-js-plugin
    targets:
      - browser
```

## Targets

A target is a named deployment environment with an ordered list of platform IDs. The order defines priority: the build system tries each platform in sequence and picks the first one each compiler supports.

### Built-in Targets

| Target    | Platforms (priority order)           | Description                         |
| --------- | ------------------------------------ | ----------------------------------- |
| `browser` | `native/js/wasm`, `js`               | Web browser environment             |
| `desktop` | `native/{host-os}/{host-arch}`, `js` | Native desktop for the current host |

### Parameterized Targets

| Target                | Platforms                   | Description                                |
| --------------------- | --------------------------- | ------------------------------------------ |
| `desktop/{os}/{arch}` | `native/{os}/{arch}`, `js`  | Cross-compile for a specific OS/arch       |
| `desktop/cross`       | All native platforms + `js` | Build for all common desktop architectures |

For example, `desktop/darwin/arm64` resolves to platforms `native/darwin/arm64, js`.

## Platform IDs

Platform IDs are the low-level build identifiers that targets abstract over:

| Platform ID          | Description                                    |
| -------------------- | ---------------------------------------------- |
| `native/{os}/{arch}` | Native Go binary (e.g., `native/darwin/arm64`) |
| `native/js/wasm`     | Go compiled to WebAssembly for browser         |
| `native/wasi/wasm`   | Go compiled to WASI WebAssembly                |
| `js`                 | Pure JavaScript/TypeScript                     |
| `none`               | Static files only                              |

Each platform ID has a base (e.g., `native` or `js`). Compilers declare which bases they support via `GetSupportedPlatforms()`.

## How Platform Selection Works

When you build with `--targets browser`, the system resolves the target to its platform list (`native/js/wasm`, `js`) and then for each manifest:

1. Looks up the manifest's compiler (Go or JS).
2. Queries the compiler's supported base platforms (`native` or `js`).
3. Walks the target's platform list in priority order.
4. Selects the first platform whose base matches the compiler.

Result for the `browser` target:

- Go plugins build for `native/js/wasm` (base `native` matches).
- JS plugins build for `js` (base `js` matches).

This means a single target can drive builds for mixed Go/JS plugin sets, with each plugin built for its best available platform.

## Multi-Platform Dist Bundles

When building a dist bundle for a target, the dist compiler searches for manifests across all of the target's platforms. This allows a single dist to include both `native/js/wasm` manifests (from Go plugins) and `js` manifests (from JS plugins), which was not possible when dist bundles filtered by a single platform ID.

## Configuration

### `BuildConfig` in `project.proto`

```protobuf
message BuildConfig {
  repeated string manifests = 1;
  repeated string platform_ids = 2;
  repeated string targets = 3;
}
```

- **`targets`**: Deployment targets to build for. Each target expands to its platform list.
- **`platform_ids`**: Explicit platform IDs. Merged with target-derived platforms (target platforms take priority in ordering).

Both fields can be used together. Explicit `platform_ids` are appended after target-derived platforms and deduplicated.

### CLI Override

The `--targets` (`-t`) flag overrides the config's `targets` field entirely. When an override is specified, the config's `targets` field is ignored but `platform_ids` are still merged.

```bash
# Override targets, ignoring config's targets field
bldr build -b myapp -t browser

# Comma-separated for multiple targets
bldr build -b myapp -t browser,desktop
```

## Compiler Interface

Compilers implement `GetSupportedPlatforms()` on the manifest builder `Controller` interface:

```go
type Controller interface {
    controller.Controller
    BuildManifest(ctx context.Context, args *BuildManifestArgs, host BuildManifestHost) (*BuilderResult, error)
    GetSupportedPlatforms() []string
}
```

Returns base platform IDs like `"native"` or `"js"`. Returns `nil` for meta-compilers (e.g., bundlers) that delegate to sub-builders.

| Compiler                                         | Supported Platforms |
| ------------------------------------------------ | ------------------- |
| Go (`plugin/compiler/go`)                        | `native`            |
| JS (`plugin/compiler/js`)                        | `js`                |
| Dist (`dist/compiler`)                           | `native`            |
| Web package (`web/pkg/compiler`)                 | `native`            |
| Web plugin (`web/plugin/compiler`)               | `native`            |
| Esbuild bundler (`web/bundler/esbuild/compiler`) | `nil` (delegates)   |
| Vite bundler (`web/bundler/vite/compiler`)       | `nil` (delegates)   |

## Key Source Files

| File                                     | Purpose                                                                              |
| ---------------------------------------- | ------------------------------------------------------------------------------------ |
| `platform/target.go`                     | `Target` type, built-in targets, `ParseTarget()`, `SelectPlatformForCompiler()`      |
| `project/project.proto`                  | `BuildConfig` with `targets` field                                                   |
| `project/controller/build.go`            | `ResolveBuildConfigPlatformIDs()`, `MergePlatformIDs()`, `FilterPlatformIDsByBase()` |
| `project/controller/manifest-builder.go` | Passes `TargetPlatformIds` to builder config                                         |
| `manifest/builder/builder.go`            | `GetSupportedPlatforms()` on `Controller` interface                                  |
| `dist/compiler/compiler.go`              | Uses target platform IDs for multi-platform manifest collection                      |
| `devtool/args.go`                        | `--targets` CLI flag                                                                 |
| `devtool/targets.go`                     | `bldr targets` list command                                                          |
