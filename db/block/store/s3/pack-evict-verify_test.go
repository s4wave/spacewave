//go:build !tinygo

package block_store_s3

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

// copyCache is a read cache holding a true copy of every pack, counting the
// reads it answers.
type copyCache struct {
	packs map[string][]byte
	reads atomic.Int64
}

// ReadCached answers from the copy.
func (c *copyCache) ReadCached(_ context.Context, id string, off int64, length int) ([]byte, bool) {
	c.reads.Add(1)
	data, ok := c.packs[id]
	if !ok || off+int64(length) > int64(len(data)) {
		return nil, false
	}
	return data[off : off+int64(length)], true
}

// TestVerifyPackReadsTheBucket checks that VerifyPack answers for the bucket
// alone: with the local index placing every block in the pack and a read
// cache holding a true copy of it, a pack the bucket lost, cut short, holds
// with other bytes of the same size, holds with a broken index, or holds
// larger than recorded fails with its reason, and the cache is never read.
func TestVerifyPackReadsTheBucket(t *testing.T) {
	cases := []struct {
		name   string
		lie    func(data string) (string, bool)
		reason string
		// noSum removes the SHA-256 recorded at upload, as for a pack
		// written before the record, so the values' own hashes must catch
		// the lie.
		noSum bool
	}{
		{"good", func(data string) (string, bool) { return data, true }, "", false},
		{"good, no recorded sum", func(data string) (string, bool) { return data, true }, "", true},
		{"missing", func(string) (string, bool) { return "", false }, VerifyMissing, false},
		{"truncated", func(data string) (string, bool) { return data[:len(data)/2], true }, VerifyShort, false},
		{"same size other bytes", func(data string) (string, bool) {
			return strings.Replace(data, "v block 1", "v block X", 1), true
		}, VerifyHash, false},
		{"same size other bytes, no recorded sum", func(data string) (string, bool) {
			return strings.Replace(data, "v block 1", "v block X", 1), true
		}, VerifyHash, true},
		{"broken index", func(data string) (string, bool) {
			return data[:len(data)-16] + strings.Repeat("\xff", 16), true
		}, VerifyIndex, true},
		{"larger than recorded", func(data string) (string, bool) {
			return data + strings.Repeat("x", 2*verifySlackBytes), true
		}, VerifyOversize, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx := t.Context()
			bucket, client := newFakeBucket(t, nil)
			kv := hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())
			cache := &copyCache{packs: make(map[string][]byte)}
			s := NewPackStore(logrus.NewEntry(logrus.New()), client, "bucket", "p/", &PackStoreOptions{KeepPacks: true, Index: kv, SoleWriter: true, ReadCache: cache})
			t.Cleanup(s.Close)
			refs := adoptBlocks(ctx, t, s, "v", 3)

			// Locate the pack from the local index, and keep its true copy.
			hashes := make([]*hash.Hash, len(refs))
			for i, ref := range refs {
				hashes[i] = ref.GetHash()
			}
			locs, err := s.LocatePacks(ctx, hashes)
			if err != nil {
				t.Fatal(err)
			}
			loc := locs[0]
			if loc.ID == "" || locs[1] != loc || locs[2] != loc {
				t.Fatalf("locations = %+v, want one pack", locs)
			}

			// The store holds every block before the bucket lies.
			exists, err := s.GetBlockExistsBatch(ctx, refs)
			if err != nil {
				t.Fatal(err)
			}
			for i, ok := range exists {
				if !ok {
					t.Fatalf("store lost block %d before the lie", i)
				}
			}
			key := "p/" + packDir + loc.ID
			bucket.mtx.Lock()
			data := bucket.objects[key]
			cache.packs[loc.ID] = []byte(data)

			// Drop the SHA-256 recorded at upload, for a pack written before
			// the record.
			if c.noSum {
				if err := deleteKey(ctx, kv, indexSumPrefix+loc.ID); err != nil {
					bucket.mtx.Unlock()
					t.Fatal(err)
				}
			}

			// Make the bucket lie.
			if lied, ok := c.lie(data); ok {
				if c.reason != "" && lied == data {
					bucket.mtx.Unlock()
					t.Fatal("the lie left the pack unchanged")
				}
				bucket.objects[key] = lied
			} else {
				delete(bucket.objects, key)
			}
			bucket.mtx.Unlock()

			// The local index still places every block in the pack.
			if after, err := s.LocatePacks(ctx, hashes); err != nil || after[0] != loc || after[1] != loc || after[2] != loc {
				t.Fatalf("local index after the lie: %+v, %v", after, err)
			}

			// VerifyPack answers for the bucket, and never reads the cache.
			cacheReads := cache.reads.Load()
			values, err := s.VerifyPack(ctx, loc.ID, loc.Size)
			if cache.reads.Load() != cacheReads {
				t.Fatal("VerifyPack read the cache")
			}
			if c.reason == "" {
				if err != nil || len(values) != len(refs) {
					t.Fatalf("good pack: %d values, %v", len(values), err)
				}
				for i, v := range values {
					found := false
					for _, ref := range refs {
						found = found || ref.GetHash().CompareHash(v.Hash)
					}
					if !found || v.Size != int64(len("v block 0")) {
						t.Fatalf("value %d = %+v", i, v)
					}
				}
				return
			}
			var failed *PackVerifyError
			if !errors.As(err, &failed) || failed.Reason != c.reason || values != nil {
				t.Fatalf("VerifyPack = %d values, %v; want reason %q", len(values), err, c.reason)
			}
		})
	}
}

