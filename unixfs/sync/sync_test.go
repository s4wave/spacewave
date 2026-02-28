package unixfs_sync

import (
	"bytes"
	"context"
	"crypto/rand"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aperturerobotics/hydra/testbed"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_billy "github.com/aperturerobotics/hydra/unixfs/billy"
	unixfs_iofs "github.com/aperturerobotics/hydra/unixfs/iofs"
	unixfs_world_testbed "github.com/aperturerobotics/hydra/unixfs/world/testbed"
	world_testbed "github.com/aperturerobotics/hydra/world/testbed"
	"github.com/go-git/go-billy/v6"
	memfs "github.com/go-git/go-billy/v6/memfs"
	billy_util "github.com/go-git/go-billy/v6/util"
	"github.com/sirupsen/logrus"
)

func setupTestbed(t *testing.T) (context.Context, *unixfs.FSHandle, billy.Filesystem) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	btb, err := testbed.NewTestbed(ctx, le, testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err.Error())
	}

	objKey := "test-fs"
	rref, _, err := unixfs_world_testbed.BuildTestbed(
		btb,
		objKey,
		true,
		world_testbed.WithWorldVerbose(false),
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	outFs := memfs.New()
	return ctx, rref, outFs
}

// TestSyncSimpleFile tests syncing a single file
func TestSyncSimpleFile(t *testing.T) {
	ctx, rref, outFs := setupTestbed(t)

	bfs := unixfs_billy.NewBillyFS(ctx, rref, "", time.Now())

	testFile := "test.txt"
	testData := []byte("Hello world!")
	err := billy_util.WriteFile(bfs, testFile, testData, 0o755)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = SyncToBilly(ctx, outFs, rref, DeleteMode_DeleteMode_DURING, nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	readData, err := billy_util.ReadFile(outFs, testFile)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !bytes.Equal(testData, readData) {
		t.Fatalf("data mismatch: %v != %v", testData, readData)
	}
}

// TestSyncDirectory tests syncing a directory structure
func TestSyncDirectory(t *testing.T) {
	ctx, rref, outFs := setupTestbed(t)

	bfs := unixfs_billy.NewBillyFS(ctx, rref, "", time.Now())

	// Create a directory structure
	err := bfs.MkdirAll("dir1/subdir", 0o755)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = billy_util.WriteFile(bfs, "dir1/file1.txt", []byte("File 1"), 0o644)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = billy_util.WriteFile(bfs, "dir1/subdir/file2.txt", []byte("File 2"), 0o644)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = SyncToBilly(ctx, outFs, rref, DeleteMode_DeleteMode_DURING, nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Check if the directory structure was synced correctly
	if _, err := outFs.Stat("dir1"); err != nil {
		t.Fatalf("dir1 not found: %v", err)
	}
	if _, err := outFs.Stat("dir1/subdir"); err != nil {
		t.Fatalf("dir1/subdir not found: %v", err)
	}
	if _, err := outFs.Stat("dir1/file1.txt"); err != nil {
		t.Fatalf("dir1/file1.txt not found: %v", err)
	}
	if _, err := outFs.Stat("dir1/subdir/file2.txt"); err != nil {
		t.Fatalf("dir1/subdir/file2.txt not found: %v", err)
	}
}

// TestSyncDeleteModes tests different delete modes
func TestSyncDeleteModes(t *testing.T) {
	testCases := []struct {
		name       string
		deleteMode DeleteMode
	}{
		{"DeleteDuring", DeleteMode_DeleteMode_DURING},
		{"DeleteAfter", DeleteMode_DeleteMode_AFTER},
		{"NoDelete", DeleteMode_DeleteMode_NONE},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, rref, outFs := setupTestbed(t)

			bfs := unixfs_billy.NewBillyFS(ctx, rref, "", time.Now())

			// Create a file in the source
			err := billy_util.WriteFile(bfs, "source.txt", []byte("Source file"), 0o644)
			if err != nil {
				t.Fatal(err.Error())
			}

			// Create a file in the destination that should be deleted
			err = billy_util.WriteFile(outFs, "dest.txt", []byte("Destination file"), 0o644)
			if err != nil {
				t.Fatal(err.Error())
			}

			err = SyncToBilly(ctx, outFs, rref, tc.deleteMode, nil)
			if err != nil {
				t.Fatal(err.Error())
			}

			// Check if source.txt exists in the destination
			if _, err := outFs.Stat("source.txt"); err != nil {
				t.Fatalf("source.txt not found: %v", err)
			}

			// Check if dest.txt still exists based on the delete mode
			_, err = outFs.Stat("dest.txt")
			if tc.deleteMode == DeleteMode_DeleteMode_NONE {
				if err != nil {
					t.Fatalf("dest.txt should exist but got error: %v", err)
				}
			} else {
				if err == nil {
					t.Fatalf("dest.txt should have been deleted")
				}
			}
		})
	}
}

