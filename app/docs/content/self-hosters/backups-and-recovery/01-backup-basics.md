---
title: Backup Basics
section: backups-and-recovery
order: 1
summary: Understand how Spacewave backups work and how to protect your data.
---

## Who This Is For

You are running Spacewave on your own hardware and want to make sure your data is safe if something goes wrong.

## What Gets Backed Up

Spacewave stores everything in a local block store: files, notes, space configurations, encryption keys, and session identity. A complete backup of this directory captures all of it. There is no separate database to worry about.

Data that exists only on other linked devices or Spacewave Cloud is not included. Ensure devices are synced before taking a backup if completeness matters.

## Backup Model

Spacewave does not include a built-in backup tool. You back up the data directory with standard OS tools (Time Machine, rsync, Borg, or similar). A daily backup is a reasonable starting point.

For browser-only sessions, there is no filesystem path to back up. The only way to create a durable copy is to link a desktop device or enable cloud sync.

Store backups on a separate physical device or remote location. A backup on the same disk does not protect against hardware failure.

## Restore

Replace the data directory with your backed-up copy and relaunch Spacewave. The session resumes at the backup point. Data created after the backup is lost unless synced to another device or cloud.

## Key Handling

Encryption keys live in the block store and are included in backups. Losing all copies of the block store with no cloud account means permanent key loss, and any data encrypted with those keys becomes unrecoverable.

Cloud sync stores your keys (encrypted) on the server side. Losing your device but retaining account access lets you restore keys through the cloud recovery flow.

## Next Steps

For advanced recovery scenarios and device migration, see [Backup Recovery and Device Moves](/docs/self-hosters/backups-and-recovery/backup-recovery-and-device-moves). For a deeper look at the backup model and restore paths, see [Backups and Recovery Guide](/docs/self-hosters/backups-and-recovery/backups-and-recovery-guide). For storage durability tradeoffs, see [Storage and Durability](/docs/self-hosters/operations/storage-and-durability).
