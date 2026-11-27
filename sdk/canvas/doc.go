// Package s4wave_canvas defines the Canvas, a World object that places text,
// shapes, drawings and views of other World objects on a two-dimensional
// surface.
//
// A CanvasState holds nodes keyed by ID, user-drawn edges, a reference to the
// freeform stroke tree, the World graph links hidden on this canvas, and layout
// metadata for nodes a projection placed. A WORLD_OBJECT node holds only the
// key and the view path of the object it shows. Moving or resizing the node
// changes this canvas, never the object. An edge is a visual record of the
// canvas and not a World graph relationship. CanvasStorage is the stored form,
// with each node in its own block. CanvasResourceService gets, updates and
// watches the state, and the world subpackage registers the ObjectType that
// serves the Canvas.
package s4wave_canvas
