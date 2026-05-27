---
title: Build a Plugin
section: plugins
order: 1
summary: Add a coherent plugin surface by owning the ObjectType, viewer, manifest, and registration path.
---

A plugin should own a product concept, not just a pile of UI components.

The usual plugin surface includes:

- One or more ObjectTypes.
- Viewers for those ObjectTypes.
- Optional ObjectWizards for setup.
- Optional Quickstart registration.
- A Manifest that lets the PluginHost load the package.

## Owner Rule

Put durable behavior where the object or plugin owns it. Do not make every caller reconstruct the same rules from object keys, route strings, or local booleans.

For example, a docs plugin should own the `notes/docs` ObjectType and viewer behavior. Bundled public docs should remain in `app/docs/content/`. Mixing those two sources makes routing and verification ambiguous.

## Development Loop

Use a small Space and one object first. Confirm the object appears in the object browser, opens through its viewer, and survives reload.

Add a Quickstart after the object and viewer are stable. A Quickstart is a first-run path, so it should create a complete enough Space to be useful immediately.
