package world_block

import (
	"bytes"
	"context"
	"encoding/binary"
	"math/big"
	"sort"
	"strconv"
	"strings"

	cayley_kv "github.com/aperturerobotics/cayley/graph/kv"
	cayley_proto "github.com/aperturerobotics/cayley/graph/proto"
	"github.com/aperturerobotics/cayley/kv/flat"
	"github.com/aperturerobotics/cayley/quad"
	"github.com/aperturerobotics/cayley/quad/pquads"
	b58 "github.com/mr-tron/base58/base58"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	kvtx_block_okra "github.com/s4wave/spacewave/db/kvtx/block/okra"
)

// GraphLogAudit reports what the graph tree's primitive log still holds and
// what a rebuild from it would lose. The log keeps one record per live node
// and quad; every other bucket derives from it. The posting indexes list only
// live quads, so a quad they list without a log record is a live quad whose
// record is missing, and its node IDs are in the posting keys.
type GraphLogAudit struct {
	// Keys is the number of readable keys.
	Keys int
	// Holes are the missing pages.
	Holes []kvtx_block_okra.SalvageHole
	// HoleBuckets counts the missing pages and their keys by the buckets
	// their range starts and ends in, as "low..high".
	HoleBuckets map[string][2]uint64
	// LogHoleKeys is the number of keys the missing pages that overlap the
	// log held.
	LogHoleKeys uint64

	// Size and Horizon are the stored live quad count and highest ID, -1 when
	// unreadable.
	Size, Horizon int64

	// Nodes and Quads are the readable live log records.
	Nodes, Quads int
	// LabeledQuads is the number of readable quads with a label, which no
	// posting index holds.
	LabeledQuads int
	// DeletedRecords is the number of readable records marked deleted.
	DeletedRecords int
	// MaxID is the highest readable or referenced ID.
	MaxID uint64
	// InlineValues and FetchedValues count the log values read from the page
	// and from their own block.
	InlineValues, FetchedValues int
	// ValueHoles are the keys whose value lives in its own block and that
	// block reads as not found or unavailable, by bucket. The key is
	// readable, its value is lost.
	ValueHoles map[string][]string

	// MissingQuads are the live quads the postings list without a log record.
	MissingQuads []*GraphMissingQuad
	// MissingNodes are the nodes live quads or the value index name without
	// a log record.
	MissingNodes []*GraphMissingNode
	// PostingNodes counts posting entries that name a node record as a quad.
	PostingNodes int
	// NodeValues are the values of the readable nodes the missing quads
	// name.
	NodeValues map[uint64]quad.Value

	// ObjectKeys is the number of World object keys hashed as candidates
	// for missing node values, and ObjectHoles the object tree's missing
	// pages.
	ObjectKeys  int
	ObjectHoles int
}

// GraphMissingQuad is a live quad with no log record.
type GraphMissingQuad struct {
	// ID is the quad ID.
	ID uint64
	// Dirs are the subject, predicate, object and label node IDs, by
	// quad.Direction minus one.
	Dirs [4]uint64
	// Known marks the directions a posting key gave.
	Known [4]bool
	// InLogHole is set when the ID's log key lies in a missing page.
	InLogHole bool
}

// GraphMissingNode is a node with no log record.
type GraphMissingNode struct {
	// ID is the node ID.
	ID uint64
	// Hash is the value hash from the value index, nil when that is missing.
	Hash []byte
	// Refs is the stored reference count, -1 when unread.
	Refs int64
	// KnownRefs is the number of quad directions known to name the node.
	KnownRefs int
	// Value is the recovered value, nil when no candidate matched.
	Value quad.Value
	// InLogHole is set when the ID's log key lies in a missing page.
	InLogHole bool
}

// Complete reports whether every direction but the label is known.
func (q *GraphMissingQuad) Complete() bool {
	return q.Known[0] && q.Known[1] && q.Known[2]
}

