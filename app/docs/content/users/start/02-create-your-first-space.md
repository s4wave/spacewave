---
title: Create Your First Space
section: start
order: 2
summary: Create a Space with a Quickstart and verify the first object opens.
---

A Space is the container for your Spacewave data. Creating one stores a shared
object for the Space, mounts its World, and adds the starter objects selected by
the Quickstart.

## From the start screen

1. Open Spacewave.
2. Choose a Quickstart such as **Create a Drive**, **Create an Empty Space**,
   **Create/clone a Git Repository**, or **Create a Canvas**.
3. If you do not already have a session, let Spacewave create a local session.
4. Wait for the creation progress to finish.

The route changes to `/u/{session}/so/{space}` when the Space is mounted. The
session number selects the local mounted session. The Space identifier selects
the shared object backing the Space.

## What each stable Quickstart creates

- **Create an Empty Space** creates the Space settings object and leaves the
  Space ready for new objects.
- **Create a Drive** creates a UnixFS file tree, writes `getting-started.md`, and
  opens Drive through a first-run guide.
- **Create/clone a Git Repository** creates a persistent Git repository wizard
  and opens the Space at that wizard.
- **Create a Canvas** creates a UnixFS object, seeds a canvas demo, and opens the
  canvas.

Other Quickstarts exist behind experimental or plugin gates. If a plugin-owned
Quickstart is unavailable, the app cannot truthfully create that surface until
the plugin registers it.

## Verify the Space

Open the object browser or current viewer and confirm the expected starter
object appears. For Drive, open `getting-started.md`; the file viewer should show
the starter guide. For a blank Space, use the create-object command or the Space
settings page to pick the first object.

## Keep the first Space

Local sessions stay on the device that created them. Spacewave Cloud sessions
add cloud-backed sync, backup, and access from linked devices. If the first Space
matters, download a backup key and choose a lock mode before relying on it.
