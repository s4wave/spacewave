---
title: Public Docs and Markdown
section: contributing
order: 2
summary: Add or change a page on this docs site, and know which of Spacewave's markdown locations your writing belongs in.
---

This page is for contributors who edit these docs. It explains how a markdown
file becomes a docs page, and how the public docs differ from the other places
Spacewave keeps markdown.

## How a page is loaded

Each docs page is a markdown file under `app/docs/content/`. The app loads the
files with Vite's raw markdown import, reads their front matter, and builds
each page URL from the file's folder path.

The docs routes are:

- `/docs`
- `/docs/:site`
- `/docs/:site/:section/:slug`

The sites are `users`, `self-hosters`, and `developers`. Section IDs are
defined per site in `app/docs/sections.ts`, so `users/cli` and
`developers/cli` can both exist. A section with no pages does not appear.

## Write a page

Each file needs `title`, `section`, `order`, and `summary` in its front matter.
Set `draft: true` to leave a page out. The slug is the filename without its
number prefix and without `.md`.

Pages render with `markdown-to-jsx`. Each page gets previous and next links, a
button that copies its markdown, and a link to its raw source.

A link to another page uses its full path, such as
`/docs/users/start/start-here`. The docs tests check that every such link
resolves. When you move or rename a page, add its old URL to
`app/docs/legacy-doc-redirects.ts` so existing links keep working.

## Source links

Raw source links point at:

```text
https://raw.githubusercontent.com/s4wave/spacewave/master/app/docs/content/{site}/{section}/{filename}
```

The sidebar links to the matching GitHub file or folder page.

## Docs are not prerendered

The blog has a prerender step that finds markdown posts and writes HTML. The
public Quickstart and landing pages are also prerendered for releases. The docs
are not part of that step yet, so the app renders them when they are opened.

## Where each kind of markdown belongs

Spacewave keeps markdown in several places. They are stored and released
differently, so content written for one does not work in another.

- **Public docs.** `app/docs/content/` holds these docs. The app renders them
  under `/docs` and links each page to its source on GitHub. They are not part
  of any user's Space.
- **Public blog.** `app/blog/posts/` holds the blog. The blog build prerenders
  the index, post, and tag pages and writes the data the page needs to become
  interactive. Blog posts are released with Spacewave. They are not private
  Space data.
- **Notes in a Space.** The notes plugin provides the `notes/notebook`,
  `notes/docs`, and `notes/blog` object types. Their markdown is stored as files
  inside a Space. The plugin registers their ObjectTypes, resources,
  Quickstarts, and viewers. Without the notes plugin, the app does not open
  them.
- **The older Documentation viewer.** `spacewave-docs/documentation` is a
  separate viewer built into the app. It follows a link to a folder object,
  lists the markdown files at its top level, and renders and edits them. It can
  also create `untitled.md`. It is not the public docs route, and it is not the
  notes plugin's docs type.

Use the public docs and the blog for content shipped on the website. Use
objects in a Space for a user's own writing. Do not load private Space content
into the public docs.
