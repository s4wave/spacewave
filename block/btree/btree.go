package btree

// TODO: push to freeList

import (
	"errors"
	"sort"
	"sync"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/object"
	"github.com/aperturerobotics/hydra/cid"
)

// maxNodeChildren is the maximum number children nodes of a node.
const maxNodeChildren = 16

// BTree is an implementation of a object-store backed BTree.
// The key is a string, and the value is a object reference.
type BTree struct {
	// mtx guards the tree
	mtx sync.Mutex
	// degree is the degree
	degree int

	rootCursor *object.Cursor
	freeList   sync.Pool
}

// NewBTree builds a new btree, writing state to the cursor.
// Any errors writing initial state will be returned.
// Degree defaults to 3.
func NewBTree(
	rootCursor *object.Cursor,
	degree int,
) (*BTree, error) {
	if degree == 0 {
		degree = 3
	}
	blockTx, blockCursor := rootCursor.BuildTransaction(nil)
	rootNod := &Root{Degree: uint32(degree)}

	blockCursor.SetBlock(rootNod)
	bevents, _, err := blockTx.Write()
	if err != nil {
		return nil, err
	}
	rootRef := bevents[len(bevents)-1].
		GetPutBlock().
		GetBlockCommon().
		GetBlockRef()
	rootCursor.SetRootRef(rootRef)

	return &BTree{
		rootCursor: rootCursor,
		freeList:   sync.Pool{New: func() interface{} { return &Node{} }},
	}, nil
}

// LoadBTree loads a btree by following a root object cursor pointing to the tree.
func LoadBTree(
	rootCursor *object.Cursor,
) (*BTree, error) {
	blk, err := rootCursor.Unmarshal(func() block.Block {
		return &Root{}
	})
	if err != nil {
		return nil, err
	}
	rootNod := blk.(*Root)

	// Follow root node reference.
	baseNodRef := rootNod.GetRootNodeRef()
	if baseNodRef.GetEmpty() {
		return nil, errors.New("root node ref was empty")
	}
	if err := baseNodRef.Validate(); err != nil {
		return nil, err
	}

	_, blkCursor := rootCursor.BuildTransaction(nil)
	blkCursor, err = blkCursor.FollowRef(1, baseNodRef)
	if err != nil {
		return nil, err
	}

	return &BTree{
		rootCursor: rootCursor,
		freeList:   sync.Pool{New: func() interface{} { return &Node{} }},
	}, nil
}

// Len returns the number of items in the tree.
func (b *BTree) Len() (int, error) {
	b.mtx.Lock()
	defer b.mtx.Unlock()

	r, _, _, _, _, err := b.fetchRoot()
	if err != nil {
		return 0, err
	}
	return int(r.GetLength()), nil
}

// GetRootNodeRef returns the reference to the root node.
func (b *BTree) GetRootNodeRef() *object.ObjectRef {
	b.mtx.Lock()
	defer b.mtx.Unlock()

	return b.rootCursor.GetRef()
}

// Get looks up an item by key.
func (b *BTree) Get(key string) (*object.ObjectRef, bool, error) {
	if key == "" {
		return nil, false, nil
	}

	b.mtx.Lock()
	defer b.mtx.Unlock()

	rn, baseNod, _, _, baseNodCursor, err := b.fetchRoot()
	if err != nil {
		return nil, false, err
	}
	if rn.GetLength() == 0 || rn.GetRootNodeRef().GetEmpty() {
		return nil, false, nil
	}

	return b.getFromNode(baseNodCursor, baseNod, key)
}

// getFromNode gets a key from a subtree.
func (b *BTree) getFromNode(
	cursor *block.Cursor,
	nod *Node,
	key string,
) (*object.ObjectRef, bool, error) {
	i, found := b.findInNode(nod, key)
	if found {
		return nod.GetItems()[i].GetRef(), true, nil
	} else if !nod.GetChildrenEmpty() {
		// follow ref at i
		ref := nod.ChildrenRefs[i]
		cc, err := cursor.FollowRef(nod.ChildRefId(i), ref)
		if err != nil {
			return nil, false, err
		}
		ccObj, err := cc.Unmarshal(b.newNodeBlock)
		if err != nil {
			return nil, false, err
		}
		cb, _ := ccObj.(*Node)
		if cb == nil {
			return nil, false, nil
		}
		return b.getFromNode(cc, cb, key)
	}
	return nil, false, nil
}

