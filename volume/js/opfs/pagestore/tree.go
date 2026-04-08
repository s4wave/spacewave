package pagestore

import (
	"github.com/pkg/errors"
)

// Tree is a B+tree backed by a Pager.
type Tree struct {
	pager    Pager
	rootID   PageID
	pageBuf  []byte // reusable page buffer
	pageBuf2 []byte // second buffer for splits
}

// NewTree creates a new B+tree with an empty root leaf.
func NewTree(pager Pager) *Tree {
	t := &Tree{
		pager:    pager,
		rootID:   InvalidPage,
		pageBuf:  make([]byte, pager.PageSize()),
		pageBuf2: make([]byte, pager.PageSize()),
	}
	return t
}

// OpenTree opens an existing B+tree with the given root page.
func OpenTree(pager Pager, rootID PageID) *Tree {
	return &Tree{
		pager:    pager,
		rootID:   rootID,
		pageBuf:  make([]byte, pager.PageSize()),
		pageBuf2: make([]byte, pager.PageSize()),
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
		if err := t.pager.ReadPage(pageID, t.pageBuf); err != nil {
			return nil, false, errors.Wrap(err, "read page")
		}
		h := DecodePageHeader(t.pageBuf)

		switch h.Type {
		case PageTypeLeaf:
			entries, err := DecodeLeafPage(t.pageBuf)
			if err != nil {
				return nil, false, err
			}
			for i := range entries {
				if string(entries[i].Key) == string(key) {
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
		// Create initial leaf.
		id := t.pager.AllocPage()
		clear(t.pageBuf)
		EncodeLeafPage(t.pageBuf, []LeafEntry{{Key: key, Value: value}})
		if err := t.pager.WritePage(id, t.pageBuf); err != nil {
			return err
		}
		t.rootID = id
		return nil
	}

	splitKey, splitPage, err := t.insert(t.rootID, key, value)
	if err != nil {
		return err
	}

	// If insert caused a root split, create a new branch root.
	if splitPage != InvalidPage {
		newRoot := t.pager.AllocPage()
		clear(t.pageBuf)
		EncodeBranchPage(t.pageBuf, []BranchEntry{
			{Key: nil, ChildID: t.rootID},
			{Key: splitKey, ChildID: splitPage},
		})
		if err := t.pager.WritePage(newRoot, t.pageBuf); err != nil {
			return err
		}
		t.rootID = newRoot
	}
	return nil
}

// insert recursively inserts into the subtree rooted at pageID.
// Returns (splitKey, splitPageID) if the page split, or (nil, InvalidPage).
func (t *Tree) insert(pageID PageID, key, value []byte) ([]byte, PageID, error) {
	buf := make([]byte, t.pager.PageSize())
	if err := t.pager.ReadPage(pageID, buf); err != nil {
		return nil, InvalidPage, err
	}
	h := DecodePageHeader(buf)

	switch h.Type {
	case PageTypeLeaf:
		return t.insertLeaf(pageID, buf, key, value)
	case PageTypeBranch:
		return t.insertBranch(pageID, buf, key, value)
	default:
		return nil, InvalidPage, errors.Errorf("unexpected page type %d in insert", h.Type)
	}
}

// insertLeaf handles leaf insertion with potential split.
func (t *Tree) insertLeaf(pageID PageID, buf []byte, key, value []byte) ([]byte, PageID, error) {
	entries, err := DecodeLeafPage(buf)
	if err != nil {
		return nil, InvalidPage, err
	}

	// Upsert: find position and replace or insert.
	found := false
	insertPos := len(entries)
	for i := range entries {
		if string(entries[i].Key) == string(key) {
			entries[i].Value = value
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
		entries[insertPos] = LeafEntry{Key: key, Value: value}
	}

	// Try to fit in one page.
	clear(buf)
	written := EncodeLeafPage(buf, entries)
	if written == len(entries) {
		// COW: write to new page, free old.
		newID := t.pager.AllocPage()
		if err := t.pager.WritePage(newID, buf); err != nil {
			return nil, InvalidPage, err
		}
		// Update parent's pointer to this new page.
		// For root, the caller updates rootID.
		// For non-root, the caller updates the branch entry.
		// We reuse the pageID slot by writing to the same ID (COW deferred).
		// Actually for simplicity, overwrite in place for now.
		if err := t.pager.WritePage(pageID, buf); err != nil {
			return nil, InvalidPage, err
		}
		t.pager.FreePage(newID)
		return nil, InvalidPage, nil
	}

	// Split: first half stays, second half goes to new page.
	mid := len(entries) / 2
	left := entries[:mid]
	right := entries[mid:]

	clear(buf)
	EncodeLeafPage(buf, left)
	if err := t.pager.WritePage(pageID, buf); err != nil {
		return nil, InvalidPage, err
	}

	rightID := t.pager.AllocPage()
	rightBuf := make([]byte, t.pager.PageSize())
	EncodeLeafPage(rightBuf, right)
	if err := t.pager.WritePage(rightID, rightBuf); err != nil {
		return nil, InvalidPage, err
	}

	return right[0].Key, rightID, nil
}

// insertBranch handles branch insertion with potential split.
func (t *Tree) insertBranch(pageID PageID, buf []byte, key, value []byte) ([]byte, PageID, error) {
	entries, err := DecodeBranchPage(buf)
	if err != nil {
		return nil, InvalidPage, err
	}

	childID := findChild(entries, key)
	splitKey, splitPage, err := t.insert(childID, key, value)
	if err != nil {
		return nil, InvalidPage, err
	}

	if splitPage == InvalidPage {
		return nil, InvalidPage, nil
	}

	// Insert the new separator + child pointer.
	newEntry := BranchEntry{Key: splitKey, ChildID: splitPage}
	insertPos := len(entries)
	for i := 1; i < len(entries); i++ {
		if string(entries[i].Key) > string(splitKey) {
			insertPos = i
			break
		}
	}
	entries = append(entries, BranchEntry{})
	copy(entries[insertPos+1:], entries[insertPos:])
	entries[insertPos] = newEntry

	// Try to fit in one page.
	clear(buf)
	written := EncodeBranchPage(buf, entries)
	if written == len(entries) {
		if err := t.pager.WritePage(pageID, buf); err != nil {
			return nil, InvalidPage, err
		}
		return nil, InvalidPage, nil
	}

	// Split the branch.
	mid := len(entries) / 2
	left := entries[:mid]
	right := entries[mid:]
	promoteKey := right[0].Key
	// The promoted key's child becomes the leftmost child of the right branch.
	right[0].Key = nil

	clear(buf)
	EncodeBranchPage(buf, left)
	if err := t.pager.WritePage(pageID, buf); err != nil {
		return nil, InvalidPage, err
	}

	rightID := t.pager.AllocPage()
	rightBuf := make([]byte, t.pager.PageSize())
	EncodeBranchPage(rightBuf, right)
	if err := t.pager.WritePage(rightID, rightBuf); err != nil {
		return nil, InvalidPage, err
	}

	return promoteKey, rightID, nil
}

// Delete removes a key from the tree. Returns true if the key was found.
func (t *Tree) Delete(key []byte) (bool, error) {
	if t.rootID == InvalidPage {
		return false, nil
	}
	return t.deleteFrom(t.rootID, key)
}

// deleteFrom removes a key from the subtree. Simple version without rebalancing.
func (t *Tree) deleteFrom(pageID PageID, key []byte) (bool, error) {
	buf := make([]byte, t.pager.PageSize())
	if err := t.pager.ReadPage(pageID, buf); err != nil {
		return false, err
	}
	h := DecodePageHeader(buf)

	switch h.Type {
	case PageTypeLeaf:
		entries, err := DecodeLeafPage(buf)
		if err != nil {
			return false, err
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
			return false, nil
		}
		clear(buf)
		EncodeLeafPage(buf, entries)
		return true, t.pager.WritePage(pageID, buf)

	case PageTypeBranch:
		entries, err := DecodeBranchPage(buf)
		if err != nil {
			return false, err
		}
		childID := findChild(entries, key)
		return t.deleteFrom(childID, key)

	default:
		return false, errors.Errorf("unexpected page type %d", h.Type)
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
	if err := t.pager.ReadPage(pageID, buf); err != nil {
		return err
	}
	h := DecodePageHeader(buf)

	switch h.Type {
	case PageTypeLeaf:
		entries, err := DecodeLeafPage(buf)
		if err != nil {
			return err
		}
		for i := range entries {
			k := entries[i].Key
			if len(k) >= len(prefix) && string(k[:len(prefix)]) == string(prefix) {
				if !fn(k, entries[i].Value) {
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
	// Linear search (branch pages are small).
	child := entries[0].ChildID
	for i := 1; i < len(entries); i++ {
		if string(key) >= string(entries[i].Key) {
			child = entries[i].ChildID
		} else {
			break
		}
	}
	return child
}
