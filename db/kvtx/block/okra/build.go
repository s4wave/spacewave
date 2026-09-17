package kvtx_block_okra

import (
	"bytes"
	"iter"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
)

// BuildEntry is one sorted key/value entry used by the Okra tree builder.
type BuildEntry struct {
	Key []byte

	ValueRef    *block.BlockRef
	ValueIsBlob bool
	ValueBlob   *blob.Blob
}

// buildNode is one entry of the in-memory level being built, with the cursor
// of the child page it references, if any.
type buildNode struct {
	// entry is the node's key entry.
	entry *Entry
	// childCursor is the child page cursor, nil for leaf entries.
	childCursor *block.Cursor
}

// BuildTree builds a packed Okra page DAG from sorted key/value refs.
func BuildTree(
	store block.StoreOps,
	xfrm block.Transformer,
	putOpts *block.PutOpts,
	entries iter.Seq2[[]byte, *block.BlockRef],
) (*block.Transaction, *block.Cursor, error) {
	return BuildTreeWithEntries(store, xfrm, putOpts, func(yield func(BuildEntry) bool) {
		for key, ref := range entries {
			if !yield(BuildEntry{Key: key, ValueRef: ref}) {
				return
			}
		}
	})
}

// BuildTreeWithEntries builds a packed Okra page DAG from sorted entries.
func BuildTreeWithEntries(
	store block.StoreOps,
	xfrm block.Transformer,
	putOpts *block.PutOpts,
	entries iter.Seq[BuildEntry],
) (*block.Transaction, *block.Cursor, error) {
	tx, rootCursor := block.NewTransaction(store, xfrm, nil, putOpts)
	if _, err := buildTreeAtCursor(rootCursor, entries); err != nil {
		return nil, nil, err
	}
	return tx, rootCursor, nil
}

// buildTreeAtCursor builds the packed page DAG under rootCursor from the
// sorted entries, returning the completed root metadata.
func buildTreeAtCursor(rootCursor *block.Cursor, entries iter.Seq[BuildEntry]) (*Root, error) {
	root := &Root{}
	rootCursor.SetBlock(root, true)
	rootCursor.ClearAllRefs()

	current := []*buildNode{{
		entry: &Entry{
			Anchor: true,
			Hash:   mustAnchorHash(),
		},
	}}
	var prevKey []byte
	var count uint64
	for ent := range entries {
		key := slices.Clone(ent.Key)
		if len(key) == 0 {
			return nil, ErrUnexpectedEntryMetadata
		}
		if prevKey != nil && bytes.Compare(prevKey, key) >= 0 {
			return nil, ErrUnsortedEntries
		}
		ref := ent.ValueRef.Clone()
		leafHash, err := hashBuildEntry(ent)
		if err != nil {
			return nil, err
		}
		current = append(current, &buildNode{
			entry: &Entry{
				Key:         key,
				Hash:        leafHash,
				Size:        1,
				ValueRef:    ref,
				ValueIsBlob: ent.ValueIsBlob,
				ValueBlob:   ent.ValueBlob.CloneVT(),
			},
		})
		prevKey = key
		count++
	}
	if count == 0 {
		return root, nil
	}

	var level uint32
	for {
		parent, err := buildParentLevel(rootCursor, level, current)
		if err != nil {
			return nil, err
		}
		level++
		if len(parent) == 1 {
			rootPage, err := createPage(rootCursor, level, parent, nil)
			if err != nil {
				return nil, err
			}
			root.Size = parent[0].entry.GetSize()
			root.Height = level
			root.RootHash = slices.Clone(parent[0].entry.GetHash())
			root.HashSize = HashSize
			root.FanoutDegree = FanoutDegree
			rootCursor.SetBlock(root, true)
			rootCursor.SetRef(rootPageRefID, rootPage)
			return root, nil
		}
		if level >= 254 {
			return nil, errors.New("okra tree exceeded maximum height")
		}
		current = parent
	}
}

// buildParentLevel groups the current level's nodes at page boundaries and
// returns one parent node per built page.
func buildParentLevel(root *block.Cursor, level uint32, current []*buildNode) ([]*buildNode, error) {
	parent := make([]*buildNode, 0, len(current)/FanoutDegree+1)
	for start := 0; start < len(current); {
		end := start + 1
		for end < len(current) && !isBoundary(current[end].entry.GetHash()) {
			end++
		}
		var upper []byte
		if end < len(current) {
			upper = current[end].entry.GetKey()
		}
		childPage, err := createPage(root, level, current[start:end], upper)
		if err != nil {
			return nil, err
		}
		parentHash, err := hashNodeRange(current[start:end])
		if err != nil {
			return nil, err
		}
		parent = append(parent, &buildNode{
			entry: &Entry{
				Anchor: current[start].entry.GetAnchor(),
				Key:    slices.Clone(current[start].entry.GetKey()),
				Hash:   parentHash,
				Size:   nodeRangeSize(current[start:end]),
			},
			childCursor: childPage,
		})
		start = end
	}
	return parent, nil
}

// createPage builds one page cursor over the nodes, recording each node's
// child cursor as a page reference.
func createPage(root *block.Cursor, level uint32, nodes []*buildNode, upper []byte) (*block.Cursor, error) {
	page := &Page{
		Level:          level,
		UpperBound:     slices.Clone(upper),
		StartsAtAnchor: nodes[0].entry.GetAnchor(),
		Entries:        make([]*Entry, len(nodes)),
	}
	for idx, node := range nodes {
		page.Entries[idx] = node.entry
		page.Size += node.entry.GetSize()
		if !node.entry.GetAnchor() && len(page.LowerBound) == 0 {
			page.LowerBound = slices.Clone(node.entry.GetKey())
		}
	}
	clonePageEntries(page.Entries)
	pageHash, err := hashPage(page)
	if err != nil {
		return nil, err
	}
	page.PageHash = pageHash

	cursor := root.Detach(false)
	cursor.ClearAllRefs()
	cursor.SetBlock(page, true)
	for idx, node := range nodes {
		if node.childCursor != nil {
			cursor.SetRef(entryChildRefID(idx), node.childCursor)
		}
	}
	return cursor, nil
}

// nodeRangeSize sums the entry sizes across the node range.
func nodeRangeSize(nodes []*buildNode) uint64 {
	var size uint64
	for _, node := range nodes {
		size += node.entry.GetSize()
	}
	return size
}

// mustAnchorHash returns the anchor entry's hash, panicking only if the
// package's own digest fails, which prevents the package from operating.
func mustAnchorHash() []byte {
	hash, err := okraDigest(nil)
	if err != nil {
		panic(err)
	}
	return hash
}
