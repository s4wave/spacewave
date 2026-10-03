package kvtx_block_iavl

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_gzip "github.com/s4wave/spacewave/db/block/transform/gzip"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_kvtest "github.com/s4wave/spacewave/db/kvtx/kvtest"
	kvtx_vlogger "github.com/s4wave/spacewave/db/kvtx/vlogger"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/sirupsen/logrus"
)

// PrintIAVLTree prints a text representation of the IAVL tree.
// Returns an error if any node traversal fails.
func PrintIAVLTree(ctx context.Context, t *AVLTree) (string, error) {
	// Open a read transaction for the tree rendering.
	btx, err := t.NewAVLTreeTransaction(ctx, false)
	if err != nil {
		return "", err
	}
	defer btx.Discard()

	// Prepare the text builder and recursive Node renderer.
	var sb strings.Builder
	var printNode func(node *Node, cursor *block.Cursor, depth int) error

	// Define the traversal that renders each Node and its children.
	printNode = func(node *Node, cursor *block.Cursor, depth int) error {
		// Finish rendering when a child Node is absent.
		if node == nil {
			return nil
		}

		// Print indentation
		for range depth {
			sb.WriteString("  ")
		}

		// Print node info
		sb.WriteString(string(node.GetKey()))
		sb.WriteString(" (h:")
		fmt.Fprint(&sb, node.GetHeight())
		sb.WriteString(")")
		sb.WriteString("\n")

		// Print left subtree
		if !node.IsLeaf() {
			leftNode, leftCursor, err := node.FollowLeft(ctx, cursor)
			if err != nil {
				return err
			}
			if leftNode != nil {
				if err := printNode(leftNode, leftCursor, depth+1); err != nil {
					return err
				}
			}

			// Print right subtree
			rightNode, rightCursor, err := node.FollowRight(ctx, cursor)
			if err != nil {
				return err
			}
			if rightNode != nil {
				if err := printNode(rightNode, rightCursor, depth+1); err != nil {
					return err
				}
			}
		}
		return nil
	}

	// Render the root and propagate any traversal error.
	if err := printNode(btx.root, btx.bcs, 0); err != nil {
		return "", err
	}

	return sb.String(), nil
}

