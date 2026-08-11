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
    "./core/sobject/world/engine",
    "./core/space/world/optypes",
    "./core/plugin/space",
    "./core/space/http/download",
    "./core/space/http/export",
    "./core/space/world/blocktype",
    "github.com/s4wave/spacewave/db/object/peer",
]

def core_go_pkgs(include_export=True):
    pkgs = []
    for pkg in CORE_GO_PKGS:
        if include_export or pkg != "./core/space/http/export":
            pkgs.append(pkg)
    return pkgs

LAUNCHER_GO_PKGS = [
    "./core/provider/spacewave/launcher/controller",
    "github.com/s4wave/spacewave/bldr/manifest/fetch/world",
    "github.com/s4wave/spacewave/core/cdn/world/controller",
    "github.com/s4wave/spacewave/core/cdn/bstore/controller",
    "github.com/s4wave/spacewave/db/block/store/bucket",
    "github.com/s4wave/spacewave/core/space/world/optypes",
    "github.com/s4wave/spacewave/db/block/store/overlay",
    "github.com/s4wave/spacewave/db/block/store/rpc/server",
    "github.com/s4wave/spacewave/db/object/peer",
]

LAUNCHER_BROWSER_GO_PKGS = [
    "./core/provider/spacewave/launcher/controller",
    "github.com/s4wave/spacewave/db/block/store/overlay",
    "github.com/s4wave/spacewave/db/block/store/rpc/server",
    "github.com/s4wave/spacewave/db/object/peer",
]

PRODUCTION_DIST_PEER_ID = "12D3KooWL2DEcvqSXXrrCmUxMdPbqFcqzhHBvqseZWHwjAt7aXfW"
PRODUCTION_CLOUD_API_ENDPOINT = "https://spacewave.app"
PRODUCTION_ACCOUNT_ENDPOINT = "https://account.spacewave.app"
PRODUCTION_RELEASE_CONFIG_URL = "https://spacewave.app/api/release/config"
BROWSER_RELEASE_CLOUD_API_ENDPOINT = "/"

# Signed by a checked-in test-only peer ID. The private key is not needed at
# runtime; this packed DistConfig only seeds the release-WASM e2e fixture.
E2E_RELEASE_WASM_INIT_DIST_CONFIG = "QnF9fgszS28jFAQYB2t0Nx0lOx8KM1VoPCAjKShHamY9cXwMfVVQbjskPBo7R3FIMg0pPws1EkRcQlQImHAm4evJX816W1ZueiWpxzExZyZWv1gerVT0-gbD34GuNied4VzXsHGfTa6sG61t8q9C0PgfEylbZYCUALSnTFVsClHn7sqR2OZPikT405ESAbYmf7rjOJ8kw5554IZXVFPXU83lSBXRaZI9DgLim3kd90GNr2qcznKz-FKKOxfcHf-kjy-pUnR7qBQ0ZF4sCZ2zJUqe6BJL-EZIGczDuXKnli5WfY8V32P9jF2D--5UwB2ZNPU"
E2E_RELEASE_WASM_DIST_PEER_ID = "12D3KooWJPNip1SbsUG7SjteoegGjfq22D4WThuctPvCwJzHesD5"

# Core configSet shared between Go plugin and CLI manifests.
# Native listeners derive their socket from the project storage root so data,
# logs, and the runtime socket honor SPACEWAVE_DATA_DIR as one owner.
def core_config_set(
        include_export=True,
        cloud_api_endpoint=PRODUCTION_CLOUD_API_ENDPOINT,
        account_endpoint=PRODUCTION_ACCOUNT_ENDPOINT):
    configs = {
        "store-peer": config_entry("object/peer", 1, {
            "objectStoreId": "s4wave-peer",
            "volumeId": "plugin-host",
        }),
        "root-resource": config_entry("resource/root", 1),
        "session-list": config_entry("session", 1),
        "provider-local": config_entry("provider/local", 1),
        "provider-spacewave": config_entry("provider/spacewave", 2, {
            "endpoint": cloud_api_endpoint,
            "accountEndpoint": account_endpoint,
            "signingEnvPrefix": "spacewave",
        }),
        "space-sobject": config_entry("space/sobject", 1, {"verbose": False}),
        "space-world-ops": config_entry("space/world/ops", 1),
        "blocktype": config_entry("db/blocktype", 1),
        "download": config_entry("space/http/download", 1),
        "resource-listener": config_entry("resource/listener", 1, {
            "storageProjectId": "spacewave",
        }),
    }
    if include_export:
        configs["export"] = config_entry("space/http/export", 1)
    return configs

