package bldr_manifest

import "github.com/aperturerobotics/bldr/util/valuelist"

// NewFetchManifestRequest constructs a new FetchManifestRequest.
func NewFetchManifestRequest(dir FetchManifest) *FetchManifestRequest {
	buildTypeStrs := make([]string, len(dir.GetBuildTypes()))
	for i, bt := range dir.GetBuildTypes() {
		buildTypeStrs[i] = bt.String()
	}

	return &FetchManifestRequest{
		ManifestId:  dir.GetManifestId(),
		BuildTypes:  buildTypeStrs,
		PlatformIds: dir.GetPlatformIds(),
		Rev:         dir.GetRev(),
	}
}

// ToDirective converts the request into a directive.
func (r *FetchManifestRequest) ToDirective() FetchManifest {
	buildTypes := make([]BuildType, len(r.GetBuildTypes()))
	for i, btStr := range r.GetBuildTypes() {
		buildTypes[i] = BuildType(btStr)
	}

	return NewFetchManifest(r.GetManifestId(), buildTypes, r.GetPlatformIds(), r.GetRev())
}

// SetValueId sets the value id field.
func (r *FetchManifestResponse) SetValueId(id uint32) {
	r.ValueId = id
}

// SetIdle sets the idle field.
func (r *FetchManifestResponse) SetIdle(idle uint32) {
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
