---
title: Browser Storage vs Desktop vs Cloud
section: start-here
order: 2
summary: Storage durability tradeoffs and recommended upgrade paths.
---

## What This Is

Spacewave offers browser, desktop, and Cloud storage modes, each with different durability and convenience tradeoffs. This choice determines where your data physically lives and what happens if that device is lost.

Understanding these tiers matters because moving data later requires a transfer. Picking the right starting point avoids unnecessary moves.

## How It Works

**Browser storage** uses your browser's built-in persistence (OPFS and IndexedDB). Data survives page reloads but the browser can evict it under storage pressure. Clearing site data removes everything. Free, instant, no installation. Best for trying Spacewave or disposable use cases.

**Desktop app** stores data on your filesystem with real file-level persistence. Your OS backup tools (Time Machine, rsync) can protect this data.

**Cloud** adds encrypted backup and cross-device sync. Data is encrypted on your device before upload. Cloud is a paid tier ($8/month) with 100 GB storage, always-on sync, and shared spaces. You can cancel and migrate off at any time.

Local and Cloud sessions share the full app, plugin SDK, and peer-to-peer sync.

## Why It Matters

Browser storage can be wiped by clearing site data, quota enforcement, or switching browsers. If your workflow depends on Spacewave, you need the desktop app or cloud backup to protect against data loss.

For self-hosted durability without Cloud, use the desktop app with filesystem backups. For managed durability, Cloud is simplest.

## Next Steps

To understand backup boundaries and what you can recover, see [Backup Basics](/docs/self-hosters/backups-and-recovery/backup-basics). To compare deployment setups in more detail, see [Choose Your Deployment](/docs/self-hosters/deployment-modes/choose-your-deployment). For information on how data is stored locally, see [How Spacewave Stores Data](/docs/users/settings/how-spacewave-stores-data).
