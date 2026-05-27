---
title: Resource SDK and Watch RPCs
section: sdk
order: 1
summary: Use SDK resources and watch RPCs for live state instead of polling from React or CLI code.
---

The Resource SDK wraps runtime resources exposed by the daemon and app. Use it when code needs typed access to sessions, Spaces, objects, and services.

For state that changes over time, prefer watch or streaming RPCs. Do not add repeated timer calls in the UI or a polling loop in Go when the owner can publish state changes.

## Shape

A typical flow is:

1. Mount or access the resource owner.
2. Subscribe to a watch or stream for changing state.
3. Render from the current snapshot.
4. Release resources when the component or command is done.

## Why It Matters

Polling hides ownership. Every caller chooses its own interval, retry behavior, and stale-state rules.

A watch RPC keeps lifecycle rules near the state owner. The caller receives updates and handles render or output formatting.

## CLI and App

The CLI also uses daemon resources. If a CLI command needs live changes, prefer an explicit `--watch` path backed by the owner instead of shell-level loops around a snapshot command.
