package pagestore

import "github.com/pkg/errors"

// Tree is a B+tree backed by a Pager.
type Tree struct {
	pager   Pager
	rootID  PageID
	pageBuf []byte // reusable page buffer
}

// NewTree creates a new B+tree with an empty root leaf.
func NewTree(pager Pager) *Tree {
	t := &Tree{
		pager:   pager,
		rootID:  InvalidPage,
		pageBuf: make([]byte, pager.PageSize()),
	}
	return t
}

// OpenTree opens an existing B+tree with the given root page.
func OpenTree(pager Pager, rootID PageID) *Tree {
	return &Tree{
		pager:   pager,
		rootID:  rootID,
		pageBuf: make([]byte, pager.PageSize()),
	}
}

// RootID returns the current root page ID.
func (t *Tree) RootID() PageID { return t.rootID }

// Get looks up a key. Returns value, found.
func (t *Tree) Get(key []byte) ([]byte, bool, error) {
	if t.rootID == InvalidPage {
		return nil, false, nil
	}

	pageID := t.rootID
	for {
		h, err := t.readTreePage(pageID, t.pageBuf)
		if err != nil {
			return nil, false, err
		}

		switch h.Type {
		case PageTypeLeaf:
			entries, err := DecodeLeafPage(t.pageBuf)
			if err != nil {
				return nil, false, err
			}
			for i := range entries {
				if string(entries[i].Key) == string(key) {
					if entries[i].OverflowLen != 0 {
						val, readErr := t.readOverflowValue(entries[i].OverflowPage, entries[i].OverflowLen)
						if readErr != nil {
							return nil, false, readErr
						}
						return val, true, nil
					}
					return entries[i].Value, true, nil
				}
			}
			return nil, false, nil

		case PageTypeBranch:
			entries, err := DecodeBranchPage(t.pageBuf)
			if err != nil {
				return nil, false, err
			}
			pageID = findChild(entries, key)

		default:
			return nil, false, errors.Errorf("unexpected page type %d", h.Type)
		}
	}
}

// Put inserts or updates a key-value pair.
func (t *Tree) Put(key, value []byte) error {
	if t.rootID == InvalidPage {
		entry, err := t.makeLeafEntry(key, value)
		if err != nil {
			return err
		}
		id, err := t.writeLeafPage([]LeafEntry{entry})
		if err != nil {
			return err
		}
		t.rootID = id
		return nil
	}

	newRoot, splitKey, splitPage, err := t.insert(t.rootID, key, value)
	if err != nil {
		return err
	}

	// If insert caused a root split, create a new branch root.
	if splitPage != InvalidPage {
		rootID, writeErr := t.writeBranchPage([]BranchEntry{
			{Key: nil, ChildID: newRoot},
			{Key: splitKey, ChildID: splitPage},
		})
		if writeErr != nil {
			return writeErr
		}
		t.rootID = rootID
		return nil
	}
	t.rootID = newRoot
	return nil
}

// insert recursively inserts into the subtree rooted at pageID.
// Returns the replacement page ID for the mutated subtree. If the page split,
// also returns the separator key and right-side page ID.
func (t *Tree) insert(pageID PageID, key, value []byte) (PageID, []byte, PageID, error) {
	buf := make([]byte, t.pager.PageSize())
	h, err := t.readTreePage(pageID, buf)
	if err != nil {
		return InvalidPage, nil, InvalidPage, err
	}

	switch h.Type {
	case PageTypeLeaf:
		return t.insertLeaf(pageID, buf, key, value)
	case PageTypeBranch:
		return t.insertBranch(pageID, buf, key, value)
	default:
		return InvalidPage, nil, InvalidPage, errors.Errorf("unexpected page type %d in insert", h.Type)
	}
}

