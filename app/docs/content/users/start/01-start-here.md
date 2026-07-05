---
title: Start Here
section: start
order: 1
summary: Open Spacewave, pick a local or cloud session, and create useful data.
---

Spacewave opens to a start screen with account actions and Quickstarts. A
Quickstart creates or reuses a local session, creates a Space when needed, seeds
starter objects, and opens the new workspace.

## Choose a session

Use **Sign in or create account** when you want a Spacewave Cloud account with
cloud sync, backup, and multi-device access. Use **Continue without account**
when you want a local session on this device first. Local sessions are useful
for testing and private offline work, but browser storage can be cleared by the
browser under storage pressure. The desktop app can also open an existing local
state root.

## Create something small

Pick **Create a Drive** for the shortest useful path. It creates a Space, adds a
UnixFS-backed file tree, writes `getting-started.md`, and opens Drive in the file
browser. Pick **Create an Empty Space** when you want a blank container and will
add objects later. **Create/clone a Git Repository** opens a Git repository
wizard. **Create a Canvas** creates a visual workspace with starter canvas data.

Experimental Quickstarts such as Notebook, Documentation, Blog, Chat, Key/Value,
SQL, V86, Device, and Forge appear only when experimental creators or their
plugins are available.

## Find commands

Press Cmd/Ctrl+K or click the Spacewave logo to open the command palette. Search
for the action you need, then run it from the list. Command availability follows
the current page and selected Space.

## Check persistence

After the first Space opens, add one file, folder, or object, then reload the
page. If the item is still present, the session and Space are mounted correctly.
For long-lived work, set up a backup key or move the session to Cloud before it
holds data you cannot recreate.
