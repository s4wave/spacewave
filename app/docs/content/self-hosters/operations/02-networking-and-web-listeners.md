---
title: Networking and Browser Access
section: operations
order: 2
summary: What Spacewave listens on, how to open it in a browser, and how devices connect when you link them.
---

A self-hosted Spacewave carries traffic over three paths: the local socket that
commands use, the local web address you open in a browser, and the direct
connection two devices make when you link them. None of them is exposed to the
public internet.

## The local socket

Commands reach the Spacewave background service over a Unix socket on the same
machine. The socket is not on the network. If you run more than one copy of
Spacewave, pass the state directory or the socket path explicitly, so a command
cannot reach the wrong copy. [Run the Background
Service](/docs/self-hosters/operations/upgrades-and-daemons) covers both flags.

## The local web address

`spacewave web` serves this machine's Spacewave at a local address. It prints a
URL that ends in `#otp=` followed by a one-time secret. The secret is what lets
the browser in, so do not share the URL.

```sh
spacewave web
spacewave web --port 8080
spacewave web --background
spacewave web list
spacewave web stop <listener-id>
```

The main options:

- `--host` sets the address to bind. It accepts only `localhost` or a loopback
  address, and the default is `127.0.0.1`.
- `--port` sets the port. The default, `0`, picks a free port.
- `--listen` takes a full listen address as a multiaddr and overrides `--host`
  and `--port`.
- `--background`, or `--bg`, keeps the address open in the background service
  after the command exits. Without it, the address stays open until you stop
  the command.
- `--print-url` prints only the URL, which helps in scripts.
- `--display` opens the item at the given path in kiosk display mode.

`web list` shows the addresses kept open in the background, and `web stop`
closes one by its ID. An open background address keeps the service from
stopping when idle.

The web address is for a browser on this machine. There is no supported way
today to expose it publicly, and no setting for relay servers.

## Linking two devices

When you link two devices directly with a QR code, they exchange connection
details by scanning or by copy and paste. Each side first collects the network
routes it can be reached on. Then both sides confirm the same row of emoji
before the link is made.

Direct linking works for both local and Spacewave Cloud accounts. A Cloud
account still contacts Spacewave Cloud to approve the new session. [Link
Devices](/docs/users/devices/link-devices) walks through each method.

## Everything else

You may notice other connections, for example between the desktop app and its
embedded browser. They are internal. There is nothing to configure there, and
no command exposes them.
