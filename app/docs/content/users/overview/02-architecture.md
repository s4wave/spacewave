---
title: How Your Data Moves
section: overview
order: 2
summary: What makes Spacewave fast, private, and resilient when you're collaborating across devices.
---

## Why This Matters

You don't need to know how Spacewave is built to use it. But a few design decisions shape what you can and can't do with the app, and they're worth understanding in plain terms.

## Offline First, Sync When You're Connected

Your spaces live on your device. When you open a file, browse a repository, or edit a canvas, the app is reading and writing locally. Nothing waits on a server round-trip.

When another of your devices comes online, the two compare notes and exchange only what's new. This is why syncing feels instant even on slow networks, and why Spacewave keeps working on the train, on a plane, or when the coffee shop Wi-Fi dies.

## Efficient Sync on Big Files

Instead of sending whole files back and forth, Spacewave only sends the parts that changed. Rename a 2 GB video and nothing re-uploads. Edit one frame and only that frame syncs. This keeps large spaces practical across slow or metered connections.

## Direct Device-to-Device Connections

When possible, your devices talk to each other directly, even on the same Wi-Fi network. If a direct connection isn't possible (strict firewalls, NAT, captive portals), an encrypted relay forwards the traffic without being able to read it. Either way, every connection is end-to-end encrypted.

## Where Your Data Lives

Depending on where you use Spacewave, your local storage is slightly different:

- **Browser.** Data is stored in the browser's built-in storage (OPFS), separate from other sites.
- **Desktop app.** Data lives in a local database on your machine for faster access and higher durability.
- **Cloud backup (optional).** Encrypted copies go to the cloud so you can recover if you lose a device. The cloud only ever sees scrambled blocks.

All three storage modes behave the same way from your perspective. See [How Spacewave Stores Data](/docs/users/settings/how-spacewave-stores-data) for a deeper look.

## Same App Everywhere

Browser, desktop, command line — it's the same code underneath. Your spaces, settings, and linked devices behave the same way on each. Multiple browser tabs on the same computer share one backend, so you're not duplicating work or memory.

## Plugins in Their Own Sandboxes

Data tools such as the file browser, Git viewer, and Canvas run through isolated viewer and plugin surfaces. A tool only gets access to the space where it is active, not every space or account on the device.

## Next Steps

- [What happens when you use Spacewave](/docs/users/overview/how-it-works)
- [Understanding spaces](/docs/users/spaces/understanding-spaces)
- [How Spacewave stores data](/docs/users/settings/how-spacewave-stores-data)
