package sqlite_ncruces_test

import (
	"context"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"

	sqlite3 "github.com/ncruces/go-sqlite3"
	"github.com/ncruces/go-sqlite3/vfs"
	block_store_kvtx "github.com/s4wave/spacewave/db/block/store/kvtx"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_prefixer "github.com/s4wave/spacewave/db/kvtx/prefixer"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	"github.com/s4wave/spacewave/db/store/kvtx/sqlite/common"
	sqlite_ncruces "github.com/s4wave/spacewave/db/store/kvtx/sqlite/ncruces"
	"github.com/s4wave/spacewave/db/volume/crashtest"
	"github.com/s4wave/spacewave/db/volume/device"
)

// TestCrashRecovery crashes the store's block and key-value writes at every
// file mutation of the crash workload, SQLite's and the store's WAL flushes
// included, and checks that the reopened store holds a state its commits
// allow.
func TestCrashRecovery(t *testing.T) {
	crashtest.Run(t, crashtest.ImmediateBlocks, func() *crashDevice {
		return newCrashDevice(t)
	}, func(ctx context.Context, d *crashDevice) (crashtest.Target, error) {
		store, err := common.OpenWithPragmas(ctx, filepath.Join(d.dir, "volume.db"), "kv", common.Pragmas{}, crashConfig{dev: d})
		if err != nil {
			return nil, err
		}
		return &crashTarget{
			Store:     kvtx_prefixer.NewPrefixer(store, []byte("k/")),
			KVTxBlock: block_store_kvtx.NewKVTxBlock(store_kvkey.NewDefaultKVKey(), store, 0, true),
			store:     store,
		}, nil
	})
}

// crashTarget is the block store and a key-value store on one SQLite store.
type crashTarget struct {
	kvtx.Store
	*block_store_kvtx.KVTxBlock

	// store is the SQLite store.
	store *common.Store[crashConfig]
}

// Sync makes every earlier commit durable.
func (c *crashTarget) Sync(ctx context.Context) (bool, error) {
	return true, c.store.Sync(ctx)
}

// Close closes the store.
func (c *crashTarget) Close() error {
	return c.store.Close()
}

// crashConfig opens the ncruces driver on a crash device's VFS and flushes
// the WAL through the device.
type crashConfig struct {
	sqlite_ncruces.SqliteWasmConfig

	// dev is the device the database lives on.
	dev *crashDevice
}

// OpenDSN selects the device's VFS.
func (c crashConfig) OpenDSN(path string) string {
	return c.SqliteWasmConfig.OpenDSN(path) + "&vfs=" + c.dev.name
}

// SyncWAL flushes the device.
func (c crashConfig) SyncWAL(string) error {
	return c.dev.mem.Write(context.Background(), nil, true)
}

// crashVFSCount numbers the registered crash VFSes.
var crashVFSCount atomic.Int64

// crashDevice is a directory whose files SQLite reads and writes through the
// OS VFS while every mutation is also applied to a memory device. The memory
// device decides which mutations a power loss keeps; PowerLoss rewrites the
// directory from its image, which also drops the WAL index a reboot drops.
type crashDevice struct {
	// mem models what a power loss keeps.
	mem *device.Memory
	// dir holds the files.
	dir string
	// name is the VFS name.
	name string
	// os is the VFS the files open through.
	os vfs.VFS
}

// newCrashDevice registers a crash VFS over an empty directory.
func newCrashDevice(t *testing.T) *crashDevice {
	d := &crashDevice{
		mem:  device.NewMemory(),
		dir:  t.TempDir(),
		name: "crash" + strconv.FormatInt(crashVFSCount.Add(1), 10),
		os:   vfs.Find("os"),
	}
	vfs.Register(d.name, d)
	return d
}

// Calls returns the number of mutating calls made so far.
func (d *crashDevice) Calls() int {
	return d.mem.Calls()
}

// CrashAfter fails every mutating call after the next n succeed.
func (d *crashDevice) CrashAfter(n int) {
	d.mem.CrashAfter(n)
}

