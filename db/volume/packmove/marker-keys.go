package packmove

import (
	"bytes"

	"github.com/s4wave/spacewave/db/block"
	block_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
)

// markerKeys returns the keys that queue the block ref, holding value, for
// upload by the write-back object store at prefix. The zero write time sends
// the block at once.
func markerKeys(prefix []byte, ref *block.BlockRef, value []byte) []volume_kvtx.CopyKey {
	mark := block_writeback.Mark{Hash: ref.GetHash(), Size: int64(len(value))}
	marked, queue := block_writeback.MarkerKeys(mark)
	return []volume_kvtx.CopyKey{
		{Key: bytes.Join([][]byte{prefix, marked}, nil)},
		{Key: bytes.Join([][]byte{prefix, queue}, nil), Value: block_writeback.MarkerQueueValue(mark)},
	}
}
