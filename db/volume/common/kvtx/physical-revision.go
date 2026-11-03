package kvtx

import (
	"context"
	"encoding/binary"
	"errors"

	"github.com/s4wave/spacewave/db/kvtx"
)

// errPhysicalRevisionChanged leaves a record pending rather than applying a
// decision from before another physical writer changed its graph or counts.
var errPhysicalRevisionChanged = errors.New("physical snapshot changed")

// physicalRevisionKey is a private sibling of the Volume format metadata.
// Every live graph/block writer commits through openPhysical; startup identity
// writes affect no counting state, and format/copy writers require an unopened
// or empty destination. Recounts also carry their own generation/phase guard.
func (v *Volume) physicalRevisionKey() []byte {
	return append(v.kvKey.GetFormatVersionKey(), []byte("/revision")...)
}

// readPhysicalRevision reads the revision from the same transaction as the
// graph snapshot. A missing key is the initial revision of an existing store.
func readPhysicalRevision(ctx context.Context, tx kvtx.TxOps, key []byte) (uint64, error) {
	// An existing store starts at revision zero until its next live write.
	data, found, err := tx.Get(ctx, key)
	if err != nil || !found {
		return 0, err
	}
	revision, size := binary.Uvarint(data)
	if size <= 0 || size != len(data) {
		return 0, errors.New("physical revision is malformed")
	}
	return revision, nil
}

// advanceRevision changes one scalar in an already-changing physical commit.
// It costs one indexed write, with no additional commit and no graph walk.
// The transaction opened by openPhysical read the old revision under its
// writer, so even independent mounts advance this one durable sequence.
func (p *physicalTx) advanceRevision(ctx context.Context) error {
	return p.tx.Set(ctx, p.revisionKey, binary.AppendUvarint(nil, p.revision+1))
}
