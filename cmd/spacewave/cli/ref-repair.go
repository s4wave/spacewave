//go:build !js

package spacewave_cli

import (
	"bufio"
	"context"
	"reflect"
	"strings"

	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/space/world/rootwalk"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_store "github.com/s4wave/spacewave/db/block/store"
	bucket_store "github.com/s4wave/spacewave/db/bucket/store"
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
	// frontOpaqueUnauthenticated is the scan's front cause when bytes fail
	// to parse as a message and the transform cannot prove them.
	frontOpaqueUnauthenticated = "not a message, under a transform that does not authenticate"
	// frontOpaqueUnlisted is the scan's front cause when opaque bytes have
	// no recorded child list to preserve.
	frontOpaqueUnlisted = "not a message, without a stored list"
	// frontOpaqueListed is the scan's front cause when the stored list
	// names children the opaque bytes cannot account for.
	frontOpaqueListed = "not a message, with a stored list that names children"
	// frontUntyped prefixes the front cause of a ref whose type the walk
	// cannot identify, followed by the walker's reason.
	frontUntyped = "untyped, "
	// frontRest is the front cause of every other block after a repair,
	// which marked each block it decoded.
	frontRest = "not reached by the walk, or its decoded list refused"
	// frontRestDry is frontRest on a dry run, which marked nothing.
	frontRestDry = "decoded by this walk, or not reached by it"
	// frontNoRoot follows frontRest for a block no named root reaches
	// through the stored lists.
	frontNoRoot = "; no named root reaches it"
	// frontRootOf follows frontRest for a block a named root reaches,
	// followed by the root's bucket and name.
	frontRootOf = "; under root "
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
	// legacySpansReleased counts obsolete roots released, or planned on a dry run.
	legacySpansReleased uint64
	// blocks is the number of distinct blocks walked.
	blocks uint64
	// absent is the number of referenced blocks no read resolves.
	absent uint64
	// lacksListing reports that the run requested a lacks listing.
	lacksListing bool
	// lacksListed counts distinct refs written to the lacks listing.
	lacksListed uint64
	// peerRead is the number of walked blocks the volume lacks that the
	// peer read resolved, whose lists the rebuild writes without bytes.
	peerRead uint64
	// peerListing reports that the run requested a peer listing.
	peerListing bool
	// peerListed counts distinct refs served by the peer and listed.
	peerListed uint64
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
	// edges is the number of children those lists reference.
	edges uint64
	// rebuild is what the list rebuild verified and wrote, or on a dry run
	// would write, empty on a volume whose old generation had left Open.
	rebuild block_gc.ListRebuildResult
	// front counts the blocks the rebuild's front holds by why the walk
	// verified nothing for them, as measureFront describes.
	front map[string]refRepairFront
	// frontListed reports that the run finished writing a front listing.
	frontListed bool
	// roots is the number of named roots and rootListed the number of
	// listed blocks they reach, both set only when the run asked for the
	// front's roots, which rooted reports.
	roots      int
	rootListed uint64
	rooted     bool
	// verified reports that the volume's old generation had left Open before
	// the run, so it rebuilt nothing.
	verified bool
	// untyped counts the refs whose type the walk cannot identify, by reason,
	// with a sample of each. Their edges are recorded, but the blocks below
	// them are not walked, so their lists stay unverified until their type is
	// known.
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
	// scan counts what a walk that scans found.
	scan refRepairScan
}

