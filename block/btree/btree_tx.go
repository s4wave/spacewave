package btree

import (
	"bytes"
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/cid"
	"github.com/aperturerobotics/hydra/kvtx"
)

// Tx is a btree transaction
type Tx struct {
	commitOnce sync.Once
	b          *BTree
	write      bool

	rn                      *Root
	baseNod                 *Node
	tx                      *block.Transaction
	rnCursor, baseNodCursor *block.Cursor
}

// Len returns the number of items in the tree.
func (t *Tx) Len() (int, error) {
	return int(t.rn.GetLength()), nil
}

// Get returns values for a key.
func (t *Tx) Get(key []byte) (data []byte, found bool, err error) {
	if t.rn.GetLength() == 0 {
		return nil, false, nil
	}

	return t.getFromNode(t.baseNodCursor, t.baseNod, key)
}

// getFromNode gets a key from a subtree.
func (t *Tx) getFromNode(
	cursor *block.Cursor,
	nod *Node,
	key []byte,
) ([]byte, bool, error) {
	i, found := t.findInNode(nod, key)
	if found {
		return nod.GetItems()[i].GetValue(), true, nil
	} else if !nod.GetChildrenEmpty() {
		// follow ref at i
		ref := nod.ChildrenRefs[i]
		cc, err := cursor.FollowRef(nod.ChildRefId(i), ref)
		if err != nil {
			return nil, false, err
		}
		ccObj, err := cc.Unmarshal(t.b.newNodeBlock)
		if err != nil {
			return nil, false, err
		}
		cb, _ := ccObj.(*Node)
		if cb == nil {
			return nil, false, nil
		}
		return t.getFromNode(cc, cb, key)
	}
	return nil, false, nil
}

// Set sets the value of a key.
// This will not be committed until Commit is called.
func (t *Tx) Set(key, value []byte, ttl time.Duration) error {
	_, err := t.ReplaceOrInsert(key, value)
	return err
}

// ReplaceOrInsert replaces or inserts an item, if replacing, returns the value.
func (t *Tx) ReplaceOrInsert(
	key []byte,
	val []byte,
) (rval []byte, rerr error) {
	if len(key) == 0 {
		return nil, errors.New("key cannot be empty")
	}

	rn := t.rn
	rnCursor := t.rnCursor
	baseNod := t.baseNod
	baseNodCursor := t.baseNodCursor
	item := &Item{Key: key, Value: val}
	if rn.GetLength() == 0 {
		nnod := &Node{Leaf: true, Items: []*Item{item}}
		baseNodCursor.SetBlock(nnod)
		baseNodCursor.SetPreWriteHook(preWriteNodeHook)
		rn.Length++
		rnCursor.SetBlock(rn)
		t.baseNod = nnod
		return nil, nil
	}

	maxItems := t.maxItems(rn)
	// Need to take special care to re-knit the block graph here.
	if len(baseNod.Items) >= maxItems {
		// outcome:
		// - root.items = [item]
		// - root.children[0].items = oldRoot.items[:i] (truncate)
		// - root.children[0].children = oldRoot.children[:i+1] (truncate)
		// - root.children[1].children = oldRoot.children[i+1:]
		// - root.children[1].items = oldRoot.items[i+1:]
		// old setup:
		//                   root
		//            children[] items[]
		// new setup:
		//                  root
		//             items[items[i]]
		//           nc1             nc2
		//        children[:i+1]    children[i+1:]
		//        items[:i]         items[i+1:]
		// Rearranging children + items:
		// 1. acquire cursors for each child of root & clear refs (pass 1)
		itemi := maxItems / 2
		item := baseNod.Items[itemi]
		childCursors := make([]*block.Cursor, len(baseNod.ChildrenRefs))
		for ci, child := range baseNod.ChildrenRefs {
			refID := baseNod.ChildRefId(ci)
			childCursor, err := rnCursor.FollowRef(
				refID,
				child,
			)
			if err != nil {
				return nil, err
			}
			childCursors[ci] = childCursor
			baseNodCursor.ClearRef(refID)
			baseNod.ChildrenRefs[ci] = nil
		}

		// 2. create 2 new blocks + cursors with FollowRef(nil) for the children
		nc1, err := baseNodCursor.FollowRef(baseNod.ChildRefId(0), nil)
		if err != nil {
			return nil, err
		}
		nc1Obj := t.b.newNode()
		nc1.SetBlock(nc1Obj)
		nc2, err := baseNodCursor.FollowRef(baseNod.ChildRefId(1), nil)
		if err != nil {
			return nil, err
		}
		nc2Obj := t.b.newNode()
		nc1.SetBlock(nc2Obj)

		// 3. assert the refs SetRef(nc1 -> children[:i+1]) SetRef(nc2 -> children[i+1:])
		for n := 0; n < itemi+1 && n < len(childCursors); n++ {
			nc1.SetRef(nc1Obj.ChildRefId(n), childCursors[n])
			nc1Obj.ChildrenRefs = append(nc1Obj.ChildrenRefs, nil)
		}
		bi := itemi + 1
		for n := 0; n+bi < len(childCursors); n++ {
			nc2.SetRef(nc2Obj.ChildRefId(n), childCursors[n+bi])
			nc2Obj.ChildrenRefs = append(nc2Obj.ChildrenRefs, nil)
		}

		// 4. root.items = [item], nc1.items = items[:i], nc2.items = items[i+1:]
		items := baseNod.Items
		baseNod.Items = []*Item{item}
		nc1Obj.Items = items[:itemi]
		nc2Obj.Items = append(nc2Obj.Items, items[itemi+1:]...)
	}

	out, err := t.insertToNode(baseNodCursor, baseNod, item, maxItems)
	if err != nil {
		return nil, err
	}
	if out == nil {
		rn.Length++
		rnCursor.SetBlock(rn)
	}
	return out.GetValue(), nil
}

