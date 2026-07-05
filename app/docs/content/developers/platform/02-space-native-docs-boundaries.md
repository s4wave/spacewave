---
title: Space-Native Docs Boundaries
section: platform
order: 2
summary: Keep public docs, public blog, in-Space documentation, and plugin notes separate.
---

Spacewave has several markdown surfaces. They are not interchangeable because
they have different storage and release owners.

## Public docs

`app/docs/content/` is the public documentation corpus. It is loaded by the app,
rendered under `/docs`, and source-linked to GitHub raw markdown. It does not
belong to a user Space.

## Public blog

`app/blog/posts/` is the public blog corpus. Blog build code prerenders index,
post, and tag pages and writes hydration data for release pages. Blog markdown is
release content, not private Space data.

## Space-native notes plugin objects

The notes plugin owns `notes/notebook`, `notes/docs`, and `notes/blog`. Those
objects store sources in UnixFS trees inside a Space. The plugin registers their
ObjectTypes, resources, Quickstarts, and viewers. Without the notes plugin, those
objects are not statically handled by the app shell.

## Legacy in-Space Documentation viewer

`spacewave-docs/documentation` is a separate app-local Documentation object
viewer. It resolves a graph edge to a UnixFS object, lists root markdown files,
renders markdown, edits files, and can create `untitled.md`. It is not the
public docs route and not the notes plugin documentation type.

## Boundary rule

Use public docs and blog for shipped website content. Use Space-native objects
for user-owned writing inside a Space. Do not wire private Space content into
the public docs loader.
