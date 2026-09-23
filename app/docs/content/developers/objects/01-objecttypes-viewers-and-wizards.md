---
title: ObjectTypes, Viewers, and Wizards
section: objects
order: 1
summary: Register a new kind of object, choose which viewer opens it, and let users create it from a wizard.
---

This page covers the three registrations that make a new kind of object usable
in a Space: its ObjectType, its viewer, and its wizard. A Space stores its
objects in a World, a database of typed objects.

An ObjectType is the code for one kind of World object. It turns an object key
into a typed resource, and it gives the app a label, an icon, and a visibility
setting for the object. A viewer is the UI component that displays the object.
A wizard creates a new object from inside a Space.

## Register an ObjectType

The Go object-type registry resolves the built-in ObjectTypes. A plugin
registers its own with `ObjectTypeRegistryResource.RegisterObjectType`. The
call needs a type ID, a plugin ID, and a `/` in the type ID. A second
registration of the same type ID is rejected. The call returns a resource. The
type stays registered until you release that resource.

The bridge controller forwards lookups for a plugin's types back to that
plugin. Each registration can set a display name, a description, an icon, and
whether the type is hidden or internal.

## How a viewer is chosen

The app installs base viewers first, then product viewers, then viewers from
plugins. For each object it picks a viewer in this order:

1. a viewer registered for the exact type;
2. a viewer registered for a prefix ending in `/*`;
3. a viewer registered for all types, such as the Debug Viewer.

The chosen viewer is saved per object or per tab. If the saved viewer no longer
exists, Spacewave uses the first visible viewer and records the missing
component ID.

## Wizards

The list of wizards comes from `space.watchWizards`. A wizard can create
objects when it has a type ID and a display name, plus either a wizard type
that stores its progress or a direct create operation with a known builder.

A wizard that stores its progress creates a `wizard/*` object and opens it.
It collects settings, runs the create operation, deletes the wizard object,
and opens the new object. A direct wizard runs its create operation and opens
the new object right away.

## Objects and links between them

World operations create objects and the links between them. Object browsers
use each type's display settings and these links to decide labels and
visibility. A viewer should read object state through typed resources and
watch RPCs. It should not walk the World itself in React.
