---
title: Backups and Recovery Guide
section: backups-and-recovery
order: 3
summary: Backup model, restore paths, and key handling.
---

## Overview

This guide consolidates the backup and recovery model for self-hosted Spacewave. It covers what to back up, how to restore, and how encryption keys interact with the process.

## The Backup Boundary

The block store data directory is the unit of backup. It contains all session data: spaces, files, encryption keys, and identity. A complete copy is a complete backup. For maximum coverage, maintain both a local backup and cloud sync.

## Backup Schedule

A daily backup is a reasonable default. Use incremental tools (rsync, Borg, restic) to keep sizes manageable. The block store is content-addressed, so unchanged blocks produce identical files across snapshots.

## Restore Paths

**From local backup:** Copy the data directory to the Spacewave data location and relaunch. Sync with linked devices or cloud to recover data created after the backup.

**From cloud:** Sign in on a new device. Cloud recovery restores account access and synced metadata. Some Spaces may still need to connect the new Session key before they open. Spacewave shows that as background connected-space status on the dashboard and asks you to unlock only when a Space needs it.

**From a linked device:** Link the new device to one that has current data. Peer-to-peer sync transfers everything.

## Key Handling

Encryption keys are part of the block store and travel with backups. If you lose all copies and have no cloud account, the keys are permanently lost and encrypted data becomes unrecoverable.

To protect against total key loss, maintain at least one off-site backup or use cloud sync. Cloud-stored keys are protected by your account credentials.

## Verify a Backup

Periodically test by restoring to a separate location and launching Spacewave against the copy. Verify that spaces, files, and settings appear correctly. An untested backup is not a backup.

## Troubleshooting

If a restored session appears empty, check that the entire data directory was copied. If encryption fails after restore, try an earlier snapshot. For cloud-backed sessions, cloud recovery is an independent path. If the dashboard loads but some Spaces show connected-space progress or ask for unlock when opened, let the background connection finish or complete the unlock prompt for that Space.

## Next Steps

For backup fundamentals, see [Backup Basics](/docs/self-hosters/backups-and-recovery/backup-basics). For device migration scenarios, see [Backup Recovery and Device Moves](/docs/self-hosters/backups-and-recovery/backup-recovery-and-device-moves). For storage durability tradeoffs, see [Storage and Durability](/docs/self-hosters/operations/storage-and-durability).
