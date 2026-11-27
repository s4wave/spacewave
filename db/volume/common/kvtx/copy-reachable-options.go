package kvtx

// CopyReachableOptions adjusts a CopyReachable.
type CopyReachableOptions struct {
	// AllowUnknown copies a source whose walk reaches blocks stored without
	// a child list. The blocks those blocks reference are dropped, though
	// the referenced blocks may be live.
	AllowUnknown bool
}
