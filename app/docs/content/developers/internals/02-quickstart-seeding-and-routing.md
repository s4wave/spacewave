---
title: Quickstart Seeding and Routing
section: internals
order: 2
summary: Quickstart option registration, seed content, and redirect targets.
---

## Overview

The quickstart system provides one-click paths from the public landing page into a working Spacewave space. Each release-visible quickstart creates a local session, provisions a new space, and populates it with type-specific seed content. Experimental quickstarts can exist in the inventory without appearing in release builds.

## Quickstart Options

Options are defined in `app/quickstart/options.ts` as a typed constant array. Each option has:

- `id` - unique identifier (for example, `drive`, `git`, or `canvas`)
- `name` and `description` - user-facing label and summary
- `category` - grouping key (`account`, `storage`, `social`, `content`, `compute`, `tools`)
- `icon` - Lucide icon component
- `path` (optional) - redirect path instead of `/quickstart/{id}`
- `hidden` (optional) - excludes from the visible UI

Options with an explicit `path` redirect to existing routes. For example, `account` redirects to `/login` and `pair` redirects to `/pair`. All other options use the `/quickstart/{id}` route, which is prerendered as a loading screen and auto-boots the WASM runtime.

## Space Creation and Seeding

The creation logic in `app/quickstart/create.ts` follows this sequence:

1. **Session resolution** - Reuses the most recent local session or creates a new local provider account.
2. **Space creation** - Calls `session.createSpace()` with the quickstart ID as the space name.
3. **World state access** - Mounts the space, accesses its world engine state, and opens a bucket lookup cursor.
4. **Content seeding** - Calls a type-specific populate function that applies world operations.

Each seed function creates the appropriate objects via `applyWorldOp`:

Every quickstart also creates a `SpaceSettings` object that sets the space's default view to the newly created content.

## Supported Quickstarts

| ID | Category | Seed Content |
|----|----------|-------------|
| space | storage | Empty space with settings |
| drive | storage | UnixFS filesystem |
| git | storage | Git repository wizard |
| canvas | storage | UnixFS + canvas object |

## Routing

Quickstart pages are prerendered as static loading screens. When the user visits `/quickstart/drive`, the prerendered page displays a loading indicator while the WASM runtime boots. Once ready, the hydration script calls `__swBoot()` to transition into the live app, which runs the `createQuickstartSetup` flow and navigates to the new space.

For in-app quickstart creation (when the user already has a session), `createQuickstartSpaceInSession` creates the space within the existing session and returns the space ID for navigation.

## Next Steps

- [Authentication Flows](/docs/developers/internals/authentication-flows) for how account-based login works.
- [Prerender and Public Web](/docs/developers/internals/prerender-and-public-web) for how quickstart loading pages are built.
