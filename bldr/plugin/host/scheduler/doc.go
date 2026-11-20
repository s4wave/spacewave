// Package plugin_host_scheduler runs the controller that turns LoadPlugin
// demand into running plugins.
//
// For each plugin it is asked to run, the scheduler watches the available
// PluginHosts and the plugin's Manifests in the World, storing newer ones that
// FetchManifest finds. It drops Manifests whose platform has no host and
// orders the rest by revision, then by platform preference. The first is the
// download Manifest, which it copies fully into local storage; the first already
// copied is the live Manifest, which it runs, so a new version replaces the old
// one only after it is local. To run one it opens the Manifest's filesystems,
// builds the host RPC mux and calls PluginHost.ExecutePlugin. A plugin is ready
// only when its RPC client is available and it has finished registering its
// initial capabilities, not when its process starts.
//
// In a browser the copy runs in the bundled bldr-materializer plugin through
// MaterializeManifest; natively the same copy engine runs in place. The
// scheduler alone publishes a copied root to the World, and only after the copy
// stream completed cleanly. The README lists the routines in order.
package plugin_host_scheduler
