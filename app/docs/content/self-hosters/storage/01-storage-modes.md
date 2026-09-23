---
title: Storage Modes
section: storage
order: 1
summary: Know which disk or account holds each Space, and who backs it up.
---

A Space is a container for one project in Spacewave. For each Space, one place
holds the data and one party is responsible for backing it up. This page tells
you which place and which party for each way of running Spacewave.

## Browser storage

Work started in a browser without an account is stored in that browser. There
is nothing to set up, and nothing you can back up from outside the browser. The
browser may clear it when disk space runs low.

Use browser storage to try Spacewave. Do not leave anything there that you
would miss.

## A state directory on disk

The desktop app keeps its data in a state directory. You can choose where it
is, back it up, and copy it. In this mode, ordinary disk backups work as you
expect.

The desktop app can also add an existing `.spacewave` state directory with
**Add state root**. Opening a single `.s4wave` file is not available in the app
yet.

## The background service

`spacewave serve` chooses a state directory from its flags and environment:

- `--state-path` or `-s`;
- otherwise `SPACEWAVE_STATE_PATH`, `SPACEWAVE_DATA_DIR`, or `BLDR_STATE_PATH`;
- otherwise `~/.spacewave` on Linux and macOS, or a `spacewave` directory in
  the user configuration directory on other systems.

The service listens on `spacewave.sock` inside that directory. The socket is
how commands reach the service. The directory is where the data lives. Back up
the directory.

## Spacewave Cloud

Spacewave Cloud is Spacewave's hosted service. It stores your data encrypted
and adds sync between devices, backup, and shared Spaces. Space usage figures
come from wherever the data is stored. They appear for Cloud and may not appear
for other storage.

## What a backup key is not

A backup key gets you back into an account. It is not a copy of your Spaces.
Back up whatever holds the data: the state directory, or Cloud, which does it
for you.
