package git_block

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/go-git/go-git/v6/plumbing"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/sirupsen/logrus"
)

const benchmarkEncodedObjectCount = 4096

func BenchmarkStoreSetEncodedObjectCommit(b *testing.B) {
	ctx := context.Background()
	objects, payloadBytes := buildBenchmarkEncodedObjects(b, benchmarkEncodedObjectCount)

	b.ReportAllocs()
	b.SetBytes(payloadBytes)
	for b.Loop() {
		b.StopTimer()
		store, release := newBenchmarkStore(b, ctx)
		b.StartTimer()

		for _, obj := range objects {
			if _, err := store.SetEncodedObject(obj); err != nil {
				release()
				b.Fatalf("SetEncodedObject failed: %v", err)
			}
		}
		if err := store.Commit(); err != nil {
			release()
			b.Fatalf("Commit failed: %v", err)
		}

		b.StopTimer()
		release()
		b.StartTimer()
	}
}

func newBenchmarkStore(b *testing.B, ctx context.Context) (*Store, func()) {
	b.Helper()

	log := logrus.New()
	log.SetLevel(logrus.WarnLevel)
	le := logrus.NewEntry(log)
	tb, err := testbed.NewTestbed(ctx, le, testbed.WithVerbose(false))
	if err != nil {
		b.Fatal(err.Error())
	}

	oc, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		tb.Release()
		b.Fatal(err.Error())
	}

	btx, bcs := oc.BuildTransaction(nil)
	bcs.SetBlock(NewRepo(), true)

	store, err := NewStore(ctx, btx, bcs, nil, nil)
	if err != nil {
		tb.Release()
		b.Fatal(err.Error())
	}

	return store, func() {
		_ = store.Close()
		tb.Release()
	}
}

func buildBenchmarkEncodedObjects(b *testing.B, count int) ([]plumbing.EncodedObject, int64) {
	b.Helper()

	objects := make([]plumbing.EncodedObject, 0, count)
	seen := make(map[plumbing.Hash]struct{}, count)
	var payloadBytes int64

	for i := range count {
		data := benchmarkObjectPayload(i)
		obj := plumbing.NewMemoryObject(nil)
		obj.SetType(benchmarkObjectType(i))
		obj.SetSize(int64(len(data)))

		writer, err := obj.Writer()
		if err != nil {
			b.Fatal(err.Error())
		}
		n, err := writer.Write(data)
		if err != nil {
			b.Fatal(err.Error())
		}
		if n != len(data) {
			b.Fatalf("wrote %d payload bytes, expected %d", n, len(data))
		}
		if err := writer.Close(); err != nil {
			b.Fatal(err.Error())
		}

		hash := obj.Hash()
		if _, ok := seen[hash]; ok {
			b.Fatalf("synthetic object %d duplicated git hash %s", i, hash)
		}
		seen[hash] = struct{}{}
		objects = append(objects, obj)
		payloadBytes += int64(len(data))
	}

	if len(objects) != count {
		b.Fatalf("built %d encoded objects, expected %d", len(objects), count)
	}
	return objects, payloadBytes
}

func benchmarkObjectType(i int) plumbing.ObjectType {
	switch i % 16 {
	case 0:
		return plumbing.CommitObject
	case 1:
		return plumbing.TreeObject
	case 2:
		return plumbing.TagObject
	default:
		return plumbing.BlobObject
	}
}

func benchmarkObjectPayload(i int) []byte {
	size := 128 + (i%29)*37 + (i%7)*11
	data := make([]byte, size)
	state := uint64(0x9e3779b97f4a7c15) ^ (uint64(i) * 0xbf58476d1ce4e5b9)
	for j := range data {
		state = state*6364136223846793005 + 1442695040888963407 + uint64(j)
		data[j] = byte(state >> 56)
	}

	binary.LittleEndian.PutUint64(data[0:8], uint64(i))
	binary.LittleEndian.PutUint64(data[8:16], uint64(size))
	copy(data[16:], "hydra synthetic git object benchmark payload")
	return data
}
