//go:build !js && !wasip1

package world_recovery

import (
	"errors"

	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	transform_gzip "github.com/s4wave/spacewave/db/block/transform/gzip"
	"github.com/s4wave/spacewave/db/bucket"
)

// validateContext admits only native transforms with a bounded decoded allocation.
// Snappy and LZ4's native decoders are unbounded and remain explicit Phase 1 gaps.
func validateContext(ref *bucket.ObjectRef) error {
	// External transform references need context resolution beyond the first adapter.
	if !ref.GetTransformConfRef().GetEmpty() {
		return errors.New("external transform configuration is unavailable")
	}
	if len(ref.GetTransformConf().GetSteps()) > 8 {
		return errors.New("transform chain exceeds the bounded interpretation limit")
	}
	for _, step := range ref.GetTransformConf().GetSteps() {
		switch step.GetId() {
		case transform_blockenc.ConfigID, transform_gzip.ConfigID:
		default:
			return errors.New("unsupported transform with no admitted decoded-size bound: " + step.GetId())
		}
	}
	return nil
}

// normalizedContext removes incidental roots and canonicalizes empty transforms.
func normalizedContext(ref *bucket.ObjectRef) *bucket.ObjectRef {
	// Canonicalize only the fields that affect interpreting these exact bytes.
	result := ref.CloneVT()
	if result == nil {
		result = &bucket.ObjectRef{}
	}
	result.RootRef = nil
	if result.GetTransformConf().GetEmpty() {
		result.TransformConf = nil
	}
	if result.GetTransformConfRef().GetEmpty() {
		result.TransformConfRef = nil
	}
	return result
}

// contextIdentity compares only bucket and resolved transformation identity.
func contextIdentity(ref *bucket.ObjectRef) []byte {
	data, _ := normalizedContext(ref).MarshalVT()
	return data
}