def desktop_status_projector_config_set():
    return {
        "desktop-status-projector": config_entry("resource/desktop/status-projector", 1),
    }

def browser_go_compiler_platform(go_compiler):
    if go_compiler == "GO_COMPILER_GOSCRIPT":
        return "js"
    return "web"


def spacewave_core_config(
        web_go_compiler=None,
        cloud_api_endpoint=PRODUCTION_CLOUD_API_ENDPOINT):
    platform_types = {
        "desktop": {
            "goPkgs": ["./core/resource/desktop/statusprojector"],
            "configSet": desktop_status_projector_config_set(),
        },
    }
    if web_go_compiler:
        platform_types[browser_go_compiler_platform(web_go_compiler)] = {
            "goCompiler": web_go_compiler,
        }
    return {
        "goPkgs": core_go_pkgs(),
        "configSet": core_config_set(cloud_api_endpoint=cloud_api_endpoint),
        "buildTypes": {
            "dev": {
                "goPkgs": ["./core/debug/trace"],
                "configSet": {
                    "debug-trace": config_entry("debug/trace", 1),
                },
            },
        },
        "platformTypes": platform_types,
        "hostConfigSet": {
            "fetch-manifest-via-spacewave-core": config_entry(
                "bldr/manifest/fetch/plugin", 1,
                {"pluginId": "spacewave-core"},
            ),
        },
    }

def browser_spacewave_core_config(web_go_compiler=None):
    return spacewave_core_config(
        web_go_compiler=web_go_compiler,
        cloud_api_endpoint=BROWSER_RELEASE_CLOUD_API_ENDPOINT,
    )

def spacewave_launcher_controller_config(
        dist_peer_ids=[PRODUCTION_DIST_PEER_ID],
        endpoints=[{"url": PRODUCTION_RELEASE_CONFIG_URL}],
        refetch_dur="1h",
        init_dist_config="",
        disable_endpoint_fetch=False):
    conf = {
        "projectId": "spacewave",
        # Public defaults are production-only. Release-owned overlays replace
        # endpoints and distPeerIds for other release environments.
        "distPeerIds": dist_peer_ids,
    }
    if not disable_endpoint_fetch:
        conf["endpoints"] = endpoints
    if disable_endpoint_fetch:
        conf["disableEndpointFetch"] = True
    if refetch_dur != "":
        conf["refetchDur"] = refetch_dur
    if init_dist_config != "":
        conf["initDistConfig"] = init_dist_config
    return conf


def release_world_config_set(
        space_id="01kqjmfxd44r7ggrq78efad3d2",
        cdn_base_url="https://cdn.spacewave.app",
        cache_block_store_id="dist",
        include_fetch=True):
    configs = {
        "release-world": config_entry("spacewave/cdn/world", 1, {
            "engineId": "spacewave-release-world",
            "spaceId": space_id,
            "cdnBaseUrl": cdn_base_url,
            "cacheBlockStoreId": cache_block_store_id,
        }),
        "release-world-ops": config_entry("space/world/ops", 1, {
            "engineId": "spacewave-release-world",
        }),
    }
    if include_fetch:
        configs["release-world-fetch"] = config_entry("bldr/manifest/fetch/world", 1, {
            "engineId": "spacewave-release-world",
            "objectKeys": ["spacewave/release/manifests"],
        })
    return configs

def release_world_cdn_config_set(
        space_id="01kqjmfxd44r7ggrq78efad3d2",
        cdn_base_url="https://cdn.spacewave.app",
        cache_block_store_id="dist",
        bucket_store_id=""):
    cdn_block_store_id = "spacewave-release-cdn"
    if bucket_store_id == "":
        bucket_store_id = cdn_block_store_id
    return {
        "release-world-cdn-store": config_entry("spacewave/cdn/bstore", 1, {
            "blockStoreId": cdn_block_store_id,
            "spaceId": space_id,
            "cdnBaseUrl": cdn_base_url,
            "cacheBlockStoreId": cache_block_store_id,
            "bucketIds": ["spacewave-release"],
        }),
        "release-world-cdn-bucket": config_entry("hydra/block/store/bucket", 1, {
            "blockStoreId": cdn_block_store_id,
            "bucketStoreId": bucket_store_id,
            "bucketConfig": {
                "id": "spacewave-release",
                "rev": 1,
            },
        }),
    }


