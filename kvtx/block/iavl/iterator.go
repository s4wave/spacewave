package kvtx_block_iavl

import (
	"bytes"
	"context"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/kvtx"
)

// Iterator implements iteration by traversing the block graph.
type Iterator struct {
	// ctx is the context for operations
	ctx context.Context
	// t is the transaction
	t *Tx
	// err holds any error that occurred
	err error
	// rev indicates reverse/descending order
	rev bool
	// prefix is the key prefix constraint
	prefix []byte

	// key is the current key
	key []byte
	// val is the cached value
	val []byte
	// nodeCursor points to current node location
	nodeCursor *block.Cursor
	// node is the current node
	node *Node
	// hasVal indicates if val is cached
	hasVal bool

	// stack tracks traversal
	stack []stackEntry
}

// stackEntry represents a node in the traversal stack
type stackEntry struct {
	node    *Node
	cursor  *block.Cursor
	visited bool // if true we visited the "first child" (left for in-order, right for reverse)
}

// NewIterator constructs a new iterator. Initial key fetch is deferred to the
// first Next() call. Reverse is equivalent to "descending" order.
//
// Note: sort is ignored, the iavl iterator is always sorted.
func NewIterator(ctx context.Context, t *Tx, prefix []byte, sort, reverse bool) *Iterator {
	it := &Iterator{
		ctx:    ctx,
		t:      t,
		rev:    reverse,
		prefix: prefix,
	}
	it.stack = make([]stackEntry, 1, 18)
	it.stack[0] = stackEntry{node: it.t.root, cursor: it.t.bcs}
	return it
}

// Err returns any error that has closed the iterator.
// May return context.Canceled if closed.
func (i *Iterator) Err() error {
	return i.err
}

// Valid returns if the iterator points to a valid entry.
//
// If err is set, returns false.
func (i *Iterator) Valid() bool {
	return len(i.Key()) != 0
}

// Key returns the current entry key, or nil if not valid.
func (i *Iterator) Key() []byte {
	if i.err != nil {
		return nil
	}
	return i.key
}

// Value returns the current entry value, or nil if not valid.
//
// May cache the value between calls, copy if modifying.
func (i *Iterator) Value() ([]byte, error) {
	if !i.Valid() {
		return nil, i.err
	}
	if err := i.checkContext(); err != nil {
		return nil, err
	}
	if !i.hasVal {
		val, err := i.t.nodeToValue(i.ctx, i.nodeCursor, i.node)
		if err != nil {
			return nil, err
		}
		i.val = val
		i.hasVal = true
	}
	return i.val, nil
}

// ValueCopy copies the value to the given byte slice and returns it.
// If the slice is not big enough (cap), it must create a new one and return it.
// May use the value cached from Value() call as the source of the data.
// May return nil if !Valid().
func (i *Iterator) ValueCopy(buf []byte) ([]byte, error) {
	val, err := i.Value()
	if err != nil {
		return nil, err
	}
	if len(val) == 0 {
		return buf[:0], nil
	}
	return append(buf[:0], val...), nil
}

// ValueCursor returns a cursor located at the "value" sub-block.
// Returns nil if the iterator is not at a valid location.
func (i *Iterator) ValueCursor() *block.Cursor {
	if !i.Valid() {
		return nil
	}
	if err := i.checkContext(); err != nil {
		return nil
	}
	valueCursor, _ := i.node.FollowValue(i.nodeCursor)
	return valueCursor
}

// Next advances to the next entry and returns Valid.
func (i *Iterator) Next() bool {
	if err := i.checkContext(); err != nil {
		return false
	}

	// XXX: possible optimization: skip sub-trees that do not match prefix

	i.resetState()
	for len(i.stack) != 0 {
		lastIdx := len(i.stack) - 1
		entry := &i.stack[lastIdx]

		if entry.node.IsLeaf() {
			i.stack = i.stack[:lastIdx]
			if i.setCurrentNode(entry.node, entry.cursor) {
				return true
			}
			continue
		}

		if !entry.visited {
			// visit first child
			var firstNode *Node
			var firstCursor *block.Cursor
			var err error
			if i.rev {
				firstNode, firstCursor, err = entry.node.FollowRight(i.ctx, entry.cursor)
			} else {
				firstNode, firstCursor, err = entry.node.FollowLeft(i.ctx, entry.cursor)
			}
			if err != nil {
				_ = i.setError(err)
				return false
			}
			if firstNode != nil {
				i.stack = append(i.stack, stackEntry{
					node:   firstNode,
					cursor: firstCursor,
				})
			}
			entry.visited = true
		} else {
			// dequeue and visit second child
			i.stack = i.stack[:lastIdx]
			var secondNode *Node
			var secondCursor *block.Cursor
			var err error
			if i.rev {
				secondNode, secondCursor, err = entry.node.FollowLeft(i.ctx, entry.cursor)
			} else {
				secondNode, secondCursor, err = entry.node.FollowRight(i.ctx, entry.cursor)
			}
			if err != nil {
				_ = i.setError(err)
				return false
			}
			if secondNode != nil {
				i.stack = append(i.stack, stackEntry{
					node:   secondNode,
					cursor: secondCursor,
				})
			}
		}
	}

	return false
}

