---
title: Backup and Recovery
section: storage
order: 2
summary: Separate auth recovery, session locks, Cloud recovery, and data transfer.
---

Spacewave recovery is several smaller mechanisms, not one universal restore
button.

## Backup keys

Cloud accounts can generate a PEM backup key through the account resource. Local
sessions can export a PEM through the local session provider. Keep the PEM away
from the device that holds the session.

The PEM is an authentication recovery key. It can help with sign-in, auth-method
recovery, and PIN reset flows. It does not contain the Space data itself.

## Session locks

Sessions can auto-unlock or use PIN-encrypted lock mode. PIN-encrypted sessions
ask for a PIN before mounting. If the PIN is lost, the reset flow asks for an
account password or backup key and creates a new session key.

## Cloud account recovery

Cloud recovery can request a recovery email for a verified email address, verify
the recovery token, and set a new password keypair. This recovers access to the
Cloud account. It is distinct from restoring a local browser state root.

## Data movement

The implemented data move path is session-to-session transfer. The transfer
wizard inventories the source session, lets you select Spaces, starts transfer
to a target session, watches progress, and resumes active or checkpointed work.

For local-only data, the state root remains the backup boundary. For Cloud data,
the Cloud provider is the backup and sync boundary.