// Delete removes the item, returning it, or nil if it doesn't exist.
func (b *BTree) Delete(key string) (objr *object.ObjectRef, found bool, rerr error) {
	if key == "" {
		return nil, false, nil
	}

	b.mtx.Lock()
	defer b.mtx.Unlock()

	rn, rnn, tx, cursor, rnCursor, err := b.fetchRoot()
	if err != nil {
		return nil, false, err
	}
	defer b.finalizeTransaction(&rerr, tx)
	if rn.GetLength() == 0 {
		return nil, false, nil
	}

	res, err := b.removeItemAtNode(key, rnCursor, rnn, b.minItems(rn))
	if err != nil {
		return nil, false, err
	}
	found = res != nil
	if found {
		objr = res.GetRef()
		rn.Length--
		cursor.SetBlock(rn)
	}
	return objr, found, nil
}

// ReplaceOrInsert replaces or inserts an item, if replacing, returns the value.
func (b *BTree) ReplaceOrInsert(
	key string,
	val *object.ObjectRef,
) (rref *object.ObjectRef, rerr error) {
	if key == "" {
		return nil, nil
	}

	b.mtx.Lock()
	defer b.mtx.Unlock()

	rn, _, tx, cursor, rnCursor, err := b.fetchRoot()
	if err != nil {
		return nil, err
	}
	defer b.finalizeTransaction(&rerr, tx)

	item := &Item{Key: key, Ref: val}
	if rn.Length == 0 {
		nnod := &Node{Leaf: true, Items: []*Item{item}}
		rnCursor.SetBlock(nnod)
		rnCursor.SetPreWriteHook(preWriteNodeHook)
		rn.Length++
		return nil, nil
	}

	rootNodBlk, err := rnCursor.Unmarshal(b.newNodeBlock)
	if err != nil {
		return nil, err
	}

	var rootNod *Node
	if rootNodBlk != nil {
		rootNod, _ = rootNodBlk.(*Node)
	}
	if rootNod == nil {
		rootNod = b.newNode()
	}

	maxItems := b.maxItems(rn)
	// Need to take special care to re-knit the block graph here.
	if len(rootNod.Items) >= maxItems {
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
		item := rootNod.Items[itemi]
		childCursors := make([]*block.Cursor, len(rootNod.ChildrenRefs))
		for ci, child := range rootNod.ChildrenRefs {
			refID := rootNod.ChildRefId(ci)
			childCursor, err := rnCursor.FollowRef(
				refID,
				child,
			)
			if err != nil {
				return nil, err
			}
			childCursors[ci] = childCursor
			rnCursor.ClearRef(refID)
			rootNod.ChildrenRefs[ci] = nil
		}

		// 2. create 2 new blocks + cursors with FollowRef(nil) for the children
		nc1, err := rnCursor.FollowRef(rootNod.ChildRefId(0), nil)
		if err != nil {
			return nil, err
		}
		nc1Obj := b.newNode()
		nc1.SetBlock(nc1Obj)
		nc2, err := rnCursor.FollowRef(rootNod.ChildRefId(1), nil)
		if err != nil {
			return nil, err
		}
		nc2Obj := b.newNode()
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
		items := rootNod.Items
		rootNod.Items = []*Item{item}
		nc1Obj.Items = items[:itemi]
		nc2Obj.Items = append(nc2Obj.Items, items[itemi+1:]...)
	}

	out, err := b.insertToNode(rnCursor, rootNod, item, maxItems)
	if err != nil {
		return nil, err
	}

	if out == nil {
		rn.Length++
		cursor.SetBlock(rn)
	}

	return out.GetRef(), nil
}

