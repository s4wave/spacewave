package dex

import (
	// "github.com/s4wave/spacewave/net/peer"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// LookupBlockFromNetwork is a directive to find a block from remote DHT.
type LookupBlockFromNetwork interface {
	// Directive indicates LookupBlockFromNetwork is a directive.
	directive.Directive

	// LookupBlockFromNetworkBucketId returns the associated bucket id.
	// Can be empty.
	LookupBlockFromNetworkBucketId() string
	// LookupBlockFromNetworkRef returns the desired block ref.
	LookupBlockFromNetworkRef() *block.BlockRef
}

// LookupBlockFromNetworkValue is the result type for LookupBlockFromNetwork.
// Contains: error, data, ref, bucket id, source peer id, source volume id (?)
type LookupBlockFromNetworkValue interface {
	// GetError returns any error.
	GetError() error
	// GetData returns the returned data.
	// Returns nil if not found.
	GetData() []byte
	// GetProvider returns the peer who provided the data.
	// May be empty.
	// GetProvider() peer.ID
}

// lookupBlockFromNetworkValue implements LookupBlockFromNetworkValue
type lookupBlockFromNetworkValue struct {
	err  error
	data []byte
}

// NewLookupBlockFromNetworkValue builds a new LookupBlockFromNetworkValue.
func NewLookupBlockFromNetworkValue(data []byte, err error) LookupBlockFromNetworkValue {
	return &lookupBlockFromNetworkValue{data: data, err: err}
}

// GetError returns any error.
func (v *lookupBlockFromNetworkValue) GetError() error {
	return v.err
}

// GetData returns the returned data.
// Returns nil if not found.
func (v *lookupBlockFromNetworkValue) GetData() []byte {
	return v.data
}

// NewLookupBlockFromNetwork constructs an LookupBlockFromNetwork.
func NewLookupBlockFromNetwork(bucketID string, ref *block.BlockRef) LookupBlockFromNetwork {
	return &LookupBlockFromNetworkRequest{
		BucketId: bucketID,
		Ref:      ref,
	}
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *LookupBlockFromNetworkRequest) Validate() error {
	if d.LookupBlockFromNetworkRef().GetEmpty() {
		return errors.New("ref cannot be empty")
	}

	return nil
}

// GetValueLookupBlockFromNetworkOptions returns options relating to value handling.
func (d *LookupBlockFromNetworkRequest) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{}
}

// LookupBlockFromNetworkBucketId returns the desired bucket id.
// Can be empty.
func (d *LookupBlockFromNetworkRequest) LookupBlockFromNetworkBucketId() string {
	return d.GetBucketId()
}

// LookupBlockFromNetworkVolumeIDRe returns the volume ID constraint.
// Can be empty.
func (d *LookupBlockFromNetworkRequest) LookupBlockFromNetworkRef() *block.BlockRef {
	return d.GetRef()
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *LookupBlockFromNetworkRequest) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(LookupBlockFromNetwork)
	if !ok {
		return false
	}

	if d.LookupBlockFromNetworkBucketId() != od.LookupBlockFromNetworkBucketId() {
		return false
	}
	if !d.LookupBlockFromNetworkRef().EqualsRef(od.LookupBlockFromNetworkRef()) {
		return false
	}
	return true
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *LookupBlockFromNetworkRequest) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *LookupBlockFromNetworkRequest) GetName() string {
	return "LookupBlockFromNetwork"
}

// GetDebugString returns the directive arguments stringified.
func (d *LookupBlockFromNetworkRequest) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	if d.LookupBlockFromNetworkBucketId() != "" {
		vals["bucket-id"] = []string{d.LookupBlockFromNetworkBucketId()}
	}
	if vre := d.LookupBlockFromNetworkRef(); vre != nil && !vre.GetEmpty() {
		vals["ref"] = []string{vre.MarshalString()}
	}
	return vals
}

// _ is a type assertion
var _ LookupBlockFromNetwork = ((*LookupBlockFromNetworkRequest)(nil))