// TestSimple is a basic iavl tree test.
func TestSimple(t *testing.T) {
	// Prepare the context and logger for the single-key test.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start the testbed that stores the IAVL tree.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Read the testbed volume identity for bucket lookup.
	vol := tb.Volume
	volID := vol.GetID()
	t.Log(volID)

	// Construct an empty transform config.
	tconf, err := block_transform.NewConfig(nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Open an empty bucket cursor with the configured transform.
	oc, _, err := bucket_lookup.BuildEmptyCursor(
		ctx,
		tb.Bus,
		tb.Logger,
		tb.StepFactorySet,
		tb.BucketId,
		volID,
		tconf,
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Construct the IAVL tree over the empty bucket cursor.
	tr := NewAVLTree(oc)

	// Open a writable transaction for the single-key test.
	btx, err := tr.NewAVLTreeTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the new transaction contains no keys.
	ilen, err := btx.Size(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if ilen != 0 {
		t.FailNow()
	}

	// Verify the new tree has no value for the test key.
	key := []byte("test")
	h, err := btx.Exists(ctx, key)
	if err != nil {
		t.Fatal(err.Error())
	}
	if _, ok, err := btx.Get(ctx, key); ok || err != nil || h {
		t.FailNow()
	}

	// Write the test value into the empty tree.
	val := []byte("tvalue")
	err = btx.Set(ctx, key, val)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the written value is reachable through the transaction.
	ival, ok, err := btx.Get(ctx, key)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !ok || !bytes.Equal(ival, val) {
		t.FailNow()
	}

	// Test basic iterator functionality
	iter := btx.Iterate(ctx, nil, true, false)
	if !iter.Next() || !iter.Valid() {
		t.Fatal("expected valid iterator")
	}
	if !bytes.Equal(iter.Key(), key) {
		t.Fatalf("expected key %q, got %q", key, iter.Key())
	}
	iterVal, err := iter.Value()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(iterVal, val) {
		t.Fatalf("expected value %q, got %q", val, iterVal)
	}
	if iter.Next() || iter.Valid() {
		t.Fatal("expected no more entries after end")
	}
	iter.Close()
}

// TestIavl is a more comprehensive test.
func TestIavl(t *testing.T) {
	// Prepare the context and logger for the persistence test.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.InfoLevel)
	le := logrus.NewEntry(log)

	// Start the testbed that stores the compressed tree.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Read the testbed volume identity for bucket lookup.
	vol := tb.Volume
	volID := vol.GetID()
	t.Log(volID)

	// construct a basic transform config.
	tconf, err := block_transform.NewConfig([]config.Config{
		&transform_gzip.Config{},
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// Open an empty bucket cursor with the compression transform.
	oc, _, err := bucket_lookup.BuildEmptyCursor(
		ctx,
		tb.Bus,
		tb.Logger,
		tb.StepFactorySet,
		tb.BucketId,
		volID,
		tconf,
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Open the IAVL tree for writing.
	tr := NewAVLTree(oc)
	btx, err := tr.NewAVLTreeTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the new tree contains no keys.
	ilen, err := btx.Size(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if ilen != 0 {
		t.FailNow()
	}

	// Verify the test key is absent before insertion.
	key := []byte("test")
	h, err := btx.Exists(ctx, key)
	if err != nil {
		t.Fatal(err.Error())
	}
	if _, ok, err := btx.Get(ctx, key); ok || err != nil || h {
		t.FailNow()
	}

	// Write a sequence of keys with nonempty values.
	kn := 5
	t.Logf("placing %d keys", kn)
	for i := range kn {
		// Prepare the key and value for this tree entry.
		key := fmt.Appendf(nil, "key-%d", i)
		val := fmt.Appendf(nil, "key-%d", kn-i)

		// Insert the entry and report its key.
		err := btx.Set(ctx, key, val)
		if err != nil {
			t.Fatal(err.Error())
		}
		t.Log(string(key))
	}

	// Define the readback check for every inserted key.
	checkAll := func() {
		for i := kn - 1; i >= 0; i-- {
			key := fmt.Appendf(nil, "key-%d", i)
			ival, ok, err := btx.Get(ctx, key)
			if err != nil {
				t.Fatal(err.Error())
			}
			if !ok || len(ival) == 0 {
				t.Fatalf("key not found %s", key)
			}
		}
	}

	// Verify the written keys before persisting the transaction.
	checkAll()
	if err := btx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}

	// Reopen the persisted tree for readback.
	btx, err = tr.NewAVLTreeTransaction(ctx, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify persisted values and count the matching prefix entries.
	checkAll()
	keyCount := 0
	err = btx.ScanPrefix(ctx, []byte("key-"), func(key, val []byte) error {
		if len(key) == 0 || len(val) == 0 {
			t.FailNow()
		}
		keyCount++
		return nil
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if keyCount != kn {
		t.Fatalf("counted %d keys expected %d", keyCount, kn)
	}

	// Replace the read transaction with a write transaction for deletion.
	btx.Discard()
	btx, err = tr.NewAVLTreeTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the keys before removing alternate entries.
	checkAll()
	for i := range kn {
		key := fmt.Appendf(nil, "key-%d", i)
		if i%2 == 0 {
			// Delete the selected even-numbered key.
			t.Logf("deleting key %s", key)
			err := btx.Delete(ctx, key)
			if err != nil {
				t.Fatal(err.Error())
			}

			// Verify the deleted key is absent from the transaction.
			_, bfound, err := btx.Get(ctx, key)
			if err != nil {
				t.Fatal(err.Error())
			}
			if bfound {
				t.Fatalf("key %s found after deleted", key)
			}
		}
	}

	// Persist the deletions before reopening the root.
	if err := btx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}

	// Follow the persisted root and open it for readback.
	rref := tr.GetRootNodeRef()
	fc, err := oc.FollowRef(ctx, rref)
	if err != nil {
		t.Fatal(err.Error())
	}
	ft := NewAVLTree(fc)
	btx, err = ft.NewAVLTreeTransaction(ctx, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the persisted tree reports the remaining key count.
	expectedSize := kn / 2
	ns, err := btx.Size(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	trs := int(ns) //nolint:gosec
	if trs != expectedSize {
		t.Fatalf("removal size mismatch %d != expected %d", trs, expectedSize)
	}

	// Verify each surviving key and count its membership.
	actLen := 0
	for i := range kn {
		key := fmt.Appendf(nil, "key-%d", i)
		keep := i%2 != 0
		_, exists, err := btx.Get(ctx, key)
		if err != nil {
			t.Fatal(err.Error())
		}
		if exists != keep {
			t.Fatalf("key %s exists %v (expected %v)", key, exists, keep)
		}
		if exists {
			actLen++
		}
	}

	// Verify the observed membership agrees with the reported size.
	if actLen != trs {
		t.Fatalf("length reported %d != actual length %d", trs, actLen)
	}

	// Release the read transaction after persistence checks.
	btx.Discard()
}

// TestKvtest is an end to end test.
func TestKvtest(t *testing.T) {
	// Prepare the context and logger for the key-value contract suite.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start the testbed for the key-value contract suite.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Read the testbed volume identity for bucket lookup.
	vol := tb.Volume
	volID := vol.GetID()
	t.Log(volID)

	// construct a transform config.
	tconf, err := block_transform.NewConfig([]config.Config{})
	if err != nil {
		t.Fatal(err.Error())
	}

	// Open an empty bucket cursor for the contract suite.
	oc, _, err := bucket_lookup.BuildEmptyCursor(
		ctx,
		tb.Bus,
		tb.Logger,
		tb.StepFactorySet,
		tb.BucketId,
		volID,
		tconf,
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Wrap the IAVL tree with transaction logging.
	tr := NewAVLTree(oc)
	vl := kvtx_vlogger.NewVLogger(le, tr)

	// Run the shared key-value contracts against the IAVL store.
	if err := kvtx_kvtest.TestAll(ctx, vl); err != nil {
		t.Fatal(err.Error())
	}
}

// TestSimpleIterate tests basic iterator behavior with a small tree.
func TestSimpleIterate(t *testing.T) {
	// Prepare the context and logger for iterator tests.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start the testbed that stores the iterator fixture.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Open an empty bucket cursor for the iterator fixture.
	oc, _, err := bucket_lookup.BuildEmptyCursor(
		ctx,
		tb.Bus,
		tb.Logger,
		tb.StepFactorySet,
		tb.BucketId,
		tb.Volume.GetID(),
		&block_transform.Config{},
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Open a write transaction for the iterator fixture.
	tr := NewAVLTree(oc)
	btx, err := tr.NewAVLTreeTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Insert keys in an order that exercises IAVL balancing.
	keys := []string{"5", "3", "7", "2", "4", "6", "8"}
	for _, k := range keys {
		err = btx.Set(ctx, []byte(k), []byte("val-"+k))
		if err != nil {
			t.Fatal(err.Error())
		}
	}

	// Persist the iterator fixture before reading it.
	if err := btx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}

	// Print the tree
	treeStr, err := PrintIAVLTree(ctx, tr)
	if err != nil {
		t.Fatal(err.Error())
	}
	os.Stderr.WriteString(treeStr + "\n")

	// Test forward iteration
	t.Run("Forward Iteration", func(t *testing.T) {
		// Open a read transaction for forward iteration.
		btx, err = tr.NewAVLTreeTransaction(ctx, false)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer btx.Discard()

		// Open the forward iterator and register its cleanup.
		iter := btx.Iterate(ctx, nil, true, false)
		defer iter.Close()

		// Sequence: 2, 3, 4, 5, 6, 7, 8
		expected := slices.Clone(keys)
		slices.Sort(expected)
		for _, exp := range expected {
			if !iter.Next() || !iter.Valid() {
				t.Fatalf("iterator invalid, expected key %s", exp)
			}
			got := string(iter.Key())
			if got != exp {
				t.Fatalf("expected key %s, got %s", exp, got)
			}
		}

		// Should be at end
		if iter.Next() || iter.Valid() {
			t.Fatal("expected iterator to be invalid after end")
		}
	})

	// Test reverse iteration
	t.Run("Reverse Iteration", func(t *testing.T) {
		// Open a read transaction for reverse iteration.
		btx, err = tr.NewAVLTreeTransaction(ctx, false)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer btx.Discard()

		// Open the reverse iterator and register its cleanup.
		iter := btx.Iterate(ctx, nil, true, true)
		defer iter.Close()

		// Sequence: 8, 7, 6, 5, 4, 3, 2
		expected := slices.Clone(keys)
		slices.Sort(expected)
		slices.Reverse(expected)
		for _, exp := range expected {
			if !iter.Next() || !iter.Valid() {
				t.Fatalf("iterator invalid, expected key %s", exp)
			}
			got := string(iter.Key())
			if got != exp {
				t.Fatalf("expected key %s, got %s", exp, got)
			}
		}

		// Should be at end
		if iter.Next() || iter.Valid() {
			t.Fatal("expected iterator to be invalid after end")
		}
	})

	// Test seek followed by forward iteration
	t.Run("Seek and Forward Iteration", func(t *testing.T) {
		// Open a read transaction for forward iteration after seeking.
		btx, err = tr.NewAVLTreeTransaction(ctx, false)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer btx.Discard()

		// Open the forward iterator and register its cleanup.
		iter := btx.Iterate(ctx, nil, true, false)
		defer iter.Close()

		// Seek to "5" and iterate forward
		if err := iter.Seek([]byte("5")); err != nil {
			t.Fatal(err)
		}
		if !iter.Valid() {
			t.Fatal("expected valid iterator after seek")
		}
		if got := string(iter.Key()); got != "5" {
			t.Fatalf("expected key 5, got %s", got)
		}

		// Expected remaining sequence: 6, 7, 8
		expected := []string{"6", "7", "8"}
		for _, exp := range expected {
			if !iter.Next() || !iter.Valid() {
				t.Fatalf("iterator invalid, expected key %s", exp)
			}
			got := string(iter.Key())
			if got != exp {
				t.Fatalf("expected key %s, got %s", exp, got)
			}
		}

		// Should be at end
		if iter.Next() || iter.Valid() {
			t.Fatal("expected iterator to be invalid after end")
		}
	})

	// Test seek followed by reverse iteration
	t.Run("Seek and Reverse Iteration", func(t *testing.T) {
		// Open a read transaction for reverse iteration after seeking.
		btx, err = tr.NewAVLTreeTransaction(ctx, false)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer btx.Discard()

		// Open the reverse iterator and register its cleanup.
		iter := btx.Iterate(ctx, nil, true, true)
		defer iter.Close()

		// Seek to "5" and iterate in reverse
		if err := iter.Seek([]byte("5")); err != nil {
			t.Fatal(err)
		}
		if !iter.Valid() {
			t.Fatal("expected valid iterator after seek")
		}
		if got := string(iter.Key()); got != "5" {
			t.Fatalf("expected key 5, got %s", got)
		}

		// Expected remaining sequence: 4, 3, 2
		expected := []string{"4", "3", "2"}
		for _, exp := range expected {
			if !iter.Next() || !iter.Valid() {
				t.Fatalf("iterator invalid, expected key %s", exp)
			}
			got := string(iter.Key())
			if got != exp {
				t.Fatalf("expected key %s, got %s", exp, got)
			}
		}

		// Should be at end
		if iter.Next() || iter.Valid() {
			t.Fatal("expected iterator to be invalid after end")
		}
	})

	// Test seek to beginning (nil key) in forward iteration
	t.Run("Seek Beginning Forward", func(t *testing.T) {
		// Open a read transaction for seeking to the first key.
		btx, err = tr.NewAVLTreeTransaction(ctx, false)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer btx.Discard()

		// Open the forward iterator and register its cleanup.
		iter := btx.Iterate(ctx, nil, true, false)
		defer iter.Close()

		// Seek to the beginning and verify the iterator remains valid.
		if err := iter.Seek(nil); err != nil {
			t.Fatal(err)
		}
		if !iter.Valid() {
			t.Fatal("expected valid iterator after seek to beginning")
		}

		// Should be at first key "2"
		if got := string(iter.Key()); got != "2" {
			t.Fatalf("expected first key 2, got %s", got)
		}

		// Expected sequence: 3, 4, 5, 6, 7, 8
		expected := []string{"3", "4", "5", "6", "7", "8"}
		for _, exp := range expected {
			if !iter.Next() || !iter.Valid() {
				t.Fatalf("iterator invalid, expected key %s", exp)
			}
			got := string(iter.Key())
			if got != exp {
				t.Fatalf("expected key %s, got %s", exp, got)
			}
		}
	})

	// Test seek to end (nil key) in reverse iteration
	t.Run("Seek End Reverse", func(t *testing.T) {
		// Open a read transaction for seeking to the last key.
		btx, err = tr.NewAVLTreeTransaction(ctx, false)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer btx.Discard()

		// Open the reverse iterator and register its cleanup.
		iter := btx.Iterate(ctx, nil, true, true)
		defer iter.Close()

		// Seek to the end and verify the iterator remains valid.
		if err := iter.Seek(nil); err != nil {
			t.Fatal(err)
		}
		if !iter.Valid() {
			t.Fatal("expected valid iterator after seek to end")
		}

		// Should be at last key "8"
		if got := string(iter.Key()); got != "8" {
			t.Fatalf("expected last key 8, got %s", got)
		}

		// Expected sequence: 7, 6, 5, 4, 3, 2
		expected := []string{"7", "6", "5", "4", "3", "2"}
		for _, exp := range expected {
			if !iter.Next() || !iter.Valid() {
				t.Fatalf("iterator invalid, expected key %s", exp)
			}
			got := string(iter.Key())
			if got != exp {
				t.Fatalf("expected key %s, got %s", exp, got)
			}
		}
	})
}

func TestIteratorSeekNilUsesPrefixBounds(t *testing.T) {
	// Start a testbed for prefix-bounded iterator checks.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	// Open an empty bucket cursor for the prefix fixture.
	oc, _, err := bucket_lookup.BuildEmptyCursor(
		ctx,
		tb.Bus,
		tb.Logger,
		tb.StepFactorySet,
		tb.BucketId,
		tb.Volume.GetID(),
		&block_transform.Config{},
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer oc.Release()

	// Open a write transaction and persist keys across prefix boundaries.
	tr := NewAVLTree(oc)
	btx, err := tr.NewAVLTreeTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	for _, key := range []string{"aa/0", "aa/1", "aa/2", "ab/0", "b/0"} {
		if err := btx.Set(ctx, []byte(key), []byte("value-"+key)); err != nil {
			t.Fatal(err.Error())
		}
	}
	if err := btx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}

	// Verify forward seeking uses the prefix lower bound.
	t.Run("forward", func(t *testing.T) {
		// Open a read transaction for the forward prefix check.
		btx, err := tr.NewAVLTreeTransaction(ctx, false)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer btx.Discard()

		// Seek the forward iterator to the prefix beginning and verify its keys.
		iter := btx.Iterate(ctx, []byte("aa/"), true, false)
		defer iter.Close()
		if err := iter.Seek(nil); err != nil {
			t.Fatal(err.Error())
		}
		assertIteratorKeys(t, iter, []string{"aa/0", "aa/1", "aa/2"})
	})

	// Verify reverse seeking uses the prefix upper bound.
	t.Run("reverse", func(t *testing.T) {
		// Open a read transaction for the reverse prefix check.
		btx, err := tr.NewAVLTreeTransaction(ctx, false)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer btx.Discard()

		// Seek the reverse iterator to the prefix end and verify its keys.
		iter := btx.Iterate(ctx, []byte("aa/"), true, true)
		defer iter.Close()
		if err := iter.Seek(nil); err != nil {
			t.Fatal(err.Error())
		}
		assertIteratorKeys(t, iter, []string{"aa/2", "aa/1", "aa/0"})
	})
}

func assertIteratorKeys(t *testing.T, iter kvtx.Iterator, expected []string) {
	t.Helper()
	for idx, exp := range expected {
		if idx != 0 && !iter.Next() {
			t.Fatalf("iterator stopped before %s", exp)
		}
		if !iter.Valid() {
			t.Fatalf("iterator invalid, expected key %s", exp)
		}
		if got := string(iter.Key()); got != exp {
			t.Fatalf("key = %s, want %s", got, exp)
		}
	}
	if iter.Next() || iter.Valid() {
		t.Fatalf("iterator returned extra key %s", iter.Key())
	}
}

// TestScanPrefixHighBytes checks that prefix scans include keys that continue
// the prefix with 0xff bytes and stop at the prefix successor.
func TestScanPrefixHighBytes(t *testing.T) {
	// Start a testbed volume for the tree.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	// Open an empty tree in the testbed bucket.
	oc, _, err := bucket_lookup.BuildEmptyCursor(
		ctx,
		tb.Bus,
		tb.Logger,
		tb.StepFactorySet,
		tb.BucketId,
		tb.Volume.GetID(),
		&block_transform.Config{},
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer oc.Release()
	tr := NewAVLTree(oc)

	// Write keys at the 0xff edges of each prefix.
	btx, err := tr.NewAVLTreeTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	keys := [][]byte{
		[]byte("a/x"),
		{'a', '/', 0xff},
		{'a', '/', 0xff, 0x01},
		{'a', '/', 0xff, 0xff, 0x02},
		[]byte("a0"),
		{0xff, 0x01},
	}
	for _, key := range keys {
		if err := btx.Set(ctx, key, []byte("value")); err != nil {
			t.Fatal(err.Error())
		}
	}
	if err := btx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}

	// Scan each prefix with both scan methods and compare the keys.
	rtx, err := tr.NewAVLTreeTransaction(ctx, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer rtx.Discard()
	for _, tc := range []struct {
		prefix []byte
		want   [][]byte
	}{
		{prefix: []byte("a/"), want: keys[:4]},
		{prefix: []byte{0xff}, want: keys[5:]},
		{prefix: nil, want: keys},
	} {
		// Collect the keys from both scans.
		var keyScan, valueScan [][]byte
		if err := rtx.ScanPrefixKeys(ctx, tc.prefix, func(key []byte) error {
			keyScan = append(keyScan, slices.Clone(key))
			return nil
		}); err != nil {
			t.Fatal(err.Error())
		}
		if err := rtx.ScanPrefix(ctx, tc.prefix, func(key, _ []byte) error {
			valueScan = append(valueScan, slices.Clone(key))
			return nil
		}); err != nil {
			t.Fatal(err.Error())
		}

		// Both scans return exactly the prefix range.
		if !slices.EqualFunc(keyScan, tc.want, bytes.Equal) {
			t.Fatalf("ScanPrefixKeys(%x) = %x, want %x", tc.prefix, keyScan, tc.want)
		}
		if !slices.EqualFunc(valueScan, tc.want, bytes.Equal) {
			t.Fatalf("ScanPrefix(%x) = %x, want %x", tc.prefix, valueScan, tc.want)
		}
	}
}
