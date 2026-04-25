---
title: How Spacewave Stores Data
section: settings
order: 3
summary: User-safe version of local-first storage model.
---

## What This Is

Spacewave is a local-first application. This means your data lives on your device first and syncs to other places second. Understanding where your data actually is helps you make good decisions about backups, device linking, and storage plans.

## How It Works

When you create a space, add files, or write notes, that data is saved to your device immediately. Nothing is sent to a server unless you have cloud sync enabled.

In the browser, data is stored in a private filesystem (OPFS) tied to the Spacewave domain. It survives reloads but can be cleared if you clear site data. In the desktop app, data lives as regular files you can back up with normal tools.

All data is encrypted on your device. If you enable cloud sync, only encrypted blocks are uploaded. The server cannot read your content.

Linked devices exchange data peer-to-peer. Each device keeps its own complete copy and continues working offline, syncing changes when reconnected.

## Why It Matters

Because your data is local-first, you get instant performance with no loading spinners and full offline capability. The tradeoff is that you are responsible for the durability of your local copy.

In the browser, clearing site data or switching browsers means your data is gone unless it was synced elsewhere. With the desktop app, a disk failure has the same consequence unless you have backups.

Cloud sync and device linking both create additional copies that protect against single-device loss. For data you care about, enable at least one of these.

## Next Steps

For information on plan choices and cloud storage, see [Plans and Storage Choices](/docs/users/sessions/plans-and-storage-choices). To link another device for redundancy, see [Link Devices](/docs/users/devices/link-devices). For self-hosters who want to understand backup boundaries in depth, see [Storage and Durability](/docs/self-hosters/operations/storage-and-durability).
