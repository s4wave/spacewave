// Package resource_client implements the client end of the Resource SDK.
//
// A Client is one ResourceClient generation: NewClient waits for its Init, and
// the Client serializes its lifecycle controls and counts references to each
// remote resource. Every ResourceRef must be released; the last release of a
// resource sends Release, ordered after its Adopt. AttachResource,
// AttachResourceTree and AttachRawInvoker lend local resources to server-side
// handlers over ResourceAttach. Release closes the generation, and Done closes
// once it has drained.
package resource_client
