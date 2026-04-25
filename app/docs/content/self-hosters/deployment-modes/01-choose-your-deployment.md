---
title: Choose Your Deployment
section: deployment-modes
order: 1
summary: Compare deployment modes and pick the right setup for your use case.
---

## What You Need to Decide

Spacewave runs wherever you want it. The deployment mode affects how data is stored, who can access it, and how much operational overhead you take on.

## Deployment Options

Today Spacewave runs as a client-side application (browser or desktop app), not a standalone server. The modes below describe where that client runs and how you protect its data.

### Local Only (Browser)

Open Spacewave in any modern browser. Data uses browser-managed storage (OPFS/IndexedDB). No installation, no account, no cost. The tradeoff is durability: browser storage can be cleared accidentally. Ideal for evaluation or disposable projects.

### Desktop App

Stores data on your filesystem so you can back it up with standard OS tools. This is the recommended self-hosted starting point for single-user setups that need more durability than browser storage.

## Which Should I Choose?

If you are just starting, use the browser and move to desktop or Cloud later. If you need durability without managing backups, Cloud is simplest. If you want self-hosted durability, use the desktop app and back up its data directory.

The key question is operational overhead. Browser and Cloud require the least maintenance. Desktop requires backup discipline.

## Next Steps

For storage durability tradeoffs between these modes, see [Browser Storage vs Desktop vs Cloud](/docs/self-hosters/start-here/browser-storage-vs-desktop-vs-cloud). To plan your backup strategy, see [Backup Basics](/docs/self-hosters/backups-and-recovery/backup-basics). For general self-hosting orientation, return to [Getting Started](/docs/self-hosters/start-here/getting-started).