// finalizeTransaction finalizes a write transaction.
func (b *BTree) finalizeTransaction(rerr *error, tx *block.Transaction) {
	if *rerr == nil {
		res, _, err := tx.Write()
		if err != nil || len(res) == 0 {
			*rerr = err
			return
		}
		rb := res[len(res)-1]
		br := rb.GetPutBlock().GetBlockCommon().GetBlockRef()
		b.rootCursor.SetRootRef(br)
	}
}

// fetchRoot fetches the root block.
func (b *BTree) fetchRoot() (
	r *Root,
	rn *Node,
	btx *block.Transaction,
	bcs, rnCursor *block.Cursor,
	err error,
) {
	btx, bcs = b.rootCursor.BuildTransaction(nil)
	bi, biErr := bcs.Unmarshal(func() block.Block {
		return &Root{}
	})
	if biErr != nil {
		return nil, nil, nil, nil, nil, biErr
	}
	if bi == nil {
		return nil, nil, nil, nil, nil, errors.New("root block not found")
	}
	r = bi.(*Root)
	rnRef := r.GetRootNodeRef()
	rnCursor, err = bcs.FollowRef(1, rnRef)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	if r.GetLength() != 0 {
		rni, err := rnCursor.Unmarshal(b.newNodeBlock)
		if err != nil {
			return nil, nil, nil, nil, nil, err
		}
		rnn, ok := rni.(*Node)
		if ok {
			rn = rnn
		}
	}
	return
}

// insertToNode inserts an item as a child of this node, making sure no nodes in
// the subtree exceed maxItems items. If an equivalent item is
// found/replaced by insert, it will be returned.
func (b *BTree) insertToNode(
	c *block.Cursor,
	n *Node,
	item *Item,
	maxItems int,
) (*Item, error) {
	i, found := b.findInNode(n, item.Key)
	if found {
		out := n.Items[i]
		n.Items[i] = item
		return out, nil
	}

	if len(n.GetChildrenRefs()) == 0 {
		b.insertInNodeAtIdx(c, n, item, i)
		return nil, nil
	}

	cii := i
	ci, ciCursor, err := b.followChildRef(c, n, i)
	if err != nil {
		return nil, err
	}

	wasSplit, err := b.maybeSplitNodeChild(c, ciCursor, n, ci, i, maxItems)
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
			return out, nil
		}
	}

	if cii != i {
		ci, ciCursor, err = b.followChildRef(c, n, i)
		if err != nil {
			return nil, err
		}
	}

	return b.insertToNode(ciCursor, ci, item, maxItems)
}

