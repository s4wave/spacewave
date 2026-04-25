---
title: Virtual Machines in a Space
section: features
order: 7
summary: Run a small program or boot an entire operating system inside a Spacewave space.
draft: true
---

## What You Can Do

Sometimes you want more than documents and files. You want to run a tool, try out an OS, or keep a small server-side process alongside your notes. Spacewave spaces can host two kinds of virtual machines for that:

- **WASI VM.** Runs a small program written for WebAssembly. Good for scripts, command-line tools, and tiny servers that you want to live alongside your data.
- **V86 VM.** Boots a full x86 operating system (such as Linux) directly inside your browser. Good for throwaway sandboxes, retro OS experiments, or running legacy software without installing anything locally.

Both are experimental. Treat them as power-user features, not the everyday core of the app.

## Creating a VM

1. Open the space where you want the VM to live.
2. Open the [object browser](/docs/users/spaces/object-browser).
3. Click the **+** button and choose **WASI VM** or **V86 VM**.
4. The VM appears in the space immediately and starts in the background.

The VM is just another object in your space. It syncs to your linked devices and is stored in the same encrypted storage as your files and notes.

## What a WASI VM Is Good For

Use a WASI VM when you want to run a small program that sits next to your data. Examples:

- A markdown formatter that tidies notes in place.
- A scheduled script that re-indexes your files.
- A long-running process that exposes an API to other objects in the space.

The VM streams its status back to the viewer (running, stopped, error). You can stop and restart it without losing the data stored alongside it.

## What a V86 VM Is Good For

Use a V86 VM when you want an entire operating system in a tab:

- Boot a Linux image and try command-line tools without installing anything.
- Run an old app that requires a specific OS.
- Hand somebody else a ready-to-run sandbox by sharing the space.

The V86 viewer is currently a placeholder that'll grow a full serial terminal and desktop UI in a future update. Expect rough edges.

## Staying Private

A VM running inside a space is still inside Spacewave's storage and sync layer. Its disk, state, and configuration are all encrypted and synced like any other object. Nothing inside the VM escapes the space it lives in.

## Next Steps

- [Automation and Publishing](/docs/users/features/automation-and-tooling-objects) for workflows and publishing to the web
- [Object browser](/docs/users/spaces/object-browser)
- [Understanding spaces](/docs/users/spaces/understanding-spaces)
