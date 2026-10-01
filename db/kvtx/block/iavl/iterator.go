package kvtx_block_iavl

import (
	"bytes"
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
)

// Iterator implements iteration by traversing the block graph.
type Iterator struct {
	// ctx is the context for operations.
	ctx context.Context
	// t is the transaction.
	t *Tx
	// err holds any error that occurred.
	err error
	// rev indicates descending order.
	rev bool
	// prefix is the key prefix constraint.
	prefix []byte
	// prefixEnd is the exclusive upper bound for prefix, when one exists.
	prefixEnd []byte

	// key is the current key.
	key []byte
	// val is the cached value.
	val []byte
	// nodeCursor points to the current node location.
	nodeCursor *block.Cursor
	// node is the current node.
	node *Node
	// hasVal indicates if val is cached.
	hasVal bool
	// positioned indicates Next or Seek has placed the traversal.
	positioned bool

	// stack tracks traversal.
	stack []stackEntry
}

// stackEntry represents a node in the traversal stack.
type stackEntry struct {
	// node is the reached tree node.
	node *Node
	// cursor locates the node in the block graph.
	cursor *block.Cursor
	// visited records descent into the left child, or the right child in reverse.
	visited bool
}

// NewIterator constructs a new iterator. Initial key fetch is deferred to the
// first Next() call. Reverse is equivalent to "descending" order.
//
// Note: sort is ignored, the iavl iterator is always sorted.
func NewIterator(ctx context.Context, t *Tx, prefix []byte, sort, reverse bool) *Iterator {
	// Retain the transaction and query bounds without fetching the first key.
	it := &Iterator{
		ctx:       ctx,
		t:         t,
		rev:       reverse,
		prefix:    prefix,
		prefixEnd: nil,
	}

	// A prefix successor supplies the exclusive forward bound when one exists.
	if len(prefix) != 0 {
		if end, ok := kvtx.PrefixSuccessor(prefix); ok {
			it.prefixEnd = end
		}
	}

	// Seed the traversal with the embedded root and room for the usual tree height.
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
	// Invalid positions and cancelled operations cannot fetch a value.
	if !i.Valid() {
		return nil, i.err
	}
	if err := i.checkContext(); err != nil {
		return nil, err
	}

	// Resolve the current leaf's value only on demand and cache successful reads.
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
	// Only a valid uncancelled position can supply a value cursor.
	if !i.Valid() {
		return nil
	}
	if err := i.checkContext(); err != nil {
		return nil
	}

	// Follow the leaf's value representation without fetching its payload.
	valueCursor, _ := i.node.FollowValue(i.nodeCursor)
	return valueCursor
}

