---
title: Building a TypeScript Plugin
section: plugins
order: 3
summary: Build a Spacewave plugin using TypeScript and the SDK.
draft: true
---

## Project Setup

A TypeScript plugin consists of a backend entry point and one or more frontend viewer components. The plugin is declared in `bldr.yaml` with the `bldr/plugin/compiler/js` builder:

```yaml
my-plugin:
  builder:
    id: bldr/plugin/compiler/js
    rev: 1
    config:
      webPluginId: web
      modules:
        - kind: JS_MODULE_KIND_BACKEND
          path: ./plugin/my-plugin/backend.ts
        - kind: JS_MODULE_KIND_FRONTEND
          path: ./plugin/my-plugin/MyViewer.tsx
      webPkgs:
        - id: '@s4wave/web'
          exclude: true
```

The `modules` array declares backend and frontend entry points. The `webPluginId` references the parent web infrastructure plugin. Shared packages listed in `webPkgs` are available at runtime without bundling.

## Plugin Manifest

The plugin manifest is generated from `bldr.yaml` at build time. It contains the plugin's compiled code, declared object types, and viewer registrations. The manifest is stored as a content-addressed block and referenced by its ID in `SpaceSettings`.

## Registering an Object Type

The backend entry point exports a default async function that receives `BackendAPI` and `AbortSignal`:

```typescript
import type { BackendAPI } from '@aptre/bldr-sdk'

export default async function backend(
  api: BackendAPI,
  signal: AbortSignal,
): Promise<void> {
  const rootRef = await api.accessRoot(signal)
  const otSvc = new ObjectTypeRegistryResourceServiceClient(
    rootRef.client,
  )
  await otSvc.RegisterObjectType({
    typeId: 'my-plugin/my-type',
    pluginId: 'my-plugin',
  }, signal)
}
```

The `typeId` identifies the object type across the system. The `pluginId` links it back to this plugin. Once registered, the runtime routes objects of this type to the plugin's handlers.

## Building a Viewer Component

A viewer is a React component that renders objects of a registered type. It receives `ObjectViewerComponentProps`:

```typescript
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'

export default function MyViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  // Use SDK hooks to access object data
  const handle = useAccessTypedHandle(
    worldState,
    objectInfo.objectKey,
    MyHandle,
  )
  // Render UI
  return <div>{/* viewer content */}</div>
}
```

Register the viewer in the backend by calling `ViewerRegistryResourceServiceClient.RegisterViewer` with the type ID and the path to the frontend module.

## Testing Your Plugin

Run `bun run tsgo --noEmit` to verify TypeScript compilation. For runtime testing, add the plugin to a space via the CLI and verify that objects of the registered type render correctly in the browser.

Integration tests use the testbed package. Create a real in-memory bus with `testbed.Default(ctx)` and add the plugin's controller factories to verify directive resolution and state management.

## Publishing

Build the plugin with `bun run build` to produce the manifest. The manifest is a content-addressed block that can be distributed through the block-DAG network. Other users install it by adding the manifest ID to their space settings.