// listingField replaces the tabs and line breaks in one field of a listing
// row, which separate the row's fields and the rows.
var listingField = strings.NewReplacer("\t", " ", "\n", " ", "\r", " ")

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
	// scanVerify verifies the lists of the blocks the walk reads by scanning.
	scanVerify bool
	// dropLegacySpans releases obsolete replay-span roots instead of walking them.
	dropLegacySpans bool
	// dryRun compares lists and counts releases without changing the Volume.
	dryRun bool
	// peer reads the blocks the volume lacks, nil to read the volume alone.
	// The walk never fills the volume from it.
	peer block.StoreOps
	// rebuild verifies the decoded blocks' lists, or on a dry run compares
	// them. Nil on a volume whose old generation had left Open.
	rebuild *block_gc.ListRebuild
	// unread records why the walk verified nothing for a block it reached,
	// by the ref's string: frontAbsent, frontNotBlob or frontUndecodable.
	unread map[string]string
	// lacks receives absent refs as they are reached, nil to omit the listing.
	lacks *bufio.Writer
	// histogram counts the walked blocks by type and size, nil when the run
	// asked for none.
	histogram *refwalk.Histogram
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
// type says, or the walk stopped at it for a type it cannot identify. A repair
// marked every other block it reached, so the rest were not reached or had
// their decoded list refused. A dry run marked nothing, so its rest also
// holds the blocks a repair would verify. With roots set it also walks the
// stored lists from every named root, as block_gc.ListRebuild.WalkRoots
// describes, and splits the rest by the first root that reaches each block:
// a block under a root is live by the lists and the typed walk stopped above
// it, and a block under none is held only by blocks nothing reaches.
// When listing is set, stream each entry in front order with its report cause
// and scan fate. Call it after the walk and the rebuild's Finish.
func (r *refRepair) measureFront(ctx context.Context, dryRun, roots bool, listing *bufio.Writer) error {
	// Classify the rest by what this run's walk did with the blocks it reached.
	rest := frontRest
	if dryRun {
		rest = frontRestDry
	}
	r.res.front = make(map[string]refRepairFront)
	count := func(ref *block.BlockRef, root *block_gc.FrontRoot) error {
		// Select the cause the walk recorded for the block, and for the rest
		// the root that reaches it.
		key := ref.MarshalString()
		cause := rest
		if why, ok := r.unread[key]; ok {
			cause = why
		} else if reason := r.walker.UntypedReason(ref); reason != "" {
			cause = frontUntyped + reason
		} else if roots && root == nil {
			cause += frontNoRoot
		} else if roots {
			cause += frontRootOf + root.BucketID + " " + root.Name
		}

		// Count its cause and fate, keeping the first block of each cause.
		f := r.res.front[cause]
		if f.count == 0 {
			f.sample = ref
		}
		f.count++
		r.res.front[cause] = f
		var fate string
		if r.res.scan.on {
			fate = r.scanFate(ref, root, roots)
			r.res.scan.front[fate]++
		}

		// Write the same cause and fate with tabs reserved for field breaks.
		if listing == nil {
			return nil
		}
		_, err := listing.WriteString(key + "\t" + listingField.Replace(cause) + "\t" + listingField.Replace(fate) + "\n")
		return err
	}

	// Walk the front in key order, with root reach when requested.
	if roots {
		// Walk the stored lists once for the front and the weak blocks.
		reach, err := r.rebuild.WalkRoots(ctx)
		if err != nil {
			return err
		}
		r.res.roots, r.res.rootListed, r.res.rooted = reach.Roots(), reach.Listed(), true
		r.res.scan.measureWeak(reach)
		return reach.EachFront(count)
	}
	return r.rebuild.EachFront(ctx, func(ref *block.BlockRef) error {
		return count(ref, nil)
	})
}

