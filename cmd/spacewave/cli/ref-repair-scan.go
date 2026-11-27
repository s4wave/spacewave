package spacewave_cli

import (
	"context"
	"maps"
	"slices"
	"strconv"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/world/refwalk"
)

// refRepairScan counts what a walk that scans found, as refwalk.ScanHits
// reads refs from bytes without their type. Its first half measures the scan
// against the types on the blocks the walk decodes. Its second half compares
// the scan with the stored lists of the blocks the walk has no type for.
type refRepairScan struct {
	// on reports that the walk scanned.
	on bool
	// typed is the number of blocks decoded by type and scanned alongside
	// that decoding, and typedRefs the number of refs their types
	// declare.
	typed, typedRefs uint64
	// missed counts the typed blocks holding a ref the scan did not find, by
	// type, and missedRefs those refs.
	missed     map[string]refRepairFront
	missedRefs uint64
	// extra is the number of hashes the scan found in typed blocks outside
	// their typed refs, and extraStored those referencing a stored block, by
	// the block's type and the hash's field.
	extra       uint64
	extraStored map[string]refRepairFront
	// shapes outlines up to refRepairScanShapes blocks with a missed ref.
	shapes []string
	// blocks counts the untyped blocks the scan read, by why the walk has
	// no type for the ref it reached them from.
	blocks map[string]uint64
	// only is the number of blocks read by scanning and under no type.
	only uint64
	// absent is the number of scanned refs no read resolves.
	absent uint64
	// undecodable counts the blocks their holder's transform does not decode.
	undecodable refRepairFront
	// opaque is the number of scanned blocks whose bytes are not a message.
	opaque uint64
	// opaqueListed counts scanned blocks whose bytes are not a message and
	// whose stored list names at least one child.
	opaqueListed refRepairFront
	// opaqueUnauthenticated counts scanned blocks whose bytes are not a
	// message and whose transform does not authenticate them.
	opaqueUnauthenticated refRepairFront
	// opaqueUnlisted counts scanned blocks whose bytes are not a message
	// and that have no stored list.
	opaqueUnlisted refRepairFront
	// messageUnauthenticated counts scanned messages whose transform does
	// not authenticate them.
	messageUnauthenticated refRepairFront
	// unlisted is the number of scanned blocks stored without a child list.
	unlisted uint64
	// short counts the scanned blocks whose list lacks a known block, and
	// shortRefs those hits, by type and the hash's field.
	short     refRepairFront
	shortRefs map[string]refRepairFront
	// knownByList counts hashes outside the list that the Volume knows
	// only from their own stored child lists.
	knownByList refRepairFront
	// knownByPeer counts hashes outside the list that only the peer knows.
	knownByPeer refRepairFront
	// unseen counts the scanned blocks whose stored list references a child
	// their bytes do not, and unseenRefs those children.
	unseen     refRepairFront
	unseenRefs uint64
	// strangers counts hashes outside the scanned block's list that neither
	// the Volume nor the peer knows.
	strangers uint64
	// strangerRefs counts those hashes by why the walk had no type and
	// the hash's field, with the first block that holds one.
	strangerRefs map[string]refRepairFront
	// failed holds the scanned refs the scan could not verify, with why.
	failed map[string]string
	// front counts the front's entries by what a walk whose scan verified
	// would do with each, as scanFate describes.
	front map[string]uint64
	// weak holds each known block that bytes name outside a list, by ref,
	// with where the first such hash sits, and weakRefs those blocks in the
	// order found.
	weak     map[string]string
	weakRefs []*block.BlockRef
	// loose counts the weak blocks no named root reaches, by where the hash
	// sits, once a run has walked the roots.
	loose map[string]refRepairFront
}

// add counts ref, keeping the first as the sample.
func (f *refRepairFront) add(ref *block.BlockRef) {
	if f.count == 0 {
		f.sample = ref
	}
	f.count++
}

// scanUntyped makes the walk scan, as refwalk.Walker.ScanUntyped describes.
func (r *refRepair) scanUntyped() {
	r.walker.ScanUntyped()
	r.res.scan = refRepairScan{
		on:           true,
		missed:       make(map[string]refRepairFront),
		extraStored:  make(map[string]refRepairFront),
		blocks:       make(map[string]uint64),
		shortRefs:    make(map[string]refRepairFront),
		strangerRefs: make(map[string]refRepairFront),
		failed:       make(map[string]string),
		front:        make(map[string]uint64),
		weak:         make(map[string]string),
		loose:        make(map[string]refRepairFront),
	}
}

