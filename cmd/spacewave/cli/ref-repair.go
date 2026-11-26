//go:build !js

package spacewave_cli

import (
	"bytes"
	"context"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/pkg/errors"
	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_store "github.com/s4wave/spacewave/db/block/store"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_store "github.com/s4wave/spacewave/db/bucket/store"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/s4wave/spacewave/db/world/refwalk"
	"github.com/sirupsen/logrus"
)

const (
	// refRepairProgress is the number of blocks walked between progress logs.
	refRepairProgress = 100_000
	// refRepairSample limits the listless blocks printed in the report.
	refRepairSample = 10

	// frontAbsent is the front cause of a block no read resolves.
	frontAbsent = "absent"
	// frontNotBlob is the front cause of a payload root the walk could not
	// confirm as a blob.
	frontNotBlob = "payload root, not certainly a blob"
	// frontUndecodable is the front cause of a block that does not decode
	// as its type.
	frontUndecodable = "undecodable as its type"
	// frontUntyped prefixes the front cause of a ref whose type the walk
	// cannot name, followed by the walker's reason.
	frontUntyped = "untyped, "
	// frontRest is the front cause of every other block after a repair,
	// which marked each block it decoded.
	frontRest = "not reached by the walk, or its decoded list refused"
	// frontRestDry is frontRest on a dry run, which marked nothing.
	frontRestDry = "decoded by this walk, or not reached by it"
)

// refRepairFront counts the blocks one cause left on the front.
type refRepairFront struct {
	// count is the number of blocks.
	count uint64
	// sample is the first of them in key order.
	sample *block.BlockRef
}

// refRepairStoredBlock describes a listless block without decoding its type.
type refRepairStoredBlock struct {
	// ref addresses the stored block.
	ref *block.BlockRef
	// size is the number of stored bytes.
	size uint64
}

// refRepairResult counts what one ref repair found and wrote.
type refRepairResult struct {
	// spaces is the number of Spaces walked.
	spaces int
	// blocks is the number of distinct blocks walked.
	blocks uint64
	// absent is the number of referenced blocks no read resolves.
	absent uint64
	// peerRead is the number of walked blocks the volume lacks that the
	// peer read resolved, whose lists the rebuild writes without bytes.
	peerRead uint64
	// synthetic is the number of blocks listed with no children because the
	// synthetic set names them, as refRepair.synthetic describes.
	synthetic uint64
	// payloadBlobs is the number of payload roots read as blobs, which the
	// walker typed by a guess it could confirm.
	payloadBlobs uint64
	// notBlob is the number of payload roots the guess could not confirm as
	// blobs, so the walk verified nothing for them.
	notBlob uint64
	// notBlobSample holds up to refRepairSample of them.
	notBlobSample []*block.BlockRef
	// undecodable is the number of blocks that failed to decode as their type.
	// A block whose type holds no refs is only checked for presence.
	undecodable uint64
	// lacking is the number of walked blocks stored without a child list.
	lacking uint64
	// edges is the number of children those lists name.
	edges uint64
	// rebuild is what the list rebuild verified and wrote, or on a dry run
	// would write, empty on a volume whose old generation had left Open.
	rebuild block_gc.ListRebuildResult
	// front counts the blocks the rebuild's front holds by why the walk
	// verified nothing for them, as measureFront describes.
	front map[string]refRepairFront
	// verified reports that the volume's old generation had left Open before
	// the run, so it rebuilt nothing.
	verified bool
	// untyped counts the refs whose type the walk cannot name, by reason, with
	// a sample of each. Their edges are recorded; the blocks below them are
	// not walked, so their lists stay unverified until their type is known.
	untyped map[string]refwalk.Untyped
	// pluginStores counts object-store keys by plugin-volume/<instance>/<plugin>/ prefix.
	// The prefix groups stores whose boundaries are not encoded in their keys.
	pluginStores map[string]uint64
	// untracked counts the stored blocks without a child list.
	untracked uint64
	// untrackedBytes is the total stored bytes of those blocks.
	untrackedBytes uint64
	// untrackedSample holds up to refRepairSample of them in key order.
	untrackedSample []refRepairStoredBlock
}

