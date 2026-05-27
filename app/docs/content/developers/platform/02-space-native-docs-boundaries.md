---
title: Space-Native Docs Boundaries
section: platform
order: 2
summary: Separate bundled app docs from docs objects stored inside user Spaces.
---

There are two docs systems with different owners.

Bundled app docs are source files under `app/docs/content/`. They explain Spacewave itself and render through the app docs routes.

Space-native docs are user-owned objects inside a Space. The active notes plugin owns the `notes/docs` ObjectType and viewer behavior for that surface.

## Active and Legacy Boundaries

Use `notes/docs` for the active Space-native docs ObjectType.

The older `spacewave-docs/documentation` viewer is a legacy boundary. Do not use it as the public docs source or as proof of the current Space-native notes plugin unless a migration plan explicitly says to touch it.

## Verification

When changing bundled docs, verify `app/docs/content/`, the docs route, search, sidebar, and raw Markdown source links.

When changing Space-native docs, verify the notes plugin registration, object creation, viewer routing, and object persistence.

Do not patch one boundary to make the other appear correct. That creates stale docs and stale product behavior at the same time.