// noteWeak records a known block that bytes at where name outside a list.
func (s *refRepairScan) noteWeak(ref *block.BlockRef, where string) {
	key := ref.MarshalString()
	if _, ok := s.weak[key]; ok {
		return
	}
	s.weak[key] = where
	s.weakRefs = append(s.weakRefs, ref)
}

// measureWeak counts the weak blocks no named root reaches through the stored
// lists: the blocks a free would delete while bytes still reference them.
func (s *refRepairScan) measureWeak(reach *block_gc.RootReach) {
	for _, ref := range s.weakRefs {
		// Count a block no root reaches under the field that references the
		// block.
		if reach.Root(ref) != nil {
			continue
		}
		where := s.weak[ref.MarshalString()]
		at := s.loose[where]
		at.add(ref)
		s.loose[where] = at
	}
}

// scanFate describes what a walk whose scan verified would do with a front
// entry: verify it from its type or from its scan, leave it for the cause
// that kept it from reading, or never reach it. root is the first named root
// whose stored lists reach the entry, when roots says the run looked.
func (r *refRepair) scanFate(ref *block.BlockRef, root *block_gc.FrontRoot, roots bool) string {
	// Prefer a read that worked to the cause of one that did not.
	key := ref.MarshalString()
	typed, scanned := r.walker.Reached(ref)
	why, unread := r.unread[key]
	failed, bad := r.res.scan.failed[key]
	switch {
	case typed && !unread:
		return "read by type"
	case scanned && !bad:
		return "read by scanning"
	case unread && bad:
		return "left, " + why + "; scanned and " + failed
	case unread:
		return "left, " + why
	case bad:
		return "left, scanned and " + failed
	case !roots:
		return "not reached"
	case root == nil:
		return "not reached, and no named root reaches it"
	default:
		return "not reached, under root " + root.BucketID + " " + root.Name
	}
}

// refRepairScanShapes bounds the outlines a scan report keeps.
const refRepairScanShapes = 6

// visitTyped counts how the scan of a block decoded by type compares with
// the refs its type declares.
func (r *refRepair) visitTyped(ctx context.Context, rd refwalk.Visit) error {
	// Count the block and the typed refs the scan missed.
	s := &r.res.scan
	if !s.on {
		return nil
	}
	s.typed++
	s.typedRefs += uint64(len(rd.Refs))
	if len(rd.ScanMissed) != 0 {
		missed := s.missed[nodeType(rd.Node)]
		missed.add(rd.Node.Ref)
		s.missed[nodeType(rd.Node)] = missed
		s.missedRefs += uint64(len(rd.ScanMissed))
		if len(s.shapes) < refRepairScanShapes {
			lost := rd.ScanMissed[0].GetHash()
			s.shapes = append(s.shapes, nodeType(rd.Node)+", lacking a type "+strconv.Itoa(int(lost.GetHashType()))+" hash of "+
				strconv.Itoa(len(lost.GetHash()))+" bytes among "+strconv.Itoa(len(rd.Refs))+" refs:"+rd.ScanShape)
		}
	}

	// Count the hashes outside the typed refs, and those referencing a block.
	for _, hit := range rd.ScanExtra {
		s.extra++
		stored, err := r.vol.GetBlockExists(ctx, hit.Ref)
		if err != nil {
			return err
		}
		if stored {
			key := nodeType(rd.Node) + " field " + hit.Path
			at := s.extraStored[key]
			at.add(rd.Node.Ref)
			s.extraStored[key] = at
			s.noteWeak(hit.Ref, key)
		}
	}
	return nil
}

// scanBlockSource is the first source that knows a scanned hash.
type scanBlockSource uint8

const (
	// scanBlockUnknown means neither store knows the hash.
	scanBlockUnknown scanBlockSource = iota
	// scanBlockBytes means the raw Volume store has the block's bytes.
	scanBlockBytes
	// scanBlockList means the Volume has the child list but no bytes.
	scanBlockList
	// scanBlockPeer means only the peer has the block.
	scanBlockPeer
)

// scanKnowsBlock reports which source knows a hash. Ask for stored bytes because
// the Volume's ordinary existence check hides parked blocks that still have them.
func (r *refRepair) scanKnowsBlock(ctx context.Context, ref *block.BlockRef) (scanBlockSource, error) {
	// Look for bytes even when the Volume has parked the block.
	found, err := r.vol.HasBlockBytes(ctx, ref)
	if err != nil {
		return scanBlockUnknown, err
	}
	if found {
		return scanBlockBytes, nil
	}

	// A recorded child list keeps a block known after its bytes leave.
	_, has, err := r.vol.GetChildren(ctx, ref)
	if err != nil {
		return scanBlockUnknown, err
	}
	if has {
		return scanBlockList, nil
	}

	// The peer may hold a child the Volume has never seen.
	if r.peer == nil {
		return scanBlockUnknown, nil
	}
	found, err = r.peer.GetBlockExists(ctx, ref)
	if err != nil || !found {
		return scanBlockUnknown, err
	}
	return scanBlockPeer, nil
}

