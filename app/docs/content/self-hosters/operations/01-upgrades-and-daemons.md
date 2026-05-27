---
title: Upgrades and Daemons
section: operations
order: 1
summary: Keep runtime, daemon, socket, and state-path ownership explicit during operations.
---

Spacewave can be running as the desktop runtime, a foreground `spacewave serve` process, or a development daemon tied to a project-local state directory.

Before upgrading, know which runtime owns the daemon socket and which state path owns the session.

## Safe Upgrade Order

1. Check `spacewave status`.
2. Record the state path or socket path you are using.
3. Stop the daemon intentionally with `spacewave stop` or quit the desktop runtime.
4. Upgrade the app or binary.
5. Start the runtime again.
6. Run `spacewave status` and open one Space.

If `spacewave status` reports an older daemon, stop the daemon with the matching state path and retry with the new binary.

## Socket Ownership

`spacewave serve` listens on the socket inside the selected state path. It does not silently take over another runtime. Use takeover behavior only when you intentionally want the current runtime to yield.

Keep long-running automation pointed at a stable state path. Avoid scripts that depend on whichever daemon happens to answer first.
