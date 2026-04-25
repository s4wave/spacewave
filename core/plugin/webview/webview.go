// Package plugin_webview provides conventions for plugin ObjectType web viewer registration.
//
// Plugins that provide ObjectType viewers use the HandleWebView directive forwarding
// mechanism from bldr. The naming convention for web view IDs is:
//
//	object-viewer:{typeID}:{objectKey}
//
// For example: object-viewer:unixfs/fs-node:my/fs
//
// The plugin's configset includes a handle-web-view-rpc controller entry that
// matches this pattern and forwards HandleWebView directives to the plugin.
// The plugin then calls SetRenderMode with RenderMode_REACT_COMPONENT and a
// script_path pointing to the viewer component in the plugin's dist UnixFS.
package plugin_webview

import "regexp"

// ObjectViewerWebViewIDPrefix is the prefix for ObjectType viewer web view IDs.
// Web views for ObjectType viewers use the pattern: "object-viewer:{typeID}:{objectKey}"
const ObjectViewerWebViewIDPrefix = "object-viewer:"

// ObjectViewerWebViewIDRegex returns the regex pattern for matching
// HandleWebView directives for a specific ObjectType.
func ObjectViewerWebViewIDRegex(typeID string) string {
	return "^object-viewer:" + regexp.QuoteMeta(typeID) + ":"
}

// AllObjectViewerWebViewIDRegex returns the regex pattern for matching
// all ObjectType viewer HandleWebView directives.
func AllObjectViewerWebViewIDRegex() string {
	return "^object-viewer:"
}

// BuildWebViewID constructs a web view ID for an ObjectType viewer.
func BuildWebViewID(typeID, key string) string {
	return ObjectViewerWebViewIDPrefix + typeID + ":" + key
}