// visitScanned compares the refs an untyped block's bytes hold with its stored list.
//
// The verified list includes every child on the stored list, so the scan never
// removes a hold. Add a hash when the Volume has its bytes or its child list,
// or the peer has its bytes. Fill unknown blocks before running verification,
// since they gain no hold from this list.
// Verify opaque bytes only under an authenticating transform with an empty
// stored list. Together these indicate a childless block such as a raw blob
// chunk. A list that names children shows the scan cannot read the content.
// Without --scan-verify, the walk writes nothing for a scanned block.
func (r *refRepair) visitScanned(ctx context.Context, rd refwalk.Visit) error {
	// Count a ref no read resolves, and a block its holder's transform does
	// not decode.
	s := &r.res.scan
	ref := rd.Node.Ref
	if !rd.Found {
		s.absent++
		if err := r.listAbsent(ref, rd.Node.Untyped); err != nil {
			return err
		}
		s.failed[ref.MarshalString()] = frontAbsent
		return nil
	}
	if rd.DecodeErr != nil {
		s.undecodable.add(ref)
		s.failed[ref.MarshalString()] = "undecodable by its holder's transform"
		return nil
	}
	s.blocks[rd.Node.Untyped]++

	// Read and index the stored list, which already holds each child once.
	list, has, err := r.vol.GetChildren(ctx, ref)
	if err != nil {
		return err
	}
	if !has {
		s.unlisted++
	}
	listed := make(map[string]struct{}, len(list))
	for _, child := range list {
		listed[child.MarshalString()] = struct{}{}
	}

	// Count suspicious reads and record the same front cause in both modes.
	auth, ok := rd.Node.Xfrm.(interface{ Authenticates() bool })
	authenticated := ok && auth.Authenticates()
	var cause string
	switch {
	case rd.Opaque:
		s.opaque++
		if len(list) != 0 {
			s.opaqueListed.add(ref)
			cause = frontOpaqueListed
		}
		if !authenticated {
			s.opaqueUnauthenticated.add(ref)
			cause = frontOpaqueUnauthenticated
		}
		if !has {
			s.opaqueUnlisted.add(ref)
			cause = frontOpaqueUnlisted
		}
	case !authenticated:
		s.messageUnauthenticated.add(ref)
	}
	if cause != "" {
		s.failed[ref.MarshalString()] = cause
	}

	// Count the hashes outside the list and add the known ones to the
	// verified refs. ScanHits gives each hash once.
	scanned := make(map[string]struct{}, len(rd.Refs))
	refs := slices.Clip(list)
	var short uint64
	for _, hit := range rd.ScanHits {
		// Find out which source knows the hash outside the list.
		child := hit.Ref
		key := child.MarshalString()
		scanned[key] = struct{}{}
		if _, ok := listed[key]; ok {
			continue
		}
		source, err := r.scanKnowsBlock(ctx, child)
		if err != nil {
			return err
		}

		// Count unknown hashes by the field where they appear.
		where := rd.Node.Untyped + ", field " + hit.Path
		if source == scanBlockUnknown {
			s.strangers++
			at := s.strangerRefs[where]
			at.add(ref)
			s.strangerRefs[where] = at
			continue
		}

		// Count and keep the known children the list lacks.
		short++
		at := s.shortRefs[where]
		at.add(ref)
		s.shortRefs[where] = at
		s.noteWeak(child, where)
		switch source {
		case scanBlockList:
			s.knownByList.add(child)
		case scanBlockPeer:
			s.knownByPeer.add(child)
		}
		if r.scanVerify {
			refs = append(refs, child)
		}
	}
	if short != 0 {
		s.short.add(ref)
	}

	// Count the children the list references and the bytes do not.
	var unseen uint64
	for key := range listed {
		if _, ok := scanned[key]; !ok {
			unseen++
		}
	}
	if unseen != 0 {
		s.unseen.add(ref)
		s.unseenRefs += unseen
	}

	// Verify the list once the scan has enough evidence.
	if r.scanVerify && cause == "" {
		return r.verify(ctx, ref, refs, rd.Node.Untyped)
	}
	return nil
}

