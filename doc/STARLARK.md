# Starlark Configuration

bldr supports [Starlark](https://github.com/google/starlark-go) as a
configuration language alongside YAML. Place a `bldr.star` file beside
`bldr.yaml` in the project root.

## Loading Order

1. `bldr.yaml` is loaded first (if it exists).
2. `bldr.star` is evaluated and its output is merged on top via
   `MergeProjectConfigs()`.
3. Either file can exist alone. Both are optional but at least one must be
   present.

The watcher hot-reloads both files (and any files loaded via `load()`). Edits
trigger re-evaluation with a 500ms debounce.

## Language Features

bldr enables these Starlark extensions beyond the base spec:

- `set` type
- `while` loops
- Top-level `if`/`for` (outside functions)
- Global variable reassignment
- Recursion

## Registration Built-ins

These functions mutate the project config directly. They do not return useful
values.

### project()

Sets top-level project fields.

```python
project(
    id="my-project",
    start=start_config(
        plugins=["web", "core"],
        loadWebStartup="app/startup.tsx",
    ),
    extends=["../base/bldr.yaml"],
)
```

| Kwarg | Type | Description |
|---|---|---|
| `id` | string | Project identifier |
| `start` | dict | Start configuration (use `start_config()` constructor) |
| `extends` | list[string] | Paths to parent configs to inherit from |

### manifest()

Registers a manifest (compilation unit). The `id` can be positional or keyword.

```python
manifest("my-plugin",
    builder="bldr/plugin/compiler/go",
    rev=5,
    config=go_plugin_config(
        goPkgs=["./pkg/core"],
        configSet={"entry": config_entry("my/controller", 1)},
    ),
)
```

| Arg/Kwarg | Type | Description |
|---|---|---|
| `id` | string | Manifest identifier (positional or kwarg, required) |
| `builder` | string | Builder ID (required) |
| `rev` | int | Builder cache buster revision (default 0). Maps to `ControllerConfig.Rev`, not `ManifestConfig.Rev`. Bump to force rebuild. |
| `config` | dict | Builder-specific configuration (use typed constructors below) |
| `description` | string | Optional description |

### build()

Registers a build target. Combines manifests into a runnable unit.

```python
build("app",
    manifests=["web", "core", "ui"],
    targets=["desktop"],
)
```

| Arg/Kwarg | Type | Description |
|---|---|---|
| `id` | string | Build identifier (positional or kwarg, required) |
| `manifests` | list[string] | Manifest IDs to include |
| `targets` | list[string] | Target platforms (`"desktop"`, `"browser"`) |
| `platformIds` / `platform_ids` | list[string] | Specific platform IDs |
| `manifestOverrides` / `manifest_overrides` | dict[string, dict] | Per-build-target builder config overrides keyed by manifest id |

`manifestOverrides` replaces the builder config of a named manifest when built
under this build target. The value is the inner builder-config payload (the
same shape accepted by `manifest(config=...)`), typically produced by a typed
constructor such as `dist_compiler_config(...)`. REPLACE semantics: the static
manifest builder config is not merged with the override. The override's id is
ignored; the manifest's declared builder id always wins. Common use: per-host
`embedManifests` selection for a shared `spacewave-dist` manifest.

```python
build("release-desktop-darwin-arm64",
    manifests=["spacewave-dist"],
    targets=["desktop/darwin/arm64"],
    manifestOverrides={
        "spacewave-dist": dist_compiler_config(embedManifests=[
            {"manifestId": "spacewave-launcher", "platformId": "desktop/darwin/arm64"},
            {"manifestId": "spacewave-loader", "platformId": "desktop/darwin/arm64"},
        ]),
    },
)
```

### remote()

Registers a remote configuration. Kwargs are converted to JSON and unmarshaled
into `RemoteConfig`.

```python
remote("origin",
    engineId="bifrost/link/wg",
    peerId="...",
    objectKey="...",
)
```

### publish()

Registers a publish configuration. Kwargs are converted to JSON and unmarshaled
into `PublishConfig`.

```python
publish("npm", registry="https://registry.npmjs.org")
```

## Convenience Constructors

These return Starlark dicts. Use them as values for registration built-in kwargs.

### config_entry()

Creates a configSet entry (maps to `ControllerConfig`). Accepts positional args.

```python
config_entry("my/controller", 1)
config_entry("my/controller", 1, {"field": "value"})
config_entry(id="my/controller", rev=1, config={"field": "value"})
```

| Arg/Kwarg | Type | Description |
|---|---|---|
| `id` | string | Controller ID (required) |
| `rev` | int | Revision (default 0) |
| `config` | dict | Controller-specific config |

### start_config()

Creates a start configuration dict. Keyword-only.

```python
start_config(
    plugins=["web", "core"],
    loadWebStartup="app/startup.tsx",
    disableBuild=False,
)
```

| Kwarg | Type | Description |
|---|---|---|
| `plugins` | list[string] | Plugin manifest IDs to start |
| `loadWebStartup` / `load_web_startup` | string | Web startup entrypoint path |
| `disableBuild` / `disable_build` | bool | Disable building on start |

### web_pkg()

Creates a web package reference dict. The `id` can be positional.

```python
web_pkg("@my/package")
web_pkg("@my/package", exclude=True)
web_pkg("@my/package", entrypoints=["./hooks", "./components"])
```

| Arg/Kwarg | Type | Description |
|---|---|---|
| `id` | string | npm package name (positional or kwarg, required) |
| `exclude` | bool | Exclude this package (default False) |
| `entrypoints` | list[string] | Subpath exports. Strings are auto-wrapped as `[{path: s}, ...]` |

### js_module()

Creates a JS module entry dict. First two args are positional.

```python
js_module("JS_MODULE_KIND_FRONTEND", "./web/App.tsx")
js_module("JS_MODULE_KIND_BACKEND", "./server/main.ts",
          disableEntrypoint=True, webViewParentId={"empty": True})
```

| Arg/Kwarg | Type | Description |
|---|---|---|
| `kind` | string | `"JS_MODULE_KIND_FRONTEND"` or `"JS_MODULE_KIND_BACKEND"` (required) |
| `path` | string | Module path (required) |
| `**kwargs` | any | Additional fields passed through to the dict (e.g. `disableEntrypoint`, `webViewParentId`) |

## Typed Builder Constructors

Each builder type has a constructor that validates field names. Unknown fields
cause an immediate error. All accept keyword arguments only. Both camelCase and
snake_case forms are accepted for every field.

### go_plugin_config()

For builder `bldr/plugin/compiler/go`.

```python
go_plugin_config(
    goPkgs=["./pkg/core", "./pkg/util"],
    configSet={"entry": config_entry("my/ctrl", 1)},
    hostConfigSet={"host": config_entry("host/ctrl", 1)},
    webPkgs=[web_pkg("@my/pkg")],
    buildTypes={"release": {"configSet": {...}}},
)
```

| Field | Type |
|---|---|
| `goPkgs` | list[string] |
| `configSet` | dict[string, config_entry] |
| `hostConfigSet` | dict[string, config_entry] |
| `webPkgs` | list[web_pkg] |
| `buildTypes` | dict[string, dict] |
| `platformTypes` | dict[string, dict] |
| `webPluginId` | string |
| `projectId` | string |
| `viteConfigPaths` | list[string] |
| `viteDisableProjectConfig` | bool |
| `disableRpcFetch` | bool |
| `delveAddr` | string |
| `enableCgo` | bool |
| `enableTinygo` | bool |
| `enableCompression` | bool |
| `esbuildFlags` | list[string] |

### js_plugin_config()

For builder `bldr/plugin/compiler/js`.

```python
js_plugin_config(
    webPluginId="web",
    modules=[js_module("JS_MODULE_KIND_FRONTEND", "./web/entry.ts")],
    webPkgs=[web_pkg("@my/pkg", exclude=True)],
)
```

| Field | Type |
|---|---|
| `modules` | list[js_module] |
| `esbuildBundles` | list[dict] |
| `esbuildFlags` | list[string] |
| `viteBundles` | list[dict] |
| `viteConfigPaths` | list[string] |
| `viteDisableProjectConfig` | bool |
| `backendEntrypoints` | list[string] |
| `frontendEntrypoints` | list[string] |
| `webPkgs` | list[web_pkg] |
| `hostConfigSet` | dict[string, config_entry] |
| `disableRpcFetch` | bool |
| `webPluginId` | string |
| `buildTypes` | dict[string, dict] |
| `platformTypes` | dict[string, dict] |

### cli_compiler_config()

For builder `bldr/cli/compiler`.

```python
cli_compiler_config(
    goPkgs=["./pkg/core"],
    cliPkgs=["./cmd/mycli/cli"],
    configSet={"entry": config_entry("my/ctrl", 1)},
)
```

| Field | Type |
|---|---|
| `goPkgs` | list[string] |
| `cliPkgs` | list[string] |
| `configSet` | dict[string, config_entry] |
| `projectId` | string |

### dist_compiler_config()

For builder `bldr/dist/compiler`.

```python
dist_compiler_config(
    embedManifests=[
        {"manifestId": "core", "platformId": "desktop/darwin/arm64"},
        {"manifestId": "web",  "platformId": "js"},
        {"manifestId": "app",  "platformId": "js"},
    ],
    loadPlugins=["core", "web", "app"],
    loadWebStartup="app/startup.tsx",
)
```

Each entry in `embedManifests` is a dict with `manifestId` and `platformId`.
Both fields are required and fully explicit: there is no implicit resolution
across the build target's expanded platform list. A single dist binary may
host multiple plugin runtimes, so one config can list embeds pointing at
different source platforms.

| Field | Type |
|---|---|
| `embedManifests` | list[dict{manifestId, platformId}] |
| `loadPlugins` | list[string] |
| `loadWebStartup` | string |
| `hostConfigSet` | dict[string, config_entry] |
| `projectId` | string |
| `enableCgo` | bool |
| `enableTinygo` | bool |
| `enableCompression` | bool |

### web_plugin_compiler_config()

For builder `bldr/web/plugin/compiler`.

```python
web_plugin_compiler_config(
    nativeApp={"appName": "MyApp", "windowTitle": "MyApp"},
)
```

| Field | Type |
|---|---|
| `nativeApp` | dict |
| `projectId` | string |
| `delveAddr` | string |
| `electronPkg` | string |

## Load / Import System

Use `load()` to import values from other `.star` files. Modules are cached by
resolved path.

### Relative paths

Resolved from the directory of the calling file (or project root if at top
level).

```python
load("lib/common.star", "CORE_PKGS", "shared_config")
```

### Vendored Go module paths

The `@go/` prefix resolves through the `vendor/` directory, following the same
layout as Go module vendoring.

```python
load("@go/github.com/example/repo/lib.star", "helpers")
```

All loaded files are automatically watched for hot-reload.

## Example

Complete example from the Spacewave (alpha) project:

```python
CORE_GO_PKGS = [
    "./core/resource/root/controller",
    "./core/session/controller",
    "./core/provider/local",
]

def core_config_set():
    return {
        "root-resource": config_entry("resource/root", 1),
        "session-list": config_entry("session", 1),
        "provider-local": config_entry("provider/local", 1),
    }

EXCLUDED_WEB_PKGS = [
    web_pkg("@my/web", exclude=True),
]

manifest("web",
    builder="bldr/web/plugin/compiler",
    rev=4,
    config={"nativeApp": {"appName": "MyApp"}},
)

manifest("core",
    builder="bldr/plugin/compiler/go",
    rev=12,
    config=go_plugin_config(
        goPkgs=CORE_GO_PKGS,
        configSet=core_config_set(),
    ),
)

manifest("cli",
    builder="bldr/cli/compiler",
    config=cli_compiler_config(
        goPkgs=CORE_GO_PKGS,
        cliPkgs=["./cmd/cli"],
        configSet=core_config_set(),
    ),
)

DEV_MANIFESTS = ["web", "core"]

build("app",     manifests=DEV_MANIFESTS, targets=["desktop"])
build("web",     manifests=DEV_MANIFESTS, targets=["browser"])
build("release", manifests=["web", "core", "dist"], targets=["desktop"])
build("cli",     manifests=["cli"])

project(
    id="my-project",
    start=start_config(
        plugins=["web", "core"],
        loadWebStartup="app/startup.tsx",
    ),
)
```
