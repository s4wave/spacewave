---
title: Install the CLI
section: cli
order: 1
summary: Install the Spacewave command line tool on macOS, Linux, or Windows.
---

## Overview

The Spacewave CLI (`spacewave`) is a single binary that runs alongside the
desktop app. It connects to your active session over a local socket so you can
script Spacewave from a terminal: list spaces, run file operations, manage
git, and open the local web listener.

This page walks you through installing the CLI on your computer. Once it is on
your PATH, head to [Quickstart](/docs/users/cli/quickstart) to confirm the
connection.

## What You Need

- A modern macOS, Linux, or Windows machine.
- The Spacewave desktop app installed and running, so the CLI has a session to
  connect to. See [Install and Update](/docs/users/settings/install-and-update)
  if you have not installed the desktop app yet.

The CLI is a single self-contained binary. There are no runtime dependencies.

## Pick the Right Build

The CLI ships as a per-platform archive on the
[download page](/download/cli). Pick the build that matches your operating
system and CPU architecture. Apple Silicon Macs use the macOS arm64 build.
Intel Macs use macOS amd64. Most Linux desktops use linux amd64. Windows on
ARM uses the windows arm64 build.

## macOS

The macOS build ships as a signed and notarized zip archive:

1. Download the macOS archive from the
   [CLI download section](/download/cli) and unzip it.
2. Move `spacewave` to a folder that is on your PATH. A common choice is
   `/usr/local/bin`.
3. Open a new terminal and run `spacewave status` to confirm the install.

## Linux

On Linux, install the CLI with a single command. Replace `<archive-url>` with
the URL for your platform copied from the
[CLI download section](/download/cli):

```bash
curl -fsSL <archive-url> | tar -xz -C /usr/local/bin spacewave
```

This downloads the archive, extracts the `spacewave` binary, and writes it to
`/usr/local/bin`. You may need `sudo` if your user does not own that
directory.

After the install completes, open a new terminal and run:

```bash
spacewave status
```

You should see a status report describing the active session, the resolved
socket path, and the connected client count. If `spacewave status` reports
that it could not reach the desktop app, make sure the desktop app is open
and the socket path matches the one shown on the
**Settings -> Command Line** page.

## Windows

The Windows build ships as a portable zip. There is no installer yet, so you
add it to your PATH manually:

1. Download the Windows archive from the
   [CLI download section](/download/cli) and unzip it.
2. Move `spacewave.exe` to a folder that is on your PATH. A common choice is
   to create a folder at `%USERPROFILE%\bin`, drop the binary inside, and add
   that folder to PATH from
   **System Properties -> Environment Variables -> Path**.
3. Open a new terminal (PowerShell or Command Prompt) so the new PATH is
   picked up, and run `spacewave status` to confirm the install.

## Updating the CLI

To update, repeat the install command for your platform. The new binary
overwrites the old one. The CLI does not store any local state of its own. It
talks to the desktop app, so updating the desktop app and the CLI separately
is fine.

## Troubleshooting

- **`spacewave: command not found` after install.** Open a new terminal so it
  picks up the updated PATH, or run the binary directly from its install
  location to confirm it is present.
- **`status` reports the CLI cannot reach the desktop app.** Open the desktop
  app, then open
  **Settings -> Command Line** in the app to see the live socket path and
  status. The page also shows the exact `spacewave status` command bound to
  this session.
- **Multiple machines.** Install the CLI on every machine you want to script
  from. Each install talks to the desktop app running on that same machine.

## Next Steps

- [Quickstart](/docs/users/cli/quickstart): three commands that prove the CLI
  is connected and exercise real session data.
- [Install and Update](/docs/users/settings/install-and-update): browser and
  desktop app install paths.
