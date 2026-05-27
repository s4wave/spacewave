---
title: Prerender and Public Docs
section: platform
order: 1
summary: Keep bundled docs source, prerendered routes, raw Markdown links, and live public deployment as separate decisions.
---

Bundled app documentation lives in `app/docs/content/`. The app loads Markdown from that tree and renders it through the `/docs` route family.

The current docs routes are:

```text
/docs
/docs/:site
/docs/:site/:section/:slug
```

Those routes are the in-app docs surface. They are also a useful source shape for a future static public docs site.

## Public Readiness Boundary

Public readiness means the corpus has stable metadata, site-owned sections, valid links, and raw Markdown source URLs that point at the current repository.

It does not mean a live `docs.spacewave.app` deployment exists. Domain, CDN, Cloudflare Pages, and cache changes are live-service work and need their own operational procedure.

## Static Route Work

If docs routes become prerendered public pages, update the static route inventory deliberately and verify crawlable paths. Do not assume the in-app route automatically creates a public static page.

## Source Links

The raw Markdown action should point at the current `s4wave/spacewave` source path for the page being read. It should not point at old repository names or a future docs-hosting repository until that repository owns the source.
