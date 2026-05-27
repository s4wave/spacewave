---
title: Backup and Lock Setup
section: accounts
order: 2
summary: Protect a local or cloud session before it holds work you cannot recreate.
---

Do backup and lock setup before a Space contains important data.

The setup flow is about two things:

- A recovery path, such as a backup key.
- A lock mode, such as auto-unlock or PIN lock.

Auto-unlock is convenient on a trusted device. PIN lock adds a local unlock step. Pick the mode that matches the device, then record the backup material somewhere outside the Spacewave session.

## Backup Key

If the setup flow offers a backup key, download it and store it outside the browser profile. Do not save the only copy inside the same Spacewave session it is meant to recover.

The CLI exposes backup-key commands under `spacewave auth method add backup`, but most users should use the app setup flow first.

## Setup Banner

For local sessions, Spacewave can surface setup guidance after the session has meaningful local data. Do not treat the banner as decoration. It appears because the risk of losing local state now matters.

## Later Changes

You can return to session settings to change lock mode or manage authentication methods. If you are preparing a device move, read [Move to Cloud](/docs/users/devices/move-to-cloud) before removing the old session.
