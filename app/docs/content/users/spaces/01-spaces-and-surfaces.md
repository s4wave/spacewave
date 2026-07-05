---
title: Spaces and Surfaces
section: spaces
order: 1
summary: Learn how Spaces, objects, viewers, and layouts fit together.
---

A Space is a shared object whose body contains a Hydra World. The World stores
typed objects, object roots, and graph relationships. In the app, a Space opens
at `/u/{session}/so/{space}`.

## Objects and viewers

Each object has an ObjectType. ObjectTypes tell Spacewave how to access the
object's resource and which viewer can render it. The app includes viewers for
Drive/UnixFS, Git, Canvas, Forge, Manifest, Chat, organizations, key/value
stores, SQL, devices, terminals, and wizards. Plugins can register more object
types and viewers.

If no installed viewer handles an object type, Spacewave shows a "can't open"
state and can offer the Debug Viewer. That is a valid current state, not a data
loss signal.

## App surfaces

A Space can point its index path at the object that should open first. Space
settings store that index path and the plugin IDs declared for the Space. A
Quickstart writes these settings when it seeds content.

## Layouts

The normal app shell uses tabs. You can open paths in tabs and, when the shell is
split into a grid, the layout is encoded into the URL. Separately, a Space can
hold an ObjectLayout object. ObjectLayout stores a durable multi-pane layout in
the Space World and renders each pane through ObjectViewer.

## Create objects

The create-object command shows visible object wizards for the current Space. A
wizard may create the object directly or create a persistent wizard object that
you finish in its own viewer.
