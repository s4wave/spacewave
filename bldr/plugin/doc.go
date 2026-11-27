// Package bldr_plugin defines how a caller asks for a running plugin and what it
// gets back.
//
// LoadPlugin is the directive that expresses standing demand for a plugin ID.
// With an empty instance key all callers share one instance. A non-empty key
// makes (plugin ID, instance key) its own instance. By default the scheduler
// follows the newest eligible Manifest. LoadPluginManifestRoot pins an exact
// Manifest and never falls back when it is missing, and LoadPluginManifests
// selects an installation and its recovery artifacts, approved by
// LoadPluginPeerID. The directive's value is a RunningPlugin, which exposes the
// plugin's RPC client for one execution generation. A caller holds the
// directive reference for the whole of its call, and a call made before the
// plugin is ready waits until the plugin is ready or the caller cancels. A
// replaced or restarted plugin is a new generation that must register its
// capabilities again.
//
// The host, scheduler and transports accept plugin IDs from their callers and
// recognize no product plugin by name.
package bldr_plugin
