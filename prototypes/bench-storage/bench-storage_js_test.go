//go:build js

package benchstorage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"strconv"
	"strings"
	"testing"
	"time"

	store_kvtx_indexeddb "github.com/s4wave/spacewave/db/store/kvtx/js/indexeddb"
	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	unixfs "github.com/s4wave/spacewave/db/unixfs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	volume_indexeddb "github.com/s4wave/spacewave/db/volume/js/indexeddb"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/sirupsen/logrus"
	"github.com/zeebo/blake3"
)

var (
	hashBenchSizes    = []int{4 << 10, 64 << 10, 1 << 20}
	storageBenchSizes = []int{4 << 10, 64 << 10}
)

func BenchmarkBlake3(b *testing.B) {
	for _, size := range hashBenchSizes {
		size := size
		b.Run(sizeLabel(size), func(b *testing.B) {
			data := bytes.Repeat([]byte{0x42}, size)
			b.ReportAllocs()
			b.SetBytes(int64(len(data)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				sum := blake3.Sum256(data)
				if sum[0] == 0xff {
					b.Fatal("unexpected checksum")
				}
			}
		})
	}
}

func BenchmarkSHA256(b *testing.B) {
	for _, size := range hashBenchSizes {
		size := size
		b.Run(sizeLabel(size), func(b *testing.B) {
			data := bytes.Repeat([]byte{0x24}, size)
			b.ReportAllocs()
			b.SetBytes(int64(len(data)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				sum := sha256.Sum256(data)
				if sum[0] == 0xff {
					b.Fatal("unexpected checksum")
				}
			}
		})
	}
}

func BenchmarkIndexedDBKVWrite(b *testing.B) {
	for _, size := range storageBenchSizes {
		size := size
		b.Run(sizeLabel(size), func(b *testing.B) {
			tb := newIndexedDBTestbed(b)
			vol, ok := tb.Volume.(volume_kvtx.KvtxVolume)
			if !ok {
				b.Fatal("volume does not implement KvtxVolume")
			}

			ctx := context.Background()
			data := bytes.Repeat([]byte{0x61}, size)
			keys := buildKeys("bench/kv/", b.N)

			b.ReportAllocs()
			b.SetBytes(int64(len(data)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				tx, err := vol.GetKvtxStore().NewTransaction(ctx, true)
				if err != nil {
					b.Fatal(err.Error())
				}
				if err := tx.Set(ctx, keys[i], data); err != nil {
					tx.Discard()
					b.Fatal(err.Error())
				}
				if err := tx.Commit(ctx); err != nil {
					tx.Discard()
					b.Fatal(err.Error())
				}
			}
		})
	}
}

// BenchmarkIndexedDBKVWriteSingleTx writes all values in a single raw IndexedDB
// transaction (no txcache), committing only once at the end. The final commit
// is excluded from the timed region. This isolates IndexedDB put latency from
// transaction commit overhead.
func BenchmarkIndexedDBKVWriteSingleTx(b *testing.B) {
	for _, size := range storageBenchSizes {
		size := size
		b.Run(sizeLabel(size), func(b *testing.B) {
			tb := newIndexedDBTestbed(b)
			vol, ok := tb.Volume.(volume_kvtx.KvtxVolume)
			if !ok {
				b.Fatal("volume does not implement KvtxVolume")
			}
			store, ok := vol.GetKvtxStore().(*store_kvtx_indexeddb.Store)
			if !ok {
				b.Fatal("kvtx store is not *store_kvtx_indexeddb.Store")
			}

			ctx := context.Background()
			data := bytes.Repeat([]byte{0x61}, size)
			keys := buildKeys("bench/kv-single/", b.N)

			tx, err := store.NewRawWriteTransaction(ctx)
			if err != nil {
				b.Fatal(err.Error())
			}

			b.ReportAllocs()
			b.SetBytes(int64(len(data)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := tx.Set(ctx, keys[i], data); err != nil {
					tx.Discard()
					b.Fatal(err.Error())
				}
			}
			b.StopTimer()

			if err := tx.Commit(ctx); err != nil {
				tx.Discard()
				b.Fatal(err.Error())
			}
		})
	}
}

// BenchmarkIndexedDBKVReadTxPerOp reads a value using a fresh read-only
// transaction for each operation, measuring per-transaction read overhead.
func BenchmarkIndexedDBKVReadTxPerOp(b *testing.B) {
	for _, size := range storageBenchSizes {
		size := size
		b.Run(sizeLabel(size), func(b *testing.B) {
			tb := newIndexedDBTestbed(b)
			vol, ok := tb.Volume.(volume_kvtx.KvtxVolume)
			if !ok {
				b.Fatal("volume does not implement KvtxVolume")
			}
			store, ok := vol.GetKvtxStore().(*store_kvtx_indexeddb.Store)
			if !ok {
				b.Fatal("kvtx store is not *store_kvtx_indexeddb.Store")
			}

			ctx := context.Background()
			data := bytes.Repeat([]byte{0x61}, size)
			keys := buildKeys("bench/kvr/", b.N)

			// seed the data with a single raw write transaction
			tx, err := store.NewRawWriteTransaction(ctx)
			if err != nil {
				b.Fatal(err.Error())
			}
			for i := 0; i < b.N; i++ {
				if err := tx.Set(ctx, keys[i], data); err != nil {
					tx.Discard()
					b.Fatal(err.Error())
				}
			}
			if err := tx.Commit(ctx); err != nil {
				tx.Discard()
				b.Fatal(err.Error())
			}

			b.ReportAllocs()
			b.SetBytes(int64(len(data)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				rtx, err := vol.GetKvtxStore().NewTransaction(ctx, false)
				if err != nil {
					b.Fatal(err.Error())
				}
				val, found, err := rtx.Get(ctx, keys[i])
				if err != nil {
					rtx.Discard()
					b.Fatal(err.Error())
				}
				if !found || len(val) != size {
					rtx.Discard()
					b.Fatal("expected value not found")
				}
				rtx.Discard()
			}
		})
	}
}

// BenchmarkIndexedDBKVReadSingleTx reads all values in a single read-only
// transaction, measuring IndexedDB get latency without per-op transaction
// overhead.
func BenchmarkIndexedDBKVReadSingleTx(b *testing.B) {
	for _, size := range storageBenchSizes {
		size := size
		b.Run(sizeLabel(size), func(b *testing.B) {
			tb := newIndexedDBTestbed(b)
			vol, ok := tb.Volume.(volume_kvtx.KvtxVolume)
			if !ok {
				b.Fatal("volume does not implement KvtxVolume")
			}
			store, ok := vol.GetKvtxStore().(*store_kvtx_indexeddb.Store)
			if !ok {
				b.Fatal("kvtx store is not *store_kvtx_indexeddb.Store")
			}

			ctx := context.Background()
			data := bytes.Repeat([]byte{0x61}, size)
			keys := buildKeys("bench/kvrs/", b.N)

			// seed the data
			tx, err := store.NewRawWriteTransaction(ctx)
			if err != nil {
				b.Fatal(err.Error())
			}
			for i := 0; i < b.N; i++ {
				if err := tx.Set(ctx, keys[i], data); err != nil {
					tx.Discard()
					b.Fatal(err.Error())
				}
			}
			if err := tx.Commit(ctx); err != nil {
				tx.Discard()
				b.Fatal(err.Error())
			}

			rtx, err := vol.GetKvtxStore().NewTransaction(ctx, false)
			if err != nil {
				b.Fatal(err.Error())
			}

			b.ReportAllocs()
			b.SetBytes(int64(len(data)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				val, found, err := rtx.Get(ctx, keys[i])
				if err != nil {
					rtx.Discard()
					b.Fatal(err.Error())
				}
				if !found || len(val) != size {
					rtx.Discard()
					b.Fatal("expected value not found")
				}
			}
			b.StopTimer()
			rtx.Discard()
		})
	}
}

func BenchmarkIndexedDBUnixFSWriteFile(b *testing.B) {
	for _, size := range storageBenchSizes {
		size := size
		b.Run(sizeLabel(size), func(b *testing.B) {
			wtb := newIndexedDBWorldTestbed(b)
			ctx := context.Background()
			objKey := "bench/fs"
			initHandle, err := initUnixFSTestbed(ctx, wtb, objKey, true)
			if err != nil {
				b.Fatal(err.Error())
			}
			b.Cleanup(initHandle.Release)

			data := bytes.Repeat([]byte{0x7a}, size)
			names := buildNames("bench-file-", ".bin", b.N)
			sender := wtb.Volume.GetPeerID()
			fsType := unixfs_world.FSType_FSType_FS_NODE

			b.ReportAllocs()
			b.SetBytes(int64(len(data)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				wtx, err := wtb.Engine.NewTransaction(ctx, true)
				if err != nil {
					b.Fatal(err.Error())
				}

				fsCursor, _ := unixfs_world.NewFSCursorWithWriter(
					ctx,
					wtb.Logger,
					wtx,
					objKey,
					fsType,
					sender,
				)
				handle, err := unixfs.NewFSHandle(fsCursor)
				if err != nil {
					wtx.Discard()
					b.Fatal(err.Error())
				}

				err = handle.MknodWithContent(
					ctx,
					names[i],
					unixfs.NewFSCursorNodeType_File(),
					int64(len(data)),
					bytes.NewReader(data),
					0o644,
					time.Time{},
				)
				handle.Release()
				if err != nil {
					wtx.Discard()
					b.Fatal(err.Error())
				}
				if err := wtx.Commit(ctx); err != nil {
					wtx.Discard()
					b.Fatal(err.Error())
				}
			}
		})
	}
}

func newIndexedDBTestbed(b *testing.B) *hydra_testbed.Testbed {
	b.Helper()

	ctx := context.Background()
	tb, err := hydra_testbed.NewTestbed(
		ctx,
		newBenchmarkLogger(),
		hydra_testbed.WithVolumeConfig(&volume_indexeddb.Config{
			DatabaseName: buildDatabaseName(b.Name()),
			NoWriteKey:   true,
		}),
	)
	if err != nil {
		b.Fatal(err.Error())
	}
	b.Cleanup(tb.Release)
	b.Cleanup(func() {
		_ = tb.Volume.Delete()
	})
	return tb
}

func newIndexedDBWorldTestbed(b *testing.B) *world_testbed.Testbed {
	b.Helper()

	htb := newIndexedDBTestbed(b)
	wtb, err := world_testbed.NewTestbed(htb)
	if err != nil {
		b.Fatal(err.Error())
	}
	b.Cleanup(wtb.Release)
	return wtb
}

func initUnixFSTestbed(
	ctx context.Context,
	tb *world_testbed.Testbed,
	objKey string,
	watchWorldChanges bool,
) (*unixfs.FSHandle, error) {
	engineID := tb.EngineID
	opc := world.NewLookupOpController("bench-fs-ops", engineID, unixfs_world.LookupFsOp)
	opcRef, err := tb.Bus.AddController(ctx, opc, nil)
	if err != nil {
		return nil, err
	}
	tb.AddReleaseFunc(opcRef)

	<-time.After(100 * time.Millisecond)

	ws := world.NewEngineWorldState(tb.Engine, true)
	sender := tb.Volume.GetPeerID()
	fsType := unixfs_world.FSType_FSType_FS_NODE
	typeID, _ := unixfs_world.FSTypeToTypeID(fsType)
	_, _, err = unixfs_world.FsInit(
		ctx,
		ws,
		sender,
		objKey,
		fsType,
		nil,
		true,
		time.Time{},
	)
	if err != nil {
		return nil, err
	}
	if err := world_types.CheckObjectType(ctx, ws, objKey, typeID); err != nil {
		return nil, err
	}

	rootCursor, err := unixfs_world.FollowUnixfsRef(
		ctx,
		tb.Logger,
		ws,
		&unixfs_world.UnixfsRef{ObjectKey: objKey},
		sender,
		watchWorldChanges,
	)
	if err != nil {
		return nil, err
	}
	return unixfs.NewFSHandle(rootCursor)
}

func newBenchmarkLogger() *logrus.Entry {
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	return logrus.NewEntry(log)
}

func buildDatabaseName(name string) string {
	var b strings.Builder
	b.Grow(len(name) + 40)
	b.WriteString("alpha/bench-storage/")
	b.WriteString(strings.ReplaceAll(name, "/", "-"))
	b.WriteByte('-')
	b.WriteString(strconv.FormatInt(time.Now().UnixNano(), 10))
	return b.String()
}

func buildKeys(prefix string, n int) [][]byte {
	keys := make([][]byte, n)
	for i := range n {
		key := make([]byte, 0, len(prefix)+24)
		key = append(key, prefix...)
		key = strconv.AppendInt(key, int64(i), 10)
		keys[i] = key
	}
	return keys
}

func buildNames(prefix, suffix string, n int) []string {
	names := make([]string, n)
	for i := range n {
		names[i] = prefix + strconv.Itoa(i) + suffix
	}
	return names
}

func sizeLabel(size int) string {
	if size >= 1<<20 {
		return strconv.Itoa(size/(1<<20)) + "MiB"
	}
	if size >= 1<<10 {
		return strconv.Itoa(size/(1<<10)) + "KiB"
	}
	return strconv.Itoa(size) + "B"
}
