---
title: CLI Quickstart
section: cli
order: 2
summary: Confirm the CLI is connected and exercise the active session in three commands.
---

## Overview

This quickstart proves the Spacewave CLI is reaching your desktop app session
and shows how the most common commands feel. Run the three commands below in
order. The whole walkthrough takes under a minute and does not change any
data.

If you have not installed the CLI yet, start with
[Install the CLI](/docs/users/cli/install).

## Before You Start

- Open the desktop app and pick the session you want the CLI to act as.
- Leave the desktop app running. The CLI talks to its session over a local
  socket, so the app must be live.
- The desktop app's
  **Settings -> Command Line** page shows the same commands bound to your
  active session, with copy buttons. Use that page if you want to skip
  retyping arguments.

## Step 1: Status

```bash
spacewave status
```

This prints a single status report: the resolved socket path, the active
session index, the lock state, and the number of spaces in this session.

If `status` errors out, the CLI could not reach the desktop app. Open the
**Settings -> Command Line** page in the desktop app and confirm the socket
shows as **Ready**.

## Step 2: Who Am I

```bash
spacewave whoami
```

Confirms which session the CLI is acting as: the session id, the account it
belongs to (cloud or local), and whether the session is locked. This is how
you check that the CLI picked the session you expected when you have multiple
sessions on this machine.

## Step 3: List Spaces

```bash
spacewave space list
```

Lists the spaces visible to this session. Each row shows the space id, name,
and primary type (Drive, Git, Canvas, and so on). This is the first command
that touches real session data, so seeing your spaces here means the CLI is
fully wired up.

## What to Try Next

- `spacewave space info <id>` for details about a specific space.
- `spacewave fs ls /u/<index>/so/<space-id>/-/<object-key>/-/` to walk a
  space's file tree from the terminal.
- `spacewave web --bg` to start a local web listener that stays running in
  the background.
- The desktop app's **Settings -> Command Line** page lists more commands and
  copy-pasteable variants bound to your active session.

For the full command reference, see the
[developer CLI reference](/docs/developers/cli/installation-and-commands).
