---
title: Link Devices
section: devices
order: 1
summary: Link another device with a pairing code or a local direct exchange.
---

Device linking adds another peer to the current session. After both sides
confirm the same verification symbols, Spacewave persists the paired device and
grants the remote peer OWNER access on the session's SharedObjects.

## Pair with a code

The normal linking flow works for local and Spacewave Cloud sessions:

1. Open the link-device flow from session setup or account settings.
2. Choose **Generate code for another device** on the first device, or **Enter a
   code from another device** on the second device.
3. Enter the 8-character pairing code.
4. Compare the verification emoji shown on both devices.
5. Confirm the match on both sides.

The pairing code flow uses the session pairing service. It is the only device
linking option shown for Spacewave Cloud sessions.

## Pair directly without cloud

Local sessions also expose direct no-cloud pairing. One device creates a WebRTC
offer payload and shows it as copyable text or a QR-style payload. The other
device accepts the offer and returns an answer payload. Both devices still use
the same emoji verification step before the pairing is confirmed.

Direct pairing is local-provider only in the current app. Cloud sessions hide the
direct QR/paste controls.

## What becomes available

The linked peer can mount the same session-owned SharedObjects. You can later
unlink a paired device; unlinking removes the paired device and revokes its
SharedObject access. Spacewave-managed Devices are a separate feature: they are
world objects with status, capabilities, and setup state, not just linked login
sessions.
