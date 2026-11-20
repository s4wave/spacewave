// Package bldr is the root of Bldr, which builds Spacewave plugins into
// Manifests, stores them in a World, and loads and runs them on native and
// browser hosts.
//
// A project (bldr/project, written in bldr.star) maps manifest IDs to compiler
// configurations and build names to manifests and target platforms. A compiler
// (bldr/plugin/compiler) turns each plugin and platform into a Manifest
// (bldr/manifest): metadata, one entrypoint, and distribution and asset UnixFS
// roots. A LoadPlugin directive (bldr/plugin) asks for a running plugin; the
// scheduler (bldr/plugin/host/scheduler) selects a Manifest and a PluginHost
// (bldr/plugin/host) for its platform and executes the entrypoint. Each running
// plugin gets its own controllerbus bus and reaches the host and other plugins
// only over RPC, through the Resource SDK (bldr/resource).
package bldr