// PowerLoss replaces the directory with an image a power loss could leave.
func (d *crashDevice) PowerLoss(rng *rand.Rand) {
	// Build the crash image.
	d.mem.PowerLoss(rng)

	// Replace the directory's files with it.
	entries, err := os.ReadDir(d.dir)
	if err != nil {
		panic(err)
	}
	for _, e := range entries {
		if err := os.Remove(filepath.Join(d.dir, e.Name())); err != nil {
			panic(err)
		}
	}
	files, err := d.mem.List(context.Background())
	if err != nil {
		panic(err)
	}
	for _, f := range files {
		data := make([]byte, f.Size)
		if err := d.mem.Read(context.Background(), []device.Read{{Name: f.Name, Data: data}}); err != nil {
			panic(err)
		}
		if err := os.WriteFile(filepath.Join(d.dir, f.Name), data, 0o600); err != nil {
			panic(err)
		}
	}
}

// Open opens a file through the OS VFS, which opens files only by Filename.
func (d *crashDevice) Open(name string, flags vfs.OpenFlag) (vfs.File, vfs.OpenFlag, error) {
	return d.os.Open(name, flags)
}

// OpenFilename opens a file through the OS VFS and mirrors the mutations of a
// named one; a temporary file has nothing to keep.
func (d *crashDevice) OpenFilename(name *vfs.Filename, flags vfs.OpenFlag) (vfs.File, vfs.OpenFlag, error) {
	f, out, err := d.os.(vfs.VFSFilename).OpenFilename(name, flags)
	if err != nil || name == nil {
		return f, out, err
	}
	return &crashFile{File: f, dev: d, name: filepath.Base(name.String())}, out, nil
}

// Delete deletes a file on the device and in the directory.
func (d *crashDevice) Delete(name string, syncDir bool) error {
	if err := d.mem.Remove(context.Background(), []string{filepath.Base(name)}); err != nil {
		return sqlite3.IOERR_DELETE
	}
	return d.os.Delete(name, syncDir)
}

// Access checks a file in the directory.
func (d *crashDevice) Access(name string, flags vfs.AccessFlag) (bool, error) {
	return d.os.Access(name, flags)
}

// FullPathname resolves a path in the directory.
func (d *crashDevice) FullPathname(name string) (string, error) {
	return d.os.FullPathname(name)
}

// crashFile is an OS file whose mutations a crash device also records.
type crashFile struct {
	vfs.File

	// dev records the mutations.
	dev *crashDevice
	// name is the file's device name.
	name string
}

// WriteAt records the write, then writes the file.
func (f *crashFile) WriteAt(p []byte, off int64) (int, error) {
	if err := f.dev.mem.Write(context.Background(), []device.Write{{Name: f.name, Offset: off, Data: p}}, false); err != nil {
		return 0, sqlite3.IOERR_WRITE
	}
	return f.File.WriteAt(p, off)
}

// Truncate records the truncation, then truncates the file.
func (f *crashFile) Truncate(size int64) error {
	if err := f.dev.mem.Truncate(context.Background(), f.name, size); err != nil {
		return sqlite3.IOERR_TRUNCATE
	}
	return f.File.Truncate(size)
}

// Sync flushes the device.
func (f *crashFile) Sync(flags vfs.SyncFlag) error {
	if err := f.dev.mem.Write(context.Background(), nil, true); err != nil {
		return sqlite3.IOERR_FSYNC
	}
	return nil
}

// DeviceCharacteristics drops the ordering and atomicity promises the device
// does not keep.
func (f *crashFile) DeviceCharacteristics() vfs.DeviceCharacteristic {
	return f.File.DeviceCharacteristics() & vfs.IOCAP_POWERSAFE_OVERWRITE
}

// SharedMemory returns the OS file's WAL index.
func (f *crashFile) SharedMemory() vfs.SharedMemory {
	if s, ok := f.File.(vfs.FileSharedMemory); ok {
		return s.SharedMemory()
	}
	return nil
}

// LockState returns the OS file's lock level.
func (f *crashFile) LockState() vfs.LockLevel {
	if s, ok := f.File.(vfs.FileLockState); ok {
		return s.LockState()
	}
	return vfs.LOCK_EXCLUSIVE
}

// Unwrap returns the OS file.
func (f *crashFile) Unwrap() vfs.File {
	return f.File
}

// _ is a type assertion
var (
	_ crashtest.Target          = (*crashTarget)(nil)
	_ crashtest.Device          = (*crashDevice)(nil)
	_ vfs.VFSFilename           = (*crashDevice)(nil)
	_ vfs.FileSharedMemory      = (*crashFile)(nil)
	_ common.SQLiteDriverConfig = crashConfig{}
)
