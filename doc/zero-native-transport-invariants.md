# Zero-Native Transport Invariants

The zero-native WebRuntime backend transport is StarPC over native IPC.

The renderer/backend boundary uses packet streams carried by the platform
native bridge: Unix-domain sockets or named pipes for the native helper probe,
and the WebView IPC packet-stream bridge for renderer integration. The Resource
SDK attaches to this StarPC packet stream when backend runtime boot is present.

Renderer code must not replace this boundary with an easier local transport.
MessagePort-only, shared-worker, in-process, or direct JavaScript callback
transports are valid for existing browser/plugin communication surfaces, but
they are not the zero-native backend transport.

The native IPC contract owns these backend requirements:

- StarPC framing crosses the renderer/backend process boundary as packets over
  native IPC.
- Stream lifecycle is explicit: open, ordered packet send, close, cancel, remote
  error propagation, and clean EOF are observable at the native boundary.
- Resource SDK traffic for the zero-native backend uses the same StarPC over
  native IPC boundary rather than a renderer-local shortcut.
- The current probe exposes transport status, echo, callback packet streams,
  and WebView IPC packet streams. It does not start renderer backend boot,
  package the runtime backend, or provide end-to-end app integration.

The focused coverage for these requirements is in
`desktop/cross/src/zero-native-ipc-test.cpp`. Browser-side packet stream coverage
for the WebView bridge shape is in `bldr/web/bldr/zero-native-webview-ipc.test.ts`.