// Next advances to the next entry and returns Valid.
func (i *Iterator) Next() bool {
	// Stop once the context ends.
	if err := i.checkContext(); err != nil {
		return false
	}

	// Start a prefixed scan at the prefix bound instead of the tree's first key.
	if !i.positioned && len(i.prefix) != 0 {
		if err := i.Seek(nil); err != nil {
			return false
		}
		return i.Valid()
	}

	// Walk in order from the current traversal to the next matching leaf.
	i.positioned = true
	i.resetState()
	for len(i.stack) != 0 {
		// Inspect the next traversal frame before following or exposing its node.
		lastIdx := len(i.stack) - 1
		entry := &i.stack[lastIdx]

		// Pop a leaf before exposing it, leaving the remaining traversal ready to advance.
		if entry.node.IsLeaf() {
			i.stack = i.stack[:lastIdx]
			if i.setCurrentNode(entry.node, entry.cursor) {
				return true
			}
			continue
		}

		// Visit the directional first child once, then continue through the second child.
		if !entry.visited {
			// Mark the parent before append can move the traversal stack.
			entry.visited = true

			// Load the first child before adding its traversal frame.
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
		} else {
			// Pop the completed parent before loading its second child.
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
	// Stop once the context ends.
	if err := i.checkContext(); err != nil {
		return err
	}

	// Reset to the root, reusing capacity or allocating after a prefix stop cleared it.
	i.positioned = true
	i.resetState()
	i.stack = append(i.stack[:0], stackEntry{node: i.t.root, cursor: i.t.bcs})

	// An empty key seeks to the prefix bound in the iteration direction.
	if len(k) == 0 {
		if len(i.prefix) != 0 {
			if i.rev {
				if i.prefixEnd != nil {
					k = i.prefixEnd
				} else {
					return i.seekToEnd()
				}
			} else {
				k = i.prefix
			}
		}
	}

	// Without a key or prefix, seek to the first entry in the iteration direction.
	if len(k) == 0 {
		if i.rev {
			return i.seekToEnd()
		}
		return i.seekToBeginning()
	}

	// Descend toward k, skipping subtrees that cannot hold the target.
	for len(i.stack) > 0 {
		// Inspect the next traversal frame before comparing it with the seek target.
		lastIdx := len(i.stack) - 1
		entry := &i.stack[lastIdx]

		// A reached leaf is eligible only on the requested side of the inclusive seek.
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

		// The separator is the right subtree minimum, so compare it to the seek target.
		cmp := bytes.Compare(entry.node.GetKey(), k)

		// Traverse eligible children in the requested direction.
		if !entry.visited {
			// Mark the parent before append can move the traversal stack.
			entry.visited = true

			// Skip the first child when it cannot satisfy this directional seek.
			var shouldVisitFirst bool
			if i.rev {
				shouldVisitFirst = cmp <= 0
			} else {
				shouldVisitFirst = cmp >= 0
			}

			// Load and queue the first child only when its interval can hold the target.
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
		} else {
			// Pop the completed parent before loading its second child.
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

	return nil
}

// Close closes the iterator.
// Note: it is not necessary to close all iterators before Discard().
func (i *Iterator) Close() {
	i.err = context.Canceled
	i.resetState()
	i.stack = nil
}

// setCurrentNode exposes a matching leaf or ends traversal at a prefix bound.
func (i *Iterator) setCurrentNode(node *Node, cursor *block.Cursor) bool {
	// Prefix mismatch ends the scan once the directional bound has been crossed.
	key := node.GetKey()
	if !i.matchesPrefix(key) {
		if len(i.prefix) != 0 {
			if i.rev {
				if bytes.Compare(key, i.prefix) < 0 {
					i.stack = nil
				}
			} else if i.prefixEnd != nil && bytes.Compare(key, i.prefixEnd) >= 0 {
				i.stack = nil
			}
		}
		return false
	}

	// Retain the leaf and its cursor without resolving its value.
	i.key = key
	i.node = node
	i.nodeCursor = cursor
	return true
}

// setError retains the first iterator error.
func (i *Iterator) setError(err error) error {
	if i.err != nil {
		return i.err
	}
	i.err = err
	return err
}

// checkContext records cancellation before an iterator operation starts.
func (i *Iterator) checkContext() error {
	if i.ctx.Err() != nil {
		return i.setError(context.Canceled)
	}
	return nil
}

// resetState clears the current position and its cached value.
func (i *Iterator) resetState() {
	// Release the previous leaf and value while retaining the traversal stack.
	i.key = nil
	i.val = nil
	i.nodeCursor = nil
	i.node = nil
	i.hasVal = false
}

// seekToEnd moves the iterator to the last matching key in the tree.
func (i *Iterator) seekToEnd() error {
	for len(i.stack) > 0 {
		// Inspect the next frame on the descending traversal.
		lastIdx := len(i.stack) - 1
		entry := &i.stack[lastIdx]

		// Expose a matching leaf after removing its traversal frame.
		if entry.node.IsLeaf() {
			i.stack = i.stack[:lastIdx]
			if i.setCurrentNode(entry.node, entry.cursor) {
				return nil
			}
			continue
		}

		// Reverse traversal visits the right subtree before the left subtree.
		if !entry.visited {
			// Mark the parent before append can move the traversal stack.
			entry.visited = true

			// Load the right child before adding its traversal frame.
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
		} else {
			// Pop the completed parent before loading its left child.
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

// seekToBeginning moves the iterator to the first matching key in the tree.
func (i *Iterator) seekToBeginning() error {
	for len(i.stack) > 0 {
		// Inspect the next frame on the ascending traversal.
		lastIdx := len(i.stack) - 1
		entry := &i.stack[lastIdx]

		// Expose a matching leaf after removing its traversal frame.
		if entry.node.IsLeaf() {
			i.stack = i.stack[:lastIdx]
			if i.setCurrentNode(entry.node, entry.cursor) {
				return nil
			}
			continue
		}

		// Forward traversal visits the left subtree before the right subtree.
		if !entry.visited {
			// Mark the parent before append can move the traversal stack.
			entry.visited = true

			// Load the left child before adding its traversal frame.
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
		} else {
			// Pop the completed parent before loading its right child.
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

// matchesPrefix checks a nonempty key against the iterator's prefix constraint.
func (i *Iterator) matchesPrefix(key []byte) bool {
	return len(key) > 0 && (len(i.prefix) == 0 || bytes.HasPrefix(key, i.prefix))
}

// _ checks the public iterator implementation.
var _ kvtx.Iterator = (*Iterator)(nil)
