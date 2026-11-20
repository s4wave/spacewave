// Package resource_server implements the server end of the Resource SDK: it
// tracks each client generation and the resource tree it holds.
//
// A handler creates a child resource with ConstructChildResource, or AddResource
// on the call's ResourceClientContext. The child is pending until the client
// adopts it; one still pending when its handler returns unpublished is released.
// ConstructChildResource gives the child a context that outlives the call and
// lasts until the child is released or the client disconnects, and runs the
// child's release function while that context is still live on an explicit
// release.
package resource_server
