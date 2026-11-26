package forge_runtime

// IsZero reports whether the identity is unset. A nil identity is unset.
func (i *BackendRuntimeIdentity) IsZero() bool {
	return i.GetBackend() == "" && i.GetId() == ""
}
