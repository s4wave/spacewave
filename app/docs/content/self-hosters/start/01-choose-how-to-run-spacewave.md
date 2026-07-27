---
title: Choose How to Run Spacewave
section: start
order: 1
summary: Pick browser, desktop, daemon, or Cloud by deciding who holds the data.
---

Spacewave runs in several shapes. The question that picks one for you is who
holds the data and who is responsible for getting it back.

## In a browser

The quickest start, and the only one that needs no install. Your Spaces go into
browser storage on this machine.

Browsers clear their own storage when disk runs low, without asking. Use this to
try things, never as the only copy of anything.

## In the desktop app

The desktop app keeps a state directory on your own disk, which you back up like
any other directory. It can also open a `.spacewave` directory you already have.

This is the local mode to choose when you want the data on a filesystem you
control.

## As a background service

```sh
spacewave serve
```

`serve` runs in the background and listens on a Unix socket, `spacewave.sock`,
inside the state directory. The `spacewave` commands connect to that socket, and
will start one for you if none is running.

## Reachable from a browser

```sh
spacewave web
```

This opens a local address you can visit in a browser, backed by the copy of
Spacewave running on this machine. Run it in the foreground and it lasts as long
as the command; run it with `--background` and it stays with the service, where
you can list and stop it later.

## In Spacewave Cloud

Cloud holds the data for you: encrypted storage, sync between devices, backup,
shared Spaces, and billing. Signing into Cloud does not move anything on its
own. Work you created locally stays your responsibility until you transfer it.
