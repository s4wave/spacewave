# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

People who run Spacewave as their own private workspace in a browser, the
Electron desktop app, or a self-hosted daemon. They keep files, notes, apps,
devices, and workflows in Spaces and sync them between their own devices and
with people they invite. A second audience builds apps and plugins on the same
stack and inspects the running runtime while developing.

## Product Purpose

Spacewave turns a general purpose computer into a private, encrypted,
local-first cloud. It works offline with no account or server, syncs
peer-to-peer, and lets the user choose where data lives and how it moves.
Success means people trust the system because they can see and control what it
is doing.

## Positioning

Every Spacewave app runs on the user's own machine through the same visible
machinery: Sessions, Spaces, a controller bus with directives, plugins loaded by
a plugin host, a bifrost peer network, and a local block store. Nothing is a
hidden server; the user can look under the hood of any part of the system while
it runs.

## Operating Context

The app is a dense dark workbench: FlexLayout tabs, ObjectViewers, a bottom bar
of session-scoped status buttons, and overlays opened from that bar. Sessions
are addressed as `/u/<index>`. Live state flows from Go controllers through
streaming `Watch*` RPCs into React; the UI never polls.

## Capabilities and Constraints

- Runs in browsers (GoScript workers), Electron, and native daemons.
- Live system facts available to the UI: controllers, directives, plugin host
  instances, bifrost peers and links, session sync status and pack statistics,
  local block-store usage, browser storage estimate and persistence, launcher
  release and update state, plugin manifest recovery, boot and root-asset
  recovery reports, sessions, Spaces, tracked SDK resources, and UI state
  atoms.
- There is no log stream RPC yet; the UI must not show sample logs.
- Alpha software with no outside users; interfaces and stored data may change
  without migration.

## Brand Commitments

Spacewave by Aperture Robotics. Serious but not too serious, in the spirit of
Portal 2. Reliable, fast, sleek, customizable, understandable, state of the art.
Visual authority lives in `DESIGN.md`.

## Evidence on Hand

Real runtime data from the running app. No customers, benchmarks, or
testimonials may be invented.

## Product Principles

- The user owns the machine: show what is running and let them reach any layer.
- Truth over reassurance: every status reflects a live watch, never a guess.
- Plain language first, exact technical names one step deeper.
- Local-first: the system works and explains itself offline.

## Accessibility & Inclusion

Keyboard reachable navigation, reduced-motion support, and legible contrast on
the dark canvas.