// noStatObjects is an object store that reports no sizes, as SFTP and FTPS.
type noStatObjects struct {
	ObjectStore
}

// TestVerifyPackRefusesLongerObjectWithoutStat checks that a backend that
// reports no sizes still has an object longer than the recorded size plus the
// slack refused, though its first bytes are the pack as uploaded, and that
// one longer by less than the slack verifies from the recorded bytes.
func TestVerifyPackRefusesLongerObjectWithoutStat(t *testing.T) {
	for _, c := range []struct {
		extra  int
		reason string
	}{{verifySlackBytes + 1, VerifyOversize}, {verifySlackBytes, ""}} {
		ctx := t.Context()
		bucket, client := newFakeBucket(t, nil)
		kv := hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())
		s := NewPackStore(logrus.NewEntry(logrus.New()), noStatObjects{client}, "bucket", "p/", &PackStoreOptions{KeepPacks: true, Index: kv, SoleWriter: true})
		t.Cleanup(s.Close)
		refs := adoptBlocks(ctx, t, s, "w", 3)

		// Append past the recorded size.
		locs, err := s.LocatePacks(ctx, []*hash.Hash{refs[0].GetHash()})
		if err != nil || locs[0].ID == "" {
			t.Fatalf("locate: %+v, %v", locs, err)
		}
		key := "p/" + packDir + locs[0].ID
		bucket.mtx.Lock()
		bucket.objects[key] += strings.Repeat("x", c.extra)
		bucket.mtx.Unlock()

		// Only the object past the slack is refused.
		values, err := s.VerifyPack(ctx, locs[0].ID, locs[0].Size)
		var failed *PackVerifyError
		switch {
		case c.reason == "" && (err != nil || len(values) != len(refs)):
			t.Fatalf("extra %d: %d values, %v", c.extra, len(values), err)
		case c.reason != "" && (!errors.As(err, &failed) || failed.Reason != c.reason):
			t.Fatalf("extra %d: VerifyPack = %v, want %s", c.extra, err, c.reason)
		}
	}
}

// TestVerifyPackRefusesHugeRecordedSize checks a recorded size past the
// largest pack is refused before any request.
func TestVerifyPackRefusesHugeRecordedSize(t *testing.T) {
	_, client := newFakeBucket(t, nil)
	kv := hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())
	s := newIndexedStore(t, client, kv)
	_, err := s.VerifyPack(t.Context(), "x", 1<<40)
	var failed *PackVerifyError
	if !errors.As(err, &failed) || failed.Reason != VerifyOversize {
		t.Fatalf("VerifyPack = %v, want oversize", err)
	}
}

// TestPublishPackSendsChecksum checks a pack's upload carries its SHA-256 in
// the X-Amz-Checksum-Sha256 header only with UploadChecksum, and that the
// store records it either way.
func TestPublishPackSendsChecksum(t *testing.T) {
	for _, on := range []bool{false, true} {
		ctx := t.Context()
		bucket, client := newFakeBucket(t, nil)
		var sent []string
		bucket.after = func(r *http.Request) {
			if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/"+packDir) {
				sent = append(sent, r.Header.Get(ChecksumSHA256Header))
			}
		}
		kv := hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())
		s := NewPackStore(logrus.NewEntry(logrus.New()), client, "bucket", "p/", &PackStoreOptions{KeepPacks: true, Index: kv, SoleWriter: true, UploadChecksum: on})
		t.Cleanup(s.Close)
		refs := adoptBlocks(ctx, t, s, "v", 3)
		locs, err := s.LocatePacks(ctx, []*hash.Hash{refs[0].GetHash()})
		if err != nil {
			t.Fatal(err)
		}
		bucket.mtx.Lock()
		data := bucket.objects["p/"+packDir+locs[0].ID]
		bucket.mtx.Unlock()
		sum := sha256.Sum256([]byte(data))
		recorded, err := s.index.sum(ctx, locs[0].ID)
		if err != nil || !bytes.Equal(recorded, sum[:]) {
			t.Fatalf("checksum %v: recorded %x, %v; want %x", on, recorded, err, sum)
		}
		want := ""
		if on {
			want = base64.StdEncoding.EncodeToString(sum[:])
		}
		if len(sent) != 1 || sent[0] != want {
			t.Fatalf("checksum %v: headers %q, want [%q]", on, sent, want)
		}
	}
}

// deleteKey deletes key from kv.
func deleteKey(ctx context.Context, kv kvtx.Store, key string) error {
	tx, err := kv.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()
	if err := tx.Delete(ctx, []byte(key)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
