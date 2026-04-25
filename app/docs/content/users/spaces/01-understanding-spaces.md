---
title: Understanding Spaces
section: spaces
order: 1
summary: What spaces are, how they work, and how to use them.
---

## What is a Space

A space is your own private workspace inside Spacewave. Think of it like a secure project folder that automatically syncs between your devices. Each space keeps its data separate from other spaces, has its own settings, and can have different tools installed.

A Drive space holds files. A Git space holds repository data. A Canvas space holds a visual layout. The space itself is the container; the objects and viewers inside it decide what it does.

## What is Inside a Space

Every space tracks everything that happens in it. Files you upload, repositories you create, canvas nodes you move, and settings you change are recorded inside the space. Each entry knows what type it is, so the right tool opens to display it.

Your spaces also keep a history of changes. This is what makes sync work: when two devices compare their copies of a space, they can quickly figure out what is different and exchange only the new parts.

## How Your Data is Stored

Your data is broken into small encrypted pieces and saved to your device's local storage. Identical content always produces the same piece, so Spacewave never stores duplicate data. When you change a file, only the pieces that actually changed need to be saved and synced.

This approach has a practical benefit: syncing a 100 MB file where you changed one paragraph is nearly instant, because only the changed portion transfers.

## How Sync Works

When you have the same space on multiple devices, they keep each other up to date. Your devices connect directly to each other over encrypted channels and exchange the pieces each one is missing.

This happens continuously in the background. Devices that are online at the same time see each other's changes almost immediately. If a device was offline, it catches up as soon as it reconnects. If your devices cannot connect directly (for example, behind strict network firewalls), an encrypted relay passes the data through without being able to read it.

## Space Lifecycle

You create a space from the dashboard. Over time, you add data, install plugins, and sync across devices. You can export a space as an archive for backup, or delete it when you no longer need it. Deleting a space removes it from the current device; other devices keep their copies until you delete those separately.
