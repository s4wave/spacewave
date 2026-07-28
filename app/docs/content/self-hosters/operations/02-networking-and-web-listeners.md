---
title: Networking and Web Listeners
section: operations
order: 2
summary: What Spacewave listens on, what it does not, and how devices reach each other.
---

Three things carry traffic in a self-hosted setup: the local socket, the web
address you can open in a browser, and the direct connection two devices make
when you pair them.

## The local socket

Commands reach Spacewave over a Unix socket on the same machine. Nothing about
it is on the network. If you run more than one copy of Spacewave, name the
directory or the socket explicitly so a command cannot reach the wrong one.

## The local web address

```sh
spacewave web
```

By default this binds to localhost on a free port and prints a URL carrying a
one-time secret, which is what lets the browser in. You can give it a host and
port, or a full listen address, run it in the foreground, or keep it with the
service using `--background`.

It is for reaching this machine from a browser on this machine. There is no
supported way today to expose it publicly, and no place to configure relay
servers for it.

## Pairing two devices

Pairing over the local network has the two devices exchange connection details
directly, by copy and paste or by scanning a code. Each side collects its
possible network routes first, then both sides confirm the same row of emoji
before the link is made.

This direct form is only offered for accounts kept on the device. Cloud accounts
pair with a code instead.

## Everything else

Other connections you may notice, between the app and its embedded browser or
inside the test suite, are internal plumbing. Nothing there is a surface to
configure, and no command exposes it.
