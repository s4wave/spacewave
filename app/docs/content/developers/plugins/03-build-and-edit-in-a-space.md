---
title: Build and Edit a Plugin in a Space
section: plugins
order: 3
summary: Store plugin source in a Space, build it on a registered device, and edit its viewer with a live preview.
---

You can keep a plugin's source code in a Space, build it on one of your
devices, and install the result into the same Space. While you work on the
viewer, a live preview shows each saved change without a rebuild.

## Build plugin source from a Space

Open the Space **Plugins** panel. Choose a source folder that contains a Bldr
project, a manifest ID, and a registered build device. **Build** takes a
snapshot of the source and starts a Forge build on that device. Forge is
Spacewave's job runner. Edits made after the build starts do not change it.
Open the build's logs to follow compilation. When the build succeeds, install
the manifest into the open Space.

The build device needs the local Bldr toolchain. The build uses the Space's
existing World and plugin host.

For a source folder that contains `app.ts`, `backend.ts`, and
`ColorViewer.tsx`, use this `bldr.yaml`:

```yaml
id: colors
manifests:
  colors:
    builder:
      id: bldr/plugin/compiler/js
      config:
        webPluginId: web
        webPkgs:
          - id: '@s4wave/web'
            exclude: true
        modules:
          - kind: JS_MODULE_KIND_BACKEND
            path: ./backend.ts
            entrypoint: true
          - kind: JS_MODULE_KIND_FRONTEND
            path: ./ColorViewer.tsx
```

In `app.ts` and `backend.ts`, import the SDK from
`@go/github.com/s4wave/spacewave/sdk/`. Set the viewer entry in
`definePlugin` to `ColorViewer.tsx`. The build device supplies the SDK source
and the compiler. Declare any other dependencies in the project's
`package.json`. `exclude: true` makes the viewer use the web package the client
has already loaded, including its React contexts, instead of bundling a second
copy.

After the installed plugin registers its Quickstart, **Create _app name_**
creates the app object in the current Space. The source and the new object live
in the same World. The action uses the session you are signed in with.

## How builds are stored

Each build stores its manifest under a hash of its content, with a record of
the source it came from. Two builds at the same revision number keep separate
outputs. Rebuilds in watch mode increase the revision, so the scheduler picks
up the new code. A build from Space source keeps a copy of that source in the
manifest's World. That copy is separate from the editable folder and from the
Forge build.

## Edit with a live preview

Live preview works only in a Bldr development client running in watch mode. In
the **Plugins** panel, choose the same source folder, device, and manifest, then
**Edit with live preview**. The source editor and the preview open side by
side.

1. Choose the frontend module and an object the plugin created. The plugin's
   **Create** action is next to the preview if you need a new object.
2. Open a UTF-8 source file, choose **Edit file**, make your change, then
   **Save file**. A save replaces the whole file, up to 512 KiB. If a save
   fails, your unsaved text stays in the editor. Saved changes reach the Vite
   compiler on the build device. React and CSS edits update the preview without
   restarting the backend or losing stored data.
3. Close the editor when you are done. This stops the compiler. To publish the
   change for other clients, choose **Build**, then **Install in this Space**.

The preview needs `@vitejs/plugin-react` for React Refresh. For the Colors
example, add this `package.json`:

```json
{
  "type": "module",
  "dependencies": { "zod": "4.3.6" },
  "devDependencies": { "@vitejs/plugin-react": "6.0.5", "vite": "8.2.2" }
}
```

Add a `vite.config.ts`:

```ts
import react from '@vitejs/plugin-react'

export default { plugins: [react()] }
```

Then set `viteConfigPaths: [vite.config.ts]` and
`viteDisableProjectConfig: true` in the builder config above.

A release client explains that a development client is needed before it starts
a compiler. The preview uses the page's own React and React Refresh, and your
changes apply only inside the preview. Installed viewers keep running their
built version until you install a new one.

## How the preview connects

The preview calls `space.openPluginFrontend(request, signal)`, which uses
Bldr's frontend RPC service. The returned Resource holds the permission to run
on the build device. Its `Watch` stream keeps the live compiler connected.
Release both when the editor closes.

If the compiler disconnects, the preview shows an error with **Restart
preview**. Restarting connects a new compiler and resets the preview's
component state.
