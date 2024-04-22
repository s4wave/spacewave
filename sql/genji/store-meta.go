package kvtx_genji

import (
	"time"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
)

// NewStoreMeta constructs a new store metadata object.
func NewStoreMeta(created time.Time) *StoreMeta {
	return &StoreMeta{
		CreatedTs: timestamp.New(created),
	}
}
