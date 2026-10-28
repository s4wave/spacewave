package world_block

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"iter"
	"maps"
	"slices"
	"sort"
	"strconv"

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

// graphCheckExamples bounds the keys a bucket check keeps of each difference.
const graphCheckExamples = 8

// GraphRebuild reports a rebuild of the graph tree from its primitive log.
// The log is the graph's only primary data: the reference counts, the value
// index, the postings and the quad count all derive from it, so the rebuild
// regenerates them from the log and never copies a stored count.
type GraphRebuild struct {
	// Audit is the log audit the lost records were recovered from.
	Audit *GraphLogAudit
	// Nodes and Quads are the live records the rebuilt log holds.
	Nodes, Quads int
	// Supplied are the IDs of records the caller supplied, such as from a
	// backup, written back to the log.
	Supplied []uint64
	// RecoveredQuads and RecoveredNodes are the IDs of lost log records
	// written back from the postings and the object keys.
	RecoveredQuads, RecoveredNodes []uint64
	// Dropped are the records the rebuilt log cannot hold, with the reason.
	Dropped []GraphDroppedRecord
	// UnidentifiedLogKeys is the number of keys under the log's missing pages
	// that no posting or value index names, so nothing identifies them.
	UnidentifiedLogKeys int64
	// LabeledPredicates holds, for each predicate of a recovered quad, how
	// many readable quads with that predicate carry a label and how many
	// there are. No posting holds the label, so a recovered quad gets none.
	LabeledPredicates map[uint64][2]int
	// Checks compares each rebuilt bucket with the readable current one.
	Checks []*GraphBucketCheck
	// Applied is set when the rebuilt tree replaced the current one.
	Applied bool
}

// GraphDroppedRecord is a log record a rebuild drops.
type GraphDroppedRecord struct {
	// ID is the primitive ID.
	ID uint64
	// Node is set for a node, clear for a quad.
	Node bool
	// Reason is why the record is dropped.
	Reason string
}

// GraphBucketCheck compares the rebuilt entries of one bucket with the
// readable entries of the current tree.
type GraphBucketCheck struct {
	// Bucket is the bucket name.
	Bucket string
	// Same and Changed count keys in both, with equal and different values.
	Same, Changed int
	// Added counts rebuilt keys the current tree cannot read, Removed
	// readable keys the rebuild drops.
	Added, Removed int
	// Examples are some changed, added and removed keys.
	Examples []string
}

// RebuildGraphTree rebuilds the graph tree from its primitive log. It audits
// the log, writes back the lost records the audit recovers and those in
// supplied, drops each quad naming a node with no record and each node no
// quad names, and regenerates the reference counts, value index, postings
// and quad count from the result. Every other key is kept. The report
// compares each regenerated bucket with the readable current one. Without
// apply it writes nothing. With apply it replaces the graph tree; the quad
// store's caches are then stale, so run it on a World nothing else writes,
// and reopen it after.
func (t *WorldState) RebuildGraphTree(ctx context.Context, supplied []*cayley_proto.Primitive, apply bool) (*GraphRebuild, error) {
	// Audit the log for the lost records.
	audit, err := t.AuditGraphLog(ctx)
	r := &GraphRebuild{Audit: audit, LabeledPredicates: make(map[uint64][2]int)}
	if err != nil {
		return r, errors.Wrap(err, "audit graph log")
	}
	tree := t.graphTree.(*kvtx_block_okra.Tx)
	qs := t.graphHd.QuadStore.(*cayley_kv.QuadStore)

	// Collect the records to write back.
	b := &graphRebuilder{tree: tree, layout: qs.Layout(), report: r}
	extra, err := b.recoveredRecords(supplied)
	if err != nil {
		return r, err
	}

	// Read the log and add the recovered records.
	if err := b.readLog(ctx, extra); err != nil {
		return r, err
	}
	for _, id := range slices.Sorted(maps.Keys(extra)) {
		if err := b.add(ctx, extra[id], true); err != nil {
			return r, err
		}
	}

	// Keep the quads whose nodes all have records, then the nodes they name.
	b.resolve()

	// Compare, and replace the tree when applying.
	if !apply {
		err := b.merge(ctx, func(kvtx_block_okra.BuildEntry) bool { return true })
		return r, err
	}
	err = tree.ReplaceAllEntries(ctx, func(yield func(kvtx_block_okra.BuildEntry, error) bool) {
		var stopped bool
		err := b.merge(ctx, func(entry kvtx_block_okra.BuildEntry) bool {
			stopped = !yield(entry, nil)
			return !stopped
		})
		if err != nil && !stopped {
			yield(kvtx_block_okra.BuildEntry{}, err)
		}
	})
	if err != nil {
		return r, errors.Wrap(err, "replace graph tree")
	}
	r.Applied = true
	return r, nil
}

