package kvtx

// CopyReachableOptions adjusts a CopyReachable.
type CopyReachableOptions struct {
	// AllowUnknown copies a source whose walk reaches blocks without a child
	// list. The blocks they name are dropped, though they may be live.
	AllowUnknown bool
	// CountUnowned counts the dropped blocks no graph record owns, at one
	// index lookup per dropped block.
	CountUnowned bool
}
