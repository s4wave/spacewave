---
title: Backup and Recovery
section: storage
order: 2
summary: Four separate recovery paths, and which one applies to what went wrong.
---

There is no single restore button. Recovery in Spacewave is four separate
mechanisms, and knowing which one applies is most of the work.

## Backup keys

Both a Cloud account and an account kept on this machine can write out a backup
key, a `.pem` file. Keep it somewhere other than the device it came from.

It gets you back in: signing in on a new device, adding another way to sign in,
resetting a forgotten PIN. It does not contain your Spaces, so it cannot restore
them.

## Locks

An account either unlocks as soon as you open the app, or asks for a PIN first.
Lose the PIN and the reset asks for your account password or your backup key
instead.

## Cloud account recovery

If you have a verified email address on a Cloud account, you can have a recovery
link sent to it, confirm it, and set a new password. This gets you back into the
account. It does nothing for data sitting in a browser on a machine you no
longer have.

## Moving data

The way to move Spaces between two places is a transfer. It lists what is on the
source, lets you choose what to move, runs, and can be resumed if it stops
partway.

So: on a disk, the directory is what you back up. On Cloud, Cloud is doing the
backing up. A backup key is neither, and a transfer is how data crosses between
them.