// maybeSplitNodeChild checks if a child should be split, and if so splits it.
// Returns whether or not a split occurred
func (b *BTree) maybeSplitNodeChild(
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
	item, second := b.splitNode(iChild, maxItems/2)
	b.insertInNodeAtIdx(c1, n, item, i)
	b.insertChildInNodeAtIdx(c1, n, second, i+1)

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
func (b *BTree) splitNode(n *Node, i int) (*Item, *Node) {
	item := n.Items[i]
	next := b.newNode()
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
func (b *BTree) insertChildInNodeAtIdx(
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
func (b *BTree) insertInNodeAtIdx(c *block.Cursor, n *Node, item *Item, i int) {
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
func (b *BTree) removeInNodeAtIdx(n *Node, idx int) *Item {
	s := n.Items
	item := s[idx]
	copy(s[idx:], s[idx+1:])
	s[len(s)-1] = nil
	s = s[:len(s)-1]
	n.Items = s
	return item
}

// findInNode finds where an item should be inserted/replaced in a node.
func (b *BTree) findInNode(n *Node, key string) (index int, found bool) {
	s := n.Items
	i := sort.Search(len(s), func(i int) bool {
		return key < s[i].GetKey()
	})
	if i > 0 && s[i-1].GetKey() == key {
		return i - 1, true
	}
	return i, false
}

// findKeyInNode finds an item by key in a node
func (b *BTree) findKeyInNode(n *Node, key string) (index int, found bool) {
	s := n.Items
	i := sort.Search(len(s), func(i int) bool {
		return key < s[i].GetKey()
	})
	if i > 0 && s[i-1].GetKey() == key {
		return i - 1, true
	}
	return i, false
}

// followChildRef looks up the child at the index.
func (b *BTree) followChildRef(
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
	bki, err := nc.Unmarshal(b.newNodeBlock)
	if err != nil {
		return nil, nil, err
	}
	mn, _ := bki.(*Node)
	return mn, nc, nil
}

// getDegree gets the degree from the root
func (t *BTree) getDegree(rn *Root) int {
	degree := int(rn.GetDegree())
	if degree == 0 {
		degree = 3
	}
	return degree
}

// maxItems is the max number of items to store in a node
func (t *BTree) maxItems(rn *Root) int {
	return t.getDegree(rn)*2 - 1
}

// minItems is the min number of items to store in a node
func (t *BTree) minItems(rn *Root) int {
	return t.getDegree(rn) - 1
}

// removeItemAtNode removes an item from a subtree rooted at node.
// key = "" -> remove max item
func (t *BTree) removeItemAtNode(
	key string,
	cursor *block.Cursor,
	n *Node,
	minItems int,
) (*Item, error) {
	var i int
	var found bool
	if key != "" {
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
		cursor.SetBlock(n)
		cursor.SetPreWriteHook(preWriteNodeHook)
		ii, err := t.removeItemAtNode("", cursor, n, minItems)
		if err != nil {
			return nil, err
		}
		n.Items[i] = ii
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
func (b *BTree) growChildAndRemove(
	key string,
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
		leftChild, leftChildCursor, err = b.followChildRef(cursor, n, i-1)
		if err != nil {
			return nil, err
		}
	}
	if i < len(n.GetItems()) {
		rightChild, rightChildCursor, err = b.followChildRef(cursor, n, i+1)
		if err != nil {
			return nil, err
		}
	}
	if i > 0 && leftChild != nil && len(leftChild.GetItems()) > minItems {
		// steal from left child
		si := popItem(&leftChild.Items)
		b.insertInNodeAtIdx(childCursor, child, n.Items[i-1], 0)
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
		mc, mcCursor, err := b.followChildRef(cursor, n, i+1)
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

	return b.removeItemAtNode(key, cursor, n, minItems)
}

// iterate iterates through the btree.
func (b *BTree) iterate(
	bc *block.Cursor,
	n *Node,
	ascending, hit bool,
	startKey, stopKey string,
	inclusiveRange bool,
	itemCallback func(key string) (ctnu bool, err error),
) (bool, bool, error) {
	var ok bool
	iterateChild := func(i int) error {
		childNode, childNodeCursor, err := b.followChildRef(
			bc,
			n,
			i,
		)
		if err != nil {
			return err
		}
		hit, ok, err = b.iterate(
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
			if startKey != "" && itemKey < startKey {
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
			if !inclusiveRange && !hit && startKey != "" && itemKey >= startKey {
				hit = true
				continue
			}
			hit = true
			if stopKey != "" && itemKey >= stopKey {
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
			if startKey != "" && itemKey >= startKey {
				if !inclusiveRange || hit || startKey < itemKey {
					continue
				}
			}
			if !n.GetChildrenEmpty() {
				if err := iterateChild(i + 1); err != nil {
					return false, false, err
				}
			}
			if stopKey != "" && stopKey >= itemKey {
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

// preWriteNodeHook purges nil references
func preWriteNodeHook(b block.Block) error {
	nod := b.(*Node)
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

// Less compares two items.
func (i *Item) Less(o *Item) bool {
	return i.GetKey() < o.GetKey()
}

// newNode builds a new node from the free list.
func (b *BTree) newNode() *Node {
	fget := b.freeList.Get()
	var n *Node
	if fget != nil {
		n = fget.(*Node)
		n.N = 0
		n.Leaf = false
		if n.ChildrenRefs != nil {
			n.ChildrenRefs = n.ChildrenRefs[:0]
		}
		if n.Items != nil {
			n.Items = n.Items[:0]
		}
		n.Reset()
	} else {
		n = &Node{}
	}
	return n
}

// newNodeBlock builds a new node block from the free list.
func (b *BTree) newNodeBlock() block.Block {
	return b.newNode()
}
