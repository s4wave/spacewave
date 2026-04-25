---
title: Merge and Transfer Sessions
section: devices
order: 3
summary: Merge, fork, and upgrade flows for empty vs non-empty local sessions.
---

## Overview

The Transfer Sessions wizard lets you move spaces between sessions on the same device. This is useful when you want to consolidate data from multiple sessions, move spaces between a local and cloud session, or create a copy of your spaces in a second session.

## Prerequisites

You need at least two sessions on the same device. Open the Transfer Sessions page from session settings. The wizard will show all available sessions as source and target options.

## Steps

1. Open Transfer Sessions from session settings. Select a source session (where spaces come from) and a target (where they go).

2. Choose a transfer mode:
   - **Merge** moves all spaces to the target and deletes the source afterward.
   - **Migrate** moves all spaces and transfers the session keypair to a different provider.
   - **Mirror** copies all spaces to the target without deleting the source.

3. Review the inventory listing all spaces to transfer. Start the transfer.

4. The progress screen shows per-space status with block-level progress. You can cancel during transfer if needed.

5. When complete, the wizard confirms how many spaces transferred and lets you navigate to the target session.

## Verify

Open the target session after transfer. Confirm that all spaces appear with their files and content intact. For merge mode, verify that the source session has been removed from your session list.

## Troubleshooting

If the source session shows "No spaces found," it may have already been transferred or may be empty. Check both sessions to confirm.

If transfer fails partway through, completed spaces remain in the target. Failed spaces stay in the source. Retry for the remaining spaces.

If you chose merge by mistake, use mirror next time. Merge is destructive to the source.

## Next Steps

To link a new device instead of transferring between sessions, see [Link Devices](/docs/users/devices/link-devices).
