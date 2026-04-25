---
title: Blogs
section: features
order: 4
summary: Blog ObjectType viewer and content management.
draft: true
---

## Overview

A Blog object provides a reading-first publishing view inside a space. Published
posts render as a blog with a featured latest post, tag filters, and full
markdown rendering with syntax-highlighted code blocks. The same files are also
editable in place, and the Blog quickstart creates a companion Notebook that
shares the same underlying UnixFS content.

## Prerequisites

- Create a space with the Blog quickstart, or create a Blog object from the [object browser](/docs/users/spaces/object-browser).

## Steps

### Browsing posts

Read mode opens first. Published posts appear in reverse date order using the
`date` field in YAML frontmatter. The newest post is featured at the top, and
tag chips filter the visible post list.

### Creating a post

Switch to **Edit** mode and click the **+** button in the post list. A new
draft markdown file is created with blog frontmatter, including `title`, `date`,
`author`, `summary`, `tags`, and `draft`.

### Editing a post

Use the **Edit** toggle to switch between the polished reading view and the file
editor. In edit mode you can open any markdown file, inspect or change its
frontmatter, and edit the body through the shared notes editor. Blog quickstart
spaces also include a companion Notebook object that reads and writes the same
files.

### Reading a post

Read mode shows only published posts: markdown files with a `date` field and
`draft: false`. Full posts render with headings, lists, links, and
syntax-highlighted code blocks. Author details load from `authors.yaml` when
present.

## Verify

After creating a post in edit mode, leave it as a draft to keep it out of the
reading view, or set `draft: false` to publish it. Changes made from either the
Blog object or the companion Notebook appear in the other view because both
objects share the same files.

## Troubleshooting

- **"No sources configured for this blog."** The Blog object is not connected to
  a file directory. Recreate it from the quickstart or object browser.
- **Post not appearing in read mode.** The file needs blog frontmatter with a
  `date` field, and drafts stay hidden until `draft: false`.
- **A note appears in edit mode but not read mode.** Files without blog
  frontmatter stay editable, but they are treated as notes rather than
  published posts.

## Next Steps

- [Documentation Sites](/docs/users/features/documentation-sites) for structured reference material
- [Files and Drives](/docs/users/features/files-and-drives) for direct file management