// refRepair walks the World graphs the Spaces of a stopped volume retain with
// a refwalk.Walker, and hands each decoded block's refs to a list rebuild,
// which writes the list when it is missing or short and marks it verified. A
// block's list is the refs block.ExtractBlockRefs reads from the decoded
// block, which are the refs a write records for it.
type refRepair struct {
	// le is the logger.
	le *logrus.Entry
	// vol is the stopped volume.
	vol *volume_s4db.Volume
	// buckets lists the volume's bucket configs.
	buckets bucket_store.Store
	// walker walks the blocks below the Worlds.
	walker *refwalk.Walker
	// peer reads the blocks the volume lacks, nil to read the volume alone.
	// The walk never fills the volume from it.
	peer block.StoreOps
	// synthetic names the blocks a rehearsal lists with no children when no
	// read decodes them, by their keys, and records whether the walk did.
	// It proves nothing about them: it lets a rehearsal reach the flip on a
	// copy whose unresolved blocks hold it.
	synthetic map[string]bool
	// rebuild verifies the decoded blocks' lists, or on a dry run compares
	// them; nil on a volume whose old generation had left Open.
	rebuild *block_gc.ListRebuild
	// unread names why the walk verified nothing for a block it reached,
	// by the ref's string: frontAbsent, frontNotBlob or frontUndecodable.
	unread map[string]string
	// res counts the findings.
	res refRepairResult
}

// newRefRepair constructs a ref repair over a stopped volume, typing objects'
// roots with lookup. With peer set the walk reads each block the volume
// lacks from peer, and never fills the volume.
func newRefRepair(le *logrus.Entry, vol *volume_s4db.Volume, peer block.StoreOps, lookup refwalk.LookupBlockType) *refRepair {
	var reads block.StoreOps = vol
	if peer != nil {
		reads = block_store.NewStoreReadThrough(
			func() block.StoreOps { return vol },
			func() block.StoreOps { return peer },
			nil,
		)
	}
	return &refRepair{
		le:      le,
		vol:     vol,
		buckets: vol,
		walker:  refwalk.NewWalker(le, reads, lookup),
		peer:    peer,
		unread:  make(map[string]string),
	}
}

// measureFront counts the blocks the rebuild's front holds by why the walk
// verified nothing for them: no read resolves the block, it is not what its
// type says, or the walk stopped at it for a type it cannot name. A repair
// marked every other block it reached, so the rest were not reached or had
// their decoded list refused; a dry run marked nothing, so its rest also
// holds the blocks a repair would verify. Call it after the walk and the
// rebuild's Finish.
func (r *refRepair) measureFront(ctx context.Context, dryRun bool) error {
	rest := frontRest
	if dryRun {
		rest = frontRestDry
	}
	r.res.front = make(map[string]refRepairFront)
	return r.rebuild.EachFront(ctx, func(ref *block.BlockRef) error {
		// Name the cause the walk recorded for the block.
		cause := rest
		if why, ok := r.unread[ref.MarshalString()]; ok {
			cause = why
		} else if reason := r.walker.UntypedReason(ref); reason != "" {
			cause = frontUntyped + reason
		}

		// Count it, keeping the first block of each cause.
		f := r.res.front[cause]
		if f.count == 0 {
			f.sample = ref
		}
		f.count++
		r.res.front[cause] = f
		return nil
	})
}

// measurePluginStores counts object-store keys under each plugin's volume prefix.
// It reads only keys; the values are root pointers whose trees are not measured.
func (r *refRepair) measurePluginStores(ctx context.Context, vol *volume_s4db.Volume) error {
	// Scan the plugin object-store namespace in one read transaction.
	tx, err := vol.GetKvtxStore().NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()
	prefix := vol.GetKvKey().GetObjectStorePrefixByID("plugin-volume")
	r.res.pluginStores = make(map[string]uint64)

	// Group by the escaped instance and plugin components, not store boundaries.
	return tx.ScanPrefixKeys(ctx, prefix, func(key []byte) error {
		// Count only keys with both escaped plugin-volume components.
		instance, rest, ok := strings.Cut(string(key[len(prefix):]), "/")
		if !ok {
			return nil
		}
		plugin, _, ok := strings.Cut(rest, "/")
		if ok {
			r.res.pluginStores["plugin-volume/"+instance+"/"+plugin+"/"]++
		}
		return nil
	})
}

