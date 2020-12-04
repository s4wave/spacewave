package kvtx_genji

import (
	"time"

	"github.com/aperturerobotics/timestamp"
)

// NewStoreMeta constructs a new store metadata object.
func NewStoreMeta(created time.Time) *StoreMeta {
	t := timestamp.ToTimestamp(created)
	return &StoreMeta{
		CreatedTs: &t,
	}
}