// measurePluginStores counts object-store keys under each plugin's volume
// prefix. It reads only keys. The values are root pointers whose trees are not
// measured.
func (r *refRepair) measurePluginStores(ctx context.Context, vol *volume_s4db.Volume) error {
	// Group logical plugin-volume keys by escaped instance and plugin.
	r.res.pluginStores = make(map[string]uint64)
	return vol.ScanObjectStoreKeys(ctx, "plugin-volume", func(key []byte) error {
		// Count only keys with both escaped plugin-volume components.
		instance, rest, ok := strings.Cut(string(key), "/")
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
	// Count blocks and listless bytes from the Volume's single-snapshot scan.
	var scanned uint64
	return vol.ScanChildListCoverage(ctx, func(ref *block.BlockRef, listed bool, size uint64) error {
		// Report progress across every stored block, including listed blocks.
		scanned++
		if scanned%refRepairProgress == 0 {
			r.le.Infof("scanned %d stored blocks, %d without a child list", scanned, r.res.untracked)
		}
		if listed {
			return nil
		}

		// Retain a bounded sample and totals for listless blocks.
		blk := refRepairStoredBlock{ref: ref, size: size}
		r.res.untracked++
		r.res.untrackedBytes += blk.size
		if len(r.res.untrackedSample) < refRepairSample {
			r.res.untrackedSample = append(r.res.untrackedSample, blk)
		}
		return nil
	})
}

// walkSpace walks every block one Space retains, as rootwalk.Walk
// describes, with the Space's roots read from the bucket that owns its
// blocks and its published graph under the account World root.
func (r *refRepair) walkSpace(ctx context.Context, sc spaceReplayCursor) error {
	// Find the bucket that owns the Space's blocks.
	bucketID, err := findSpaceBucket(ctx, r.buckets, sc.spaceID)
	if err != nil {
		return err
	}
	r.res.spaces++

	// Read the Space's roots from its bucket, releasing the legacy span
	// when the run asked to.
	sp := rootwalk.Space{
		World: sc.world(),
		GetRoot: func(ctx context.Context, name string) (*block.BlockRef, error) {
			return r.vol.GetBucketRoot(ctx, bucketID, name)
		},
		RootBlocks:    r.vol,
		Cursor:        sc.cursor,
		PublishedRoot: provider_local.AccountWorldRoot,
	}
	if r.dropLegacySpans {
		sp.ReleaseLegacySpan = func(ctx context.Context, ref *block.BlockRef) error {
			return r.releaseLegacySpan(ctx, bucketID, ref)
		}
	}

	// Walk the Space.
	r.le.Infof("walking space %s in bucket %s", sc.spaceID, bucketID)
	return rootwalk.Walk(ctx, r.walker, sp, rootwalk.Visitor{Root: r.visitRoot, Blocks: r.walkVisit})
}

// releaseLegacySpan releases the legacy replay span root of bucketID, which
// holds ref, instead of walking its chain; a dry run counts it. It is a
// compare-and-set on ref, so a root that moved fails the run. It writes no
// block and deletes none: Land takes the root's holder from the span, by a
// logged down on an adopted Volume and by the adoption's recount otherwise,
// and the collector frees only what no other holder names. The walk reaches
// it before the Space's Worlds, so the rebuild never verifies the discarded
// chain or pushes its children onto the front.
//
// The release relies on the adapter holding every payload it replays under
// its own roots, which it does each time it mounts the Space. A build from
// before the adapter that wrote the Space after the adapter's last mount
// would have held its payloads under the span alone, so run this only on a
// Volume no such build has written.
func (r *refRepair) releaseLegacySpan(ctx context.Context, bucketID string, ref *block.BlockRef) error {
	if !r.dryRun {
		roots := []block.NamedRoot{{Name: rootwalk.LegacyReplaySpanRoot, Previous: ref}}
		if err := r.vol.SetBucketRoots(ctx, bucketID, nil, roots); err != nil {
			return err
		}
	}
	r.res.legacySpansReleased++
	return nil
}

// visitRoot counts a root block the walk read and verifies its list: an
// absent block keeps frontAbsent as its front cause, and an undecodable one
// goes to the rebuild as undecodable.
func (r *refRepair) visitRoot(ctx context.Context, rb rootwalk.RootBlock) error {
	switch {
	case !rb.Found:
		r.res.absent++
		err := r.listAbsent(rb.Ref, "root")
		r.unread[rb.Ref.MarshalString()] = frontAbsent
		return err
	case rb.DecodeErr != nil:
		return r.undecodableRoot(ctx, rb.Ref, rb.Type, rb.DecodeErr)
	default:
		return r.visitDecoded(ctx, rb.Ref, rb.Refs, rb.Type)
	}
}

// listAbsent streams the first absence of a ref across roots and both walks.
// The caller records frontAbsent in its existing map after this returns.
func (r *refRepair) listAbsent(ref *block.BlockRef, reached string) error {
	// Use the walk's cause maps to avoid a second set of listed refs.
	key := ref.MarshalString()
	if r.lacks == nil || r.unread[key] == frontAbsent || r.res.scan.failed[key] == frontAbsent {
		return nil
	}

	// Write the row.
	if _, err := r.lacks.WriteString(key + "\t" + listingField.Replace(reached) + "\n"); err != nil {
		return err
	}
	r.res.lacksListed++
	return nil
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

// walkVisit counts a batch the walk read in the histogram, when the run asked
// for one, and visits it.
func (r *refRepair) walkVisit(ctx context.Context, batch []refwalk.Visit) error {
	if r.histogram != nil {
		for _, rd := range batch {
			r.histogram.Add(rd)
		}
	}
	return r.visit(ctx, batch)
}

// visit counts a batch of reads and verifies each decoded block's list.
func (r *refRepair) visit(ctx context.Context, batch []refwalk.Visit) error {
	for _, rd := range batch {
		// Compare a block read by scanning with its stored list and verify
		// the list if the run asked for it.
		if rd.Node.Untyped != "" {
			if err := r.visitScanned(ctx, rd); err != nil {
				return err
			}
			continue
		}

		// Count a block no read resolves, a leaf, and an undecodable block.
		if !rd.Found {
			r.res.absent++
			if err := r.listAbsent(rd.Node.Ref, nodeType(rd.Node)); err != nil {
				return err
			}
			r.unread[rd.Node.Ref.MarshalString()] = frontAbsent
			continue
		}
		// Verify nothing for a payload root that is not certainly a blob,
		// which stays on the front if held, so the flip reports the root.
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
		if err := r.visitTyped(ctx, rd); err != nil {
			return err
		}
		if err := r.visitDecoded(ctx, rd.Node.Ref, rd.Refs, nodeType(rd.Node)); err != nil {
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

// verify hands the block's refs and its type or scan reason to the list
// rebuild, unless the volume's old generation had left Open.
func (r *refRepair) verify(ctx context.Context, ref *block.BlockRef, refs []*block.BlockRef, typ string) error {
	if r.rebuild == nil {
		return nil
	}
	return r.rebuild.VerifyAs(ctx, ref, refs, typ)
}

// nodeType returns the type the walker decodes n as, empty when n has none.
func nodeType(n refwalk.Node) string {
	if n.Ctor == nil {
		return ""
	}
	return typeName(n.Ctor())
}

// typeName returns the Go type of a decoded block, for the rebuild's samples.
func typeName(blk any) string {
	return reflect.TypeOf(blk).String()
}

// close releases the walker and keeps what it could not type.
func (r *refRepair) close() {
	r.res.untyped = r.walker.Untyped()
	r.walker.Close()
}
