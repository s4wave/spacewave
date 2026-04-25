---
title: Documentation Sites
section: features
order: 3
summary: Docs ObjectType viewer, in-space docs vs public docs.
draft: true
---

## Overview

A Documentation object lets you build a multi-page documentation site inside a space. Pages are written in markdown, organized in a sidebar, and rendered with syntax highlighting for code blocks. It is designed for project wikis, knowledge bases, and reference material.

## Prerequisites

- Create a space with the Docs quickstart, or create a Documentation object from the [object browser](/docs/users/spaces/object-browser).

## Steps

### Layout

The documentation viewer has two panels. A sidebar on the left lists all pages as a file list. The main area on the right displays the selected page's rendered markdown.

### Browsing pages

Click any page in the sidebar to view it. The sidebar shows page names (without the `.md` extension). Use the search bar at the top of the sidebar to filter pages by name.

### Creating a page

Click the **+** button next to the search bar. A new markdown file is created with a default name (`untitled.md`, incrementing if that name exists). The page opens in the editor immediately so you can start writing.

### Editing a page

Click the **Edit** button in the content header to switch to a plain text editor. Write or modify the markdown content. Click **Save** to save your changes back to the file. Click **Cancel** to discard unsaved edits and return to the rendered view.

Changes are saved directly to the underlying file storage and sync across your linked devices.

### Viewing rendered content

When not in edit mode, the content area renders markdown with full formatting: headings, lists, links, images, and syntax-highlighted code blocks.

## Verify

After creating a new page, it appears in the sidebar immediately. Edit the page, save, and the rendered view updates to show your changes.

## Troubleshooting

- **"No documentation source linked."** The Documentation object is not connected to a file directory. This can happen if the object was created without its linked storage. Recreate it from the quickstart or object browser.
- **Page not appearing in sidebar.** Only `.md` files are listed. Files with other extensions are ignored.

## Next Steps

- [Blogs](/docs/users/features/blogs) for date-ordered content
- [Notebooks](/docs/users/features/notebooks) for personal note-taking