// insertLeaf handles leaf insertion with potential split.
func (t *Tree) insertLeaf(pageID PageID, buf []byte, key, value []byte) (PageID, []byte, PageID, error) {
	entries, err := DecodeLeafPage(buf)
	if err != nil {
		return InvalidPage, nil, InvalidPage, err
	}

	entry, err := t.makeLeafEntry(key, value)
	if err != nil {
		return InvalidPage, nil, InvalidPage, err
	}

	found := false
	insertPos := len(entries)
	for i := range entries {
		if string(entries[i].Key) == string(key) {
			entries[i] = entry
			found = true
			break
		}
		if string(entries[i].Key) > string(key) {
			insertPos = i
			break
		}
	}
	if !found {
		entries = append(entries, LeafEntry{})
		copy(entries[insertPos+1:], entries[insertPos:])
		entries[insertPos] = entry
	}

	// Try to fit in one page.
	work := make([]byte, t.pager.PageSize())
	written := EncodeLeafPage(work, entries)
	if written == len(entries) {
		newID, writeErr := t.writePage(work)
		if writeErr != nil {
			return InvalidPage, nil, InvalidPage, writeErr
		}
		return newID, nil, InvalidPage, nil
	}

	// Split: first half stays, second half goes to new page.
	mid := len(entries) / 2
	left := entries[:mid]
	right := entries[mid:]

	leftID, writeErr := t.writeLeafPage(left)
	if writeErr != nil {
		return InvalidPage, nil, InvalidPage, writeErr
	}
	rightID, writeErr := t.writeLeafPage(right)
	if writeErr != nil {
		return InvalidPage, nil, InvalidPage, writeErr
	}

	return leftID, right[0].Key, rightID, nil
}

// insertBranch handles branch insertion with potential split.
func (t *Tree) insertBranch(pageID PageID, buf []byte, key, value []byte) (PageID, []byte, PageID, error) {
	entries, err := DecodeBranchPage(buf)
	if err != nil {
		return InvalidPage, nil, InvalidPage, err
	}

	childIdx := findChildIndex(entries, key)
	childID := entries[childIdx].ChildID
	newChildID, splitKey, splitPage, err := t.insert(childID, key, value)
	if err != nil {
		return InvalidPage, nil, InvalidPage, err
	}

	entries[childIdx].ChildID = newChildID
	if splitPage != InvalidPage {
		// Insert the new separator + child pointer after the split child.
		newEntry := BranchEntry{Key: splitKey, ChildID: splitPage}
		insertPos := childIdx + 1
		entries = append(entries, BranchEntry{})
		copy(entries[insertPos+1:], entries[insertPos:])
		entries[insertPos] = newEntry
	}

	// Try to fit in one page.
	work := make([]byte, t.pager.PageSize())
	written := EncodeBranchPage(work, entries)
	if written == len(entries) {
		newID, writeErr := t.writePage(work)
		if writeErr != nil {
			return InvalidPage, nil, InvalidPage, writeErr
		}
		return newID, nil, InvalidPage, nil
	}

	// Split the branch.
	mid := len(entries) / 2
	left := entries[:mid]
	right := entries[mid:]
	promoteKey := right[0].Key
	// The promoted key's child becomes the leftmost child of the right branch.
	right[0].Key = nil

	leftID, writeErr := t.writeBranchPage(left)
	if writeErr != nil {
		return InvalidPage, nil, InvalidPage, writeErr
	}
	rightID, writeErr := t.writeBranchPage(right)
	if writeErr != nil {
		return InvalidPage, nil, InvalidPage, writeErr
	}

	return leftID, promoteKey, rightID, nil
}

// Delete removes a key from the tree. Returns true if the key was found.
func (t *Tree) Delete(key []byte) (bool, error) {
	if t.rootID == InvalidPage {
		return false, nil
	}
	found, newRoot, err := t.deleteFrom(t.rootID, key)
	if err != nil {
		return false, err
	}
	if found {
		t.rootID = newRoot
	}
	return found, nil
}

