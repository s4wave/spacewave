---
title: Choose How to Run Spacewave
section: start
order: 1
summary: Pick browser, desktop, daemon, or Cloud-backed operation by ownership boundary.
---

Spacewave can run in several shapes. Pick the one that matches who owns runtime,
storage, and recovery.

## Browser local

Browser local mode is the quickest way to start. It creates a local session and
stores data in browser storage. Use it for experiments. Do not use it as the only
copy of important work because browser storage may be cleared.

## Desktop local

The desktop app owns a native state directory and can add an existing `.spacewave`
state root. This is the local mode to use when you want the state root under the
operating system's filesystem instead of browser storage.

## CLI daemon

`spacewave serve` starts a daemon that listens on `spacewave.sock` inside the
resolved state path. CLI commands connect to that socket. Without an explicit
socket path, client commands may autostart a daemon for the selected state path.

## Local web listener

`spacewave web` opens a localhost browser listener for a native runtime.
Foreground listeners last until the process exits. Background listeners stay in
the daemon and can be listed or stopped.

## Spacewave Cloud

Cloud mode uses the Spacewave provider for encrypted cloud storage, sync, backup,
billing, and multi-device access. It does not remove local responsibility for
local sessions until you transfer or recreate the data in the Cloud session.
