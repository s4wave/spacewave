# Web Plugin

The **web** plugin builder produces a plugin providing a web runtime to the
other plugins via the Bldr web_runtime interfaces.

It behaves differently depending on the distribution target and config:

 - Web: forwards requests to the plugin host (the browser provides WebRuntime)
 - Native: includes the Electron redistributable.

## Enabling the Plugin

Add the following to your bldr.yaml plugins list and startup plugins:

```yaml
id: my-project
start:
  plugins:
    - my-project
    - web
plugins:
  web:
  my-project:
    id: bldr/plugin/compiler
    config:
      # delveAddr: wait
      goPkgs: [...]
```

If you don't include "web" in the startup plugins list, you can start it
on-demand by adding a `LoadPlugin<plugin-id=web>` directive.

When the plugin is loaded, it will automatically start the web runtime.

You can optionally include it in the embed plugins list to embed Electron into
the distribution binary as well.