// deleteFrom removes a key from the subtree. Simple version without rebalancing.
func (t *Tree) deleteFrom(pageID PageID, key []byte) (bool, PageID, error) {
	buf := make([]byte, t.pager.PageSize())
	h, err := t.readTreePage(pageID, buf)
	if err != nil {
		return false, InvalidPage, err
	}

	switch h.Type {
	case PageTypeLeaf:
		entries, err := DecodeLeafPage(buf)
		if err != nil {
			return false, InvalidPage, err
		}
		found := false
		for i := range entries {
			if string(entries[i].Key) == string(key) {
				entries = append(entries[:i], entries[i+1:]...)
				found = true
				break
			}
		}
		if !found {
			return false, pageID, nil
		}
		newID, writeErr := t.writeLeafPage(entries)
		if writeErr != nil {
			return false, InvalidPage, writeErr
		}
		return true, newID, nil

	case PageTypeBranch:
		entries, err := DecodeBranchPage(buf)
		if err != nil {
			return false, InvalidPage, err
		}
		childIdx := findChildIndex(entries, key)
		found, newChildID, deleteErr := t.deleteFrom(entries[childIdx].ChildID, key)
		if deleteErr != nil {
			return false, InvalidPage, deleteErr
		}
		if !found {
			return false, pageID, nil
		}
		entries[childIdx].ChildID = newChildID
		newID, writeErr := t.writeBranchPage(entries)
		if writeErr != nil {
			return false, InvalidPage, writeErr
		}
		return true, newID, nil

	default:
		return false, InvalidPage, errors.Errorf("unexpected page type %d", h.Type)
	}
}

// ScanPrefix iterates over all entries with keys matching the prefix.
// Calls fn for each entry. Stops if fn returns false.
func (t *Tree) ScanPrefix(prefix []byte, fn func(key, value []byte) bool) error {
	if t.rootID == InvalidPage {
		return nil
	}
	return t.scanFrom(t.rootID, prefix, fn)
}

// scanFrom scans the subtree for prefix matches.
func (t *Tree) scanFrom(pageID PageID, prefix []byte, fn func(key, value []byte) bool) error {
	buf := make([]byte, t.pager.PageSize())
	h, err := t.readTreePage(pageID, buf)
	if err != nil {
		return err
	}

	switch h.Type {
	case PageTypeLeaf:
		entries, err := DecodeLeafPage(buf)
		if err != nil {
			return err
		}
		for i := range entries {
			k := entries[i].Key
			if len(k) >= len(prefix) && string(k[:len(prefix)]) == string(prefix) {
				value := entries[i].Value
				if entries[i].OverflowLen != 0 {
					var readErr error
					value, readErr = t.readOverflowValue(entries[i].OverflowPage, entries[i].OverflowLen)
					if readErr != nil {
						return readErr
					}
				}
				if !fn(k, value) {
					return nil
				}
			} else if string(k) > string(prefix)+"~" {
				// Past the prefix range in sorted order.
				return nil
			}
		}
		return nil

	case PageTypeBranch:
		entries, err := DecodeBranchPage(buf)
		if err != nil {
			return err
		}
		// Scan all children that might contain prefix matches.
		for i := range entries {
			if err := t.scanFrom(entries[i].ChildID, prefix, fn); err != nil {
				return err
			}
		}
		return nil

	default:
		return errors.Errorf("unexpected page type %d", h.Type)
	}
}

// findChild returns the child page for a given key in a branch page.
func findChild(entries []BranchEntry, key []byte) PageID {
	return entries[findChildIndex(entries, key)].ChildID
}

// findChildIndex returns the child entry index for a given key in a branch page.
func findChildIndex(entries []BranchEntry, key []byte) int {
	// Linear search (branch pages are small).
	childIdx := 0
	for i := 1; i < len(entries); i++ {
		if string(key) >= string(entries[i].Key) {
			childIdx = i
		} else {
			break
		}
	}
	return childIdx
}

func (t *Tree) writeLeafPage(entries []LeafEntry) (PageID, error) {
	buf := make([]byte, t.pager.PageSize())
	written := EncodeLeafPage(buf, entries)
	if written != len(entries) {
		return InvalidPage, errors.New("leaf entries exceed page size")
	}
	return t.writePage(buf)
}

func (t *Tree) writeBranchPage(entries []BranchEntry) (PageID, error) {
	buf := make([]byte, t.pager.PageSize())
	written := EncodeBranchPage(buf, entries)
	if written != len(entries) {
		return InvalidPage, errors.New("branch entries exceed page size")
	}
	return t.writePage(buf)
}

