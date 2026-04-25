---
title: Objects and Types
section: spaces
order: 2
summary: How typed objects organize data within a space.
---

## What are Objects

Everything stored in a space is an object. A file is an object. A Git repository is an object. A canvas item or configuration setting is an object. Each object has a type that tells Spacewave how to display and edit it.

You rarely need to think about objects directly. When you upload a file, the file browser creates file data for you. When you create a canvas node, the canvas viewer records it in the space. The object system works behind the scenes to keep everything organized.

## Object Types

An object type is a category, like "file", "git repository", or "canvas". Each viewer or plugin registers the types it knows how to handle. When you open something in a space, Spacewave checks its type and loads the right surface to show it.

Spacewave comes with built-in types for files, directories, Git repositories, and canvas items. Plugins can add new types for more specialized data.

## How Objects are Created

You create objects by using the tools in your space. Uploading a file creates file data. Creating a Git repository adds repository objects. Adding a canvas node records canvas state. Each tool provides the interface for working with its types.

You do not need to manually manage objects. The plugins handle creation, editing, and display. Objects are just how Spacewave organizes your data under the hood.

## The Object Browser

If you want to see everything in a space at once, the object browser gives you a behind-the-scenes view. Open it from the space settings panel in the bottom bar. It lists every object in the space, grouped by type, and lets you inspect their details.

The object browser is handy when you want to understand what a space contains beyond what the normal interface shows. For everyday use, the regular viewers are more convenient.

## What Happens When You Delete Something

When you delete an object, it is marked as removed. The change syncs to your other devices like any other update. Over time, the storage used by deleted objects is reclaimed automatically. Like all changes, deletions are encrypted and synced through the same private channels as everything else.
