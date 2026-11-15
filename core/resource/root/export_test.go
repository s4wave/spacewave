package resource_root

import "slices"

// PluginCallerDenied reports whether the deny list holds the method.
func PluginCallerDenied(serviceID, methodID string) bool {
	_, denied := pluginCallerDenied[webMethod(serviceID, methodID)]
	return denied
}

// PluginCallerDeniedMethods returns the deny list's service/method keys.
func PluginCallerDeniedMethods() []string {
	keys := make([]string, 0, len(pluginCallerDenied))
	for key := range pluginCallerDenied {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
