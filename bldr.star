# bldr.star - Spacewave project configuration
#
# Higher-order config using Starlark. Evaluated at runtime alongside bldr.yaml.
# Star config merges on top of YAML via MergeProjectConfigs().

# Shared Go packages for the core runtime
CORE_GO_PKGS = [
    "./core/resource/root/controller",
    "./core/resource/listener",
    "./core/session/controller",
    "./core/provider/local",
    "./core/provider/spacewave",
    "./core/space/sobject",
    "./core/space/world/ops",
    "./core/plugin/space",
    "./core/space/http/download",
    "./core/space/http/export",
    "./db/blocktype/controller-factory",
    "github.com/s4wave/spacewave/db/object/peer",
]

# Shared encryption key for peer object store
PEER_ENCRYPTION_KEY = "KY8Lo3c7L+bXa8BFZcU/YFfHysRdl4aZqmDd9TeZ+p4="

# Core configSet shared between Go plugin and CLI manifests.
def core_config_set(listener_path="git:.spacewave/spacewave.sock"):
    return {
        "store-peer": config_entry("object/peer", 1, {
            "objectStoreId": "s4wave-peer",
            "volumeId": "plugin-host",
            "transformConf": {
                "steps": [{
                    "id": "hydra/transform/blockenc",
                    "config": {
                        "blockEnc": "BlockEnc_XCHACHA20_POLY1305",
                        "key": PEER_ENCRYPTION_KEY,
                    },
                }],
            },
        }),
        "root-resource": config_entry("resource/root", 1),
        "session-list": config_entry("session", 1),
        "provider-local": config_entry("provider/local", 1),
        "provider-spacewave": config_entry("provider/spacewave", 2, {
            "endpoint": "https://spacewave.app",
            "signingEnvPrefix": "spacewave",
        }),
        "space-sobject": config_entry("space/sobject", 1, {"verbose": False}),
        "space-world-ops": config_entry("space/world/ops", 1),
        "blocktype": config_entry("db/blocktype", 1),
        "download": config_entry("space/http/download", 1),
        "export": config_entry("space/http/export", 1),
        "resource-listener": config_entry("resource/listener", 1, {
            "listenerSocketPath": listener_path,
        }),
    }

# Web packages excluded by JS plugins that consume spacewave-web packages.
EXCLUDED_WEB_PKGS = [
    web_pkg("@s4wave/web", exclude=True),
    web_pkg("@fontsource-variable/manrope", exclude=True),
    web_pkg("@fontsource/commit-mono", exclude=True),
    web_pkg("sonner", exclude=True),
]

WEB_STARTUP = "app/prerender/startup.tsx"

# -- Manifests --

manifest("web",
    builder="bldr/web/plugin/compiler",
    rev=5,
    config={
        "nativeApp": {
            "appName": "Spacewave",
            "windowTitle": "Spacewave",
            "themeSource": "dark",
            "iconPath": "web/images/spacewave-icon.png",
        },
    },
)

# spacewave-launcher is the minimal embedded plugin that drives the launcher
# binary. It carries just enough to fetch a DistConfig, apply its
# launcher_config_set, mount the remote world over HTTP, and resolve plugin
# manifests from it. Everything else (spacewave-core, UI plugins) loads from
# the remote world on first launch.
manifest("spacewave-launcher",
    builder="bldr/plugin/compiler/go",
    rev=1,
    config={
        "goPkgs": [
            "./core/provider/spacewave/launcher/controller",
            "github.com/s4wave/spacewave/bldr/manifest/fetch/world",
            "github.com/s4wave/spacewave/db/block/store/kvfile/http",
            "github.com/s4wave/spacewave/db/block/store/overlay",
            "github.com/s4wave/spacewave/db/block/store/rpc/server",
            "github.com/s4wave/spacewave/db/world/block/engine",
            "github.com/s4wave/spacewave/db/object/peer",
        ],
        "configSet": {
            "spacewave-launcher": config_entry(
                "spacewave/launcher/controller", 1,
                {
                    "projectId": "spacewave",
                    # DistConfig endpoints come from BuildTimeDistConfigEndpoints
                    # in the launcher controller (build-tag-selected: prod vs
                    # staging Worker). Leaving Config.Endpoints empty means
                    # every binary hits its own environment's Worker.
                    "refetchDur": "1h",
                },
            ),
            "store-peer": config_entry("object/peer", 1, {
                "objectStoreId": "s4wave-peer",
                "volumeId": "plugin-host",
                "transformConf": {
                    "steps": [{
                        "id": "hydra/transform/blockenc",
                        "config": {
                            "blockEnc": "BlockEnc_XCHACHA20_POLY1305",
                            "key": PEER_ENCRYPTION_KEY,
                        },
                    }],
                },
            }),
        },
    },
)

