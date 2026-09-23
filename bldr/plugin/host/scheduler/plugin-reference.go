package plugin_host_scheduler

// pluginReference identifies one logical binding and optional immutable executable.
// Latest and pinned requests never share a runtime, even when their source plugin matches.
type pluginReference struct {
	// pluginID identifies the plugin manifest family.
	pluginID string
	// instanceKey identifies the caller's authorized plugin binding.
	instanceKey string
	// manifestRoot selects an exact manifest content hash; empty follows updates.
	manifestRoot string
}

// String identifies this reference in scheduler diagnostics.
func (k pluginReference) String() string {
	key := pluginInstanceKey(k.pluginID, k.instanceKey)
	if k.manifestRoot != "" {
		key += "@" + k.manifestRoot
	}
	return key
}

// executionKey gives pinned versions independent host workers within the same binding.
func (k pluginReference) executionKey() string {
	if k.manifestRoot == "" {
		return k.instanceKey
	}
	return k.instanceKey + "/manifest/" + k.manifestRoot
}
