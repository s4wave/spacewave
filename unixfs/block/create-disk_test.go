//go:build !windows && !js

package unixfs_block

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/aperturerobotics/hydra/testbed"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"golang.org/x/sys/unix"
)

// TestCreateFromDiskPreservesXattrs verifies that CreateFromDisk captures
// xattrs from the source filesystem and populates FSNode.Xattrs, enabling
// round-trip through SyncXattrs.
func TestCreateFromDiskPreservesXattrs(t *testing.T) {
	// Create a temp dir with files and xattrs.
	srcDir, err := os.MkdirTemp("", "hydra-xattr-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(srcDir)

	// Create a file.
	filePath := filepath.Join(srcDir, "test.txt")
	if err := os.WriteFile(filePath, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create a subdirectory with a file.
	subDir := filepath.Join(srcDir, "sub")
	if err := os.Mkdir(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	subFile := filepath.Join(subDir, "nested.txt")
	if err := os.WriteFile(subFile, []byte("nested"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Set xattrs on the file and directory.
	testXattrName := "user.test.hydra"
	testXattrValue := []byte("xattr-value-123")
	if err := unix.Lsetxattr(filePath, testXattrName, testXattrValue, 0); err != nil {
		t.Skipf("xattrs not supported on this filesystem: %v", err)
	}

	dirXattrName := "user.test.dir"
	dirXattrValue := []byte("dir-xattr")
	if err := unix.Lsetxattr(subDir, dirXattrName, dirXattrValue, 0); err != nil {
		t.Fatal(err)
	}

	nestedXattrName := "user.test.nested"
	nestedXattrValue := []byte("nested-xattr")
	if err := unix.Lsetxattr(subFile, nestedXattrName, nestedXattrValue, 0); err != nil {
		t.Fatal(err)
	}

	// CreateFromDisk should capture xattrs.
	ctx := context.Background()
	writeTs := timestamp.Now()
	testbed.RunSubtest(t, "CreateFromDisk", func(t *testing.T, tb *testbed.Testbed) {
		bls, err := tb.BuildEmptyCursor(ctx)
		if err != nil {
			t.Fatal(err)
		}
		btx, bcs := bls.BuildTransaction(nil)
		if err := CreateFromDisk(ctx, bcs, srcDir, writeTs); err != nil {
			t.Fatal(err)
		}
		_, bcs, err = btx.Write(ctx, true)
		if err != nil {
			t.Fatal(err)
		}

		// Verify xattrs on the tree.
		fsTree, err := NewFSTree(ctx, bcs, NodeType_NodeType_DIRECTORY)
		if err != nil {
			t.Fatal(err)
		}

		// Check file xattr.
		fileTree, _, err := fsTree.LookupFollowDirent("test.txt")
		if err != nil {
			t.Fatal(err)
		}
		fileNode := fileTree.GetFSNode()
		val := fileNode.GetXattrValue(testXattrName)
		if val == nil {
			t.Fatalf("expected xattr %s on test.txt, got none", testXattrName)
		}
		if string(val) != string(testXattrValue) {
			t.Fatalf("xattr value mismatch: got %q, want %q", val, testXattrValue)
		}

		// Check directory xattr.
		subTree, _, err := fsTree.LookupFollowDirent("sub")
		if err != nil {
			t.Fatal(err)
		}
		subNode := subTree.GetFSNode()
		val = subNode.GetXattrValue(dirXattrName)
		if val == nil {
			t.Fatalf("expected xattr %s on sub/, got none", dirXattrName)
		}
		if string(val) != string(dirXattrValue) {
			t.Fatalf("dir xattr value mismatch: got %q, want %q", val, dirXattrValue)
		}

		// Check nested file xattr.
		nestedTree, _, err := subTree.LookupFollowDirent("nested.txt")
		if err != nil {
			t.Fatal(err)
		}
		nestedNode := nestedTree.GetFSNode()
		val = nestedNode.GetXattrValue(nestedXattrName)
		if val == nil {
			t.Fatalf("expected xattr %s on sub/nested.txt, got none", nestedXattrName)
		}
		if string(val) != string(nestedXattrValue) {
			t.Fatalf("nested xattr value mismatch: got %q, want %q", val, nestedXattrValue)
		}
	})
}

// TestCreateFromDiskFiltersTransientXattrs verifies that transient macOS
// xattrs (quarantine, lastuseddate, kMDItemWhereFroms) are excluded.
func TestCreateFromDiskFiltersTransientXattrs(t *testing.T) {
	srcDir, err := os.MkdirTemp("", "hydra-xattr-filter-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(srcDir)

	filePath := filepath.Join(srcDir, "app.bin")
	if err := os.WriteFile(filePath, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Set a real xattr and a transient one.
	realName := "user.test.keep"
	realValue := []byte("keep-this")
	if err := unix.Lsetxattr(filePath, realName, realValue, 0); err != nil {
		t.Skipf("xattrs not supported: %v", err)
	}
	if err := unix.Lsetxattr(filePath, "com.apple.quarantine", []byte("transient"), 0); err != nil {
		// May fail on Linux (com.apple.quarantine is macOS-specific) but that is OK,
		// the test still validates that the real xattr is preserved.
		t.Logf("could not set com.apple.quarantine (expected on Linux): %v", err)
	}

	ctx := context.Background()
	writeTs := timestamp.Now()
	testbed.RunSubtest(t, "FilterTransient", func(t *testing.T, tb *testbed.Testbed) {
		bls, err := tb.BuildEmptyCursor(ctx)
		if err != nil {
			t.Fatal(err)
		}
		btx, bcs := bls.BuildTransaction(nil)
		if err := CreateFromDisk(ctx, bcs, srcDir, writeTs); err != nil {
			t.Fatal(err)
		}
		_, bcs, err = btx.Write(ctx, true)
		if err != nil {
			t.Fatal(err)
		}

		fsTree, err := NewFSTree(ctx, bcs, NodeType_NodeType_DIRECTORY)
		if err != nil {
			t.Fatal(err)
		}
		fileTree, _, err := fsTree.LookupFollowDirent("app.bin")
		if err != nil {
			t.Fatal(err)
		}
		node := fileTree.GetFSNode()

		// Real xattr should be present.
		val := node.GetXattrValue(realName)
		if val == nil {
			t.Fatalf("expected xattr %s, got none", realName)
		}

		// Transient xattr should be filtered.
		if node.GetXattrValue("com.apple.quarantine") != nil {
			t.Fatal("com.apple.quarantine should have been filtered")
		}
	})
}