# spacewave-loader spawns the cross-platform loading-UI helper during plugin
# bootstrap and terminates it when the controller context ends. Embedded
# alongside spacewave-launcher so the loading window appears immediately on
# cold start, covering the remote-world fetch phase. The controller watches
# LoadPlugin directive state for the listed plugin ids and forwards progress
# to the helper over pipesock.
manifest("spacewave-loader",
    builder="bldr/plugin/compiler/go",
    rev=2,
    config={
        "goPkgs": [
            "./core/provider/spacewave/loader/controller",
        ],
        "configSet": {
            "spacewave-loader": config_entry(
                "spacewave/loader/controller", 1,
                {
                    "projectId": "spacewave",
                    "watchPluginIds": [
                        "spacewave-core",
                        "spacewave-web",
                        "spacewave-app",
                        "web",
                    ],
                },
            ),
        },
    },
)

manifest("spacewave-core",
    builder="bldr/plugin/compiler/go",
    rev=10,
    config={
        "goPkgs": CORE_GO_PKGS,
        "configSet": core_config_set(),
        "buildTypes": {
            "dev": {
                "goPkgs": ["./core/debug/trace"],
                "configSet": {
                    "debug-trace": config_entry("debug/trace", 1),
                },
            },
            "release": {
                "configSet": {
                    "resource-listener": config_entry("resource/listener", 1, {
                        "listenerSocketPath": "~/.spacewave/spacewave.sock",
                    }),
                },
            },
        },
        "hostConfigSet": {
            "fetch-manifest-via-spacewave-core": config_entry(
                "bldr/manifest/fetch/plugin", 1,
                {"pluginId": "spacewave-core"},
            ),
        },
    },
)

manifest("spacewave-debug",
    builder="bldr/plugin/compiler/go",
    rev=2,
    config={
        "webPluginId": "web",
        "goPkgs": ["./core/debug/bridge"],
        "configSet": {
            "debug-bridge": config_entry("debug/bridge", 1),
        },
    },
)

manifest("spacewave",
    builder="bldr/cli/compiler",
    config={
        "goPkgs": CORE_GO_PKGS,
        "cliPkgs": ["./cmd/spacewave-cli/cli"],
        "configSet": core_config_set(),
        "projectId": "spacewave",
    },
)

manifest("spacewave-cli",
    builder="bldr/cli/compiler",
    config={
        "goPkgs": CORE_GO_PKGS,
        "cliPkgs": ["./cmd/spacewave-cli/cli"],
        "configSet": core_config_set(),
        "projectId": "spacewave",
    },
)

manifest("spacewave-web",
    builder="bldr/plugin/compiler/js",
    rev=11,
    config={
        "webPluginId": "web",
        "modules": [
            js_module("JS_MODULE_KIND_FRONTEND", "./web/entry.ts",
                      disableEntrypoint=True),
        ],
        "webPkgs": [
            web_pkg("@s4wave/web", entrypoints=[
                "./command", "./contexts", "./debug", "./devtools",
                "./editors/file-browser", "./forge", "./frame",
                "./hooks", "./images", "./launcher", "./layout",
                "./object", "./router", "./space", "./state",
                "./style", "./transform", "./ui", "./ui/credential",
                "./ui/list", "./ui/tree",
            ]),
            web_pkg("@fontsource-variable/manrope"),
            web_pkg("@fontsource/commit-mono"),
            web_pkg("sonner"),
        ],
    },
)

# JS plugins sharing the same exclusion pattern.
def js_plugin(name, rev, modules, extra_web_pkgs=None):
    manifest(name,
        builder="bldr/plugin/compiler/js",
        rev=rev,
        config={
            "webPluginId": "web",
            "modules": modules,
            "webPkgs": EXCLUDED_WEB_PKGS + (extra_web_pkgs or []),
        },
    )

js_plugin("spacewave-app", rev=221, modules=[
    js_module("JS_MODULE_KIND_FRONTEND", "./app/App.tsx",
              webViewParentId={"empty": True}),
    js_module("JS_MODULE_KIND_BACKEND", "./plugin/notes/backend.ts"),
    js_module("JS_MODULE_KIND_BACKEND", "./plugin/vm/backend.ts"),
])

DESKTOP_RELEASE_LOAD_PLUGINS = [
    "spacewave-launcher", "spacewave-loader",
    "spacewave-core", "spacewave-web", "spacewave-app", "web",
]

BROWSER_RELEASE_LOAD_PLUGINS = [
    # spacewave-loader is intentionally omitted in browser release builds. It
    # exists only to spawn the native spacewave-helper loading window; in WASM
    # it has no helper binary to launch and just creates a no-op plugin worker.
    "spacewave-launcher",
    "spacewave-core", "spacewave-web", "spacewave-app", "web",
]

