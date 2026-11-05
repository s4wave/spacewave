package kvtx

// CopyReachableOptions adjusts a CopyReachable.
type CopyReachableOptions struct {
	// AllowUnknown copies a source whose walk reaches blocks stored without a
	// child list. The blocks they name are dropped, though they may be live.
	AllowUnknown bool
}
