---
title: Start Here
section: dev-start-here
order: 1
summary: First plugin or first SDK interaction.
draft: true
---

## Who This Is For

This page is for developers who want to build on Spacewave. Whether you are creating a plugin that adds a new object type, building a viewer component, or integrating with the SDK from TypeScript, this is your starting point.

You should be comfortable with TypeScript and have basic familiarity with React. Go experience is helpful for backend plugins but not required for TypeScript-only plugins.

## What You Will Do

By the end of this guide, you will have a working TypeScript plugin that registers an object type and renders a viewer component in the browser. The plugin will:

1. Declare a manifest in the bldr project configuration.
2. Export a backend function that registers an object type.
3. Provide a React component that renders objects of that type.

## Prerequisites

- [Bun](https://bun.sh/) installed (used instead of npm/yarn)
- The Spacewave repository cloned and set up (`bun install`)
- Familiarity with the [plugin model](/docs/developers/plugins/what-are-plugins)

## Steps

### 1. Declare the Plugin Manifest

Add a manifest entry to the bldr project config. The builder specifies the TypeScript plugin compiler with backend and frontend modules:

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

### 2. Write the Backend

Create `plugin/my-plugin/backend.ts`. The backend registers your object type with the runtime:

```typescript
import type { BackendAPI } from '@aptre/bldr-sdk'

export default async function backend(
  api: BackendAPI,
  signal: AbortSignal,
): Promise<void> {
  const rootRef = await api.accessRoot(signal)
  // Register your object type and viewer here
}
```

The backend function runs in a Web Worker. It receives a `BackendAPI` for accessing the SDK and an `AbortSignal` that fires when the plugin is being shut down.

### 3. Build a Viewer Component

Create `plugin/my-plugin/MyViewer.tsx`. The viewer receives `ObjectViewerComponentProps` with access to the object's world state:

```typescript
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'

export default function MyViewer({
  objectInfo,
}: ObjectViewerComponentProps) {
  return (
    <div>
      <h2>{objectInfo.objectKey}</h2>
      <p>Your viewer content here</p>
    </div>
  )
}
```

### 4. Build and Test

Run the TypeScript compiler to check for errors:

```bash
bun run tsgo --noEmit
```

Start the dev server to see your plugin in action:

```bash
bun run start:web
```

Create a space, add an object of your registered type, and verify that your viewer renders correctly.

## Verify

Your plugin is working when:

- The dev server starts without build errors.
- Creating an object of your type shows your viewer component.
- The browser console logs `WebView: set render mode` when the viewer mounts.

## Next Steps

- [Building a TypeScript Plugin](/docs/developers/plugins/building-a-ts-plugin) for the full plugin development guide with object type registration, viewer registration, and testing.
- [Resource System](/docs/developers/sdk/resource-system) for understanding how resources and streaming RPCs power the SDK.
- [SDK Hooks and Patterns](/docs/developers/sdk/hooks-and-patterns) for the React hook reference.
- [Plugin Lifecycle](/docs/developers/platform/plugin-lifecycle) for understanding how plugins are loaded and executed.
