package bldr_manifest

import "github.com/aperturerobotics/bldr/util/valuelist"

// NewFetchManifestRequest constructs a new FetchManifestRequest.
func NewFetchManifestRequest(dir FetchManifest) *FetchManifestRequest {
	return &FetchManifestRequest{
		ManifestMeta: dir.FetchManifestMeta(),
	}
}

// ToDirective converts the request into a directive.
func (r *FetchManifestRequest) ToDirective() FetchManifest {
	return NewFetchManifest(r.GetManifestMeta())
}

// SetValueId sets the value id field.
func (r *FetchManifestResponse) SetValueId(id uint32) {
	r.ValueId = id
}

// SetIdle sets the idle field.
func (r *FetchManifestResponse) SetIdle(idle bool) {
	r.Idle = idle
}

// SetRemoved sets the removed field.
func (r *FetchManifestResponse) SetRemoved(removed bool) {
	r.Removed = removed
}

// SetValue sets the value field.
func (r *FetchManifestResponse) SetValue(val *FetchManifestValue) {
	r.Value = val
}

// _ is a type assertion
var _ valuelist.WatchDirectiveResponse[*FetchManifestValue] = (*FetchManifestResponse)(nil)
