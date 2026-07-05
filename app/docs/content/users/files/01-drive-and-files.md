---
title: Drive and Files
section: files
order: 1
summary: Use Drive as the user-facing file browser over UnixFS objects.
---

Drive is the user-facing file surface. Under the hood it uses a UnixFS object,
which is Spacewave's Hydra-backed file and folder tree inside a Space.

## Start with Drive

The Drive Quickstart creates a UnixFS tree, writes `getting-started.md`, and
opens a first-run Drive guide. At the Drive root, the guide header appears while
the starter file is the only content. Uploading files, creating a folder, opening
the guide, inviting people, or dismissing the header hides it for that viewer
state.

## File actions

The Drive toolbar and context menu support:

- upload files and folders by picker or native drag and drop;
- create folders and files;
- open folders and supported files;
- rename with the menu or F2;
- delete with confirmation;
- move entries with the move dialog or by dragging into folders;
- download a file, a folder as a zip, or a multi-selection as `selection.zip`.

Directory listings are live. When another tab, CLI command, or background
process changes the directory, the browser watches the UnixFS directory and
updates the listing.

## Paths and the CLI

Drive paths map to UnixFS object paths. The CLI uses the same `/u/{session}/so/
{space}/-/{object}/-/{path}` shape as the app URL. Short CLI paths such as
`my-object/-/docs/report.pdf` use the default session and Space.

## Current limits

Text preview reads are capped for large files. Download actions need the current
session and Space IDs. Browser local storage is not a substitute for backup; use
Cloud or a desktop state root for files you need to keep.
