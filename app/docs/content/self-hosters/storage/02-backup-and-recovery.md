---
title: Backup and Recovery
section: storage
order: 2
summary: Four separate ways to recover, and which one fits what went wrong.
---

Spacewave has no single restore button. It has four separate ways to recover,
and each one fixes a different problem. Most of the work is picking the right
one.

## Backup keys: get back into an account

Both Spacewave Cloud accounts and accounts kept on one machine can save a
backup key, a `.pem` file. Store it somewhere other than the device that made
it.

A backup key gets you back in. With it you can sign in on a new device, add
another way to sign in, or reset a forgotten PIN. It does not contain your
Spaces, the containers that hold your work, so it cannot restore them.

## Locks: reset a forgotten PIN

An account either unlocks as soon as you open the app, or asks for a PIN first.
If you forget the PIN, the reset asks for your account password or your backup
key instead. An account with neither cannot reset its PIN.

## Cloud account recovery: reset a password

If a Cloud account has a verified email address, you can have a recovery link
sent to it, confirm it, and set a new password. This gets you back into the
account. It does nothing for data stored in a browser on a machine you no
longer have.

## Transfers: move data between places

A transfer moves Spaces from one place to another, such as from this device to
Cloud. It lists what is on the source and lets you choose what to move. If it
stops partway, it resumes where it stopped.

## Which one to use

For data in a state directory on disk, back up the directory. For data in
Cloud, Cloud backs it up for you. A backup key is neither of these. A transfer
is how data moves between them.
