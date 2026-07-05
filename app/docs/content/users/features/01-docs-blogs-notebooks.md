---
title: Docs, Blogs, and Notebooks
section: features
order: 1
summary: Understand public docs, public blog pages, and Space-native writing objects.
---

Spacewave currently has separate writing surfaces that look similar but store
data in different places.

## Public app docs

The documentation you are reading is markdown under `app/docs/content/`. The app
loads it at build time with Vite, renders it under `/docs`, and links each page
to the raw markdown on GitHub. These docs are product documentation, not private
Space data.

## Public blog

The public blog uses markdown under `app/blog/posts/`. Blog posts have their own
loader and prerender pipeline. Blog pages are public release content, not
Space-native documents.

## Space-native notes, docs, and blogs

Notebook, Documentation, and Blog objects are owned by the notes plugin. They
store their markdown sources in UnixFS objects inside a Space. The plugin seeds
starter files, registers object types and viewers, and renders notebooks, docs
trees, or blog posts from those source files.

These Quickstarts are experimental and plugin-gated in the current app. If the
notes plugin is not loaded, those objects may fall back to the Debug Viewer
instead of a polished editor.

## What to choose

Use public docs or the public blog only for Spacewave's shipped website content.
Use a Space-native Notebook, Documentation, or Blog object when the writing
belongs inside your Space and should move with that Space's storage and sharing.
