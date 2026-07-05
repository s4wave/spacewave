---
title: ObjectTypes, Viewers, and Wizards
section: objects
order: 1
summary: Register object types, viewers, and wizards at the layer that owns the behavior.
---

An ObjectType is the runtime contract for a world object. It resolves the object
key to a typed resource and supplies metadata the app can use for labels,
visibility, and viewer dispatch.

## ObjectType registration

Core ObjectTypes are resolved by the Go object-type registry. Plugin ObjectTypes
register through `ObjectTypeRegistryResource.RegisterObjectType`. Dynamic
registrations require a non-empty type ID, a plugin ID, and a slash in the type
ID. The returned resource owns the registration lifetime; releasing it removes
the dynamic type.

The bridge controller proxies lookup for plugin-registered types back to the
source plugin. UI metadata from those registrations supplies display name,
description, icon, and hidden/internal visibility.

## Viewer selection

Runtime providers install base viewers, product viewers, and downstream dynamic
viewers. Viewer matching is deterministic:

1. exact type matches;
2. prefix registrations ending in `/*`;
3. wildcard viewers such as the Debug Viewer.

The selected component is persisted per object or tab state namespace. If a
preferred component is missing, Spacewave falls back to the first visible viewer
and records the missing component ID.

## Wizards

Object wizards are data entries from `space.watchWizards`, not a single React
owner. A wizard is creatable when it has a type ID and display name, plus either
a persistent wizard type or a direct create operation with a known builder.

Persistent wizards create a `wizard/*` object, navigate to it, collect
configuration, apply the target create operation, delete the wizard object, and
navigate to the created object. Direct wizards apply their create operation and
open the new object immediately.

## Graph and object flow

World operations create object roots and graph edges. Object browsers use object
type metadata and graph relationships for labels and visibility. Viewers should
read object state through typed resources and watch RPCs, not by reimplementing
World traversal in React.