// Delete deletes a key.
// This will not be committed until Commit is called.
// Not found should not return an error.
func (t *Tx) Delete(key []byte) error {
	_, _, err := t.Remove(key)
	return err
}

// Remove deletes a key and returns the previous value.
func (t *Tx) Remove(key []byte) (rval []byte, found bool, rerr error) {
	if len(key) == 0 {
		return nil, false, nil
	}
	if t.rn.GetLength() == 0 {
		return nil, false, nil
	}
	res, err := t.removeItemAtNode(
		key,
		t.baseNodCursor,
		t.baseNod,
		t.minItems(t.rn),
	)
	if err != nil {
		return nil, false, err
	}
	found = res != nil
	if found {
		rval = res.GetValue()
		t.rn.Length--
		t.rnCursor.SetBlock(t.rn)
	}
	return
}

// Exists checks if a key exists.
func (t *Tx) Exists(key []byte) (bool, error) {
	_, found, err := t.Get(key)
	return found, err
}

// insertToNode inserts an item as a child of this node, making sure no nodes in
// the subtree exceed maxItems items. If an equivalent item is found/replaced by
// insert, it will be returned.
func (t *Tx) insertToNode(
	c *block.Cursor,
	n *Node,
	item *Item,
	maxItems int,
) (*Item, error) {
	i, found := t.findInNode(n, item.Key)
	if found {
		out := n.Items[i]
		n.Items[i] = item
		c.SetBlock(n)
		c.SetPreWriteHook(preWriteNodeHook)
		return out, nil
	}

	if len(n.GetChildrenRefs()) == 0 {
		t.insertInNodeAtIdx(c, n, item, i)
		return nil, nil
	}

	cii := i
	ci, ciCursor, err := t.followChildRef(c, n, i)
	if err != nil {
		return nil, err
	}

	wasSplit, err := t.maybeSplitNodeChild(c, ciCursor, n, ci, i, maxItems)
	if err != nil {
		return nil, err
	}
	if wasSplit {
		inTree := n.Items[i]
		switch {
		case item.Less(inTree):
			// no change
		case inTree.Less(item):
			i++
		default:
			out := n.Items[i]
			n.Items[i] = item
			c.SetBlock(n)
			c.SetPreWriteHook(preWriteNodeHook)
			return out, nil
		}
	}

	if cii != i {
		ci, ciCursor, err = t.followChildRef(c, n, i)
		if err != nil {
			return nil, err
		}
	}

	return t.insertToNode(ciCursor, ci, item, maxItems)
}

