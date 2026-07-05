---
title: Quickstarts and App Surfaces
section: objects
order: 2
summary: Connect Quickstarts, SpaceSettings, dynamic plugin options, and first-run routes.
---

A Quickstart is a first-run or in-session creation path. It creates a Space,
seeds world objects, writes Space settings, and opens the most useful initial
route.

## Static and dynamic Quickstarts

Static Quickstarts live in `app/quickstart/options.ts`. Normal visible creation
options include Space, Drive, Git, and Canvas. Notebook, Chat, KV, SQL, Docs,
Blog, V86, Device, and Forge are experimental in the current catalog.

Dynamic Quickstarts come from the Quickstart registry. They must supply an ID,
name, description, category, and plugin ID. Dynamic options can add required
plugin IDs and a default Space name. They are app options, not public static
`/quickstart/:id` pages.

## Creation flow

The standalone Quickstart route creates or reuses a local session, creates a
Space for non-local creation options, populates content, stages handoff
resources, and redirects to `/u/{session}/so/{space}` plus an initial object
route when one exists.

The in-session create-space route uses the current session or organization,
mounts the new Space, populates it, and navigates to the result. Canceling the
progress UI returns to the dashboard and does not promise to delete an in-flight
Space.

## SpaceSettings

SpaceSettings live in the hidden settings object. Today they carry the Space
index path and plugin IDs. Helpers that update the index path preserve existing
settings. Dynamic Quickstarts can return plugin IDs and index path values, which
are written into SpaceSettings.

## First-run boundary

Seed only the content that exists today. If a Quickstart depends on a plugin,
wait for that plugin registration or report that the surface is unavailable.
Do not advertise a first-run route for a dynamic Quickstart until the app route
accepts it.
