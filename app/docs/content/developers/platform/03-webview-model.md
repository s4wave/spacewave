---
title: WebView Model
section: platform
order: 3
summary: WebView boundary, render modes, and style and script injection.
---

## What This Is

A WebView is the boundary between the Go runtime and the browser DOM. The Go side controls what a portion of the page renders by setting render modes, injecting scripts, and managing CSS links. The React side renders content based on these instructions. WebViews are the mechanism by which plugins display their UI without direct DOM access from the WASM sandbox.

## How It Works

The `WebView` React component (`@aptre/bldr-react`) registers itself with a `WebDocument` (the Go-side page controller) on mount. The Go runtime communicates with the WebView through three operations:

**SetRenderMode** - Controls what the WebView displays:

| Mode | Behavior |
|------|----------|
| `NONE` | Empty, nothing rendered |
| `REACT_COMPONENT` | Lazy-loads a React component from a script path |
| `FUNCTION` | Loads a function-based component from a script path |
| `REACT_CHILDREN` | Renders the React children passed to the WebView |

When `REACT_COMPONENT` or `FUNCTION` mode is set, the WebView lazy-loads the script from the provided path. This is how plugins inject their viewer components into the page.

**SetHtmlLinks** - Manages stylesheet and other HTML link elements. Supports adding, removing, and clearing links. Stylesheets are tracked for load state. The WebView content is hidden (via React `Activity`) until all stylesheets have loaded, preventing unstyled content flashes.

**ResetView** - Returns the WebView to its initial state, clearing all links and scripts.

## Render Lifecycle

1. The WebView mounts and registers with the WebDocument.
2. The Go runtime calls `setRenderMode` with a script path.
3. The WebView lazy-loads the script and renders the component inside `Suspense`.
4. CSS links are loaded in parallel. The component is hidden until all stylesheets are ready.
5. Once CSS is loaded and the component signals readiness, the loading fallback is replaced with the live component.

The `refreshNonce` counter forces re-mount when the Go runtime requests a refresh (e.g., after a plugin manifest update).

## WebView Hierarchy

WebViews form a parent-child tree. Each WebView has a UUID and an optional parent UUID. The root WebView is permanent (cannot be removed). Child WebViews can be removed by calling `remove()`, which invokes the `onRemove` callback or closes the window if it was script-opened.

The hierarchy enables multi-panel layouts where the Go runtime controls several independent rendering regions. Each plugin can have its own WebView within the page layout.

## Style Isolation

Plugin stylesheets are injected via `setHtmlLinks` with `rel: "stylesheet"`. The WebView tracks each link's load state and only reveals the rendered component after all stylesheets have fired their `onload` event. Previously loaded stylesheets (cached by the browser) are detected on mount by checking the `sheet` property of the link element.

## Why It Matters

The WebView model is the reason plugins can render rich React UIs from within a WASM sandbox. The Go runtime never touches the DOM directly. Instead, it sends declarative instructions (render mode, script path, CSS links) through the starpc RPC channel. The React WebView component translates these instructions into DOM operations. This separation keeps plugins isolated while giving them full access to React's component model.

## Next Steps

- [Plugin Lifecycle](/docs/developers/platform/plugin-lifecycle) for how plugins are loaded and begin rendering.
- [Web Entrypoint and Runtime](/docs/developers/platform/web-entrypoint-and-runtime) for how the WebDocument and workers are initialized.