def dist_release_config(embed_manifests, load_plugins):
    return dist_compiler_config(
        cliPkgs=["./cmd/spacewave-cli/cli"],
        embedManifests=embed_manifests,
        loadPlugins=load_plugins,
        loadWebStartup=WEB_STARTUP,
    )

manifest("spacewave-dist",
    builder="bldr/dist/compiler",
    # embedManifests is empty in the static manifest because every release
    # build supplies its own (manifestId, platformId) tuples via
    # manifestOverrides (REPLACE semantics). This keeps the static config
    # host-agnostic and makes the build target the single source of truth for
    # what ships in each bundle.
    config=dist_release_config([], DESKTOP_RELEASE_LOAD_PLUGINS),
)

# -- Build targets --

DEV_MANIFESTS = [
    "web", "spacewave-core", "spacewave-web",
    "spacewave-app", "spacewave-debug",
]
BROWSER_RELEASE_MANIFESTS = [
    # The browser release should not even build spacewave-loader: it is a
    # native helper-window plugin, and loading it in WASM shows up as an
    # extra shared worker that exits after helper lookup fails.
    "spacewave-launcher",
    "spacewave-core", "spacewave-web", "spacewave-app", "web",
    "spacewave-dist",
]
DESKTOP_RELEASE_MANIFESTS = [
    "spacewave-launcher", "spacewave-loader",
    "spacewave-core", "spacewave-web", "spacewave-app", "web",
    "spacewave-dist",
]
# REMOTE_WORLD_MANIFESTS are the manifests that ship in the R2-hosted plugin
# world. Desktop entrypoints still embed the startup app manifests for a
# reliable first boot; plugin-promote can replace them after launch by updating
# the remote plugin world.
REMOTE_WORLD_MANIFESTS = [
    "spacewave-core", "spacewave-web", "spacewave-app", "web",
]
BROWSER_RELEASE_EMBED_MANIFESTS = [
    {"manifestId": "spacewave-launcher",
     "platformId": "web/js/wasm"},
    {"manifestId": "spacewave-core",
     "platformId": "web/js/wasm"},
    {"manifestId": "web",
     "platformId": "web/js/wasm"},
    {"manifestId": "spacewave-web",
     "platformId": "js"},
    {"manifestId": "spacewave-app",
     "platformId": "js"},
]

build("app",         manifests=DEV_MANIFESTS,     targets=["desktop"])
build("web",         manifests=DEV_MANIFESTS,     targets=["browser"])
build("release-web",
    manifests=BROWSER_RELEASE_MANIFESTS,
    targets=["browser"],
    manifestOverrides={
        "spacewave-dist": dist_release_config(
            BROWSER_RELEASE_EMBED_MANIFESTS,
            BROWSER_RELEASE_LOAD_PLUGINS,
        ),
    },
)
build("cli",         manifests=["spacewave", "spacewave-cli"])

# plugin-release-browser builds the browser-side plugin channel surface: the
# wasm spacewave-core manifest plus the JS plugin manifests that live in the
# remote world.
build("plugin-release-browser",
    manifests=REMOTE_WORLD_MANIFESTS,
    targets=["browser"],
)

# Per-host release builds. Each (host_key, platform_id) pair drives one
# `release-<host_key>` build target that release.go invokes via
# `bldr build -b release-<host_key>`. The manifestOverrides entry REPLACES
# the static spacewave-dist builder config for that slot, embedding only
# the (manifestId, platformId) tuples that belong in this host's binary.
RELEASE_HOSTS = [
    ("desktop-darwin-arm64",  "desktop/darwin/arm64"),
    ("desktop-darwin-amd64",  "desktop/darwin/amd64"),
    ("desktop-linux-arm64",   "desktop/linux/arm64"),
    ("desktop-linux-amd64",   "desktop/linux/amd64"),
    ("desktop-windows-arm64", "desktop/windows/arm64"),
    ("desktop-windows-amd64", "desktop/windows/amd64"),
]

def define_release_build(host_key, platform_id):
    desktop_embed_manifests = [
        {"manifestId": "spacewave-launcher",
         "platformId": platform_id},
        {"manifestId": "spacewave-loader",
         "platformId": platform_id},
        {"manifestId": "spacewave-core",
         "platformId": platform_id},
        {"manifestId": "web",
         "platformId": platform_id},
        {"manifestId": "spacewave-web",
         "platformId": "js"},
        {"manifestId": "spacewave-app",
         "platformId": "js"},
    ]
    build("release-" + host_key,
        manifests=DESKTOP_RELEASE_MANIFESTS,
        platform_ids=[platform_id],
        manifestOverrides={
            "spacewave-dist": dist_release_config(
                desktop_embed_manifests,
                DESKTOP_RELEASE_LOAD_PLUGINS,
            ),
        },
    )

for host_key, platform_id in RELEASE_HOSTS:
    define_release_build(host_key, platform_id)

