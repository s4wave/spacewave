package forge_runtime

// BackendRuntimeIdentity identifies one backend runtime instance for one
// reservation generation. The identity is stable across Worker restarts so
// reconcile can resume observation without another launch.
type BackendRuntimeIdentity struct {
	// Backend names the runtime backend that owns the runtime.
	Backend string
	// ID is the backend-scoped runtime identifier, for example a container id.
	ID string
	// StopCommand is the runtime CLI executable used to stop this instance.
	StopCommand string
	// StopEnv is its complete subprocess environment, retained across restarts.
	StopEnv []string
	// StopTimeoutSeconds is the Docker stop grace period for this instance.
	StopTimeoutSeconds uint32
}

// IsZero reports whether the identity is unset.
func (i BackendRuntimeIdentity) IsZero() bool {
	return i.Backend == "" && i.ID == ""
}
