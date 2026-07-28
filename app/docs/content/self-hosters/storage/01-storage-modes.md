---
title: Storage Modes
section: storage
order: 1
summary: Know which disk or account actually holds each Space, and who backs it up.
---

Choosing a storage mode is choosing who holds the data. For each Space, one
thing holds the bytes and one person is responsible for getting them back. This
page tells you which is which.

## Browser storage

Work started in a browser without an account lives in that browser's storage.
Nothing to set up, and nothing you can back up from outside the browser. The
browser is allowed to clear it when disk runs low.

Fine for trying Spacewave. Not a place to leave anything you would miss.

## A directory on disk

The desktop app keeps its data in a directory you can point at, back up, and
copy. This is the mode where ordinary disk backups do what you expect. It can
also open a directory that already holds Spacewave data.

Other kinds of storage location exist in the protocol, including a
browser-granted directory and a single-file store, but neither is usable yet.

## The background service

`spacewave serve` picks a state directory from its flags and environment, then
listens on `spacewave.sock` inside it. Keep those two apart in your head: the
socket is how you reach the service, and the directory is where the data lives.
Back up the directory.

## Spacewave Cloud

Cloud holds the blocks for you, encrypted, and gives you sync between devices,
backup, and shared Spaces. Space usage figures come from whatever holds the
data, so they appear for Cloud and may not appear elsewhere.

## What a backup key is not

A backup key gets you back into an account. It is not a copy of your Spaces.
Whatever holds the blocks, a disk or Cloud, is what has to be backed up.