// AuditGraphLog walks the current graph tree and reports what its primitive
// log holds, which live quads and nodes have no log record, and which missing
// node values the World's object keys recover. It writes nothing.
func (t *WorldState) AuditGraphLog(ctx context.Context) (*GraphLogAudit, error) {
	// Apply held graph writes so the walk sees the current tree.
	if err := t.flushGraphWrites(ctx); err != nil {
		return nil, errors.Wrap(err, "graph write flush")
	}
	tree, ok := t.graphTree.(*kvtx_block_okra.Tx)
	if !ok {
		return nil, errors.Errorf("graph tree is %T, not an okra tree", t.graphTree)
	}
	qs, ok := t.graphHd.QuadStore.(*cayley_kv.QuadStore)
	if !ok {
		return nil, errors.Errorf("graph store is %T, not a kv quad store", t.graphHd.QuadStore)
	}
	postings := make(map[string][]quad.Direction)
	for _, index := range qs.Layout() {
		postings[postingBucketName(index)] = index.Dirs
	}

	a := &graphAudit{
		tree:     tree,
		postings: postings,
		report:   &GraphLogAudit{HoleBuckets: make(map[string][2]uint64), ValueHoles: make(map[string][]string), Size: -1, Horizon: -1},
		missingQ: make(map[uint64]*GraphMissingQuad),
	}
	if err := a.walkGraph(ctx); err != nil {
		return a.report, err
	}
	if err := a.resolveNodes(ctx); err != nil {
		return a.report, err
	}
	if err := a.matchObjectKeys(ctx, t); err != nil {
		return a.report, err
	}
	a.finish()
	if err := a.readNodeValues(ctx); err != nil {
		return a.report, err
	}
	return a.report, nil
}

// graphAudit is one AuditGraphLog walk.
type graphAudit struct {
	tree     *kvtx_block_okra.Tx
	postings map[string][]quad.Direction
	report   *GraphLogAudit

	// have marks the IDs with a readable log record, nodes those that are
	// nodes, and refs counts the quad directions naming each ID.
	have, nodes idBitset
	refs        []uint32
	// missingQ are the live quads without a log record.
	missingQ map[uint64]*GraphMissingQuad
	// missingN are the missing nodes, fixed when the value index is reached.
	missingN map[uint64]*GraphMissingNode
	// logHoles are the key ranges of missing pages overlapping the log.
	logHoles []kvtx_block_okra.SalvageHole
}

// walkGraph streams the graph tree once. The buckets sort as log, meta, node
// counts, postings, values, so the postings are read after every log record
// and the value index after every posting.
func (a *graphAudit) walkGraph(ctx context.Context) error {
	var walkErr error
	err := a.tree.Salvage(ctx, nil, nil, func(entry kvtx_block_okra.BuildEntry) bool {
		a.report.Keys++
		walkErr = a.entry(ctx, entry)
		if errors.Is(walkErr, errValueHole) {
			walkErr = nil
		}
		return walkErr == nil
	}, func(hole kvtx_block_okra.SalvageHole) bool {
		a.hole(hole)
		return true
	})
	if err == nil {
		err = walkErr
	}
	return errors.Wrap(err, "walk graph tree")
}

// hole records a missing page by the buckets its range spans.
func (a *graphAudit) hole(hole kvtx_block_okra.SalvageHole) {
	a.report.Holes = append(a.report.Holes, hole)
	name := flatBucket(hole.Low, "start") + ".." + flatBucket(hole.High, "end")
	got := a.report.HoleBuckets[name]
	a.report.HoleBuckets[name] = [2]uint64{got[0] + 1, got[1] + hole.Keys}
	if rangeOverlapsBucket(hole.Low, hole.High, "l") {
		a.logHoles = append(a.logHoles, hole)
		a.report.LogHoleKeys += hole.Keys
	}
}

// entry reads one graph key.
func (a *graphAudit) entry(ctx context.Context, entry kvtx_block_okra.BuildEntry) error {
	parts := flat.KeyUnescape(flat.Key(entry.Key))
	if len(parts) < 2 {
		return nil
	}
	bucket := string(parts[0])
	switch {
	case bucket == "l":
		return a.logRecord(ctx, entry)
	case bucket == "m":
		return a.meta(ctx, entry, string(parts[1]))
	case bucket == "v":
		return a.valueIndex(ctx, entry, parts[1])
	case a.postings[bucket] != nil:
		return a.posting(ctx, entry, a.postings[bucket], parts[1])
	}
	return nil
}

