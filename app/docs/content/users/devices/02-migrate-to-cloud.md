---
title: Migrate to Cloud
section: devices
order: 2
summary: Migration wizard steps, progress, completion, and crash recovery.
draft: true
---

## Overview

If you started with local storage and later subscribe to Spacewave Cloud, you can migrate your existing data to the cloud. Provider migration is currently being built. The settings page shows your current provider and will offer the full migration flow once it is ready.

## Prerequisites

You need an active Spacewave Cloud subscription and the local session containing your data, both accessible on the same device.

## Steps

1. When Spacewave detects an active subscription and a local session with data, it shows a migration decision page with two options: "Migrate my data to Cloud" or "Keep sessions separate."

2. "Migrate my data to Cloud" opens the migration settings page. The full transfer process will launch from here once available.

3. "Keep sessions separate" unlinks the local session from your cloud account. Both remain accessible independently.

4. The full migration wizard (when ready) will scan local spaces, show an inventory, copy blocks, and report per-space progress.

## Verify

After migration, open your spaces from the cloud session. Confirm that your files, notes, and settings are present. The local session can be removed once you are confident the migration completed successfully.

## Troubleshooting

If the migration page shows "coming soon," the full migration flow is not yet available for your session type. In the meantime, you can use the Transfer Sessions wizard to manually move spaces between sessions.

If migration is interrupted (browser crash, network loss), the transfer is designed to be resumable. Reopen the migration page to continue where it left off.

## Next Steps

For manual session transfers including merge, migrate, and mirror modes, see [Merge and Transfer Sessions](/docs/users/devices/merge-and-transfer-sessions). For information on storage providers and plans, see [How Spacewave Stores Data](/docs/users/settings/how-spacewave-stores-data).
