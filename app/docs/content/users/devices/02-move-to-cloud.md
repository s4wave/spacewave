---
title: Move to Cloud
section: devices
order: 2
summary: Move from local work to Cloud with linked sessions and explicit transfers.
---

Moving to Cloud is not a magic device migration. Current Spacewave behavior is a
set of explicit flows: create or sign into a Cloud account, link or mount a
local session, transfer selected Spaces between sessions, and keep a backup key.

## Start from the Cloud account

Sign into or create a Spacewave Cloud account from the account flow. Cloud adds
encrypted storage, sync, backup, shared Spaces, and multi-device access. Local
data stays local until you move it.

If the app detects a linked local session with content, the migration decision
screen can send you to the migration settings page or keep the sessions
separate. Keeping sessions separate unlinks the local session from the Cloud
flow.

## Transfer selected Spaces

The migration settings page uses the transfer wizard. It inventories the source
session, lets you choose Spaces, starts a transfer from source session to target
session, watches progress, and can resume active or checkpointed transfers.

Transfer is the implemented data-move path. A backup PEM helps you recover
account access; it is not a full local-data restore archive.

## Link devices after the move

After the Cloud session owns the Spaces you need, link other devices to that
session with a pairing code. Spacewave Cloud sessions do not expose the direct
no-cloud pairing path. Use account session settings to revoke old cloud sessions
or unlink old local linked sessions.

If a paid Cloud plan is canceled, current product copy says full access lasts
until the end date and cloud data becomes read-only for 30 days afterward.
