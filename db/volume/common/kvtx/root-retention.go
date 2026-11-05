package kvtx

// SupportsRootRetention reports whether the Volume holds roots, pins and
// stages: it does when its store commits liveness with the blocks.
func (v *Volume) SupportsRootRetention() bool {
	return v.atomic
}