// maybeSplitNodeChild checks if a child should be split, and if so splits it.
// Returns whether or not a split occurred
func (t *Tx) maybeSplitNodeChild(
	c1, c2 *block.Cursor,
	n, iChild *Node,
	i, maxItems int,
) (bool, error) {
	if len(iChild.GetItems()) < maxItems {
		return false, nil
	}

	// items after index (maxItems/2) are placed into second
	// item at index is returned
	// items before index are left in iChild
	item, second := t.splitNode(iChild, maxItems/2)
	t.insertInNodeAtIdx(c1, n, item, i)
	t.insertChildInNodeAtIdx(c1, n, second, i+1)

	c2.SetBlock(iChild)
	c2.SetPreWriteHook(preWriteNodeHook)
	secondCursor, err := c1.FollowRef(n.ChildRefId(i+1), nil)
	if err != nil {
		return false, err
	}
	secondCursor.SetBlock(second)
	secondCursor.SetPreWriteHook(preWriteNodeHook)

	return true, nil
}

// splitNode splits the given node at the given index. The current node shrinks.
// This function returns the item that existed at that index and a new node
// containing all items / children after it.
//
func (t *Tx) splitNode(n *Node, i int) (*Item, *Node) {
	item := n.Items[i]
	next := t.b.newNode()
	next.Items = append(next.Items, n.Items[i+1:]...)
	n.Items = n.Items[:i]

	if len(n.ChildrenRefs) > 0 {
		next.ChildrenRefs = append(next.ChildrenRefs, n.ChildrenRefs[i+1:]...)
		n.ChildrenRefs = n.ChildrenRefs[:i]
	}

	return item, next
}

// insertChildInNodeAtIdx inserts a child in a node at an index, pushing all
// subsequent values forward.
func (t *Tx) insertChildInNodeAtIdx(
	c *block.Cursor,
	n, child *Node, i int,
) {

	/*
					items []Item
					*s = append(*s, nil)
					if index < len(*s) {
		                // shift index onward forward by 1
						copy((*s)[index+1:], (*s)[index:])
					}
				    (*s)[index] = item
	*/

	s := n.ChildrenRefs
	// TODO: ??
	s = append(s, nil)
	if i < len(s) {
		copy(s[i+1:], s[i:])
	}
	s[i] = nil
	n.ChildrenRefs = s
}

// insertInNodeAtIdx inserts an item in a node at an index.
func (t *Tx) insertInNodeAtIdx(c *block.Cursor, n *Node, item *Item, i int) {
	s := n.Items
	s = append(s, nil)
	if i < len(s) {
		copy(s[i+1:], s[i:])
	}
	s[i] = item
	n.Items = s
	c.SetBlock(n)
	c.SetPreWriteHook(preWriteNodeHook)
}

// removeInNodeAtIdx removes an item in a node at an index.
func (t *Tx) removeInNodeAtIdx(n *Node, idx int) *Item {
	s := n.Items
	item := s[idx]
	copy(s[idx:], s[idx+1:])
	s[len(s)-1] = nil
	s = s[:len(s)-1]
	n.Items = s
	return item
}

// findInNode finds where an item should be inserted/replaced in a node.
func (t *Tx) findInNode(n *Node, key []byte) (index int, found bool) {
	s := n.Items
	i := sort.Search(len(s), func(i int) bool {
		return bytes.Compare(key, s[i].GetKey()) < 0
	})
	if i > 0 && bytes.Equal(s[i-1].GetKey(), key) {
		return i - 1, true
	}
	return i, false
}

// findKeyInNode finds an item by key in a node
func (t *Tx) findKeyInNode(n *Node, key []byte) (index int, found bool) {
	s := n.Items
	i := sort.Search(len(s), func(i int) bool {
		return bytes.Compare(key, s[i].GetKey()) < 0
	})
	if i > 0 && bytes.Equal(s[i-1].GetKey(), key) {
		return i - 1, true
	}
	return i, false
}

