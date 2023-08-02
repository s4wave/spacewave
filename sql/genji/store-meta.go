package kvtx_genji

import (
	"time"

	"github.com/aperturerobotics/timestamp"
)

// NewStoreMeta constructs a new store metadata object.
func NewStoreMeta(created time.Time) *StoreMeta {
	return &StoreMeta{
		CreatedTs: timestamp.ToTimestamp(created),
	}
}