// Seek moves the iterator to the first key >= the provided key (or <= in reverse mode).
// Pass nil to seek to the beginning (or end if reversed).
// It is not necessary to call Next() after seek.
func (i *Iterator) Seek(k []byte) error {
	if err := i.checkContext(); err != nil {
		return err
	}

	// reset the state
	i.resetState()
	i.stack = i.stack[:1]
	i.stack[0] = stackEntry{node: i.t.root, cursor: i.t.bcs}

	if len(k) == 0 {
		if i.rev {
			return i.seekToEnd()
		}
		return i.seekToBeginning()
	}

	for len(i.stack) > 0 {
		lastIdx := len(i.stack) - 1
		entry := &i.stack[lastIdx]

		if entry.node.IsLeaf() {
			i.stack = i.stack[:lastIdx]
			cmp := bytes.Compare(entry.node.GetKey(), k)
			if (!i.rev && cmp >= 0) || (i.rev && cmp <= 0) {
				if i.setCurrentNode(entry.node, entry.cursor) {
					return nil
				}
			}
			continue
		}

		// The Seek logic is equivalent to calling Next() until we find the target key.
		//
		// However: we can optimize this by skipping subtrees that we know cannot contain the target.
		//
		// In forward iteration, we are looking for key >= k
		// In reverse iteration, we are looking for key <= k.
		//
		// AVL trees have the properties:
		//  - keys left of a node are less than that node
		//  - keys right of a node are greater than or equal to that node
		//
		// Therefore, we know that if we are seeking to key k:
		// - In forward mode (looking for >= k):
		//   - If k > node.key: we can skip the left subtree
		//   - If k <= node.key: we must check both subtrees
		// - In reverse mode (looking for <= k):
		//   - If k < node.key: we can skip the right subtree
		//   - If k >= node.key: we must check both subtrees

		// Compare current node's key with search key to determine which subtrees to visit
		cmp := bytes.Compare(entry.node.GetKey(), k)

		if !entry.visited {
			var shouldVisitFirst bool
			if i.rev {
				// In reverse mode (looking for <= k):
				// - If k < node.key (cmp > 0): skip right subtree
				// - If k >= node.key (cmp <= 0): check both subtrees
				shouldVisitFirst = cmp <= 0
			} else {
				// In forward mode (looking for >= k):
				// - If k > node.key (cmp < 0): skip left subtree
				// - If k <= node.key (cmp >= 0): check both subtrees
				shouldVisitFirst = cmp >= 0
			}

			if shouldVisitFirst {
				var firstNode *Node
				var firstCursor *block.Cursor
				var err error
				if i.rev {
					firstNode, firstCursor, err = entry.node.FollowRight(i.ctx, entry.cursor)
				} else {
					firstNode, firstCursor, err = entry.node.FollowLeft(i.ctx, entry.cursor)
				}
				if err != nil {
					return i.setError(err)
				}
				if firstNode != nil {
					i.stack = append(i.stack, stackEntry{
						node:   firstNode,
						cursor: firstCursor,
					})
				}
			}
			entry.visited = true
		} else {
			// dequeue and visit second child
			i.stack = i.stack[:lastIdx]
			var shouldVisitSecond bool
			if i.rev {
				// In reverse mode:
				// - Always check left subtree if we checked right
				shouldVisitSecond = true
			} else {
				// In forward mode:
				// - Always check right subtree if we checked left
				shouldVisitSecond = true
			}

			if shouldVisitSecond {
				var secondNode *Node
				var secondCursor *block.Cursor
				var err error
				if i.rev {
					secondNode, secondCursor, err = entry.node.FollowLeft(i.ctx, entry.cursor)
				} else {
					secondNode, secondCursor, err = entry.node.FollowRight(i.ctx, entry.cursor)
				}
				if err != nil {
					return i.setError(err)
				}
				if secondNode != nil {
					i.stack = append(i.stack, stackEntry{
						node:   secondNode,
						cursor: secondCursor,
					})
				}
			}
		}
	}

	return nil
}

