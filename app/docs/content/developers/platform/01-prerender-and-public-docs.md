---
title: Prerender and Public Docs
section: platform
order: 1
summary: Explain the current docs loader, route inventory, source links, and prerender gap.
---

Public docs currently come from markdown files under `app/docs/content/`. The
loader imports them with Vite's raw markdown glob, parses flat frontmatter, and
builds page URLs from the directory path.

## Route inventory

Implemented docs routes are:

- `/docs`
- `/docs/:site`
- `/docs/:site/:section/:slug`

The current sites are `users`, `self-hosters`, and `developers`. Section IDs are
scoped by site, so `users/cli` and `developers/cli` can both exist.

## Page rules

A docs markdown file needs `title`, `section`, `order`, and `summary`
frontmatter. `draft: true` excludes a page. The slug is the filename without the
numeric prefix and `.md` suffix.

Docs article pages render markdown through `markdown-to-jsx`, provide
previous/next navigation, expose a copy-markdown action, and show raw source
links.

## Source links

Raw source links point at:

```text
https://raw.githubusercontent.com/s4wave/spacewave/master/app/docs/content/{site}/{section}/{filename}
```

The sidebar uses the matching GitHub blob or tree URL.

## Prerender boundary

The blog has a static prerender pipeline that discovers markdown posts and writes
HTML. Public quickstart and landing pages are also part of the release
prerender list. No current docs prerender collector is wired into that pipeline,
so docs should be treated as app-rendered routes until that changes.
