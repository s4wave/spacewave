---
title: Add a Quickstart
section: contributing
order: 1
summary: Add a built-in Quickstart to Spacewave, fill its new Space with content, test where it opens, and rebuild its public page.
---

A Quickstart is a one-click way to create something in Spacewave. Most
Quickstarts create a new Space, fill it with starter content, save its
settings, and open the first view. Some only open a setup page: the local
session option creates or reuses a session without creating a Space, and the
account and pairing options open their existing setup pages.

This page is for contributors changing Spacewave itself. To add a Quickstart
from a plugin, see [Build a Plugin](/docs/developers/plugins/build-a-plugin).

## Built-in and plugin Quickstarts

Built-in Quickstarts are listed in `app/quickstart/options.ts`. The visible
ones today are Space, Drive, Git, and Canvas. Notebook, Chat, KV, SQL, Docs,
Blog, V86, Device, and Forge are marked experimental.

Plugin Quickstarts come from the Quickstart registry. Each one needs an ID, a
name, a description, a category, and a plugin ID. It can also list required
plugin IDs and a default Space name. Plugin Quickstarts appear in the app only.
They do not get a public `/quickstart/:id` page.

## Add a built-in Quickstart

In `app/quickstart/create.ts`, use `drive` as the example for content from the
core plugin, and `notebook` for content from another plugin.

1. Add the option to `QUICKSTART_OPTIONS` in `app/quickstart/options.ts`. Give
   it a stable ID, a name, a description, a category, and an icon. Add
   `seoDescription` for its public page. The metadata tests require 120 to 160
   characters. Set `experimental: true` while the feature is experimental. Leave
   `path` unset: a `path` makes the option only open a page instead of creating
   a Space.
2. Add the ID to `QUICKSTART_SEEDS` in `app/quickstart/create.ts` with its
   default Space name. Set `initialObjectKey` and `initialObjectType` only when
   the first view should open one object directly. Otherwise the Space opens
   its configured index. The table must have an entry for every
   `QuickstartSpaceCreateId`.
3. Add a case to `populateSpace`. Use the existing SDK operation or resource
   that creates the content. Pass the attempt's abort signal to each call, and
   release each resource you acquire the way its API expects. For plugin
   content, install the plugin and wait for its Quickstart or ObjectType to
   register before you call it. The Notes and SQL cases show both steps.
4. Save the index path with `createSpaceSettingsObject`, or with
   `ensureSpacePlugins` when you also install required plugins. Both keep any
   settings the new content does not replace. Point the index at an object the
   Quickstart actually created. A plugin Quickstart can return its index path
   and plugin IDs through `executeDynamicQuickstart`.
5. Extend the tests described below. If the option is visible in releases,
   open it from its public Quickstart URL. Also open it from the create-Space
   screen of an existing session. Check that the new content opens at the right
   view, and that it still opens after you leave the Space and come back.

`QuickstartId` is derived from the option table. The options that only open a
page are excluded by name: `account` and `pair` are not creation IDs, and
`local` does not create a Space. A new option of that kind must be added to the
same helpers. Setting `path` alone does not change those types.

## Test what the Quickstart creates

Add a focused case to `app/quickstart/create.test.ts` with its existing
`buildQuickstartWorld` fixture. Check the content operation, the saved index
path, and any required plugin IDs. Extend the test “indexes every quickstart to
the object it creates or seeds”, so the index can never point at a missing
object. For plugin content, also check that the plugin registers before the
Quickstart runs.

Update `app/quickstart/options.test.ts` for which options are visible, and
`app/prerender/static-pages.test.ts` for the list of release pages. Run these
from the repository root:

```sh
bun run test:js:alpha -- app/quickstart/create.test.ts app/quickstart/options.test.ts app/prerender/static-pages.test.ts
bun run typecheck
```

These tests check what gets created and where it opens. Use the running app to
check that the plugin is available, that the first view renders, and that
canceling during setup works.

## Rebuild the public Quickstart pages

`PUBLIC_QUICKSTART_OPTIONS` in `app/prerender/static-pages.ts` lists the
Quickstarts that get public pages. It leaves out experimental, hidden, plugin,
and page-only options. A visible built-in option gets its `/quickstart/{id}`
metadata and the shared `QuickstartLoading` page automatically. There is no
second route list to edit.

From the repository root, build the browser assets, the hydration bundle, and
the prerender bundle, in this order:

```sh
bun run build:release:web
bun run vite build --config app/prerender/vite.hydrate.config.ts
bun run vite build --config app/prerender/vite.ssr.config.ts
bun run app/prerender/ssr-dist/build.js --dist-dir .bldr-dist/build/js/spacewave-browser/dist
```

The last command writes the public HTML, `static-manifest.ts`, and
`sitemap.xml` under `app/prerender/dist/`. Check that the new route is in the
manifest and the sitemap. Then open the generated page in the local release
preview. Keep these files in the release output. Publishing them is a separate
release step.

## What happens when a user picks a Quickstart

From the public Quickstart route, Spacewave creates or reuses a local session.
For options that create content, it creates a Space and fills it. Then it
redirects to `/u/{session}/so/{space}`, plus the first object's route when
there is one.

From inside a session, the create-Space route uses the current session or
organization. It opens the new Space, fills it, and navigates to the result.
Canceling the progress screen returns to the dashboard. It does not promise to
delete a Space that is already being created.

## Space settings

Each Space keeps its settings in a hidden settings object. Today the settings
hold the Space's index path and its plugin IDs. The helpers that change the
index path keep the other settings. A plugin Quickstart can return plugin IDs
and an index path, and Spacewave writes them into the settings.

## Seed only what exists

Create only content that works today. If a Quickstart depends on a plugin, wait
for the plugin to register, or tell the user the feature is unavailable. Do not
advertise a first-run route for a plugin Quickstart until the app route accepts
it.
