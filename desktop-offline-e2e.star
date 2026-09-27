# Append this overlay to bldr.star in an isolated Bldr configuration directory.
# Bldr resolves embedded dependencies from manifest definitions, so a build
# target override alone is insufficient.

offline_core = spacewave_core_config()
offline_core["configSet"]["provider-local"] = config_entry("provider/local", 1)
offline_core["configSet"].pop("provider-spacewave")

offline_launcher = spacewave_launcher_config(
    launcher_controller_config=spacewave_launcher_controller_config(
        dist_peer_ids=[],
        refetch_dur="",
        disable_endpoint_fetch=True,
    ),
    include_release_world=False,
)

manifest("spacewave-launcher",
    builder="bldr/plugin/compiler/go",
    rev=1,
    config=offline_launcher,
)
manifest("spacewave-core",
    builder="bldr/plugin/compiler/go",
    rev=13,
    config=offline_core,
)

OFFLINE_DESKTOP_MANIFESTS = [
    "spacewave-launcher", "spacewave-core", "spacewave-web",
    "spacewave-code", "spacewave-app", "web",
]
build("desktop-offline-e2e-assets",
    manifests=OFFLINE_DESKTOP_MANIFESTS,
    platformIds=["desktop/darwin/arm64", "js"],
)
build("desktop-offline-e2e-dist",
    manifests=["spacewave-dist"],
    platformIds=["desktop/darwin/arm64"],
    manifestOverrides={
        "spacewave-dist": dist_release_config([
            {"manifestId": "spacewave-launcher", "platformId": "desktop/darwin/arm64"},
            {"manifestId": "spacewave-core", "platformId": "desktop/darwin/arm64"},
            {"manifestId": "spacewave-web", "platformId": "js"},
            {"manifestId": "spacewave-code", "platformId": "js"},
            {"manifestId": "spacewave-app", "platformId": "js"},
            {"manifestId": "web", "platformId": "desktop/darwin/arm64"},
        ], OFFLINE_DESKTOP_MANIFESTS),
    },
)
