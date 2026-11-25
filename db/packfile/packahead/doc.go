// Package packahead writes new block values sequentially into local
// packfiles, in the format remote pack stores hold, so the local packs are
// also the upload queue.
//
// A Log is a directory of packs. Writers append batches to the one open pack;
// each batch is a record followed by its values, and a sync makes every batch
// written so far durable with one fdatasync shared by every waiting writer. A
// full pack rotates: the writer moves to a spare pack opened ahead of time, and
// a background maintainer writes the full pack's sorted kvfile index after its
// last record, syncs it and renames it to its finalized name. A finalized pack
// is a valid kvfile: each index entry carries its value's offset and size, so
// the records between values are bytes no reader looks at.
//
// # Files
//
//	<seq>.open           a pack being written, or one a crash left unfinalized
//	<seq>-<pack id>.pack a finalized pack
//
// seq is 16 hex digits, so names sort in write order.
//
// # Batch record
//
//	magic   uint32 "PAH1"
//	bodyLen uint32
//	body:
//	  end     uint64  the pack's length after this batch's values
//	  dataLen uint64  the bytes of values that follow the record
//	  dataCRC uint32  CRC32C of those values
//	  count   uvarint
//	  count x (keyLen uvarint, key, size uvarint)
//	recCRC  uint32  CRC32C of magic through body
//	values  dataLen bytes, in entry order
//
// Each record carries the absolute end of its batch. Recovery uses that
// offset and both checksums to find intact batches past a damaged record.
// The finalized index omits damaged spans without moving the retained bytes.
//
// # Invariants
//
//   - D1: A batch's Append acknowledges durable bytes only while its pack has
//     no known failure (RowAfterSync and ReceiptedHeld in PackAhead.tla).
//   - D2: Recovery indexes every verified batch, including those past a
//     damaged record, and truncates only the trailing damaged bytes.
//   - A pack file is created and its directory synced before any batch is
//     written to it, so an acknowledged batch never lives in a file a crash
//     can drop from the directory.
//   - The table holds only locations some local pack holds: an entry is added
//     after its batch is written and removed if recovery discards that batch.
//   - A local pack is unlinked only after the remote holds its object and
//     entry and the pack index registers it (EvictCheck and
//     EvictedRegistered); eviction is added with the upload. The one
//     exception is EvictCarry, for a Log of copies: it drops what it does not
//     carry, and a carried value is relocated under the lock, so a reader
//     finds it in one pack or the other.
package packahead