// graphRebuilder is one RebuildGraphTree run.
type graphRebuilder struct {
	tree   *kvtx_block_okra.Tx
	layout []cayley_kv.QuadIndex
	report *GraphRebuild

	// nodes are the node records, hashed; quads the quad records.
	nodes []rebuildNode
	quads []rebuildQuad
	// isNode marks the node IDs; dropped the IDs the rebuilt log drops.
	isNode, dropped idBitset
	// refs counts the kept quad directions naming each node.
	refs []uint32
	// written are the recovered records to write, by log key.
	written []graphKV
	// size is the number of kept quads.
	size int64
}

// rebuildNode is a node record.
type rebuildNode struct {
	id   uint64
	hash []byte
}

// rebuildQuad is a quad record: subject, predicate, object and label.
type rebuildQuad struct {
	id   uint64
	dirs [4]uint64
	// recovered is set for a quad rebuilt from the audit or supplied, clear
	// for one read from the log.
	recovered bool
}

// graphKV is a rebuilt entry.
type graphKV struct {
	key, value []byte
}

// recoveredRecords returns the records to write back by ID: the supplied
// ones first, then the nodes and whole quads the audit recovered. It drops
// each lost node with no value and each lost quad with no object.
func (b *graphRebuilder) recoveredRecords(supplied []*cayley_proto.Primitive) (map[uint64]*cayley_proto.Primitive, error) {
	r, audit := b.report, b.report.Audit
	extra := make(map[uint64]*cayley_proto.Primitive)
	for _, p := range supplied {
		extra[p.ID] = p.CloneVT()
		r.Supplied = append(r.Supplied, p.ID)
	}
	var identified int64
	for _, mn := range audit.MissingNodes {
		if mn.InLogHole {
			identified++
		}
		if extra[mn.ID] != nil {
			continue
		}
		if mn.Value == nil {
			b.drop(mn.ID, true, "lost node whose value no object key matches")
			continue
		}
		data, err := pquads.MarshalValue(mn.Value)
		if err != nil {
			return nil, errors.Wrapf(err, "encode value of node %d", mn.ID)
		}
		extra[mn.ID] = &cayley_proto.Primitive{ID: mn.ID, Value: data}
		r.RecoveredNodes = append(r.RecoveredNodes, mn.ID)
	}
	for _, mq := range audit.MissingQuads {
		if mq.InLogHole {
			identified++
		}
		if extra[mq.ID] != nil {
			continue
		}
		if !mq.Complete() {
			b.drop(mq.ID, false, "lost quad whose "+missingDirs(mq)+" no posting holds")
			continue
		}
		p := &cayley_proto.Primitive{ID: mq.ID, Subject: mq.Dirs[0], Predicate: mq.Dirs[1], Object: mq.Dirs[2], Label: mq.Dirs[3]}
		extra[mq.ID] = p
		r.RecoveredQuads = append(r.RecoveredQuads, mq.ID)
		r.LabeledPredicates[p.Predicate] = [2]int{}
	}
	r.UnidentifiedLogKeys = int64(audit.LogHoleKeys) - identified
	return extra, nil
}

// missingDirs names the unknown directions of a lost quad.
func missingDirs(mq *GraphMissingQuad) string {
	var out []string
	for i, name := range []string{"subject", "predicate", "object"} {
		if !mq.Known[i] {
			out = append(out, name)
		}
	}
	switch len(out) {
	case 0:
		return "nothing"
	case 1:
		return out[0]
	}
	return fmt.Sprintf("%v", out)
}

