---
title: Developer Start Here
section: start
order: 1
summary: Build from Space, ObjectType, viewer, wizard, plugin, and SDK boundaries instead of copying UI paths.
---

Spacewave development starts with the domain boundary.

A **Space** owns the workspace. A **SharedObject** inside the Space stores a durable object. An **ObjectType** declares what that object is. An **ObjectViewer** renders it. An **ObjectWizard** can collect setup state before the target object exists.

Plugins package object types, viewers, Quickstarts, and runtime behavior behind a Manifest and PluginHost lifecycle.

## First Build Path

1. Pick the object you want to own.
2. Define its ObjectType and data model.
3. Add or reuse a viewer.
4. Add a wizard only if the object needs setup before it can render.
5. Register a Quickstart only if the object is useful as a first Space.
6. Use SDK resources and watch RPCs for state that changes after render.

## Read Next

Read [Build a Plugin](/docs/developers/plugins/build-a-plugin) for packaging, then [ObjectTypes, Viewers, and Wizards](/docs/developers/objects/objecttypes-viewers-and-wizards) for object ownership.

Use [CLI Reference](/docs/developers/cli/cli-reference) to inspect the running system while building.
