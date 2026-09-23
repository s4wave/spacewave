---
title: Choose How to Run Spacewave
section: start
order: 1
summary: Pick a browser, the desktop app, a background service, or Spacewave Cloud by deciding who holds the data.
---

Spacewave keeps your work in Spaces. A Space is a container for one project,
and its data has to be stored somewhere. You can run Spacewave in several ways.
Choose by asking who holds the data and who is responsible for getting it back.

## In a browser

A browser is the quickest start and needs no install. Your Spaces are stored in
that browser's storage on this machine.

Browsers clear their own storage when disk space runs low, and they do not ask
first. Use a browser to try Spacewave, never as the only copy of anything.

## In the desktop app

The desktop app keeps a state directory on your own disk. The state directory
holds your accounts and Spaces. You back it up like any other directory. The
app can also open a `.spacewave` state directory you already have.

Choose the desktop app when you want the data on a filesystem you control.

## As a background service

```sh
spacewave serve
```

`spacewave serve` runs Spacewave as a background service, also called a
daemon. It listens on a Unix socket named `spacewave.sock` inside the state
directory. Other `spacewave` commands connect to that socket. If no service is
running, they start one.

By default the state directory is `~/.spacewave` on Linux and macOS. [Run the
Background Service](/docs/self-hosters/operations/upgrades-and-daemons) covers
flags, shutdown, and replacing a running service.

## Reachable from a browser

```sh
spacewave web
```

`spacewave web` serves the Spacewave running on this machine at a local address
that you open in a browser on the same machine. [Networking and Browser
Access](/docs/self-hosters/operations/networking-and-web-listeners) explains
its options.

## In Spacewave Cloud

Spacewave Cloud is Spacewave's hosted service. It holds the data for you and
adds encrypted storage, sync between devices, backup, shared Spaces, and
billing. Signing in to Cloud does not move anything by itself. Work you created
locally stays your responsibility until you transfer it to Cloud.
