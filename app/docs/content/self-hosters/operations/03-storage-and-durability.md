---
title: Storage and Durability
section: operations
order: 3
summary: Storage durability, backup boundaries, and remote access limits.
---

## What This Is

Spacewave is local-first, which means your data lives on your device before it goes anywhere else. As a self-hoster, you are responsible for the durability of that local copy. This page explains what guarantees the application provides, where those guarantees end, and what you need to do to fill the gaps.

## How It Works

Each session stores data in a content-addressed block store. In the browser this uses OPFS and IndexedDB. In the desktop app it uses the native filesystem.

Replication is not backup. If you delete a space, that deletion propagates to all synced copies. Backups must be taken independently of sync to protect against accidental deletion or corruption.

The application does not perform automatic local backups. Browser storage is subject to eviction under quota pressure. Filesystem storage is as durable as the underlying disk.

## Why It Matters

The durability of your data depends entirely on where the session runs and what backup measures you have in place. Here is a rough ranking from least to most durable:

**Browser storage** is the least durable. Clearing site data, switching browsers, or browser storage quota enforcement can destroy everything with no recovery path.

**Desktop app on a single disk** is durable against browser-level issues but not against hardware failure. A disk crash loses everything unless you have filesystem-level backups.

**Desktop app with filesystem backups** (Time Machine, rsync, etc.) gives you point-in-time recovery. This is the recommended self-hosted baseline.

**Cloud sync** adds geographic redundancy. Your data is encrypted and stored on remote infrastructure. Combined with local backups, this provides strong durability.

If you are running Spacewave for anything you cannot afford to lose, you need at least one backup layer beyond the primary storage device.

## Next Steps

To set up a backup strategy, see [Backup Basics](/docs/self-hosters/backups-and-recovery/backup-basics). For a comparison of storage tiers, see [Browser Storage vs Desktop vs Cloud](/docs/self-hosters/start-here/browser-storage-vs-desktop-vs-cloud). For the user-facing explanation of how data is stored, see [How Spacewave Stores Data](/docs/users/settings/how-spacewave-stores-data).
