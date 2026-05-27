---
title: Quickstarts and App Surfaces
section: objects
order: 2
summary: Use Quickstarts for first-use Spaces and Space App Surfaces for the route that opens afterward.
---

A Quickstart creates a useful first Space. A Space App Surface is the primary route that opens when the user returns to that Space.

Do not add a Quickstart just because an object exists. Add one when a new user can pick it and immediately understand the Space that appears.

## Static Quickstarts

Release-visible Quickstarts include empty Space, Drive, Git, and Canvas. Account and pairing entries navigate to account flows instead of creating storage objects.

Experimental Quickstarts include Notebook, Docs, Blog, Chat, V86, and Forge in development builds.

## Dynamic Quickstarts

Plugins can register app-only Quickstarts. A dynamic Quickstart should declare its plugin owner and required plugin IDs so the Space can open with the right surface.

## Space App Surface

The Space's `SpaceSettings.index_path` selects the primary route. It can point to a viewer route or to a persistent wizard route while setup is incomplete.

Keep the object browser available as the escape hatch, but make the primary route carry the product experience.