// Close closes the iterator.
// Note: it is not necessary to close all iterators before Discard().
func (i *Iterator) Close() {
	i.err = context.Canceled
	i.resetState()
	i.stack = nil
}

// setCurrentNode sets the current node state from a stack entry
func (i *Iterator) setCurrentNode(node *Node, cursor *block.Cursor) bool {
	key := node.GetKey()
	if !i.matchesPrefix(key) {
		return false
	}
	i.key = key
	i.node = node
	i.nodeCursor = cursor
	return true
}

// setError sets the error state and marks iterator as out of bounds
func (i *Iterator) setError(err error) error {
	if i.err != nil {
		return i.err
	}
	i.err = err
	return err
}

// checkContext checks if context is canceled and sets error state if it is
func (i *Iterator) checkContext() error {
	if i.ctx.Err() != nil {
		return i.setError(context.Canceled)
	}
	return nil
}

// resetState resets the iterator's state variables
func (i *Iterator) resetState() {
	i.key = nil
	i.val = nil
	i.nodeCursor = nil
	i.node = nil
	i.hasVal = false
}

// seekToEnd moves the iterator to the last key in the tree
func (i *Iterator) seekToEnd() error {
	for len(i.stack) > 0 {
		lastIdx := len(i.stack) - 1
		entry := &i.stack[lastIdx]

		if entry.node.IsLeaf() {
			i.stack = i.stack[:lastIdx]
			if i.setCurrentNode(entry.node, entry.cursor) {
				return nil
			}
			continue
		}

		if !entry.visited {
			// visit right child first in reverse mode
			rightNode, rightCursor, err := entry.node.FollowRight(i.ctx, entry.cursor)
			if err != nil {
				return i.setError(err)
			}
			if rightNode != nil {
				i.stack = append(i.stack, stackEntry{
					node:   rightNode,
					cursor: rightCursor,
				})
			}
			entry.visited = true
		} else {
			// dequeue and visit left child
			i.stack = i.stack[:lastIdx]
			leftNode, leftCursor, err := entry.node.FollowLeft(i.ctx, entry.cursor)
			if err != nil {
				return i.setError(err)
			}
			if leftNode != nil {
				i.stack = append(i.stack, stackEntry{
					node:   leftNode,
					cursor: leftCursor,
				})
			}
		}
	}
	return nil
}

// seekToBeginning moves the iterator to the first key in the tree
func (i *Iterator) seekToBeginning() error {
	for len(i.stack) > 0 {
		lastIdx := len(i.stack) - 1
		entry := &i.stack[lastIdx]

		if entry.node.IsLeaf() {
			i.stack = i.stack[:lastIdx]
			if i.setCurrentNode(entry.node, entry.cursor) {
				return nil
			}
			continue
		}

		if !entry.visited {
			// visit left child first in forward mode
			leftNode, leftCursor, err := entry.node.FollowLeft(i.ctx, entry.cursor)
			if err != nil {
				return i.setError(err)
			}
			if leftNode != nil {
				i.stack = append(i.stack, stackEntry{
					node:   leftNode,
					cursor: leftCursor,
				})
			}
			entry.visited = true
		} else {
			// dequeue and visit right child
			i.stack = i.stack[:lastIdx]
			rightNode, rightCursor, err := entry.node.FollowRight(i.ctx, entry.cursor)
			if err != nil {
				return i.setError(err)
			}
			if rightNode != nil {
				i.stack = append(i.stack, stackEntry{
					node:   rightNode,
					cursor: rightCursor,
				})
			}
		}
	}
	return nil
}

// matchesPrefix checks if a key matches the iterator's prefix constraint
func (i *Iterator) matchesPrefix(key []byte) bool {
	return len(key) > 0 && (len(i.prefix) == 0 || bytes.HasPrefix(key, i.prefix))
}

// _ is a type assertion
var _ kvtx.Iterator = ((*Iterator)(nil))