# Per-host CLI-only release builds. Each (host_key, platform_id) pair drives
# one `release-cli-<host_key>` build target that produces a standalone
# `spacewave-cli` binary for the matching host. Release automation packages
# the binary into the platform-specific archive (macOS/Windows: `.zip`, Linux:
# `.tar.gz`) advertised in the `/download` page CLI manifest. The host_key uses
# the Go GOOS naming (`darwin`, `linux`, `windows`) to match the existing
# `release-desktop-<host>` convention; release packaging renames `darwin`
# to the user-facing `macos` label (e.g. `spacewave-cli-macos-arm64.zip`)
# so the public artifact names match the existing
# `spacewave-macos-*.dmg` installer naming. Host matrix matches RELEASE_HOSTS
# so the CLI ships everywhere the desktop app does.
for host_key, platform_id in RELEASE_HOSTS:
    cli_host_key = host_key.replace("desktop-", "")
    build("release-cli-" + cli_host_key,
        manifests=["spacewave-cli"],
        platform_ids=[platform_id],
    )

# Per-host plugin-only release builds. These produce just the native
# spacewave-core manifests for the plugin channel; the browser-side wasm + JS
# manifests are built once by plugin-release-browser.
for host_key, platform_id in RELEASE_HOSTS:
    build("plugin-release-" + host_key,
        manifests=["spacewave-core"],
        platform_ids=[platform_id],
    )

# Build browser-side manifests once per release run. The per-host release
# targets stay native-only so they do not try to build spacewave-dist for JS.
build("release-remote-web",
    manifests=["web"],
    platform_ids=["web/js/wasm"],
)
build("release-remote-js",
    manifests=["spacewave-web", "spacewave-app"],
    platform_ids=["js"],
)

# -- Publish --
#
# The spacewave-release remote is the staging area for uploading manifests to
# the R2-hosted remote world. =bldr publish -p spacewave-release= copies the
# selected REMOTE_WORLD_MANIFESTS from the devtool world into a local bolt DB
# at =.bldr/release-spacewave.bdb=. Release automation exports that bolt DB as
# a kvfile and uploads it to the plugin channel namespace at
# =release/plugins/world/<plugin-rev>.kvfile=. Transform config (s2 + blockenc
# with PEER_ENCRYPTION_KEY) is applied during the copy so the remote-world
# blocks are compressed and encrypted at rest. The publish timestamp is pinned
# so identical inputs yield byte-identical bolt output across runs; bump
# =RELEASE_PIN_TIMESTAMP_SECONDS= at each release cut.

# Pinned timestamp used for publish so reproducible builds stay stable
# across rebuilds but advance with real releases. Bump at each release cut to
# match the git tag date. RFC3339 UTC ("Z" suffix required).
RELEASE_PIN_TIMESTAMP = "2026-04-16T00:00:00Z"

# RELEASE_TRANSFORM mirrors the blockenc step used by core_config_set so the
# published remote world uses the same encryption key as the runtime peer
# store. s2 compresses first (better compression of unencrypted bytes), then
# blockenc encrypts the compressed output.
RELEASE_TRANSFORM = {
    "steps": [
        {"id": "hydra/transform/s2"},
        {
            "id": "hydra/transform/blockenc",
            "config": {
                "blockEnc": "BlockEnc_XCHACHA20_POLY1305",
                "key": PEER_ENCRYPTION_KEY,
            },
        },
    ],
}

remote("spacewave-release",
    engineId="spacewave-release-world",
    objectKey="spacewave/release/manifests",
    hostConfigSet={
        "release-volume": config_entry("hydra/volume/bolt", 1, {
            "path": ".bldr/release-spacewave.bdb",
            "noWriteKey": True,
            "volumeConfig": {
                "volumeIdAlias": ["release-volume"],
            },
        }),
        "release-bucket": config_entry("hydra/bucket/setup", 1, {
            "applyBucketConfigs": [{
                "config": {"id": "spacewave-release", "rev": 1},
                "volumeIdList": ["release-volume"],
            }],
        }),
        "release-engine": config_entry("hydra/world/block/engine", 1, {
            "engineId": "spacewave-release-world",
            "volumeId": "release-volume",
            "bucketId": "spacewave-release",
            "objectStoreId": "spacewave-release",
        }),
    },
)

publish("spacewave-release",
    remotes=["spacewave-release"],
    manifests=REMOTE_WORLD_MANIFESTS,
    storage={
        "transformConf": RELEASE_TRANSFORM,
        "timestamp": RELEASE_PIN_TIMESTAMP,
    },
)

# -- Project --

project(
    id="spacewave",
    start=start_config(
        plugins=["web", "spacewave-web", "spacewave-app",
                 "spacewave-core", "spacewave-debug"],
        loadWebStartup=WEB_STARTUP,
    ),
)