// readLog reads every readable log record. A record whose value block is
// missing is dropped; a recovered record replaces nothing readable.
func (b *graphRebuilder) readLog(ctx context.Context, extra map[uint64]*cayley_proto.Primitive) error {
	var readErr error
	err := b.tree.Salvage(ctx, []byte("l/"), []byte("l0"), func(entry kvtx_block_okra.BuildEntry) bool {
		id, err := graphLogID(entry.Key)
		if err != nil {
			readErr = err
			return false
		}
		data, found, err := b.entryValue(ctx, entry)
		if err != nil {
			readErr = err
			return false
		}
		if !found {
			b.drop(id, false, "log record whose value block is missing")
			return true
		}
		var p cayley_proto.Primitive
		if err := p.UnmarshalVT(data); err != nil {
			readErr = errors.Wrapf(err, "decode log record %q", entry.Key)
			return false
		}
		delete(extra, p.ID)
		readErr = b.add(ctx, &p, false)
		return readErr == nil
	}, nil)
	if err == nil {
		err = readErr
	}
	return errors.Wrap(err, "read graph log")
}

// entryValue returns an entry's value, from the page when inline. A missing
// value block reads as not found.
func (b *graphRebuilder) entryValue(ctx context.Context, entry kvtx_block_okra.BuildEntry) ([]byte, bool, error) {
	if v := entry.ValueBlob; v != nil && v.GetBlobType() == blob.BlobType_BlobType_RAW {
		return v.GetRawData(), true, nil
	}
	value, found, err := b.tree.Get(ctx, entry.Key)
	if errors.Is(err, block.ErrNotFound) || errors.Is(err, block.ErrUnavailable) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, errors.Wrapf(err, "read value of %q", entry.Key)
	}
	return value, found, nil
}

// add adds one log record, keeping a recovered one to write.
func (b *graphRebuilder) add(ctx context.Context, p *cayley_proto.Primitive, recovered bool) error {
	if p.Deleted {
		b.drop(p.ID, p.IsNode(), "log record marked deleted")
		return nil
	}
	if recovered {
		data, err := p.MarshalVT()
		if err != nil {
			return err
		}
		b.written = append(b.written, graphKV{key: graphLogKey(p.ID), value: data})
	}
	if p.IsNode() {
		v, err := pquads.UnmarshalValue(ctx, p.Value)
		if err != nil {
			return errors.Wrapf(err, "decode value of node %d", p.ID)
		}
		b.nodes = append(b.nodes, rebuildNode{id: p.ID, hash: quad.HashOf(v)})
		b.isNode.set(p.ID)
		return nil
	}
	b.quads = append(b.quads, rebuildQuad{id: p.ID, dirs: [4]uint64{p.Subject, p.Predicate, p.Object, p.Label}, recovered: recovered})
	return nil
}

// drop records a dropped log record.
func (b *graphRebuilder) drop(id uint64, node bool, reason string) {
	b.dropped.set(id)
	b.report.Dropped = append(b.report.Dropped, GraphDroppedRecord{ID: id, Node: node, Reason: reason})
}

// resolve drops each quad that names a node with no record, counts the
// directions of the kept quads naming each node, and drops each node no kept
// quad names.
func (b *graphRebuilder) resolve() {
	r := b.report
	var maxID uint64
	for _, n := range b.nodes {
		maxID = max(maxID, n.id)
	}
	b.refs = make([]uint32, maxID+1)
	for _, q := range b.quads {
		if pred, ok := r.LabeledPredicates[q.dirs[1]]; ok {
			pred[1]++
			if q.dirs[3] != 0 {
				pred[0]++
			}
			r.LabeledPredicates[q.dirs[1]] = pred
		}
		lost := uint64(0)
		for _, id := range q.dirs {
			if id != 0 && !b.isNode.has(id) {
				lost = id
				break
			}
		}
		if lost != 0 {
			source := "readable"
			if q.recovered {
				source = "recovered"
			}
			b.drop(q.id, false, source+" quad names node "+strconv.FormatUint(lost, 10)+", which has no record")
			continue
		}
		for _, id := range q.dirs {
			if id != 0 {
				b.refs[id]++
			}
		}
		b.size++
	}
	r.Quads = int(b.size)
	for _, n := range b.nodes {
		if b.refs[n.id] == 0 {
			b.drop(n.id, true, "no kept quad names it")
			continue
		}
		r.Nodes++
	}
	sort.Slice(r.Dropped, func(i, j int) bool { return r.Dropped[i].ID < r.Dropped[j].ID })
}