func (t *Tree) writePage(buf []byte) (PageID, error) {
	id := t.pager.AllocPage()
	if err := t.pager.WritePage(id, buf); err != nil {
		return InvalidPage, err
	}
	return id, nil
}

func (t *Tree) makeLeafEntry(key, value []byte) (LeafEntry, error) {
	if len(key) > int(^uint16(0)) {
		return LeafEntry{}, errors.New("leaf key length overflows uint16")
	}
	needed := PageHeaderSize + LeafEntryOverhead + len(key) + len(value)
	if needed <= t.pager.PageSize() && len(value) < int(OverflowSentinel) {
		return LeafEntry{Key: key, Value: value}, nil
	}
	refNeeded := PageHeaderSize + LeafEntryOverhead + len(key) + 8
	if refNeeded > t.pager.PageSize() {
		return LeafEntry{}, errors.New("leaf key exceeds page size")
	}
	if uint64(len(value)) > uint64(^uint32(0)) {
		return LeafEntry{}, errors.New("overflow value length overflows uint32")
	}
	firstPage, err := t.writeOverflowValue(value)
	if err != nil {
		return LeafEntry{}, err
	}
	return LeafEntry{
		Key:          key,
		OverflowPage: firstPage,
		OverflowLen:  uint32(len(value)),
	}, nil
}

func (t *Tree) writeOverflowValue(value []byte) (PageID, error) {
	capacity := OverflowPageCapacity(t.pager.PageSize())
	if capacity < 1 {
		return InvalidPage, errors.New("page size too small for overflow value")
	}

	pageCount := (len(value) + capacity - 1) / capacity
	pages := make([]PageID, pageCount)
	for i := range pages {
		pages[i] = t.pager.AllocPage()
	}

	buf := make([]byte, t.pager.PageSize())
	off := 0
	for i, page := range pages {
		nextPage := InvalidPage
		if i+1 < len(pages) {
			nextPage = pages[i+1]
		}
		clear(buf)
		written := EncodeOverflowPage(buf, nextPage, value[off:])
		if written == 0 {
			return InvalidPage, errors.New("overflow page wrote zero bytes")
		}
		if err := t.pager.WritePage(page, buf); err != nil {
			return InvalidPage, err
		}
		off += written
	}
	return pages[0], nil
}

func (t *Tree) readOverflowValue(pageID PageID, valueLen uint32) ([]byte, error) {
	buf := make([]byte, t.pager.PageSize())
	out := make([]byte, 0, int(valueLen))
	for pageID != InvalidPage && len(out) < int(valueLen) {
		if err := t.pager.ReadPage(pageID, buf); err != nil {
			return nil, NewCorruptPageError(pageID, errors.Wrap(err, "read overflow page"))
		}
		nextPage, value, err := DecodeOverflowPage(buf)
		if err != nil {
			return nil, NewCorruptPageError(pageID, err)
		}
		if len(out)+len(value) > int(valueLen) {
			return nil, NewCorruptPageError(pageID, errors.New("overflow value exceeds declared length"))
		}
		out = append(out, value...)
		pageID = nextPage
	}
	if len(out) != int(valueLen) {
		return nil, errors.Errorf("overflow value truncated: got %d want %d", len(out), valueLen)
	}
	return out, nil
}

func (t *Tree) readTreePage(pageID PageID, buf []byte) (*PageHeader, error) {
	if err := t.pager.ReadPage(pageID, buf); err != nil {
		return nil, NewCorruptPageError(pageID, errors.Wrap(err, "read page"))
	}
	h := DecodePageHeader(buf)
	switch h.Type {
	case PageTypeLeaf, PageTypeBranch:
	default:
		return nil, NewCorruptPageError(pageID, errors.Errorf("unexpected page type %d", h.Type))
	}
	if err := ValidatePage(buf); err != nil {
		return nil, NewCorruptPageError(pageID, err)
	}
	return h, nil
}
