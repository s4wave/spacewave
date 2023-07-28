# Web Plugin

The **web** plugin builder produces a plugin providing a web runtime to the
other plugins via the Bldr web_runtime interfaces.

It behaves differently depending on the distribution target and config:

 - Development mode: does nothing (Bldr provides the web runtime).
 - Web: does nothing (the browser provides this already)
 - Native desktop (windows, mac, linux): includes the Electron redistributable.
   - note: uses the electron from node_modules (run yarn add --dev electron)

Normally if you run "yarn start:electron" Bldr will use the development version
of electron located in node_modules to launch the web runtime.

When distributing the app, Electron should be distributed as a plugin, so that
it can be easily remotely updated as necessary.

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

You can optionally include it in the embed plugins list to embed Electron into
the distribution binary as well.

If you don't include "web" in the startup plugins list, you can start it
on-demand by adding a `LoadPlugin<plugin-id=web>` directive.

When the plugin is loaded, it will automatically start the web runtime.