def spacewave_launcher_config(
        launcher_controller_config=spacewave_launcher_controller_config(),
        web_go_compiler=None,
        include_release_world=True):
    config_set = {
        "spacewave-launcher": config_entry(
            "spacewave/launcher/controller", 1,
            launcher_controller_config,
        ),
        "store-peer": config_entry("object/peer", 1, {
            "objectStoreId": "s4wave-peer",
            "volumeId": "plugin-host",
        }),
    }
    if include_release_world:
        config_set.update(release_world_config_set(
            cache_block_store_id="plugin-host",
            include_fetch=False,
        ))
        config_set.update(release_world_cdn_config_set(
            cache_block_store_id="plugin-host",
        ))
    conf = {
        "goPkgs": LAUNCHER_GO_PKGS if include_release_world else LAUNCHER_BROWSER_GO_PKGS,
        "configSet": config_set,
    }
    if include_release_world:
        # Release World providers must be applied to the plugin-host bus as
        # host configuration so they mount before plugin startup begins.
        conf["hostConfigSet"] = release_world_config_set(cache_block_store_id="dist")
        conf["hostConfigSet"].update(release_world_cdn_config_set(
            cache_block_store_id="dist",
        ))
    if web_go_compiler:
        conf["platformTypes"] = {
            browser_go_compiler_platform(web_go_compiler): {
                "goCompiler": web_go_compiler,
            },
        }
    return conf

def browser_release_launcher_config(web_go_compiler=None):
    return spacewave_launcher_config(
        web_go_compiler=web_go_compiler,
        include_release_world=False,
    )

def e2e_release_wasm_launcher_config(web_go_compiler=None):
    return spacewave_launcher_config(
        spacewave_launcher_controller_config(
            dist_peer_ids=[E2E_RELEASE_WASM_DIST_PEER_ID],
            refetch_dur="",
            init_dist_config=E2E_RELEASE_WASM_INIT_DIST_CONFIG,
            disable_endpoint_fetch=True,
        ),
        web_go_compiler=web_go_compiler,
        include_release_world=False,
    )

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
    rev=6,
    config={
        "nativeApp": {
            "appName": "Spacewave",
            "windowTitle": "Spacewave",
            "themeSource": "dark",
            "iconPath": "web/images/spacewave-icon.png",
            "desktopPresencePolicy": "DESKTOP_PRESENCE_POLICY_TRAY_BACKGROUND",
            "trayIconPath": "web/images/spacewave-icon.png",
            "macosTemplateTrayIconPath": "web/images/spacewave-tray-template.png",
        },
    },
)