// measureUntracked scans every stored block, including blocks outside the typed
// walk, and measures those stored without a child list.
func (r *refRepair) measureUntracked(ctx context.Context, vol *volume_s4db.Volume) error {
	// Iterate block keys and their child lists in one snapshot.
	tx, err := vol.GetKvtxStore().NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()
	rec := volume_kvtx.OpenRecords(tx)
	prefix := vol.GetKvKey().GetBlockFullPrefix()
	it := tx.Iterate(ctx, prefix, true, false)
	defer it.Close()

	// Count each block without a list.
	var scanned uint64
	for it.Next() {
		ref := &block.BlockRef{}
		if err := ref.UnmarshalVT(bytes.Clone(it.Key()[len(prefix):])); err != nil {
			return errors.Wrap(err, "decode stored block key")
		}
		listed, err := rec.HasChildren(ctx, ref)
		if err != nil {
			return err
		}
		scanned++
		if scanned%refRepairProgress == 0 {
			r.le.Infof("scanned %d stored blocks, %d without a child list", scanned, r.res.untracked)
		}
		if listed {
			continue
		}
		value, err := it.Value()
		if err != nil {
			return err
		}
		blk := refRepairStoredBlock{ref: ref, size: uint64(len(value))}
		r.res.untracked++
		r.res.untrackedBytes += blk.size
		if len(r.res.untrackedSample) < refRepairSample {
			r.res.untrackedSample = append(r.res.untrackedSample, blk)
		}
	}
	return it.Err()
}

// walkSpace walks the roots of one Space's replay: the roots its bucket holds
// by name, the base and head Worlds of the cursor with their retained roots,
// and the World and payloads of each outcome. It walks every World before any
// payload, so a payload the Worlds name as an object root is walked under its
// object's type, as refwalk.Walker.PayloadNode describes.
func (r *refRepair) walkSpace(ctx context.Context, sc spaceReplayCursor) error {
	// Find the bucket that owns the Space's blocks.
	bucketID, err := findSpaceBucket(ctx, r.buckets, sc.spaceID)
	if err != nil {
		return err
	}
	spaceWorld := sc.world()
	r.res.spaces++

	// Collect the Worlds and payloads the bucket's named roots hold.
	worlds, payloads, err := r.bucketRoots(ctx, bucketID, spaceWorld)
	if err != nil {
		return err
	}

	// Collect the cursor's Worlds with their retained roots, and the World and
	// payloads of each outcome.
	for _, state := range []*space_world.InnerState{sc.cursor.GetBase(), sc.cursor.GetHead()} {
		head := state.GetHeadRef()
		if head == nil {
			continue
		}
		refs := []*block.BlockRef{head.GetRootRef()}
		for _, retained := range state.GetRetainedRoots() {
			refs = append(refs, retained.GetRootRef())
		}
		for _, ref := range refs {
			node, err := r.walker.WorldNode(head, ref)
			if err != nil {
				return err
			}
			worlds = append(worlds, node)
		}
	}
	for _, outcome := range sc.cursor.GetOutcomes() {
		node, err := r.walker.WorldNode(spaceWorld, outcome.GetWorld())
		if err != nil {
			return err
		}
		worlds = append(worlds, node)
		payloads = append(payloads, outcome.GetPayloads()...)
	}

	// Walk each World, then each payload the Worlds did not type.
	r.le.Infof("walking %d worlds and %d payloads of space %s in bucket %s", len(worlds), len(payloads), sc.spaceID, bucketID)
	for _, root := range worlds {
		if err := r.walker.Walk(ctx, root, r.visit); err != nil {
			return err
		}
	}
	for _, ref := range payloads {
		root, ok, err := r.walker.PayloadNode(spaceWorld, ref)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if err := r.walker.Walk(ctx, root, r.visit); err != nil {
			return err
		}
	}
	return nil
}