// generated returns the rebuilt entries in key order: the recovered log
// records, the quad count, the reference counts, the postings and the value
// index.
func (b *graphRebuilder) generated() iter.Seq[graphKV] {
	return func(yield func(graphKV) bool) {
		// The recovered log records.
		written := slices.Clone(b.written)
		slices.SortFunc(written, func(x, y graphKV) int { return bytes.Compare(x.key, y.key) })
		for _, kv := range written {
			if !b.dropped.has(mustLogID(kv.key)) && !yield(kv) {
				return
			}
		}

		// The quad count.
		size := binary.LittleEndian.AppendUint64(nil, uint64(b.size))
		if !yield(graphKV{key: graphSizeKey(), value: size}) {
			return
		}

		// The reference counts, then the postings, then the value index; the
		// count and value keys share their order.
		nodes := b.keptNodesByHash()
		for _, n := range nodes {
			key := flat.KeyEscape([][]byte{[]byte("n"), n.b58})
			if !yield(graphKV{key: key, value: binary.AppendUvarint(nil, uint64(b.refs[n.id]))}) {
				return
			}
		}
		indexes := slices.Clone(b.layout)
		slices.SortFunc(indexes, func(x, y cayley_kv.QuadIndex) int {
			return bytes.Compare([]byte(postingBucketName(x)), []byte(postingBucketName(y)))
		})
		for _, index := range indexes {
			if !b.postings(index, yield) {
				return
			}
		}
		for _, n := range nodes {
			key := flat.KeyEscape([][]byte{[]byte("v"), n.b58})
			if !yield(graphKV{key: key, value: binary.AppendUvarint(nil, n.id)}) {
				return
			}
		}
	}
}

// hashedNode is a kept node with its value hash in base58.
type hashedNode struct {
	id  uint64
	b58 []byte
}

// keptNodesByHash returns the kept nodes sorted by value hash key.
func (b *graphRebuilder) keptNodesByHash() []hashedNode {
	out := make([]hashedNode, 0, len(b.nodes))
	for _, n := range b.nodes {
		if !b.dropped.has(n.id) {
			out = append(out, hashedNode{id: n.id, b58: []byte(b58.Encode(n.hash))})
		}
	}
	slices.SortFunc(out, func(x, y hashedNode) int { return bytes.Compare(x.b58, y.b58) })
	return out
}

// postings yields the posting entries of one index in key order, each
// listing its quad IDs in ascending order, as the index rebuilder writes them.
func (b *graphRebuilder) postings(index cayley_kv.QuadIndex, yield func(graphKV) bool) bool {
	// Encode each kept quad's posting key into one arena.
	var arena []byte
	offs := make([]uint64, 0, len(b.quads)+1)
	quads := make([]int32, 0, len(b.quads))
	vals := make([]uint64, len(index.Dirs))
	for i, q := range b.quads {
		if b.dropped.has(q.id) {
			continue
		}
		for j, d := range index.Dirs {
			vals[j] = q.dirs[int(d)-1]
		}
		offs = append(offs, uint64(len(arena)))
		arena = append(arena, flat.KeyEscape(index.Key(vals))...)
		quads = append(quads, int32(i))
	}
	offs = append(offs, uint64(len(arena)))
	keyOf := func(i int) []byte { return arena[offs[i]:offs[i+1]] }

	// Sort by key, then by quad ID within a key.
	perm := make([]int32, len(quads))
	for i := range perm {
		perm[i] = int32(i)
	}
	slices.SortFunc(perm, func(x, y int32) int {
		if c := bytes.Compare(keyOf(int(x)), keyOf(int(y))); c != 0 {
			return c
		}
		return cmpUint64(b.quads[quads[x]].id, b.quads[quads[y]].id)
	})

	// Yield one entry per key.
	for i := 0; i < len(perm); {
		key := keyOf(int(perm[i]))
		var value []byte
		j := i
		for ; j < len(perm) && bytes.Equal(keyOf(int(perm[j])), key); j++ {
			value = binary.AppendUvarint(value, b.quads[quads[perm[j]]].id)
		}
		if !yield(graphKV{key: bytes.Clone(key), value: value}) {
			return false
		}
		i = j
	}
	return true
}

// cmpUint64 compares two IDs.
func cmpUint64(x, y uint64) int {
	switch {
	case x < y:
		return -1
	case x > y:
		return 1
	}
	return 0
}

