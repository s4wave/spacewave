project(
    id="bldr-downstream-e2e",
    start=start_config(
        plugins=["web", "downstream-core", "downstream-web"],
        loadWebStartup="bldr/e2e/downstreamapp/testdata/app/web/startup.tsx",
    ),
)

manifest("web",
    builder="bldr/web/plugin/compiler",
    config=web_plugin_compiler_config(),
)

manifest("downstream-core",
    builder="bldr/plugin/compiler/go",
    rev=1,
    config=go_plugin_config(
        webPluginId="web",
        goPkgs=[
            "./bldr/e2e/downstreamapp/testdata/app/core",
            "github.com/s4wave/spacewave/bldr/plugin/load",
            "github.com/s4wave/spacewave/bldr/web/plugin/handle-rpc",
        ],
        disableRpcFetch=True,
        configSet={
            "downstream-core-root": config_entry("bldr/e2e/downstreamapp/core", 1),
            "load-web": config_entry("bldr/plugin/load", 1, {
                "pluginId": "web",
            }),
            "handle-rpc": config_entry("bldr/web/plugin/handle-rpc", 1, {
                "webPluginId": "web",
                "handlePluginId": "downstream-core",
                "serverIdRe": "web-view/.*",
            }),
        },
    ),
)

manifest("downstream-web",
    builder="bldr/plugin/compiler/js",
    rev=1,
    config=js_plugin_config(
        webPluginId="web",
        modules=[
            js_module("JS_MODULE_KIND_FRONTEND",
                      "./bldr/e2e/downstreamapp/testdata/app/web/App.tsx",
                      entrypoint=True,
                      webViewParentId={"empty": True}),
        ],
        webPkgs=[
            web_pkg("sonner"),
        ],
    ),
)

build("web",
    manifests=["web", "downstream-core", "downstream-web"],
    targets=["browser"],
)