// followChildRef looks up the child at the index.
func (t *Tx) followChildRef(
	c *block.Cursor,
	n *Node,
	i int,
) (*Node, *block.Cursor, error) {
	nc, err := c.FollowRef(
		n.ChildRefId(i),
		n.GetChildrenRefs()[i],
	)
	if err != nil {
		return nil, nil, err
	}
	bki, err := nc.Unmarshal(t.b.newNodeBlock)
	if err != nil {
		return nil, nil, err
	}
	mn, _ := bki.(*Node)
	return mn, nc, nil
}

// maxItems is the max number of items to store in a node
func (t *Tx) maxItems(rn *Root) int {
	return degree*2 - 1
}

// minItems is the min number of items to store in a node
func (t *Tx) minItems(rn *Root) int {
	return degree - 1
}

// removeItemAtNode removes an item from a subtree rooted at node.
// key = "" -> remove max item
func (t *Tx) removeItemAtNode(
	key []byte,
	cursor *block.Cursor,
	n *Node,
	minItems int,
) (*Item, error) {
	var i int
	var found bool
	if len(key) != 0 {
		// find item
		i, found = t.findKeyInNode(n, key)
		if n.GetChildrenEmpty() {
			if found {
				item := t.removeInNodeAtIdx(n, i)
				cursor.SetBlock(n)
				cursor.SetPreWriteHook(preWriteNodeHook)
				return item, nil
			}
			return nil, nil
		}
	} else {
		if n.GetChildrenEmpty() {
			cursor.SetBlock(n)
			cursor.SetPreWriteHook(preWriteNodeHook)
			return popItem(&n.Items), nil
		}
		i = len(n.Items)
	}

	childNod, childCursor, err := t.followChildRef(cursor, n, i)
	if err != nil {
		return nil, err
	}
	if len(childNod.GetItems()) <= minItems {
		return t.growChildAndRemove(
			key,
			cursor,
			n,
			minItems,
			i,
			childNod,
			childCursor,
		)
	}
	if found {
		out := n.Items[i]
		ii, err := t.removeItemAtNode(nil, cursor, n, minItems)
		if err != nil {
			return nil, err
		}
		n.Items[i] = ii
		cursor.SetBlock(n)
		cursor.SetPreWriteHook(preWriteNodeHook)
		return out, nil
	}

	return t.removeItemAtNode(key, cursor, n, minItems)
}

// popItem pops an item from an item slice.
func popItem(items *[]*Item) *Item {
	idx := len(*items) - 1
	item := (*items)[idx]
	(*items)[idx] = nil
	(*items) = (*items)[:idx]
	return item
}

// popItemIdx pops an item from an index of an item slice.
func popItemIdx(s *[]*Item, index int) *Item {
	item := (*s)[index]
	copy((*s)[index:], (*s)[index+1:])
	(*s)[len(*s)-1] = nil
	*s = (*s)[:len(*s)-1]
	return item
}

// popChildNode removes and returns the last child from the slice.
func popChildNode(items *[]*cid.BlockRef) *cid.BlockRef {
	idx := len(*items) - 1
	return popChildIdx(items, idx)
}

// popChildIdx removes and returns a child from the slice.
func popChildIdx(items *[]*cid.BlockRef, idx int) *cid.BlockRef {
	item := (*items)[idx]
	(*items)[idx] = nil
	(*items) = (*items)[:idx]
	return item
}

