---
title: Prerender and Public Web
section: internals
order: 1
summary: Static prerender, SEO metadata, and sitemap generation.
---

## Overview

Spacewave prerenders public-facing pages (landing, pricing, legal, quickstart, blog) at build time using React 19's `prerender()` API. The result is a set of static HTML files served from R2 via a Cloudflare Worker. Each page includes full SEO metadata, structured data, and a hydration script that attaches event handlers without loading the full WASM runtime.

The prerender pipeline runs as a post-build step after bldr produces the application bundle. It reads the bldr dist manifest for asset URLs, renders each page component to HTML, injects bootstrap scripts, and writes per-route HTML files.

## Configuration

The build script lives at `app/prerender/build.ts`. It accepts two flags:

- `--dist-dir <path>` overrides the bldr dist output directory (defaults to `.bldr-dist/build/desktop/js/wasm/spacewave-dist/dist`).
- `--quiet` suppresses verbose logging.

The site origin defaults to `https://spacewave.app` and can be overridden via the `SITE_ORIGIN` environment variable.

## Static Page Registry

Public pages are declared in `app/prerender/static-pages.ts`. The `STATIC_PAGES` array maps URL paths to React components:

```typescript
export const STATIC_PAGES: Array<{ path: string; component: FC }> = [
  { path: '/landing', component: Landing },
  { path: '/pricing', component: Pricing },
  // ...
]
```

Quickstart option pages are added dynamically from `QUICKSTART_OPTIONS`. Options with explicit redirect paths (like `/login`) or hidden options are excluded.

## SEO and Metadata

Each page exports a `metadata` object with `title`, `description`, `canonicalPath`, `ogImage`, and optional `jsonLd`. The build validates that every page has a title, description, and canonical path. Descriptions outside the 120-160 character range produce warnings.

The build generates `sitemap.xml` with priority tiers: `/` gets 1.0, landing and pricing get 0.8, blog gets 0.7, and legal pages get 0.3.

## Hydration

The lightweight hydration entry point (`app/prerender/hydrate.tsx`) loads after the static HTML is visible. It calls `hydrateRoot()` to attach interactive behavior (accordion toggles, scroll handlers, navigation) without booting the WASM runtime or SharedWorker.

When a user navigates from a static page to an app route, the hydration script triggers the full boot sequence: it calls `__swBoot()` to initialize the runtime and transition from the prerendered landing to the live application. Return visitors with an existing session skip the landing and auto-boot immediately.

## Build Outputs

The prerender produces:

- Per-route HTML files (e.g., `landing.html`, `pricing.html`)
- A root `index.html` with dual containers (`sw-landing` and `sw-loading`)
- A `static-manifest.ts` mapping URL paths to R2 keys
- A `sitemap.xml` for search engine crawlers
- Copied font and image assets with rewritten CSS URLs

## Next Steps

- [Quickstart Seeding and Routing](/docs/developers/internals/quickstart-seeding-and-routing) for how quickstart pages transition into the full app.
- [Public Docs Architecture](/docs/developers/internals/public-docs-architecture) for how documentation pages are loaded and rendered.
