# bldr.star - Bldr demo project configuration

RELEASE_DESKTOP_EMBEDS = [
    {"manifestId": "web", "platformId": "desktop/darwin/arm64"},
    {"manifestId": "bldr-demo", "platformId": "desktop/darwin/arm64"},
]

RELEASE_WEB_EMBEDS = [
    {"manifestId": "web", "platformId": "web/js/wasm"},
    {"manifestId": "bldr-demo", "platformId": "web/js/wasm"},
]

def demo_dist_config(embed_manifests):
    return dist_compiler_config(
        embedManifests=embed_manifests,
        loadPlugins=["bldr-demo"],
        loadWebStartup="example/startup.tsx",
    )

project(
    id="bldr-demo",
    start=start_config(
        plugins=["bldr-demo"],
    ),
)

manifest("web",
    builder="bldr/web/plugin/compiler",
    config=web_plugin_compiler_config(),
)

manifest("bldr-demo",
    builder="bldr/plugin/compiler/go",
    rev=5,
    config=go_plugin_config(
        webPluginId="web",
        goPkgs=["./example"],
        webPkgs=[
            web_pkg("lucide-react"),
        ],
        configSet={
            "demo-1": config_entry("bldr/example/demo", 0, {
                "runDemo": False,
            }),
        },
        buildTypes={
            "release": {
                "hostConfigSet": {
                    "fetch-plugin-via-bldr-demo": config_entry(
                        "bldr/manifest/fetch/plugin", 0,
                        {
                            "pluginId": "bldr-demo",
                            "fetchManifestIdRe": "demo-.*",
                        },
                    ),
                },
            },
        },
    ),
)

manifest("bldr-demo-cli",
    builder="bldr/cli/compiler",
    config=cli_compiler_config(
        goPkgs=["./example"],
        cliPkgs=["./example/cli"],
        configSet={
            "demo-1": config_entry("bldr/example/demo", 0, {
                "runDemo": False,
            }),
        },
    ),
)

manifest("bldr-demo-release",
    builder="bldr/dist/compiler",
    config=demo_dist_config(RELEASE_DESKTOP_EMBEDS),
)

build("desktop",
    manifests=["web", "bldr-demo"],
    targets=["desktop"],
)

build("web",
    manifests=["web", "bldr-demo"],
    targets=["browser"],
)

build("release",
    manifests=["web", "bldr-demo", "bldr-demo-release"],
    targets=["desktop"],
)

build("release-web",
    manifests=["web", "bldr-demo", "bldr-demo-release"],
    targets=["browser"],
    manifestOverrides={
        "bldr-demo-release": demo_dist_config(RELEASE_WEB_EMBEDS),
    },
)

build("release-cross",
    manifests=["web", "bldr-demo", "bldr-demo-release"],
    platformIds=[
        "js",
        "desktop/linux/amd64",
        "desktop/windows/amd64",
        "desktop/darwin/amd64",
    ],
)

remote("demo-dist",
    engineId="demo-dist",
    objectKey="dist/demo",
    hostConfigSet={
        "demo-dist-bucket": config_entry("hydra/bucket/setup", 0, {
            "applyBucketConfigs": [
                {
                    "volumeIdList": ["devtool"],
                    "config": {"id": "demo-dist"},
                },
            ],
        }),
        "demo-dist-world": config_entry("hydra/world/block/engine", 0, {
            "engineId": "demo-dist",
            "bucketId": "demo-dist",
            "volumeId": "devtool",
            "objectStoreId": "demo-dist",
            "disableChangelog": True,
        }),
    },
)

publish("demo-dist",
    remotes=["demo-dist"],
    manifests=["web", "bldr-demo", "bldr-demo-dist"],
    storage={
        "timestamp": "2023-10-18T18:38:40Z",
        "transformConf": {
            "steps": [
                {"id": "hydra/transform/s2"},
                {
                    "id": "hydra/transform/blockenc",
                    "config": {
                        "blockEnc": "BlockEnc_XCHACHA20_POLY1305",
                        "key": "DEn7LStBTb2ZOP1mlLixkmPLg6x773/TQ1mnyyXJu1A=",
                    },
                },
            ],
        },
    },
)
