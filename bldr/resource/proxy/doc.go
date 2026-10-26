// Package resource_proxy serves a client's Resource SDK from a resource tree of
// its own, mapped onto an upstream tree whose connection can be replaced. When
// the upstream goes away and another takes its place, each call the client has
// open follows the Rule its method was given, and each child resource the
// client holds is opened again beneath its parent, so a client of a restarted
// server keeps its resources and most of its calls.
package resource_proxy
