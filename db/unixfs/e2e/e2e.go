package unixfs_e2e

import (
	"bytes"
	"context"
	"io"
	"strconv"
	"time"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/go-git/go-billy/v6"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
)

// TestUnixFS runs end to end tests on a UnixFS handle.
//
// The FS should be clean and point to a temporary directory.
func TestUnixFS(ctx context.Context, fsHandle *unixfs.FSHandle) error {
	ts := time.Date(2023, time.January, 1, 12, 0, 0, 0, time.UTC)
	// create hello-dir-1
	err := fsHandle.Mknod(ctx, true, []string{"hello-dir-1"}, unixfs.NewFSCursorNodeType_Dir(), 0, ts)
	if err != nil {
		return err
	}

	// get a handle to hello-dir-1
	dirHandle, err := fsHandle.Lookup(ctx, "hello-dir-1")
	if err != nil {
		return err
	}

	// create a new file test.txt in hello-dir-1
	err = dirHandle.Mknod(ctx, true, []string{"test.txt"}, unixfs.NewFSCursorNodeType_File(), 0, ts)
	if err != nil {
		return err
	}

	// lookup test.txt in hello-dir-1
	fhandle, err := dirHandle.Lookup(ctx, "test.txt")
	if err != nil {
		return err
	}

	// write some data to test.txt
	testData := []byte("hello world")
	err = fhandle.WriteAt(ctx, 0, testData, time.Now())
	if err != nil {
		return err
	}

	// check the file size immediately following the write
	fileInfo, err := fhandle.GetFileInfo(ctx)
	if err != nil {
		return err
	}
	if fileInfo.Size() != int64(len(testData)) {
		return errors.Errorf("returned file size %d when expected %d after writing", fileInfo.Size(), len(testData))
	}

	// read data
	checkReadFromFhandle := func() error {
		buf := make([]byte, 1500)
		nread, err := fhandle.ReadAt(ctx, 0, buf)
		if err == io.EOF && nread != 0 {
			err = nil
		}
		if err != nil {
			return err
		}
		buf = buf[:nread]
		if !bytes.Equal(buf, testData) {
			return errors.Errorf("read incorrect data: %#v != %#v", buf, string(testData))
		}
		return nil
	}
	if err := checkReadFromFhandle(); err != nil {
		return err
	}

	// change permissions
	err = fhandle.SetPermissions(ctx, 0o644, ts)
	if err == billy.ErrNotSupported {
		err = nil
	}
	if err != nil {
		return err
	}

	// change mod time
	nts := timestamp.Now()
	setTs := nts.AsTime().Add(time.Minute * -1)
	err = fhandle.SetModTimestamp(ctx, setTs)
	skipModTimestamp := err == billy.ErrNotSupported
	if skipModTimestamp {
		err = nil
	}
	if err != nil {
		return err
	}

	getTs, err := fhandle.GetModTimestamp(ctx)
	if err == nil && !getTs.Equal(setTs) && !skipModTimestamp {
		err = errors.Errorf("failed to update ts: expected %s but got %s", setTs.String(), getTs.String())
	}
	if err == billy.ErrNotSupported {
		err = nil
	}
	if err != nil {
		return err
	}

	// rename to renamed-dir-1
	/*
		err = dirHandle.Rename(ctx, fsHandle, "renamed-dir-1", ts)
		if err != nil {
			return err
		}

		// ensure old path doesn't exist
		_, err = fsHandle.Lookup(ctx, "hello-dir-1")
		if err != unixfs_errors.ErrNotExist {
			return err
		}
	*/

	// ensure new path exists
	/*
		dirHandle, err = fsHandle.Lookup(ctx, "renamed-dir-1")
		if err != nil {
			return err
		}
	*/

	// ensure file exists
	fhandle, err = dirHandle.Lookup(ctx, "test.txt")
	if err != nil {
		return err
	}
	if err := checkReadFromFhandle(); err != nil {
		return err
	}

	// test ReadFile
	readFileDat, err := unixfs.ReadFile(ctx, fhandle)
	if err != nil {
		return err
	}
	if !bytes.Equal(readFileDat, testData) {
		return errors.New("data from ReadFile does not match expected data")
	}

	// test WriteFile with 500KB of data
	newTestData := make([]byte, 500*1024) // 500KB
	for i := range newTestData {
		newTestData[i] = byte(i % 256) // Fill with repeating pattern
	}
	newFileName := "writefile_test.txt"

	// Create a new file
	err = dirHandle.Mknod(ctx, true, []string{newFileName}, unixfs.NewFSCursorNodeType_File(), 0o644, ts)
	if err != nil {
		return errors.Wrap(err, "failed to create new file")
	}

	// Get a handle to the new file
	newFileHandle, err := dirHandle.Lookup(ctx, newFileName)
	if err != nil {
		return errors.Wrap(err, "failed to lookup new file")
	}
	defer newFileHandle.Release()

	// Write data to the new file
	err = unixfs.WriteFile(ctx, newFileHandle, newTestData, ts)
	if err != nil {
		return errors.Wrap(err, "WriteFile failed")
	}

	// Verify the file contains the correct data
	verifyData, err := unixfs.ReadFile(ctx, newFileHandle)
	if err != nil {
		return errors.Wrap(err, "failed to read new file")
	}
	if !bytes.Equal(verifyData, newTestData) {
		return errors.New("data from WriteFile does not match expected data")
	}

	// Verify file size
	newFileInfo, err := newFileHandle.GetFileInfo(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get file info for new file")
	}
	if newFileInfo.Size() != int64(len(newTestData)) {
		return errors.Errorf("unexpected file size: got %d, want %d", newFileInfo.Size(), len(newTestData))
	}

	// Verify file permissions
	if newFileInfo.Mode().Perm() != 0o644 {
		return errors.Errorf("unexpected file permissions: got %v, want %v", newFileInfo.Mode().Perm(), 0o644)
	}

	// test renaming twice in a row
	err = dirHandle.Rename(ctx, fsHandle, "renamed-2", ts)
	skipRename := err == billy.ErrNotSupported || err == unixfs_errors.ErrCrossFsRename
	if skipRename {
		err = nil
	}
	if err != nil {
		return err
	}

	if !skipRename {
		err = dirHandle.Rename(ctx, fsHandle, "renamed-3", ts)
		if err != nil {
			return err
		}
	}

	// traverse to subdir
	fsHandle = dirHandle

	nfilenames := 100
	fileNames := make([]string, nfilenames)
	for i := range fileNames {
		fileNames[i] = "file-" + strconv.Itoa(i)
	}

	// create them
	err = fsHandle.Mknod(ctx, true, fileNames, unixfs.NewFSCursorNodeType_File(), 0, ts)
	if err != nil {
		return err
	}

	// check they all exist & open handles
	fsHandles := make([]*unixfs.FSHandle, nfilenames)
	for i, fileName := range fileNames {
		fileHandle, err := fsHandle.Lookup(ctx, fileName)
		if err != nil {
			return err
		}
		fsHandles[i] = fileHandle
	}

	swap := func(i, j int) error {
		filei, filej := fileNames[i], fileNames[j]
		filek := "file-tmp"

		// XXX: is it possible to swap files without a tmp file?

		fhi, fhj := fsHandles[i], fsHandles[j]
		if err := fhi.Rename(ctx, fsHandle, filek, ts); err != nil {
			return err
		}
		if err := fhj.Rename(ctx, fsHandle, filei, ts); err != nil {
			return err
		}
		if err := fhi.Rename(ctx, fsHandle, filej, ts); err != nil {
			return err
		}

		fileNames[i], fileNames[j] = filej, filei
		return nil
	}

	// rename them randomly
	// rand.Shuffle(nfilenames, swap)
	if !skipRename {
		for x := 0; x < len(fileNames)/2; x++ {
			j := len(fileNames) - x - 1
			if err := swap(x, j); err != nil {
				return err
			}
		}
	}

	// release handles
	for _, h := range fsHandles {
		h.Release()
	}

	// build handles again
	for i, fileName := range fileNames {
		fileHandle, err := fsHandle.Lookup(ctx, fileName)
		if err != nil {
			return err
		}
		fsHandles[i] = fileHandle
	}

	// release handles
	for _, h := range fsHandles {
		h.Release()
	}

	return nil
}
