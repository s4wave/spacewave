---
title: Upgrades and Daemons
section: operations
order: 1
summary: Run the background service, point it at the right directory, and hand it over cleanly.
---

The `spacewave` command and the desktop app meet at one Unix socket,
`spacewave.sock`, which sits inside whichever state directory is in use. Almost
everything on this page follows from that.

## Choosing the directory

Pass `--state-path`, or set the state-path environment variable, when you want a
specific directory. Commands find the socket inside it, and start the service if
none is running.

Pass `--socket-path`, or set `SPACEWAVE_SOCKET_PATH`, when you want to reach one
exact socket you already know about. That form connects to what is there and
starts nothing.

A socket left behind by a service that is gone gets cleaned up before a new one
starts, so a crashed process does not block the next run.

## Running it

```sh
spacewave serve
spacewave serve --takeover
spacewave stop
```

`serve` creates the directory if it needs to, binds the socket with the right
permissions, brings up plugins and device support, and removes the socket when
it exits.

`--takeover` asks whatever currently owns the socket to step aside first. Use it
when you know something else is running and you want this process to win.

`stop` asks the running service to shut down, and does nothing quietly if there
is none.

## When the desktop app is also running

If a command asks to take over while the desktop app owns the socket, the app
shows a prompt and hands control across. While a handover is in progress the
app's own local actions are unavailable, and its banner is how you take control
back.

## Browser access

```sh
spacewave web
spacewave web list
spacewave web stop <listener-id>
```

`spacewave web` opens a local address for reaching this machine's Spacewave from
a browser. Add `--background` and it stays with the service after the command
returns, where `list` and `stop` can manage it.
