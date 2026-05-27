---
title: Docs, Blogs, and Notebooks
section: features
order: 1
summary: Know which writing surfaces are bundled app docs and which are Space-native experimental objects.
---

Spacewave has several writing surfaces, and they do different jobs.

The documentation you are reading is bundled app documentation. It ships with the app and is stored in `app/docs/content/` in the source repository.

Space-native Docs, Blog, and Notebook surfaces are objects inside a Space. They are backed by ObjectTypes from the notes plugin, including `notes/docs`, `notes/blog`, and `notes/notebook`.

## Current Release Shape

In release-visible Quickstarts, Docs, Blog, and Notebook are not the default first-run writing path. They are development or experimental surfaces unless your build exposes them.

Use Drive for normal files and Markdown documents today. Use the experimental writing surfaces when you are testing Space-native object behavior or building on the plugin system.

## Boundary to Remember

Bundled app docs explain Spacewave.

Space-native docs are content you own inside a Space.

The older `spacewave-docs/documentation` viewer is a legacy boundary. Do not treat it as the public docs source unless a migration explicitly says so.
