---
title: Public Docs Architecture
section: internals
order: 4
summary: Docs route model, audience split, sidebar, search, and page utilities.
---

## Overview

Spacewave's public documentation is a multi-audience system organized into three sites: Users, Self-Hosters, and Developers. Documentation pages are authored as Markdown files with YAML frontmatter, imported at build time via Vite's `import.meta.glob`, parsed into structured data, and rendered through a set of React route components.

## Content Structure

Documentation lives in `app/docs/content/` organized by site and section:

```
app/docs/content/
  users/
    overview/
      01-what-is-spacewave.md
      02-architecture.md
    getting-started/
      01-quick-start.md
  developers/
    plugins/
      01-what-are-plugins.md
    sdk/
      01-resource-system.md
  self-hosters/
    start-here/
      01-getting-started.md
```

Each Markdown file has frontmatter with `title`, `section`, `order`, and `summary` fields. The numeric prefix in the filename controls sort order within a section and is stripped from the URL slug.

## Data Model

Three types defined in `app/docs/types.ts` represent the data:

- **DocFrontmatter** - The parsed YAML header: title, section, order, summary, and optional draft flag.
- **DocPage** - A fully resolved page with slug, URL, site, section, body content, and filename.
- **DocSection** - A group of pages sharing a section ID, label, and site.

## Site and Section Definitions

Sites and sections are declared in `app/docs/sections.ts`. Each site has an ID, label, description, and display order:

```typescript
export const siteDefs: DocSite[] = [
  { id: 'users', label: 'Users', description: '...', order: 1 },
  { id: 'self-hosters', label: 'Self-Hosters', description: '...', order: 2 },
  { id: 'developers', label: 'Developers', description: '...', order: 3 },
]
```

Sections belong to a site and define the sidebar navigation groups. Section order values are scoped within the site (Users sections start at 1, Developers sections start at 20).

## Page Loading

The `loadDocs()` function in `app/docs/load-docs.ts` handles parsing. It uses Vite's `import.meta.glob('./content/**/*.md', { query: '?raw', eager: true })` to import all Markdown files as raw strings at build time. For each file, it:

1. Parses the YAML frontmatter with a simple line-by-line parser.
2. Skips pages without required fields or with `draft: true`.
3. Derives the site and section from the directory structure.
4. Strips the numeric prefix from the filename to produce the URL slug.
5. Constructs the URL as `/docs/{site}/{section}/{slug}`.

Results are cached after first load and sorted by site order, section order, then page order.

## Routing

Three route components in `app/routes/DocsRoutes.tsx` handle documentation URLs:

| Route | Component | Purpose |
|-------|-----------|---------|
| `/docs` | DocsIndexRoute | Top-level index listing all sites |
| `/docs/:site` | DocsSiteRoute | Site index with section listing |
| `/docs/:site/:section/:slug` | DocsPageRoute | Individual page with sidebar |

The sidebar renders all sections for the current site. The active page is highlighted and scroll position is maintained across navigation.

## Adding a New Page

To add a documentation page:

1. Create a Markdown file in the appropriate `app/docs/content/{site}/{section}/` directory.
2. Add frontmatter with `title`, `section`, `order`, and `summary`.
3. Use a numeric prefix on the filename to control ordering (e.g., `04-my-new-page.md`).
4. The page appears automatically on the next build.

## Next Steps

- [Prerender and Public Web](/docs/developers/internals/prerender-and-public-web) for how static pages are built and served.
- [Project Configuration](/docs/developers/platform/project-configuration) for the current bldr project shape.
