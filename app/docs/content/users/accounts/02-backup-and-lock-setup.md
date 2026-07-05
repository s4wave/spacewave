---
title: Backup and Lock Setup
section: accounts
order: 2
summary: Protect a session with a backup key and a clear lock mode.
---

Spacewave has two separate protection tools: backup keys and session locks. A
backup key is an authentication key in PEM format. It is not a copy of every
file, block, or Space.

## Download a backup key

Cloud accounts generate a backup PEM through the account resource. Local
sessions export a PEM through the local session provider and require a password
for the export. Store the PEM outside the device that holds the session.

Use a backup key to sign in with `spacewave login --pem-file`, add or recover an
auth method, or reset a locked session when the reset flow asks for account
credentials. Do not treat the PEM as a full data archive.

## Choose a lock mode

Auto-unlock stores the session key on disk and opens without a PIN. PIN lock
encrypts the session key with a PIN and asks for that PIN on launch or mount.
The settings page lets you switch between these modes and change the PIN.

If you forget a PIN, Spacewave can reset the local session key after you
re-authenticate with an account password or backup key. Resetting a PIN-locked
session generates a new session key.

## Long-lived work

Before keeping real work in Spacewave, do all three checks:

1. Download a backup PEM and store it separately.
2. Pick auto-unlock or PIN lock intentionally.
3. Use Cloud or a desktop state root for data you need to keep.

Cloud sync and backup protect cloud-backed data. Local browser storage still
belongs to the browser and operating system.