// legacyReplaySpanRootName names the root a Space engine before the adapter
// held its replay span under.
const legacyReplaySpanRootName = "replay-span"

// bucketRoots reads every root a Space's bucket holds by name, as
// space_world's roots.go lists them, and the legacy replay span. It verifies
// the list of each block that holds roots, and returns the Worlds and
// payload roots those blocks name.
func (r *refRepair) bucketRoots(ctx context.Context, bucketID string, spaceWorld *bucket.ObjectRef) ([]refwalk.Node, []*block.BlockRef, error) {
	// Each named root adds its Worlds or payload roots.
	var worlds []refwalk.Node
	var payloads []*block.BlockRef
	addWorlds := func(refs []*block.BlockRef) error {
		for _, ref := range refs {
			node, err := r.walker.WorldNode(spaceWorld, ref)
			if err != nil {
				return err
			}
			worlds = append(worlds, node)
		}
		return nil
	}
	for _, name := range spaceRootNames {
		ref, err := r.vol.GetBucketRoot(ctx, bucketID, name)
		if err != nil {
			return nil, nil, err
		}
		if ref.GetEmpty() {
			continue
		}
		switch name {
		case space_world.RootReplayBase:
			err = addWorlds([]*block.BlockRef{ref})
		case space_world.RootRetained:
			var roots []*block.BlockRef
			roots, err = r.retainedRoots(ctx, ref)
			if err == nil {
				err = addWorlds(roots)
			}
		case space_world.RootReplayPayloads:
			var roots []*block.BlockRef
			roots, err = r.heldRoots(ctx, ref)
			payloads = append(payloads, roots...)
		case legacyReplaySpanRootName:
			var spanWorlds, spanPayloads []*block.BlockRef
			spanWorlds, spanPayloads, err = r.spanRoots(ctx, ref)
			if err == nil {
				err = addWorlds(spanWorlds)
			}
			payloads = append(payloads, spanPayloads...)
		default:
			var roots []*block.BlockRef
			roots, err = r.heldRoots(ctx, ref)
			if err == nil {
				err = addWorlds(roots)
			}
		}
		if err != nil {
			return nil, nil, err
		}
	}
	return worlds, payloads, nil
}

// spaceRootNames are the names a Space's bucket holds roots under. The
// payloads root holds operation payloads, the replay base a World, the
// retained root a RetainedRootSet and the legacy span a ReplaySpan chain;
// every other root holds a HeldRoots chain of Worlds.
var spaceRootNames = []string{
	space_world.RootReplayBase,
	space_world.RootCheckpoint,
	space_world.RootImport,
	space_world.RootReplayCursor,
	space_world.RootReplayPayloads,
	space_world.RootRetained,
	space_world.RootPending,
	space_world.RootBackfill,
	legacyReplaySpanRootName,
}

// readRootBlock reads a block a named root holds, or reports it absent.
func (r *refRepair) readRootBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	// Read it from the volume alone.
	data, found, err := r.vol.GetBlock(ctx, ref)
	if err != nil || found {
		return data, found, err
	}

	// Count it absent and keep that as its front cause.
	r.res.absent++
	r.unread[ref.MarshalString()] = frontAbsent
	return nil, false, nil
}

// undecodableRoot counts a root block that does not decode as typ, its
// root's type, and hands it to the rebuild.
func (r *refRepair) undecodableRoot(ctx context.Context, ref *block.BlockRef, typ string, err error) error {
	// Count it and keep that as its front cause.
	r.le.WithError(err).Warnf("cannot decode root block %s", ref.MarshalString())
	r.res.undecodable++
	r.unread[ref.MarshalString()] = frontUndecodable

	// Let the rebuild count it when its list is marked.
	if r.rebuild == nil {
		return nil
	}
	return r.rebuild.Undecodable(ctx, ref, typ)
}

