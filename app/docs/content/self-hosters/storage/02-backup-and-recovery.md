---
title: Backup and Recovery
section: storage
order: 2
summary: Back up the owning session and recovery keys before changing runtimes or devices.
---

A useful Spacewave backup has two parts:

- Session recovery material, such as backup keys or auth methods.
- The storage backing the Spaces you care about.

Do not keep both parts only inside the same local session.

## Before a Risky Change

Before upgrades, device moves, or storage migrations:

1. Identify the active state path or provider account.
2. Confirm `spacewave status` shows the expected session.
3. Open the Space in the app.
4. Verify one important object.
5. Store recovery material outside Spacewave.

## Recovery Practice

Do a small recovery rehearsal before the first real incident. Use a non-critical Space, a new state directory, or another device. Confirm you can authenticate and open the Space.

## What Backups Do Not Fix

A backup cannot recover data that never reached the storage you backed up. If a file is still in an upload flow, wait for the Drive or object viewer to show it before considering it protected.