// looseFields returns the report rows of the weak blocks no named root
// reaches, in all and by the field that references them.
func (s *refRepairScan) looseFields() [][2]string {
	// Total the unreachable weak blocks, then list each field with its first
	// block.
	var loose uint64
	for _, at := range s.loose {
		loose += at.count
	}
	fields := [][2]string{{"Scan: of them no named root reaches", strconv.FormatUint(loose, 10)}}
	for _, where := range slices.Sorted(maps.Keys(s.loose)) {
		at := s.loose[where]
		fields = append(fields, [2]string{
			"Scan: of them no named root reaches, named in " + where,
			strconv.FormatUint(at.count, 10) + " (" + at.sample.MarshalString() + ")",
		})
	}
	return fields
}

// scanFields returns the report rows of a walk that scanned. rooted says the
// run also walked from the named roots. That walk counts the weak blocks
// no root reaches.
func scanFields(s refRepairScan, rooted bool) [][2]string {
	// Measure the scan against the types.
	num := func(n uint64) string { return strconv.FormatUint(n, 10) }
	sampled := func(f refRepairFront) string {
		if f.count == 0 {
			return "0"
		}
		return num(f.count) + " (" + f.sample.MarshalString() + ")"
	}
	fields := [][2]string{
		{"Scan: typed blocks scanned", num(s.typed)},
		{"Scan: refs their types name", num(s.typedRefs)},
		{"Scan: typed refs the scan missed", num(s.missedRefs)},
		{"Scan: hashes in typed blocks outside their typed refs", num(s.extra)},
	}
	for _, at := range slices.Sorted(maps.Keys(s.extraStored)) {
		fields = append(fields, [2]string{"Scan: of them naming a stored block, in " + at, sampled(s.extraStored[at])})
	}
	for _, typ := range slices.Sorted(maps.Keys(s.missed)) {
		fields = append(fields, [2]string{"Scan: blocks with a missed ref, " + typ, sampled(s.missed[typ])})
	}
	for _, shape := range s.shapes {
		fields = append(fields, [2]string{"Scan: outline of a block with a missed ref", shape})
	}

	// Compare the scan with the stored lists below the untyped refs.
	for _, reason := range slices.Sorted(maps.Keys(s.blocks)) {
		fields = append(fields, [2]string{"Scan: untyped blocks read below " + reason, num(s.blocks[reason])})
	}
	for _, at := range slices.Sorted(maps.Keys(s.shortRefs)) {
		fields = append(fields, [2]string{"Scan: known blocks a list lacks, below " + at, sampled(s.shortRefs[at])})
	}
	for _, at := range slices.Sorted(maps.Keys(s.strangerRefs)) {
		fields = append(fields, [2]string{"Scan: hashes naming no known block, outside the list, below " + at, sampled(s.strangerRefs[at])})
	}
	for _, fate := range slices.Sorted(maps.Keys(s.front)) {
		fields = append(fields, [2]string{"Scan: front entries " + fate, num(s.front[fate])})
	}
	fields = append(fields, [2]string{"Scan: known blocks that bytes name outside a list", num(uint64(len(s.weakRefs)))})
	if rooted {
		fields = append(fields, s.looseFields()...)
	}
	return append(fields,
		[2]string{"Scan: blocks read by scanning alone", num(s.only)},
		[2]string{"Scan: scanned refs no read resolves", num(s.absent)},
		[2]string{"Scan: blocks the holder's transform does not decode", sampled(s.undecodable)},
		[2]string{"Scan: scanned blocks that are not messages", num(s.opaque)},
		[2]string{"Scan: scanned blocks that are not messages, with a non-empty list", sampled(s.opaqueListed)},
		[2]string{"Scan: scanned blocks that are not messages, under a transform that does not authenticate", sampled(s.opaqueUnauthenticated)},
		[2]string{"Scan: scanned blocks that are not messages, without a stored list", sampled(s.opaqueUnlisted)},
		[2]string{"Scan: scanned messages under a transform that does not authenticate", sampled(s.messageUnauthenticated)},
		[2]string{"Scan: scanned blocks without a stored list", num(s.unlisted)},
		[2]string{"Scan: scanned blocks whose list lacks a known block their bytes name", sampled(s.short)},
		[2]string{"Scan: scanned blocks whose list names a child their bytes do not", sampled(s.unseen)},
		[2]string{"Scan: the children those bytes do not name", num(s.unseenRefs)},
		[2]string{"Scan: hashes naming no known block, outside the list", num(s.strangers)},
		[2]string{"Scan: hashes naming blocks with a stored list and no bytes, outside the list", sampled(s.knownByList)},
		[2]string{"Scan: hashes naming blocks known only by the peer, outside the list", sampled(s.knownByPeer)},
	)
}