# spacewave-launcher is the minimal browser launcher plugin. It carries just
# enough to fetch a DistConfig, mount the public release world from the CDN, and
# resolve plugin manifests from it; browser release still embeds the full
# startup app closure below so first startup does not depend on that fetch plane.
manifest("spacewave-launcher",
    builder="bldr/plugin/compiler/go",
    rev=1,
    config=spacewave_launcher_config(),
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
    rev=13,
    config=spacewave_core_config(),
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

manifest("spacewave-cli-plugin",
    builder="bldr/plugin/compiler/go",
    rev=1,
    config={
        "goPkgs": ["./plugin/cli"],
        "configSet": {
            "spacewave-cli-plugin": config_entry("plugin/cli", 1),
        },
        "platformTypes": {
            "js": {
                "goCompiler": "GO_COMPILER_GOSCRIPT",
            },
        },
    },
)

manifest("spacewave-sql",
    builder="bldr/plugin/compiler/go",
    rev=1,
    config={
        "goPkgs": ["./plugin/sql"],
        "configSet": {
            "spacewave-sql": config_entry("plugin/sql", 1),
        },
        "platformTypes": {
            "js": {
                "goCompiler": "GO_COMPILER_GOSCRIPT",
            },
        },
    },
)

manifest("spacewave",
    builder="bldr/cli/compiler",
    config={
        "goPkgs": CORE_GO_PKGS,
        "cliPkgs": ["./cmd/spacewave/cli"],
        "configSet": core_config_set(),
        "projectId": "spacewave",
    },
)

manifest("spacewave-web",
    builder="bldr/plugin/compiler/js",
    rev=12,
    config={
        "webPluginId": "web",
        "modules": [
            js_module("JS_MODULE_KIND_FRONTEND", "./web/entry.ts"),
        ],
        "webPkgs": [
            web_pkg("@s4wave/web", entrypoints=[
                "./command", "./configtype", "./contexts", "./debug", "./devtools",
                "./dnd", "./download", "./editors/file-browser", "./forge", "./frame",
                "./hooks", "./images", "./launcher", "./layout", "./object",
                "./platform", "./router", "./sdk/app", "./space", "./state",
                "./style", "./transform", "./ui", "./ui/credential",
                "./ui/list", "./ui/loading", "./ui/tree",
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

js_plugin("spacewave-app", rev=224, modules=[
    js_module("JS_MODULE_KIND_FRONTEND", "./app/App.tsx",
              entrypoint=True,
              webViewParentId={"empty": True}),
])

js_plugin("spacewave-notes", rev=1, modules=[
    js_module("JS_MODULE_KIND_BACKEND", "./plugin/notes/backend.ts",
              entrypoint=True),
    js_module("JS_MODULE_KIND_FRONTEND", "./plugin/notes/NotebookViewer.tsx"),
    js_module("JS_MODULE_KIND_FRONTEND", "./plugin/notes/BlogViewer.tsx"),
    js_module("JS_MODULE_KIND_FRONTEND", "./plugin/notes/DocsViewer.tsx"),
    js_module("JS_MODULE_KIND_FRONTEND", "./plugin/notes/NotesWizardViewer.tsx"),
])

js_plugin("spacewave-v86", rev=1, modules=[
    js_module("JS_MODULE_KIND_BACKEND", "./plugin/v86/backend.ts",
              entrypoint=True),
    js_module("JS_MODULE_KIND_FRONTEND", "./plugin/v86/VmV86Viewer.tsx"),
])

DESKTOP_RELEASE_LOAD_PLUGINS = [
    "spacewave-launcher", "spacewave-loader",
    "spacewave-core", "spacewave-web", "spacewave-app", "web",
]

CLI_RELEASE_LOAD_PLUGINS = [
    "spacewave-launcher",
    "spacewave-core", "spacewave-web", "spacewave-app", "web",
]

BROWSER_RELEASE_LOAD_PLUGINS = [
    # spacewave-loader is intentionally omitted in browser release builds. It
    # exists only to spawn the native spacewave-helper loading window; in WASM
    # it has no helper binary to launch and just creates a no-op plugin worker.
    "spacewave-launcher",
    "spacewave-core", "spacewave-web", "spacewave-app", "web",
]

BROWSER_RELEASE_E2E_LOAD_PLUGINS = [
    "spacewave-launcher",
    "spacewave-core", "spacewave-web", "spacewave-app", "web",
]

def dist_release_config(embed_manifests, load_plugins, entrypoint_role="desktop", go_compiler=None):
    conf = dist_compiler_config(
        cliPkgs=["./cmd/spacewave/cli"],
        embedManifests=embed_manifests,
        entrypointRole=entrypoint_role,
        channelKey="stable",
        loadPlugins=load_plugins,
        loadWebStartup=WEB_STARTUP,
    )
    if go_compiler:
        conf["goCompiler"] = go_compiler
    return conf

manifest("spacewave-dist",
    builder="bldr/dist/compiler",
    # embedManifests is empty in the static manifest because every release
    # build supplies its own (manifestId, platformId) tuples via
    # manifestOverrides (REPLACE semantics). This keeps the static config
    # host-agnostic and makes the build target the single source of truth for
    # what ships in each bundle.
    config=dist_release_config([], DESKTOP_RELEASE_LOAD_PLUGINS),
)
manifest("spacewave-browser",
    builder="bldr/dist/compiler",
    # Browser entrypoint packaging is a web shell artifact, not the native
    # desktop DistConfig slot. Keep it on its own manifest id so browser release
    # builds never wait on or publish native spacewave-dist refs.
    config=dist_release_config([], BROWSER_RELEASE_LOAD_PLUGINS, entrypoint_role="browser"),
)
manifest("spacewave-cli",
    builder="bldr/dist/compiler",
    # CLI uses the same dist compiler and native CLI path as spacewave-dist,
    # but has its own Manifest id so Release World metadata can distinguish
    # CLI rollout from desktop app rollout without overloading platform ids.
    config=dist_release_config([], CLI_RELEASE_LOAD_PLUGINS, entrypoint_role="cli"),
)

# -- Build targets --

DEV_MANIFESTS = [
    "web", "spacewave-core", "spacewave-web",
    "spacewave-app", "spacewave-notes", "spacewave-v86", "spacewave-sql", "spacewave-cli-plugin", "spacewave-debug",
]
BROWSER_RELEASE_MANIFESTS = [
    "spacewave-launcher",
    "spacewave-browser",
]
BROWSER_RELEASE_E2E_MANIFESTS = [
    "spacewave-launcher",
    "spacewave-core", "spacewave-web", "spacewave-app", "spacewave-sql", "spacewave-cli-plugin", "web",
    "spacewave-browser",
]
DESKTOP_RELEASE_MANIFESTS = [
    "spacewave-launcher",
    "spacewave-dist",
]
CLI_RELEASE_MANIFESTS = [
    "spacewave-launcher",
    "spacewave-cli",
]
# REMOTE_WORLD_MANIFESTS are the manifests that ship in the R2-hosted plugin
# world. Desktop entrypoints still embed the startup app manifests for a
# reliable first boot; plugin-promote can replace them after launch by updating
# the remote plugin world.
REMOTE_WORLD_MANIFESTS = [
    "spacewave-loader", "spacewave-core", "spacewave-web", "spacewave-app", "spacewave-notes", "spacewave-v86", "spacewave-sql",
    "spacewave-cli-plugin", "web",
]
PLUGIN_RELEASE_BROWSER_MANIFESTS = [
    "spacewave-core", "spacewave-web", "spacewave-app", "spacewave-notes", "spacewave-v86", "spacewave-sql",
    "spacewave-cli-plugin", "web",
]

# The launcher mounts and verifies Release World before the scheduler resolves
# ordinary startup plugins. Entrypoint artifacts therefore embed no desired
# plugin state besides the launcher itself.
def browser_release_embed_manifests(go_platform_id):
    return [
        {"manifestId": "spacewave-launcher",
         "platformId": go_platform_id},
    ]


BROWSER_RELEASE_LAZY_PLUGIN_FIXTURE_EMBEDS = browser_release_embed_manifests("js")
BROWSER_RELEASE_LAZY_PLUGIN_FIXTURE_LOAD_PLUGINS = BROWSER_RELEASE_LOAD_PLUGINS + [
    "spacewave-cli-plugin",
]

# Browser e2e embeds the startup closure plus the spacewave-sql plugin so the
# sql plugin cold-builds under the e2e release without a populated Release World.
def browser_e2e_embed_manifests(go_platform_id):
    return browser_release_embed_manifests(go_platform_id) + [
        {"manifestId": "spacewave-sql",
         "platformId": "js"},
    ]


BROWSER_RELEASE_EMBED_MANIFESTS = browser_release_embed_manifests("js")
BROWSER_RELEASE_WASM_EMBED_MANIFESTS = browser_release_embed_manifests("web/js/wasm")
BROWSER_RELEASE_E2E_EMBED_MANIFESTS = browser_e2e_embed_manifests("web/js/wasm")
BROWSER_RELEASE_E2E_GOSCRIPT_EMBED_MANIFESTS = browser_e2e_embed_manifests("js")

build("app",         manifests=DEV_MANIFESTS,     targets=["desktop"])
build("web",         manifests=DEV_MANIFESTS,     targets=["browser"])
build("release-web",
    manifests=BROWSER_RELEASE_MANIFESTS,
    targets=["browser"],
    manifestOverrides={
        "spacewave-launcher": spacewave_launcher_config(web_go_compiler="GO_COMPILER_GOSCRIPT"),
        "spacewave-core": browser_spacewave_core_config("GO_COMPILER_GOSCRIPT"),
        "spacewave-browser": dist_release_config(
            BROWSER_RELEASE_EMBED_MANIFESTS,
            BROWSER_RELEASE_LOAD_PLUGINS,
            entrypoint_role="browser",
            go_compiler="GO_COMPILER_GOSCRIPT",
        ),
    },
)
build("release-web-lazy-plugin-fixture",
    manifests=BROWSER_RELEASE_MANIFESTS,
    targets=["browser"],
    manifestOverrides={
        "spacewave-launcher": spacewave_launcher_config(web_go_compiler="GO_COMPILER_GOSCRIPT"),
        "spacewave-core": browser_spacewave_core_config("GO_COMPILER_GOSCRIPT"),
        "spacewave-browser": dist_release_config(
            BROWSER_RELEASE_LAZY_PLUGIN_FIXTURE_EMBEDS,
            BROWSER_RELEASE_LAZY_PLUGIN_FIXTURE_LOAD_PLUGINS,
            entrypoint_role="browser",
            go_compiler="GO_COMPILER_GOSCRIPT",
        ),
    },
)
build("release-web-e2e",
    manifests=BROWSER_RELEASE_E2E_MANIFESTS,
    targets=["browser"],
    manifestOverrides={
        "spacewave-launcher": e2e_release_wasm_launcher_config(),
        "spacewave-core": browser_spacewave_core_config(),
        "spacewave-browser": dist_release_config(
            BROWSER_RELEASE_E2E_EMBED_MANIFESTS,
            BROWSER_RELEASE_E2E_LOAD_PLUGINS,
            entrypoint_role="browser",
        ),
    },
)
# CI serves the js dist output. Split the asset and dist phases so GitHub
# runners do not optimize the unused web/js/wasm dist runtime in parallel.
build("release-web-e2e-assets",
    manifests=[
        "spacewave-launcher",
        "spacewave-core", "spacewave-web", "spacewave-app", "spacewave-sql", "spacewave-cli-plugin", "web",
    ],
    targets=["browser"],
    manifestOverrides={
        "spacewave-launcher": e2e_release_wasm_launcher_config(),
        "spacewave-core": browser_spacewave_core_config(),
    },
)
build("release-web-e2e-dist",
    manifests=["spacewave-browser"],
    platformIds=["js"],
    manifestOverrides={
        "spacewave-browser": dist_release_config(
            BROWSER_RELEASE_E2E_EMBED_MANIFESTS,
            BROWSER_RELEASE_E2E_LOAD_PLUGINS,
            entrypoint_role="browser",
        ),
    },
)
build("release-web-e2e-tinygo-core",
    manifests=["spacewave-core"],
    targets=["browser"],
    manifestOverrides={
        "spacewave-core": browser_spacewave_core_config("GO_COMPILER_TINYGO"),
    },
)
build("release-web-e2e-tinygo-assets",
    manifests=[
        "spacewave-launcher",
        "spacewave-core", "spacewave-web", "spacewave-app", "spacewave-sql", "spacewave-cli-plugin", "web",
    ],
    targets=["browser"],
    manifestOverrides={
        "spacewave-core": browser_spacewave_core_config("GO_COMPILER_TINYGO"),
        "spacewave-launcher": e2e_release_wasm_launcher_config(),
    },
)
build("release-web-tinygo",
    manifests=BROWSER_RELEASE_MANIFESTS,
    targets=["browser"],
    manifestOverrides={
        "spacewave-core": browser_spacewave_core_config("GO_COMPILER_TINYGO"),
        "spacewave-browser": dist_release_config(
            BROWSER_RELEASE_WASM_EMBED_MANIFESTS,
            BROWSER_RELEASE_LOAD_PLUGINS,
            entrypoint_role="browser",
            go_compiler="GO_COMPILER_TINYGO",
        ),
    },
)
build("release-web-e2e-tinygo",
    manifests=BROWSER_RELEASE_E2E_MANIFESTS,
    targets=["browser"],
    manifestOverrides={
        "spacewave-core": browser_spacewave_core_config("GO_COMPILER_TINYGO"),
        "spacewave-launcher": e2e_release_wasm_launcher_config(),
        "spacewave-browser": dist_release_config(
            BROWSER_RELEASE_E2E_EMBED_MANIFESTS,
            BROWSER_RELEASE_E2E_LOAD_PLUGINS,
            entrypoint_role="browser",
            go_compiler="GO_COMPILER_TINYGO",
        ),
    },
)
build("release-web-e2e-goscript",
    manifests=BROWSER_RELEASE_E2E_MANIFESTS,
    targets=["browser"],
    manifestOverrides={
        "spacewave-core": browser_spacewave_core_config("GO_COMPILER_GOSCRIPT"),
        "spacewave-launcher": e2e_release_wasm_launcher_config(web_go_compiler="GO_COMPILER_GOSCRIPT"),
        "spacewave-browser": dist_release_config(
            BROWSER_RELEASE_E2E_GOSCRIPT_EMBED_MANIFESTS,
            BROWSER_RELEASE_E2E_LOAD_PLUGINS,
            entrypoint_role="browser",
            go_compiler="GO_COMPILER_GOSCRIPT",
        ),
    },
)
build("cli-plugin",  manifests=["spacewave-cli-plugin"], targets=["browser"])
build("cli",         manifests=["spacewave"])

# plugin-release-browser records the browser-side plugin channel manifest set.
# The CI release helper builds each manifest with its release platform because
# the remote world mixes the GoScript core/js manifest, JS plugin manifests, and
# web/web/js/wasm manifest in one channel.
PLUGIN_RELEASE_BROWSER_MANIFEST_OVERRIDES = {
    "spacewave-core": browser_spacewave_core_config("GO_COMPILER_GOSCRIPT"),
}
def plugin_release_browser_manifest_overrides(manifest_id):
    if manifest_id == "spacewave-core":
        return PLUGIN_RELEASE_BROWSER_MANIFEST_OVERRIDES
    return {}

def plugin_release_browser_manifest_platform_ids(manifest_id):
    if manifest_id == "web":
        return ["web/js/wasm"]
    return ["js"]


build("plugin-release-browser",
    manifests=PLUGIN_RELEASE_BROWSER_MANIFESTS,
    targets=["browser"],
    manifestOverrides=PLUGIN_RELEASE_BROWSER_MANIFEST_OVERRIDES,
)

build("plugin-release-browser-tinygo",
    manifests=PLUGIN_RELEASE_BROWSER_MANIFESTS,
    targets=["browser"],
    manifestOverrides={
        "spacewave-core": browser_spacewave_core_config("GO_COMPILER_TINYGO"),
    },
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

# Per-host managed CLI entrypoint builds. Each (host_key, platform_id) pair
# drives one `release-cli-<host_key>` build target that produces a
# `spacewave-cli` dist entrypoint for the matching host. The terminal path
# loads the same release-world product plugin surface as desktop, but omits the
# native helper-window loader so CLI startup owns progress and failure output.
for host_key, platform_id in RELEASE_HOSTS:
    cli_host_key = host_key.replace("desktop-", "")
    cli_embed_manifests = [
        {"manifestId": "spacewave-launcher",
         "platformId": platform_id},
    ]
    build("release-cli-" + cli_host_key,
        manifests=CLI_RELEASE_MANIFESTS,
        platform_ids=[platform_id],
        manifestOverrides={
            "spacewave-cli": dist_release_config(
                cli_embed_manifests,
                CLI_RELEASE_LOAD_PLUGINS,
                entrypoint_role="cli",
            ),
        },
    )

# Per-host plugin-only release builds. These produce just the native
# spacewave-core manifests for the plugin channel; the browser-side wasm + JS
# manifests are built once by plugin-release-browser.
for host_key, platform_id in RELEASE_HOSTS:
    build("plugin-release-" + host_key,
        manifests=["spacewave-loader", "spacewave-core", "spacewave-sql"],
        platform_ids=[platform_id],
    )

# -- Publish --
#
# The spacewave-release remote is the staging area for uploading manifests to
# the R2-hosted remote world. =bldr publish -p spacewave-release= copies the
# selected REMOTE_WORLD_MANIFESTS from the devtool world into a local bolt DB
# at =.bldr/release-spacewave.bdb=. Release automation exports that bolt DB as
# a kvfile and uploads it to the plugin channel namespace at
# =release/plugins/world/<plugin-rev>.kvfile=. The publish timestamp is pinned
# so identical inputs yield byte-identical bolt output across runs; bump
# =RELEASE_PIN_TIMESTAMP_SECONDS= at each release cut.

# Pinned timestamp used for publish so reproducible builds stay stable
# across rebuilds but advance with real releases. Bump at each release cut to
# match the git tag date. RFC3339 UTC ("Z" suffix required).
RELEASE_PIN_TIMESTAMP = "2026-04-16T00:00:00Z"

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

