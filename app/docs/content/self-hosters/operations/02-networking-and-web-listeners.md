---
title: Networking and Web Listeners
section: operations
order: 2
summary: Use localhost web listeners and daemon sockets without exposing more than intended.
---

The CLI includes a `web` command for localhost web listeners attached to the native runtime. Use it when you need a browser route backed by the running daemon.

```bash
spacewave web
spacewave web list
spacewave web stop <listener-id>
```

The listener is not a public hosting product by itself. Treat it as a local operational bridge.

## Socket Paths

Daemon sockets are local Unix sockets. They are controlled by the state path or an explicit `--socket-path`.

Use file permissions and user boundaries as part of your operational model. If a script can reach the socket, it can ask the daemon for Spacewave resources according to the session state behind that daemon.

## Public Access

Do not point public traffic at a local listener unless you have separately designed authentication, TLS, logging, and recovery.

For public docs readiness, see [Prerender and Public Docs](/docs/developers/platform/prerender-and-public-docs). That page describes the source and route boundary; it does not authorize live deployment.
