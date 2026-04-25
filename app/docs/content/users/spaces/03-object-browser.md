---
title: Object Browser
section: spaces
order: 3
summary: Tree navigation, create, open, set-as-index, and delete flows.
---

## Overview

The object browser gives you a tree view of every object in a space. It appears in the space settings panel in the bottom bar. Use it to see what a space contains, open objects directly, create new ones, set the default view, or remove objects you no longer need.

## Prerequisites

- A space must be open. The object browser reads the space's current contents.

## Steps

### Browsing objects

Open the space settings panel from the bottom bar. The **Objects** section shows a count and a tree of all objects, grouped by their key path. Virtual grouping nodes (like folders) help organize the list. Click a tree row to select it, or double-click to open the object in its viewer.

### Creating an object

Click the **+** button next to the Objects heading. A dropdown menu lists the object types available in the current build. Select a type to create a new object with a default name. The new object appears in the tree immediately.

### Opening an object

Double-click any row in the tree, or right-click and select **Open**. The space navigates to that object's viewer. You can also click the folder icon that appears on hover next to each object.

### Setting the index

The index object is the default view when someone opens the space. Right-click an object and choose **Set as Index**, then confirm. You can also click the home icon next to an object. A confirmation step prevents accidental changes. The current index shows the home icon dimmed.

### Deleting an object

Right-click an object and choose **Delete**. A confirmation prompt appears asking you to confirm. Deletion removes the object from the space. This change syncs to all linked devices.

### Copying an object key

Right-click and choose **Copy Object Key** to copy the object's internal key path to your clipboard. This is useful for debugging or linking objects in plugins.

## Verify

After creating, deleting, or setting the index, the object tree updates to reflect the change. The object count in the section header adjusts accordingly.

## Troubleshooting

- **Tree is empty.** The space has no user-visible objects. Some internal objects (like settings) are hidden by default.
- **Cannot delete an object.** Make sure you right-click the correct row. Virtual grouping nodes cannot be deleted.

## Next Steps

- [Learn about objects and types](/docs/users/spaces/objects-and-types)
- [Space settings and collaboration](/docs/users/spaces/space-settings-and-collaboration)