// heldRoots reads the HeldRoots chain at ref, verifying each block's list,
// and returns its roots.
func (r *refRepair) heldRoots(ctx context.Context, ref *block.BlockRef) ([]*block.BlockRef, error) {
	var roots []*block.BlockRef
	for !ref.GetEmpty() {
		data, found, err := r.readRootBlock(ctx, ref)
		if err != nil || !found {
			return roots, err
		}
		msg := &space_world.HeldRoots{}
		if err := msg.UnmarshalVT(data); err != nil {
			return roots, r.undecodableRoot(ctx, ref, typeName(msg), err)
		}
		if err := r.visitDecoded(ctx, ref, appendRef(msg.GetRoots(), msg.GetPrev()), typeName(msg)); err != nil {
			return nil, err
		}
		roots = append(roots, msg.GetRoots()...)
		ref = msg.GetPrev()
	}
	return roots, nil
}

// retainedRoots reads the RetainedRootSet at ref, verifying its list, and
// returns its World roots.
func (r *refRepair) retainedRoots(ctx context.Context, ref *block.BlockRef) ([]*block.BlockRef, error) {
	// Read and decode the set, then verify its list.
	data, found, err := r.readRootBlock(ctx, ref)
	if err != nil || !found {
		return nil, err
	}
	msg := &space_world.RetainedRootSet{}
	if err := msg.UnmarshalVT(data); err != nil {
		return nil, r.undecodableRoot(ctx, ref, typeName(msg), err)
	}
	roots := make([]*block.BlockRef, 0, len(msg.GetRoots()))
	for _, root := range msg.GetRoots() {
		roots = appendRef(roots, root.GetRootRef())
	}
	return roots, r.visitDecoded(ctx, ref, roots, typeName(msg))
}

// spanRoots reads the ReplaySpan chain at ref, raw protobuf blocks, verifying
// each block's list, and returns its Worlds and payload roots.
func (r *refRepair) spanRoots(ctx context.Context, ref *block.BlockRef) ([]*block.BlockRef, []*block.BlockRef, error) {
	var worlds, payloads []*block.BlockRef
	for !ref.GetEmpty() {
		data, found, err := r.readRootBlock(ctx, ref)
		if err != nil || !found {
			return worlds, payloads, err
		}
		span := &space_world.ReplaySpan{}
		if err := span.UnmarshalVT(data); err != nil {
			return worlds, payloads, r.undecodableRoot(ctx, ref, typeName(span), err)
		}
		refs := appendRef(slices.Concat(span.GetWorlds(), span.GetPayloads()), span.GetPrev())
		if err := r.visitDecoded(ctx, ref, refs, typeName(span)); err != nil {
			return nil, nil, err
		}
		worlds = append(worlds, span.GetWorlds()...)
		payloads = append(payloads, span.GetPayloads()...)
		ref = span.GetPrev()
	}
	return worlds, payloads, nil
}

// appendRef appends ref to refs unless it is empty.
func appendRef(refs []*block.BlockRef, ref *block.BlockRef) []*block.BlockRef {
	if ref.GetEmpty() {
		return refs
	}
	return append(refs, ref)
}