// value returns an entry's value, from the page when it is inline.
func (a *graphAudit) value(ctx context.Context, entry kvtx_block_okra.BuildEntry) ([]byte, error) {
	if v := entry.ValueBlob; v != nil && v.GetBlobType() == blob.BlobType_BlobType_RAW {
		a.report.InlineValues++
		return v.GetRawData(), nil
	}
	a.report.FetchedValues++
	value, found, err := a.tree.Get(ctx, entry.Key)
	if err != nil && !errors.Is(err, block.ErrNotFound) && !errors.Is(err, block.ErrUnavailable) {
		return nil, errors.Wrapf(err, "read value of %q", entry.Key)
	}
	if err != nil || !found {
		bucket := flatBucket(entry.Key, "")
		a.report.ValueHoles[bucket] = append(a.report.ValueHoles[bucket], string(entry.Key))
		return nil, errValueHole
	}
	return value, nil
}

// errValueHole skips a key whose value block is missing.
var errValueHole = errors.New("value block missing")

// logRecord reads one live primitive.
func (a *graphAudit) logRecord(ctx context.Context, entry kvtx_block_okra.BuildEntry) error {
	data, err := a.value(ctx, entry)
	if err != nil {
		return err
	}
	var p cayley_proto.Primitive
	if err := p.UnmarshalVT(data); err != nil {
		return errors.Wrapf(err, "decode log record %q", entry.Key)
	}
	a.have.set(p.ID)
	a.seeID(p.ID)
	if p.Deleted {
		a.report.DeletedRecords++
	}
	if p.IsNode() {
		a.nodes.set(p.ID)
		a.report.Nodes++
		return nil
	}
	a.report.Quads++
	if p.Label != 0 {
		a.report.LabeledQuads++
	}
	for _, d := range quad.Directions {
		a.ref(p.GetDirection(d))
	}
	return nil
}

// meta reads the stored quad count and ID horizon.
func (a *graphAudit) meta(ctx context.Context, entry kvtx_block_okra.BuildEntry, name string) error {
	if name != "size" && name != "horizon" {
		return nil
	}
	data, err := a.value(ctx, entry)
	if err != nil || len(data) != 8 {
		return err
	}
	v := int64(binary.LittleEndian.Uint64(data))
	if name == "size" {
		a.report.Size = v
	} else {
		a.report.Horizon = v
	}
	return nil
}

// posting reads one posting list, recording each listed quad that has no log
// record with the node IDs the posting key gives.
func (a *graphAudit) posting(ctx context.Context, entry kvtx_block_okra.BuildEntry, dirs []quad.Direction, key []byte) error {
	ids, err := decodePostingKey(key, len(dirs))
	if err != nil {
		return errors.Wrapf(err, "decode posting key %q", entry.Key)
	}
	data, err := a.value(ctx, entry)
	if err != nil {
		return err
	}
	quads, err := decodeUvarints(data)
	if err != nil {
		return errors.Wrapf(err, "decode posting %q", entry.Key)
	}
	for _, id := range quads {
		if a.have.has(id) {
			if a.nodes.has(id) {
				a.report.PostingNodes++
			}
			continue
		}
		mq := a.missingQ[id]
		if mq == nil {
			mq = &GraphMissingQuad{ID: id}
			a.missingQ[id] = mq
			a.seeID(id)
		}
		for i, d := range dirs {
			slot := int(d) - 1
			if mq.Known[slot] {
				continue
			}
			mq.Dirs[slot], mq.Known[slot] = ids[i], true
			a.ref(ids[i])
		}
	}
	return nil
}