// growChildAndRemove grows the child at index i to ensure that removing an item
// will maintain the minItems constraint.
func (t *Tx) growChildAndRemove(
	key []byte,
	cursor *block.Cursor,
	n *Node,
	minItems int,
	i int,
	child *Node,
	childCursor *block.Cursor,
) (*Item, error) {
	var err error
	var leftChild, rightChild *Node
	var leftChildCursor, rightChildCursor *block.Cursor

	if i > 0 {
		leftChild, leftChildCursor, err = t.followChildRef(cursor, n, i-1)
		if err != nil {
			return nil, err
		}
	}
	if i < len(n.GetItems()) {
		rightChild, rightChildCursor, err = t.followChildRef(cursor, n, i+1)
		if err != nil {
			return nil, err
		}
	}
	if i > 0 && leftChild != nil && len(leftChild.GetItems()) > minItems {
		// steal from left child
		si := popItem(&leftChild.Items)
		t.insertInNodeAtIdx(childCursor, child, n.Items[i-1], 0)
		n.Items[i-1] = si
		if len(leftChild.GetChildrenRefs()) > 0 {
			// pop last child from leftChild
			pcn := popChildNode(&leftChild.ChildrenRefs)
			// acquire the cursor to that
			pcnRefID := leftChild.ChildRefId(len(leftChild.ChildrenRefs))
			pcnCursor, err := leftChildCursor.FollowRef(
				pcnRefID,
				pcn,
			)
			if err != nil {
				return nil, err
			}
			// push the child ref
			leftChildCursor.ClearRef(pcnRefID)
			// set the link
			childCursor.SetRef(child.ChildRefId(len(child.ChildrenRefs)), pcnCursor)
			child.ChildrenRefs = append(child.ChildrenRefs, pcn)
		}
	} else if i < len(n.GetItems()) && rightChild != nil && len(rightChild.GetItems()) > minItems {
		// steal from right child
		si := popItemIdx(&rightChild.Items, 0)
		child.Items = append(child.Items, n.Items[i])
		n.Items[i] = si
		if len(rightChild.GetChildrenRefs()) > 0 {
			rightChildCursors := make([]*block.Cursor, len(rightChild.ChildrenRefs)-1)
			// shift all index cursors left by 1
			var crCursor *block.Cursor
			for i, cr := range rightChild.ChildrenRefs {
				refID := rightChild.ChildRefId(i)
				iCursor, err := rightChildCursor.FollowRef(
					refID,
					cr,
				)
				if err != nil {
					return nil, err
				}
				rightChildCursor.ClearRef(refID)
				if i == 0 {
					crCursor = iCursor
				} else {
					rightChildCursors[i-1] = iCursor
				}
			}
			cr := popChildIdx(&rightChild.ChildrenRefs, 0)
			// re-instate references
			for i, ref := range rightChildCursors {
				rightChildCursor.SetRef(rightChild.ChildRefId(i), ref)
			}

			// set reference
			childCursor.SetRef(child.ChildRefId(len(child.ChildrenRefs)), crCursor)
			child.ChildrenRefs = append(child.ChildrenRefs, cr)
		}
	} else {
		if i >= len(n.Items) {
			i--
		}
		// merge with right child
		// merge item
		mi := popItemIdx(&n.Items, i)
		// merge child
		// remove child at index i+1
		// shift cursors [i+2:] left
		mc, mcCursor, err := t.followChildRef(cursor, n, i+1)
		if err != nil {
			return nil, err
		}
		cursor.ClearRef(n.ChildRefId(i + 1))
		mcRef := popChildIdx(&n.ChildrenRefs, i+1)

		child.Items = append(child.Items, mi)
		child.Items = append(child.Items, mc.Items...)
		childCursor.SetRef(child.ChildRefId(len(child.ChildrenRefs)), mcCursor)
		child.ChildrenRefs = append(child.ChildrenRefs, mcRef)
		for ni := i + 1; ni < len(n.ChildrenRefs); ni++ {
			// original index was ni+1
			refID := child.ChildRefId(ni + 1)
			niCursor, err := cursor.FollowRef(refID, n.ChildrenRefs[ni])
			if err != nil {
				return nil, err
			}
			cursor.ClearRef(refID)
			cursor.SetRef(refID-1, niCursor)
		}
	}
	childCursor.SetBlock(child)
	childCursor.SetPreWriteHook(preWriteNodeHook)
	cursor.SetBlock(n)
	cursor.SetPreWriteHook(preWriteNodeHook)

	return t.removeItemAtNode(key, cursor, n, minItems)
}

// Less compares two items.
func (i *Item) Less(o *Item) bool {
	return bytes.Compare(i.GetKey(), o.GetKey()) == -1
}

// Commit commits the transaction to storage.
// Can return an error to indicate tx failure.
func (t *Tx) Commit(ctx context.Context) (cerr error) {
	t.commitOnce.Do(func() {
		if t.write {
			res, _, err := t.tx.Write()
			if err != nil || len(res) == 0 {
				cerr = err
			} else {
				rb := res[len(res)-1]
				br := rb.GetPutBlock().GetBlockCommon().GetBlockRef()
				nc := *t.b.rootCursor
				nc.SetRootRef(br)
				t.b.rootCursor = &nc
			}
			t.b.rmtx.Unlock()
		} else {
			t.b.rmtx.RUnlock()
		}
	})
	return
}