// merge walks the current tree and the rebuilt entries together in key
// order, passing out the entries of the rebuilt tree: every rebuilt entry,
// and each readable entry of a bucket the rebuild does not regenerate, except
// the log records it drops. It fills the report's bucket checks.
func (b *graphRebuilder) merge(ctx context.Context, out func(kvtx_block_okra.BuildEntry) bool) error {
	// Index the bucket checks.
	checks := make(map[string]*GraphBucketCheck)
	check := func(bucket string) *GraphBucketCheck {
		c := checks[bucket]
		if c == nil {
			c = &GraphBucketCheck{Bucket: bucket}
			checks[bucket] = c
			b.report.Checks = append(b.report.Checks, c)
		}
		return c
	}
	regenerated := map[string]bool{"n": true, "v": true}
	for _, index := range b.layout {
		regenerated[postingBucketName(index)] = true
	}
	sizeKey := graphSizeKey()
	isRegenerated := func(key []byte) bool {
		return regenerated[flatBucket(key, "")] || bytes.Equal(key, sizeKey)
	}
	// Keep the first examples of each kind, given the kind's count so far.
	example := func(c *GraphBucketCheck, kind string, n int, key []byte) {
		if n <= graphCheckExamples {
			c.Examples = append(c.Examples, kind+" "+strconv.Quote(string(key)))
		}
	}

	// Pull the rebuilt entries alongside the walk.
	next, stop := iter.Pull(b.generated())
	defer stop()
	gen, more := next()
	emit := func(kv graphKV) (bool, error) {
		entry, err := b.tree.ValueEntry(ctx, kv.key, kv.value)
		if err != nil {
			return false, err
		}
		return out(entry), nil
	}
	added := func(kv graphKV) (bool, error) {
		c := check(flatBucket(kv.key, ""))
		c.Added++
		example(c, "added", c.Added, kv.key)
		return emit(kv)
	}

	var walkErr error
	stopped := false
	err := b.tree.Salvage(ctx, nil, nil, func(entry kvtx_block_okra.BuildEntry) bool {
		// Pass out the rebuilt entries before this key.
		for more && bytes.Compare(gen.key, entry.Key) < 0 {
			ok, err := added(gen)
			if err != nil || !ok {
				walkErr, stopped = err, err == nil
				return false
			}
			gen, more = next()
		}
		same := more && bytes.Equal(gen.key, entry.Key)
		bucket := flatBucket(entry.Key, "")

		// Keep a readable entry of a bucket the rebuild leaves, except a
		// dropped log record.
		if !isRegenerated(entry.Key) && !same {
			if bucket == "l" {
				id, err := graphLogID(entry.Key)
				if err != nil {
					walkErr = err
					return false
				}
				if b.dropped.has(id) {
					c := check(bucket)
					c.Removed++
					example(c, "removed", c.Removed, entry.Key)
					return true
				}
			}
			if !out(entry) {
				stopped = true
				return false
			}
			return true
		}

		// Compare a regenerated entry with the readable one.
		c := check(bucket)
		if !same {
			c.Removed++
			example(c, "removed", c.Removed, entry.Key)
			return true
		}
		value, found, err := b.entryValue(ctx, entry)
		if err != nil {
			walkErr = err
			return false
		}
		if found && bytes.Equal(value, gen.value) {
			c.Same++
		} else {
			c.Changed++
			example(c, "changed", c.Changed, entry.Key)
		}
		ok, err := emit(gen)
		if err != nil || !ok {
			walkErr, stopped = err, err == nil
			return false
		}
		gen, more = next()
		return true
	}, nil)
	if err == nil {
		err = walkErr
	}
	if err != nil || stopped {
		return err
	}

	// Pass out the rebuilt entries past the last readable key.
	for more {
		ok, err := added(gen)
		if err != nil || !ok {
			return err
		}
		gen, more = next()
	}
	return ctx.Err()
}

// graphSizeKey returns the key of the stored quad count.
func graphSizeKey() []byte {
	return flat.KeyEscape([][]byte{[]byte("m"), []byte("size")})
}

// graphLogID returns the ID of a log key.
func graphLogID(key []byte) (uint64, error) {
	parts := flat.KeyUnescape(flat.Key(key))
	if len(parts) != 2 {
		return 0, errors.Errorf("bad log key %q", key)
	}
	id, err := strconv.ParseUint(string(parts[1]), 10, 64)
	return id, errors.Wrapf(err, "bad log key %q", key)
}

// mustLogID returns the ID of a log key built by graphLogKey.
func mustLogID(key []byte) uint64 {
	id, _ := graphLogID(key)
	return id
}
