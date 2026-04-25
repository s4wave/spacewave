---
title: Notebooks
section: features
order: 2
summary: Notebook creation, markdown editing, and offline-first behavior.
draft: true
---

## Overview

A Notebook space is a markdown note-taking workspace. It organizes your notes into sources (collections), lets you browse and filter them, and provides a built-in editor for writing and reading. Everything works offline and syncs when your devices reconnect.

## Prerequisites

- Create a Notebook space from the dashboard quickstart, or add the notes plugin to an existing space.

## Steps

### Layout

The notebook viewer uses a three-panel layout. The left sidebar lists your note sources (collections). The middle panel lists the notes in the selected source. The right panel shows the content of the selected note. On mobile, the panels stack and you navigate between them.

### Browsing notes

Select a source in the sidebar to see its notes. The note list shows each note's title. Click a note to view it in the content panel. Use the tag filter to narrow the list by tag.

### Writing and editing

Click the **Edit** button in the content header to switch to the markdown editor. The editor is a plain text area where you write standard markdown. Click **Save** to save your changes, or **Cancel** to discard them.

Notes support frontmatter (metadata at the top of the file between `---` markers). Frontmatter fields like tags are displayed above the note content and can be used for filtering.

### Creating notes

New notes are created through the note list. The notebook plugin manages file creation within its sources. Each source maps to a directory of markdown files, so adding a note creates a new `.md` file.

### Offline behavior

Everything you write is saved to local storage immediately. If you are offline, your edits are preserved and sync to other devices when connectivity returns. There is no save-to-cloud step; the local copy is the primary copy.

## Verify

After editing a note, switch away and come back. Your changes persist. On a second linked device, the updated note appears once sync completes.

## Troubleshooting

- **"No sources configured."** The notebook has no source directories linked. This can happen with a blank space. Create a notebook from the quickstart to get a pre-configured source.
- **Notes not appearing.** Make sure you selected the correct source in the sidebar. Each source is an independent collection.

## Next Steps

- [Files and Drives](/docs/users/features/files-and-drives) for file-level management
- [Documentation Sites](/docs/users/features/documentation-sites) for structured multi-page docs
