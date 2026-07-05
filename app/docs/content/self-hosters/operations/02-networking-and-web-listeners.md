---
title: Networking and Web Listeners
section: operations
order: 2
summary: Use local listeners and pairing transports without overclaiming public relay controls.
---

Current user-facing networking has three practical surfaces: the daemon socket,
local web listeners, and device pairing transports.

## Daemon socket

CLI commands use a local Unix socket. This is local process communication, not a
public network listener. Keep the state path and socket path explicit when
running multiple runtimes.

## Local web listener

`spacewave web` asks the root resource for a web listener. The default listener
binds to localhost on an available port and returns a URL with a single-use
bootstrap secret. You can pass host/port or a listen multiaddr, run foreground,
or keep the listener in the daemon with `--background`.

This listener is for access to the native runtime from your local browser. The
current docs source does not show a stable public listener or admin-facing TURN
or STUN configuration surface.

## WebRTC pairing

Local direct pairing exchanges complete WebRTC offer and answer payloads by
copy/paste or QR-style transfer. It gathers candidates before returning the
payload, then both sides verify the same emoji sequence. This direct path is
local-provider only in the current UI.

## WebSocket and embedded streams

The browser app and tests use SRPC streams over WebView host streams or test
WebSockets. Treat those as runtime plumbing unless a user-facing command exposes
them.
