// Package resource defines the Resource SDK protocol: one client's tree of live
// server-side resources, each an SRPC mux with an ID, reached through a single
// ResourceService.
//
// A client opens a ResourceClient stream, which starts one client generation and
// carries its lifecycle controls: Init, then Adopt and Release, each numbered by
// a control ID that rises by one and confirmed by a ControlAck. A method that
// creates a child returns its resource ID; the child is pending until the client
// adopts it, and releasing a parent releases its still-pending descendants while
// adopted ones survive. Ending the stream releases the whole generation.
// ResourceRpc carries calls to one resource: an Init naming it, an Ack that
// accepts the route or gives a typed ResourceFailure, then SRPC data, and an
// Abandon when a unary caller failed to receive a result that created children.
// ResourceAttach runs the other way, letting a client lend the server resources
// that server-side handlers call.
//
// The server and client subpackages implement the two ends; proxy keeps a
// client's tree alive across an upstream restart.
package resource