// valueIndex reads one value index entry, recording the hash of each node
// with no log record. The first one fixes the missing nodes: every live quad
// the walk can know of has been read by then.
func (a *graphAudit) valueIndex(ctx context.Context, entry kvtx_block_okra.BuildEntry, key []byte) error {
	if a.missingN == nil {
		a.collectMissingNodes()
	}
	data, err := a.value(ctx, entry)
	if err != nil {
		return err
	}
	id, n := binary.Uvarint(data)
	if n <= 0 {
		return errors.Errorf("decode value index %q", entry.Key)
	}
	if a.have.has(id) {
		return nil
	}
	hash, err := b58.Decode(string(key))
	if err != nil {
		return errors.Wrapf(err, "decode value hash %q", entry.Key)
	}
	mn := a.missingN[id]
	if mn == nil {
		mn = &GraphMissingNode{ID: id, Refs: -1}
		a.missingN[id] = mn
		a.seeID(id)
	}
	mn.Hash = hash
	return nil
}

// collectMissingNodes records each ID a live quad names that has no log
// record.
func (a *graphAudit) collectMissingNodes() {
	a.missingN = make(map[uint64]*GraphMissingNode)
	for id, n := range a.refs {
		if n != 0 && !a.have.has(uint64(id)) {
			a.missingN[uint64(id)] = &GraphMissingNode{ID: uint64(id), Refs: -1, KnownRefs: int(n)}
		}
	}
}

// resolveNodes reads the stored reference count of each missing node whose
// hash is known.
func (a *graphAudit) resolveNodes(ctx context.Context) error {
	if a.missingN == nil {
		a.collectMissingNodes()
	}
	for _, mn := range a.missingN {
		if mn.Hash == nil {
			continue
		}
		data, found, err := a.tree.Get(ctx, flat.KeyEscape([][]byte{[]byte("n"), []byte(b58.Encode(mn.Hash))}))
		if err != nil {
			continue
		}
		if found {
			if refs, n := binary.Uvarint(data); n > 0 {
				mn.Refs = int64(refs)
			}
		}
	}
	return nil
}

// matchObjectKeys hashes every World object key as an IRI and a string and
// recovers each missing node value that matches.
func (a *graphAudit) matchObjectKeys(ctx context.Context, t *WorldState) error {
	want := make(map[string]*GraphMissingNode)
	for _, mn := range a.missingN {
		if mn.Hash != nil {
			want[string(mn.Hash)] = mn
		}
	}
	if len(want) == 0 {
		return nil
	}
	prefix := []byte(objectKeyPrefix)
	hash := make([]byte, quad.HashSize)
	match := func(key []byte) {
		a.report.ObjectKeys++
		k := string(key[len(prefix):])
		for _, v := range []quad.Value{quad.IRI(k), quad.String(k)} {
			quad.HashTo(v, hash)
			if mn := want[string(hash)]; mn != nil && mn.Value == nil {
				mn.Value = v
			}
		}
	}

	// Walk an okra object tree past its missing pages.
	if objTree, ok := t.objTree.(*kvtx_block_okra.Tx); ok {
		past := append(bytes.Clone(prefix[:len(prefix)-1]), prefix[len(prefix)-1]+1)
		err := objTree.Salvage(ctx, prefix, past, func(entry kvtx_block_okra.BuildEntry) bool {
			match(entry.Key)
			return true
		}, func(kvtx_block_okra.SalvageHole) bool {
			a.report.ObjectHoles++
			return true
		})
		return errors.Wrap(err, "walk object tree")
	}

	// Iterate any other object tree.
	it := t.objTree.Iterate(ctx, prefix, true, false)
	defer it.Close()
	for it.Next() {
		match(it.Key())
	}
	return errors.Wrap(it.Err(), "iterate object tree")
}

// finish places each missing ID against the log's missing pages and sorts
// the results by ID.
func (a *graphAudit) finish() {
	for _, mq := range a.missingQ {
		mq.InLogHole = a.inLogHole(mq.ID)
		a.report.MissingQuads = append(a.report.MissingQuads, mq)
	}
	for _, mn := range a.missingN {
		mn.InLogHole = a.inLogHole(mn.ID)
		if int(mn.ID) < len(a.refs) {
			mn.KnownRefs = int(a.refs[mn.ID])
		}
		a.report.MissingNodes = append(a.report.MissingNodes, mn)
	}
	sort.Slice(a.report.MissingQuads, func(i, j int) bool { return a.report.MissingQuads[i].ID < a.report.MissingQuads[j].ID })
	sort.Slice(a.report.MissingNodes, func(i, j int) bool { return a.report.MissingNodes[i].ID < a.report.MissingNodes[j].ID })
}

