// Package s4wave_plugin connects one plugin to another plugin's Resource SDK.
//
// ConnectPluginResources loads the target plugin, waits for it, and opens a
// resource client on its ResourceService. ConnectPluginInstanceResources does
// the same for one instance, such as one Space's installation. The returned
// PluginResources holds the plugin load and the client together, and Release
// ends both.
package s4wave_plugin
