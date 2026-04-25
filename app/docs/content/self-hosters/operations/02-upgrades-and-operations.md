---
title: Upgrades and Operations
section: operations
order: 2
summary: Upgrade model, manifest-driven updates, and operational expectations.
---

## Overview

Spacewave uses release artifacts plus independently loaded web/plugin bundles. Understanding this model helps you plan maintenance windows and know what to expect when a new release ships.

## How Updates Work

The desktop app ships as a lightweight binary. Web plugins and UI bundles are loaded independently from the native shell.

Plugin updates can apply without replacing the desktop binary. Binary updates require installing or launching a newer build.

## What This Means for Self-Hosters

In the browser, updates happen automatically when you reload. The service worker fetches the latest version on the next navigation.

For desktop deployments, versioned artifacts are published for manual download.

There is no separate server process to upgrade. The application is self-contained.

## Verify

After an update, check the version displayed in the app settings. Plugin versions are visible in the plugin list. If the version numbers match the expected release, the update succeeded.

## Troubleshooting

If a desktop download fails, check network connectivity and confirm outbound HTTPS is allowed. For browser deployments, a hard refresh bypasses the service worker cache and forces a re-fetch of the latest version.

## Next Steps

For information on data durability across updates, see [Storage and Durability](/docs/self-hosters/operations/storage-and-durability). To understand backup strategy before major upgrades, see [Backup Basics](/docs/self-hosters/backups-and-recovery/backup-basics).
