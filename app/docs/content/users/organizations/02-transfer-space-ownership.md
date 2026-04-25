---
title: Transfer Space Ownership
section: organizations
order: 2
summary: Personal-to-org and org-to-personal transfer flows.
---

## Overview

You can move a space from your personal account to an organization, or from an organization back to your personal account. Transfers change who owns the space without affecting the data inside it.

## Prerequisites

- You must be the **owner** of the organization you are transferring to or from.
- The space must already exist and be accessible from your dashboard.
- If transferring to an organization, that organization must already be created.

## Steps

1. Open the space's settings panel. You can reach it from the bottom bar or the space dashboard.
2. Find the **Transfer** section. This shows a dropdown listing your personal account and all organizations you own.
3. Select the destination. Choose an organization name to move the space there, or choose **Personal** to move it back to your personal account.
4. Click **Transfer**. The space moves to the new owner immediately.

The transfer dialog prevents no-op transfers. If the space is already owned by the selected destination, the Transfer button stays disabled.

## Verify

After transferring, open the destination's dashboard. The space should appear in the Spaces list of the target organization, or in your personal space list if you transferred it back.

## Troubleshooting

- **Transfer button is disabled.** You selected the same owner the space already belongs to. Pick a different destination.
- **Organization not listed.** You are not the owner of that organization. Only organization owners can receive transfers.
- **Space still appears in the old location.** Refresh the page. The change takes effect immediately on the server, but the UI may need a moment to update.

## Next Steps

- [Understand organizations](/docs/users/organizations/organizations)
- [Delete an organization](/docs/users/organizations/organizations) (see the Danger Zone section in Settings)