// visit counts a batch of reads and verifies each decoded block's list.
func (r *refRepair) visit(ctx context.Context, batch []refwalk.Visit) error {
	for _, rd := range batch {
		// List a synthetic block no read decodes with no children.
		if (!rd.Found || rd.DecodeErr != nil) && r.listSynthetic(rd.Node.Ref) {
			if err := r.verify(ctx, rd.Node.Ref, nil, ""); err != nil {
				return err
			}
			continue
		}

		// Count a block no read resolves, a leaf, and an undecodable block.
		if !rd.Found {
			r.res.absent++
			r.unread[rd.Node.Ref.MarshalString()] = frontAbsent
			continue
		}
		// Verify nothing for a payload root that is not certainly a blob,
		// which stays on the front if held, so the flip names it.
		if rd.Unsure {
			r.res.notBlob++
			r.unread[rd.Node.Ref.MarshalString()] = frontNotBlob
			if len(r.res.notBlobSample) < refRepairSample {
				r.res.notBlobSample = append(r.res.notBlobSample, rd.Node.Ref)
			}
			if r.rebuild != nil {
				if err := r.rebuild.Undecodable(ctx, rd.Node.Ref, nodeType(rd.Node)); err != nil {
					return err
				}
			}
			continue
		}
		if rd.Node.Guess {
			r.res.payloadBlobs++
		}
		if rd.DecodeErr != nil {
			r.le.WithError(rd.DecodeErr).Warnf("cannot decode %s", rd.Node.Ref.MarshalString())
			r.countBlock()
			r.res.undecodable++
			r.unread[rd.Node.Ref.MarshalString()] = frontUndecodable
			if r.rebuild != nil {
				if err := r.rebuild.Undecodable(ctx, rd.Node.Ref, nodeType(rd.Node)); err != nil {
					return err
				}
			}
			continue
		}
		if rd.Leaf {
			r.countBlock()
			if err := r.verify(ctx, rd.Node.Ref, nil, ""); err != nil {
				return err
			}
			continue
		}

		// Count and verify the decoded block's list.
		if err := r.visitDecoded(ctx, rd.Node.Ref, rd.Refs, nodeType(rd.Node)); err != nil {
			return err
		}
	}
	return nil
}

// listSynthetic reports whether the synthetic set names ref, and records
// that the walk listed it.
func (r *refRepair) listSynthetic(ref *block.BlockRef) bool {
	// Mark the named ref listed and count it.
	key := ref.MarshalString()
	if _, ok := r.synthetic[key]; !ok {
		return false
	}
	r.synthetic[key] = true
	r.res.synthetic++
	return true
}

// listUnwalkedSynthetic lists with no children each block of the synthetic
// set the walk never reached, such as an object root whose type does not
// read.
func (r *refRepair) listUnwalkedSynthetic(ctx context.Context) error {
	for _, key := range slices.Sorted(maps.Keys(r.synthetic)) {
		if r.synthetic[key] {
			continue
		}
		ref, err := block.UnmarshalBlockRefB58(key)
		if err != nil {
			return err
		}
		r.listSynthetic(ref)
		if err := r.verify(ctx, ref, nil, ""); err != nil {
			return err
		}
	}
	return nil
}

// visitDecoded counts a block decoded as typ, and its list when the volume
// lacks one, and verifies the list.
func (r *refRepair) visitDecoded(ctx context.Context, ref *block.BlockRef, refs []*block.BlockRef, typ string) error {
	// Count the block, and a block only the peer read resolved.
	r.countBlock()
	if r.peer != nil {
		stored, err := r.vol.GetBlockExists(ctx, ref)
		if err != nil {
			return err
		}
		if !stored {
			r.res.peerRead++
		}
	}

	// Count its refs when the volume lacks its list.
	listed, err := r.vol.HasChildLists(ctx, []*block.BlockRef{ref})
	if err != nil {
		return err
	}
	if !listed[0] {
		r.res.lacking++
		r.res.edges += uint64(len(refs))
	}
	return r.verify(ctx, ref, refs, typ)
}

// countBlock counts one walked block that the volume stores.
func (r *refRepair) countBlock() {
	r.res.blocks++
	if r.res.blocks%refRepairProgress == 0 {
		r.le.Infof("walked %d blocks", r.res.blocks)
	}
}

// verify hands the refs of a block decoded as typ to the list rebuild,
// unless the volume's old generation had left Open.
func (r *refRepair) verify(ctx context.Context, ref *block.BlockRef, refs []*block.BlockRef, typ string) error {
	if r.rebuild == nil {
		return nil
	}
	return r.rebuild.VerifyAs(ctx, ref, refs, typ)
}

// nodeType names the type the walker decodes n as, empty when it has none.
func nodeType(n refwalk.Node) string {
	if n.Ctor == nil {
		return ""
	}
	return typeName(n.Ctor())
}

// typeName names the Go type of a decoded block, for the rebuild's samples.
func typeName(blk any) string {
	return reflect.TypeOf(blk).String()
}

// close releases the walker and keeps what it could not type.
func (r *refRepair) close() {
	r.res.untyped = r.walker.Untyped()
	r.walker.Close()
}
