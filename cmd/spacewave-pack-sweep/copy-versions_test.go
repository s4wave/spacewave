package main

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/fastjson"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/block/store/s3/s3test"
)

// TestCommandCopyVersions checks the version copy on the in-memory bucket: a
// dry run counts and writes nothing, the copy finishes a key a stopped copy
// left partway and gives the destination the source's version listing, hide
// markers included, and a second run skips every key. It refuses a
// destination holding other bytes of the same sizes, a destination inside the
// source prefixes, and a bucket without versioning.
func TestCommandCopyVersions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Write a key with three versions, a hidden key with one version under
	// its marker, a key hidden and written again, and a key outside the
	// prefixes.
	bucket, client := s3test.New(t)
	put := func(key, data string) {
		if err := client.PutObject(ctx, s3test.BucketName, key, []byte(data), ""); err != nil {
			t.Fatal(err)
		}
	}
	put("world/packs/a", "a1")
	put("world/packs/a", "a22")
	put("world/packs/a", "a333")
	put("world/packs/b", "b1")
	bucket.Hide("world/packs/b")
	put("world/entries/c", "c1")
	bucket.Hide("world/entries/c")
	put("world/entries/c", "c22")
	put("other/d", "d1")
	source := bucket.Versions("world/")

	// Leave the first key as a stopped copy would: its oldest version alone.
	var oldest string
	if err := client.ListObjectVersions(ctx, s3test.BucketName, "world/packs/a", func(v block_store_s3.ObjectVersion) error {
		oldest = v.VersionID
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := client.CopyObjectVersion(ctx, s3test.BucketName, "world/packs/a", oldest, s3test.BucketName, "copy/world/packs/a"); err != nil {
		t.Fatal(err)
	}

	// Run the command against the bucket, by default from the two prefixes
	// into the copy/ prefix.
	run := func(dryRun bool, prefixes []string, toPrefix string) (*fastjson.Value, error) {
		var out bytes.Buffer
		args := &Args{out: &out}
		args.versionCopy = copyArgs{
			From:     s3test.BucketName,
			To:       s3test.BucketName,
			Prefixes: *cli.NewStringSlice(prefixes...),
			ToPrefix: toPrefix,
			DryRun:   dryRun,
			client:   client,
		}
		runErr := args.runCopyVersions(nil)
		if out.Len() == 0 {
			return nil, runErr
		}
		res, err := fastjson.ParseBytes(out.Bytes())
		if err != nil {
			t.Fatalf("copy printed %q: %v", out.String(), err)
		}
		return res, runErr
	}
	prefixes := []string{"world/packs/", "world/entries/"}
	copyAll := func(dryRun bool) *fastjson.Value {
		res, err := run(dryRun, prefixes, "copy/")
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	counts := func(res *fastjson.Value) []uint64 {
		var out []uint64
		for _, name := range []string{"keys", "versions", "markers", "bytes", "skipped", "resumed"} {
			out = append(out, res.GetUint64(name))
		}
		return out
	}

	// The dry run counts three keys, one of them resumed, the five versions
	// of 14 bytes the destination lacks and two markers, and writes nothing.
	before := bucket.Versions("copy/")
	want := []uint64{3, 5, 2, 3 + 4 + 2 + 2 + 3, 0, 1}
	if got := counts(copyAll(true)); !slices.Equal(got, want) {
		t.Fatalf("dry run = %v, want %v", got, want)
	}
	if got := bucket.Versions("copy/"); !slices.Equal(got, before) {
		t.Fatalf("dry run wrote %v", got)
	}

	// The copy writes the same, and the destination lists the source's
	// versions in the same order.
	if got := counts(copyAll(false)); !slices.Equal(got, want) {
		t.Fatalf("copy = %v, want %v", got, want)
	}
	var copied []string
	for _, v := range bucket.Versions("copy/") {
		copied = append(copied, strings.TrimPrefix(v, "copy/"))
	}
	var wantCopied []string
	for _, v := range source {
		if !strings.HasPrefix(v, "other/") {
			wantCopied = append(wantCopied, v)
		}
	}
	if !slices.Equal(copied, wantCopied) {
		t.Fatalf("copied versions = %v, want %v", copied, wantCopied)
	}
	if keys := bucket.Keys("copy/"); !slices.Equal(keys, []string{"copy/world/entries/c", "copy/world/packs/a"}) {
		t.Fatalf("visible copied keys = %v", keys)
	}

	// A second run finds every key copied and skips it.
	if got := counts(copyAll(false)); !slices.Equal(got, []uint64{0, 0, 0, 0, 3, 0}) {
		t.Fatalf("second run = %v, want three skipped", got)
	}

	// A destination holding other bytes of the same size is a conflict, left
	// alone, and fails the run.
	put("world/entries/e", "e1")
	put("copy/world/entries/e", "x1")
	res, err := run(false, prefixes, "copy/")
	if err == nil || res.GetUint64("conflicts") != 1 || string(res.GetStringBytes("conflict_keys", "0")) != "world/entries/e" {
		t.Fatalf("other bytes = %s, %v, want one conflict", res, err)
	}

	// A destination inside the source prefixes in the same bucket is refused
	// before any write.
	copies := bucket.Copies()
	if _, err := run(false, []string{"world/"}, "world/copy/"); err == nil || bucket.Copies() != copies {
		t.Fatalf("overlapping copy = %v after %d copies, want refused", err, bucket.Copies()-copies)
	}

	// A bucket without versioning is refused before any write.
	bucket.SetVersioning(false)
	if _, err := run(false, prefixes, "fresh/"); err == nil || bucket.Copies() != copies {
		t.Fatalf("unversioned copy = %v after %d copies, want refused", err, bucket.Copies()-copies)
	}
}
