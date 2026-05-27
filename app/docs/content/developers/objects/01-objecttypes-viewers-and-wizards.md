---
title: ObjectTypes, Viewers, and Wizards
section: objects
order: 1
summary: Model durable objects separately from the UI that creates or renders them.
---

An ObjectType is the durable identity of a SharedObject. A viewer is how the app renders that object. A wizard is temporary setup state for creating or configuring a target object.

Keep those roles separate.

## ObjectType

Use an ObjectType when the Space needs to store a durable object with a stable meaning. Examples include file storage, Git repositories, Canvas, Forge entities, VMs, and Space-native notes objects.

## Viewer

Use a viewer to render an existing object. A viewer should assume the object exists and should make object state understandable.

## Wizard

Use a wizard when creation needs choices that cannot be guessed safely. The Git Quickstart uses a persistent wizard so the user can choose create or clone before the repository object is finalized.

The wizard should hand off to the created object and remove or retire setup state when appropriate.

## Test Shape

Test the owner boundary: the object type registers, the viewer opens, the wizard persists across reload, and finalization changes the Space route only when intended.
