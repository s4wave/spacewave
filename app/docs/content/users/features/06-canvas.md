---
title: Canvas
section: features
order: 6
summary: Canvas surface, commands, and spatial layout.
---

## Overview

A Canvas is a spatial workspace where you arrange items freely on an infinite surface. You can place text, shapes, drawings, and embedded space objects, then connect them with edges. The canvas is useful for brainstorming, diagramming, mind mapping, and visual organization.

## Prerequisites

- Create a Canvas object from the [object browser](/docs/users/spaces/object-browser), or use a space template that includes one.

## Steps

### Navigating the canvas

Pan by clicking and dragging the background. Zoom in and out with the scroll wheel or pinch gesture. A minimap in the corner shows your current viewport position relative to all placed items. A scale indicator shows the current zoom level.

### Tool modes

The toolbar on the left side offers several tools:

- **Select** (default) lets you click nodes to select them, drag to move them, and drag the background to pan or box-select multiple items.
- **Text** lets you click anywhere on the canvas to place a text node. Type your content and it commits as a node.
- **Object** lets you drag a rectangle on the canvas to create an embedded world object node.
- **Draw** lets you freehand draw strokes directly on the canvas.

Switch tools using the toolbar buttons or keyboard shortcuts.

### Working with nodes

Click a node to select it. Drag to reposition it. Drag the edges of a selected node to resize it. Select multiple nodes with a box-select drag and move them together. Delete selected nodes with the keyboard shortcut.

Text nodes support inline editing: double-click a text node to edit its content.

### Edges

Edges connect nodes visually. They update automatically as you move connected nodes. Edges can represent relationships, flow, or any connection you want to visualize.

### Sync and persistence

Every change you make is saved and synced to linked devices. A sync status indicator shows when mutations are in flight. Changes from other devices appear in real time.

## Verify

Create a text node, move it around, and refresh the page. The node should appear in the same position. On a linked device, the canvas should show the same layout.

## Troubleshooting

- **Canvas feels slow.** Very large canvases with hundreds of nodes may render more slowly. The viewer culls off-screen nodes for performance, but extremely dense layouts can still be heavy.
- **Nodes not moving.** Make sure the Select tool is active. Other tools (Text, Draw) capture clicks for different purposes.

## Next Steps

- [Objects and types](/docs/users/spaces/objects-and-types) to understand what canvas nodes can embed