// TestSyncLargeFileWithAppend tests syncing a large file and then appending to it
func TestSyncLargeFileWithAppend(t *testing.T) {
	ctx, rref, _ := setupTestbed(t)

	// Create a temporary directory
	tempDir, err := os.MkdirTemp("", "hydra-test-unixfs-sync_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create a temporary file within the temporary directory
	tempFile, err := os.CreateTemp(tempDir, "large_file_*.bin")
	if err != nil {
		t.Fatal(err)
	}
	defer tempFile.Close()

	// Write 1M of random data
	data := make([]byte, 1024*1024)
	_, err = rand.Read(data)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tempFile.Write(data)
	if err != nil {
		t.Fatal(err)
	}
	tempFile.Close()

	// Sync the directory to the testbed FSHandle
	err = SyncFromDisk(ctx, rref, tempDir, DeleteMode_DeleteMode_DURING, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Append 1M bytes to the on-disk file
	appendData := make([]byte, 1024*1024)
	_, err = rand.Read(appendData)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(tempFile.Name(), appendData, os.ModeAppend)
	if err != nil {
		t.Fatal(err)
	}

	// Sync again
	err = SyncFromDisk(ctx, rref, tempDir, DeleteMode_DeleteMode_DURING, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Verify that the synced file matches the on-disk file
	tmpFilename := filepath.Base(tempFile.Name())
	tmpFileFsh, err := rref.Lookup(ctx, tmpFilename)
	if err != nil {
		t.Fatal(err)
	}
	defer tmpFileFsh.Release()

	syncedFile := unixfs_iofs.NewFSFile(ctx, tmpFileFsh)
	defer syncedFile.Close()

	originalData, err := os.ReadFile(tempFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	syncedData, err := io.ReadAll(syncedFile)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(originalData, syncedData) {
		t.Fatalf("Synced file does not match the original file. Original size: %d, Synced size: %d", len(originalData), len(syncedData))
	}
}

// TestSyncWithFilter tests syncing with a filter callback
func TestSyncWithFilter(t *testing.T) {
	ctx, rref, outFs := setupTestbed(t)

	bfs := unixfs_billy.NewBillyFS(ctx, rref, "", time.Now())

	// Create files in the source
	err := billy_util.WriteFile(bfs, "include.txt", []byte("Include this file"), 0o644)
	if err != nil {
		t.Fatal(err.Error())
	}
	err = billy_util.WriteFile(bfs, "exclude.txt", []byte("Exclude this file"), 0o644)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Define a filter callback
	filterCb := func(ctx context.Context, path string, nodeType unixfs.FSCursorNodeType) (bool, error) {
		return filepath.Base(path) != "exclude.txt", nil
	}

	err = SyncToBilly(ctx, outFs, rref, DeleteMode_DeleteMode_DURING, filterCb)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Check if include.txt exists in the destination
	if _, err := outFs.Stat("include.txt"); err != nil {
		t.Fatalf("include.txt not found: %v", err)
	}

	// Check if exclude.txt does not exist in the destination
	if _, err := outFs.Stat("exclude.txt"); err == nil {
		t.Fatalf("exclude.txt should not have been synced")
	}
}