// Discard cancels the transaction.
// If called after Commit, does nothing.
// Cannot return an error.
// Can be called unlimited times.
func (t *Tx) Discard() {
	t.commitOnce.Do(func() {
		if t.write {
			t.b.rmtx.Unlock()
		} else {
			t.b.rmtx.RUnlock()
		}
	})
}

// preWriteNodeHook purges nil references
func preWriteNodeHook(b block.Block) error {
	nod := b.(*Node)
	/*
		if nod == nil {
			return nil
		}
	*/
	// sweep nil refs
	for i := 0; i < len(nod.ChildrenRefs); i++ {
		ref := nod.ChildrenRefs[i]
		if ref == nil {
			nod.ChildrenRefs[i] = nod.ChildrenRefs[len(nod.ChildrenRefs)-1]
			nod.ChildrenRefs[len(nod.ChildrenRefs)-1] = nil
			nod.ChildrenRefs = nod.ChildrenRefs[:len(nod.ChildrenRefs)-1]
			i--
		}
	}
	return nil
}

// iterate iterates through the btree.
func (t *Tx) iterate(
	bc *block.Cursor,
	n *Node,
	ascending, hit bool,
	startKey, stopKey []byte,
	inclusiveRange bool,
	itemCallback func(key []byte) (ctnu bool, err error),
) (bool, bool, error) {
	var ok bool
	iterateChild := func(i int) error {
		childNode, childNodeCursor, err := t.followChildRef(
			bc,
			n,
			i,
		)
		if err != nil {
			return err
		}
		hit, ok, err = t.iterate(
			childNodeCursor,
			childNode,
			ascending,
			hit,
			startKey,
			stopKey,
			inclusiveRange,
			itemCallback,
		)
		return err
	}
	if ascending {
		for i := 0; i < len(n.GetItems()); i++ {
			itemKey := n.Items[i].GetKey()
			if len(startKey) != 0 && bytes.Compare(itemKey, startKey) == -1 {
				continue
			}
			if !n.GetChildrenEmpty() && i < len(n.ChildrenRefs) {
				if err := iterateChild(i); err != nil {
					return false, false, err
				}
				if !ok {
					return hit, false, nil
				}
			}
			if !inclusiveRange &&
				!hit &&
				len(startKey) != 0 &&
				bytes.Compare(itemKey, startKey) >= 0 {
				hit = true
				continue
			}
			hit = true
			if len(stopKey) != 0 && bytes.Compare(itemKey, stopKey) >= 0 {
				return hit, false, nil
			}
			ctnu, err := itemCallback(itemKey)
			if !ctnu || err != nil {
				return hit, false, err
			}
		}
		if !n.GetChildrenEmpty() {
			if err := iterateChild(len(n.GetChildrenRefs()) - 1); err != nil {
				return false, false, err
			}
			if !ok {
				return hit, false, nil
			}
		}
	} else {
		for i := len(n.GetItems()) - 1; i >= 0; i-- {
			itemKey := n.GetItems()[i].GetKey()
			if len(startKey) != 0 && bytes.Compare(itemKey, startKey) >= 0 {
				if !inclusiveRange ||
					hit ||
					bytes.Compare(startKey, itemKey) == -1 {
					continue
				}
			}
			if !n.GetChildrenEmpty() {
				if err := iterateChild(i + 1); err != nil {
					return false, false, err
				}
			}
			if len(stopKey) != 0 && bytes.Compare(stopKey, itemKey) >= 0 {
				return hit, false, nil
			}
			hit = true
			ctnu, err := itemCallback(itemKey)
			if !ctnu || err != nil {
				return hit, false, err
			}
		}
		if !n.GetChildrenEmpty() {
			if err := iterateChild(0); err != nil || !ok {
				return hit, false, err
			}
		}
	}
	return hit, true, nil
}

// _ is a type assertion
var _ kvtx.Tx = ((*Tx)(nil))
