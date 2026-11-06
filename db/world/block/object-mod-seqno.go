package world_block

import (
	"context"
	"os"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	kvtx_block_okra "github.com/s4wave/spacewave/db/kvtx/block/okra"
)

// ModSeqnoEnv names the environment variable that turns ModSeqno stamping off
// when set to "off". Commits then write records as before ModSeqno existed: a
// written record keeps whatever ModSeqno it carried, and stays stale until a
// commit with stamping on writes it again. It is the fallback while stamping
// is new. ModSeqno is part of each record's hash, so every replica of a Shared
// Object must use the same setting, or their checkpoints disagree.
const ModSeqnoEnv = "SPACEWAVE_WORLD_MOD_SEQNO"

// modSeqnoOff is read once at startup from ModSeqnoEnv.
var modSeqnoOff = os.Getenv(ModSeqnoEnv) == "off"

// markObjectWritten records that the object record at key now lives in
// cursor, so the next Commit stamps it with the seqno it reaches. A key
// written twice keeps both cursors, since a changelog entry may hold the
// first.
//
// Every write of an Object record into the object tree calls it; a record
// written without it keeps the ModSeqno of an earlier commit.
func (t *WorldState) markObjectWritten(key string, cursor *block.Cursor) {
	if t.modObjects == nil {
		t.modObjects = make(map[string][]*block.Cursor)
	}
	cursors := t.modObjects[key]
	if len(cursors) != 0 && cursors[len(cursors)-1] == cursor {
		return
	}
	t.modObjects[key] = append(cursors, cursor)
}

// forgetObjectWritten drops key when its record leaves the object tree, so
// Commit never writes a record back under a deleted or renamed key.
func (t *WorldState) forgetObjectWritten(key string) {
	delete(t.modObjects, key)
}

// stampModSeqno sets ModSeqno to seqno on every object record written since
// the last Commit. It runs after the changelog flush, when seqno is the one
// this commit reaches, and before the block write. The written set stays
// until the write succeeds, so a retried Commit stamps it again.
//
// Invariant: after Commit, every record a commit wrote carries that commit's
// seqno, and no other record changes. The stamp writes through the cursor the
// record was written to, which the changelog entry for the write also holds,
// so the entry and the tree name the same block.
func (t *WorldState) stampModSeqno(ctx context.Context, seqno uint64) error {
	// Leave records untouched when stamping is off.
	if modSeqnoOff {
		return nil
	}

	// Rewrite each written record that does not carry seqno yet. A packed tree
	// stores records inline, so it takes the stamped tree record back
	// explicitly.
	_, packed := t.objTree.(*kvtx_block_okra.Tx)
	for key, cursors := range t.modObjects {
		for i, cursor := range cursors {
			stamped, err := stampObjectCursor(ctx, cursor, seqno)
			if err != nil {
				return errors.Wrapf(err, "object %q", key)
			}
			if !stamped || !packed || i != len(cursors)-1 {
				continue
			}
			err = t.objTree.SetCursorAtKey(ctx, []byte(objectKeyPrefix+key), cursor, false)
			if err != nil {
				return errors.Wrapf(err, "object %q", key)
			}
		}
	}
	return nil
}

// stampObjectCursor sets ModSeqno on the record in cursor, reporting whether
// it changed the record.
func stampObjectCursor(ctx context.Context, cursor *block.Cursor, seqno uint64) (bool, error) {
	// Skip a missing record or one already stamped.
	obj, err := UnmarshalObject(ctx, cursor)
	if err != nil || obj == nil || obj.GetModSeqno() == seqno {
		return false, err
	}

	// Write a stamped copy, leaving the cached record unchanged.
	obj = obj.Clone()
	obj.ModSeqno = seqno
	cursor.SetBlock(obj, true)
	return true, nil
}

// GetModSeqno returns the World seqno after the last commit that wrote this
// object's record. A record written in the open transaction reports the
// seqno of the commit before it until Commit stamps it.
func (o *ObjectState) GetModSeqno(ctx context.Context) (uint64, error) {
	root, err := o.GetRoot(ctx)
	if err != nil {
		return 0, err
	}
	return root.GetModSeqno(), nil
}
