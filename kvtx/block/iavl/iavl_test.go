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
	"github.com/aperturerobotics/hydra/block"
	block_transform "github.com/aperturerobotics/hydra/block/transform"
	transform_chksum "github.com/aperturerobotics/hydra/block/transform/chksum"
	transform_s2 "github.com/aperturerobotics/hydra/block/transform/s2"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	kvtx_kvtest "github.com/aperturerobotics/hydra/kvtx/kvtest"
	kvtx_vlogger "github.com/aperturerobotics/hydra/kvtx/vlogger"
	"github.com/aperturerobotics/hydra/testbed"
	"github.com/sirupsen/logrus"
)

// PrintIAVLTree prints a text representation of the IAVL tree.
// Returns an error if any node traversal fails.
func PrintIAVLTree(ctx context.Context, t *AVLTree) (string, error) {
	btx, err := t.NewAVLTreeTransaction(ctx, false)
	if err != nil {
		return "", err
	}
	defer btx.Discard()

	var sb strings.Builder
	var printNode func(node *Node, cursor *block.Cursor, depth int) error

	printNode = func(node *Node, cursor *block.Cursor, depth int) error {
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
		sb.WriteString(fmt.Sprint(node.GetHeight()))
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

	if err := printNode(btx.root, btx.bcs, 0); err != nil {
		return "", err
	}

	return sb.String(), nil
}

// TestSimple is a basic iavl tree test.
func TestSimple(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	vol := tb.Volume
	volID := vol.GetID()
	t.Log(volID)

	// construct a basic transform config.
	tconf, err := block_transform.NewConfig([]config.Config{
		// &transform_chksum.Config{},
		// &transform_s2.Config{},
	})
	if err != nil {
		t.Fatal(err.Error())
	}

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

	tr := NewAVLTree(oc)

	btx, err := tr.NewAVLTreeTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	ilen, err := btx.Size(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if ilen != 0 {
		t.FailNow()
	}

	key := []byte("test")
	h, err := btx.Exists(ctx, key)
	if err != nil {
		t.Fatal(err.Error())
	}
	if _, ok, err := btx.Get(ctx, key); ok || err != nil || h {
		t.FailNow()
	}

	val := []byte("tvalue")
	err = btx.Set(ctx, key, val)
	if err != nil {
		t.Fatal(err.Error())
	}

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
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.InfoLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	vol := tb.Volume
	volID := vol.GetID()
	t.Log(volID)

	// construct a basic transform config.
	tconf, err := block_transform.NewConfig([]config.Config{
		&transform_chksum.Config{},
		&transform_s2.Config{},
	})
	if err != nil {
		t.Fatal(err.Error())
	}

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

	tr := NewAVLTree(oc)
	btx, err := tr.NewAVLTreeTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	ilen, err := btx.Size(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if ilen != 0 {
		t.FailNow()
	}

	key := []byte("test")
	h, err := btx.Exists(ctx, key)
	if err != nil {
		t.Fatal(err.Error())
	}
	if _, ok, err := btx.Get(ctx, key); ok || err != nil || h {
		t.FailNow()
	}

	kn := 5
	t.Logf("placing %d keys", kn)
	for i := range kn {
		key := fmt.Appendf(nil, "key-%d", i)
		val := fmt.Appendf(nil, "key-%d", kn-i)

		err := btx.Set(ctx, key, val)
		if err != nil {
			t.Fatal(err.Error())
		}
		t.Log(string(key))
	}

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

	checkAll()
	if err := btx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}

	btx, err = tr.NewAVLTreeTransaction(ctx, false)
	if err != nil {
		t.Fatal(err.Error())
	}

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

	btx.Discard()
	btx, err = tr.NewAVLTreeTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	checkAll()
	for i := range kn {
		key := fmt.Appendf(nil, "key-%d", i)
		if i%2 == 0 {
			t.Logf("deleting key %s", key)
			err := btx.Delete(ctx, key)
			if err != nil {
				t.Fatal(err.Error())
			}
			_, bfound, err := btx.Get(ctx, key)
			if err != nil {
				t.Fatal(err.Error())
			}
			if bfound {
				t.Fatalf("key %s found after deleted", key)
			}
		}
	}

	if err := btx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}

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

	expectedSize := kn / 2
	ns, err := btx.Size(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	trs := int(ns) //nolint:gosec
	if trs != expectedSize {
		t.Fatalf("removal size mismatch %d != expected %d", trs, expectedSize)
	}
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
	if actLen != trs {
		t.Fatalf("length reported %d != actual length %d", trs, actLen)
	}

	btx.Discard()
}

// TestKvtest is an end to end test.
func TestKvtest(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	vol := tb.Volume
	volID := vol.GetID()
	t.Log(volID)

	// construct a transform config.
	tconf, err := block_transform.NewConfig([]config.Config{})
	if err != nil {
		t.Fatal(err.Error())
	}

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

	tr := NewAVLTree(oc)
	vl := kvtx_vlogger.NewVLogger(le, tr)

	if err := kvtx_kvtest.TestAll(ctx, vl); err != nil {
		t.Fatal(err.Error())
	}
}

// TestSimpleIterate tests basic iterator behavior with a small tree.
func TestSimpleIterate(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

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

	tr := NewAVLTree(oc)
	btx, err := tr.NewAVLTreeTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	keys := []string{"5", "3", "7", "2", "4", "6", "8"}
	for _, k := range keys {
		err = btx.Set(ctx, []byte(k), []byte("val-"+k))
		if err != nil {
			t.Fatal(err.Error())
		}
	}

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
		btx, err = tr.NewAVLTreeTransaction(ctx, false)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer btx.Discard()

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
		btx, err = tr.NewAVLTreeTransaction(ctx, false)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer btx.Discard()

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
		btx, err = tr.NewAVLTreeTransaction(ctx, false)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer btx.Discard()

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
		btx, err = tr.NewAVLTreeTransaction(ctx, false)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer btx.Discard()

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
		btx, err = tr.NewAVLTreeTransaction(ctx, false)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer btx.Discard()

		iter := btx.Iterate(ctx, nil, true, false)
		defer iter.Close()

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
		btx, err = tr.NewAVLTreeTransaction(ctx, false)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer btx.Discard()

		iter := btx.Iterate(ctx, nil, true, true)
		defer iter.Close()

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