// readNodeValues reads the value of each readable node a missing quad names.
func (a *graphAudit) readNodeValues(ctx context.Context) error {
	a.report.NodeValues = make(map[uint64]quad.Value)
	for _, mq := range a.report.MissingQuads {
		for i, id := range mq.Dirs {
			if !mq.Known[i] || !a.nodes.has(id) {
				continue
			}
			if _, ok := a.report.NodeValues[id]; ok {
				continue
			}
			data, found, err := a.tree.Get(ctx, graphLogKey(id))
			if err != nil || !found {
				continue
			}
			var p cayley_proto.Primitive
			if err := p.UnmarshalVT(data); err != nil {
				continue
			}
			v, err := pquads.UnmarshalValue(ctx, p.Value)
			if err != nil {
				continue
			}
			a.report.NodeValues[id] = v
		}
	}
	return ctx.Err()
}

// inLogHole reports whether the log key of id lies in a missing page.
func (a *graphAudit) inLogHole(id uint64) bool {
	key := graphLogKey(id)
	for _, hole := range a.logHoles {
		if (len(hole.Low) == 0 || bytes.Compare(key, hole.Low) >= 0) && (len(hole.High) == 0 || bytes.Compare(key, hole.High) < 0) {
			return true
		}
	}
	return false
}

// ref counts one quad direction naming id.
func (a *graphAudit) ref(id uint64) {
	if id == 0 {
		return
	}
	a.seeID(id)
	for uint64(len(a.refs)) <= id {
		a.refs = append(a.refs, make([]uint32, max(len(a.refs), 1<<16))...)
	}
	a.refs[id]++
}

// seeID raises the highest ID seen.
func (a *graphAudit) seeID(id uint64) {
	a.report.MaxID = max(a.report.MaxID, id)
}

// graphLogKey returns the log key of id.
func graphLogKey(id uint64) []byte {
	return flat.KeyEscape([][]byte{[]byte("l"), []byte(strconv.FormatUint(id, 10))})
}

// idBitset is a growable set of IDs.
type idBitset []uint64

func (b *idBitset) set(id uint64) {
	w := id / 64
	for uint64(len(*b)) <= w {
		*b = append(*b, make([]uint64, max(len(*b), 1024))...)
	}
	(*b)[w] |= 1 << (id % 64)
}

func (b idBitset) has(id uint64) bool {
	w := id / 64
	return w < uint64(len(b)) && b[w]&(1<<(id%64)) != 0
}

// decodePostingKey returns the IDs of a posting key: n base62 IDs joined by
// ':'.
func decodePostingKey(key []byte, n int) ([]uint64, error) {
	parts := strings.Split(string(key), ":")
	if len(parts) != n {
		return nil, errors.Errorf("want %d IDs, got %d", n, len(parts))
	}
	ids := make([]uint64, n)
	for i, part := range parts {
		var v big.Int
		if _, ok := v.SetString(part, 62); !ok || !v.IsUint64() {
			return nil, errors.Errorf("bad ID %q", part)
		}
		ids[i] = v.Uint64()
	}
	return ids, nil
}

// decodeUvarints decodes a list of uvarints.
func decodeUvarints(b []byte) ([]uint64, error) {
	var out []uint64
	for len(b) > 0 {
		v, n := binary.Uvarint(b)
		if n <= 0 {
			return out, errors.New("bad uvarint")
		}
		out = append(out, v)
		b = b[n:]
	}
	return out, nil
}

// flatBucket returns the bucket a flat key starts in, or def for an empty key.
func flatBucket(key []byte, def string) string {
	if len(key) == 0 {
		return def
	}
	parts := flat.KeyUnescape(flat.Key(key))
	return string(parts[0])
}

// rangeOverlapsBucket reports whether [low, high) meets the keys of bucket.
func rangeOverlapsBucket(low, high []byte, bucket string) bool {
	first := []byte(bucket + "/")
	past := []byte(bucket + "0")
	if len(low) != 0 && bytes.Compare(low, past) >= 0 {
		return false
	}
	return len(high) == 0 || bytes.Compare(high, first) > 0
}
