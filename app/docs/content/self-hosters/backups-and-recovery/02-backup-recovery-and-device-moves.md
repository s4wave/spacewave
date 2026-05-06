---
title: Backup Recovery and Device Moves
section: backups-and-recovery
order: 2
summary: Backup boundaries, restore paths, and device migration from ops perspective.
---

## Overview

When you move Spacewave to a new device or recover from a backup, some data travels with the backup and some relationships need to be re-established.

## Prerequisites

You need a backup of the block store data directory (see [Backup Basics](/docs/self-hosters/backups-and-recovery/backup-basics)). For cloud-backed sessions, your account credentials are sufficient to restore account access and synced metadata without a local backup. A newly restored Session may still need to connect its Session key to individual Spaces before they open.

## Recovery from Backup

Copy the backed-up data directory to the new device and launch Spacewave. The session resumes at backup time. Data created after the backup is lost unless synced elsewhere. Linking the restored session to devices that have newer data will sync the missing pieces.

## Device Migration

If the old device is functional, link the new device to it. Peer-to-peer sync transfers all data without needing a filesystem backup. Once synced, decommission the old device.

If the old device is gone, restore from backup or sign in to cloud. Local-only sessions require a filesystem backup. Cloud-backed Sessions may show connected-space progress on the dashboard after sign-in while Spacewave connects the new Session key to Spaces you can already read.

## Re-establishing Linked Devices

A restored session will not reconnect to previously linked devices automatically. Device pairings are cryptographic relationships. After restoring, re-link devices through the pairing flow. See [Link Devices](/docs/users/devices/link-devices).

## Troubleshooting

If a restored session shows no spaces, verify the backup includes the complete data directory. If linked devices refuse to pair, remove the old pairing on both sides and re-pair from scratch. If cloud recovery fails, confirm credentials are correct. If a specific Space asks for unlock after the dashboard loads, complete that prompt so Spacewave can connect this Session key to that Space and continue the background connection for the rest.

## Next Steps

For a comprehensive guide to backup strategy and key handling, see [Backups and Recovery Guide](/docs/self-hosters/backups-and-recovery/backups-and-recovery-guide).
