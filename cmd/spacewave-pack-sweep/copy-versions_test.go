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
	"github.com/s4wave/spacewave/db/block/store/s3/s3test"
)

// TestCommandCopyVersions checks the version copy on the in-memory bucket: a
// dry run counts and writes nothing, the copy gives the destination the
// source's version listing, hide markers included, and a second run skips
// every key.
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

	// Run the command against the bucket, into the copy/ prefix.
	run := func(dryRun bool) *fastjson.Value {
		var out bytes.Buffer
		args := &Args{out: &out}
		args.versionCopy = copyArgs{
			From:     s3test.BucketName,
			To:       s3test.BucketName,
			Prefixes: *cli.NewStringSlice("world/packs/", "world/entries/"),
			ToPrefix: "copy/",
			DryRun:   dryRun,
			client:   client,
		}
		if err := args.runCopyVersions(nil); err != nil {
			t.Fatal(err)
		}
		res, err := fastjson.ParseBytes(out.Bytes())
		if err != nil {
			t.Fatalf("copy printed %q: %v", out.String(), err)
		}
		return res
	}
	counts := func(res *fastjson.Value) []uint64 {
		var out []uint64
		for _, name := range []string{"keys", "versions", "markers", "bytes", "skipped"} {
			out = append(out, res.GetUint64(name))
		}
		return out
	}

	// The dry run counts three keys, six versions of 16 bytes and two
	// markers, and writes nothing.
	want := []uint64{3, 6, 2, 2 + 3 + 4 + 2 + 2 + 3, 0}
	if got := counts(run(true)); !slices.Equal(got, want) {
		t.Fatalf("dry run = %v, want %v", got, want)
	}
	if got := bucket.Versions("copy/"); len(got) != 0 {
		t.Fatalf("dry run wrote %v", got)
	}

	// The copy writes the same, and the destination lists the source's
	// versions in the same order.
	if got := counts(run(false)); !slices.Equal(got, want) {
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
	if got := counts(run(false)); !slices.Equal(got, []uint64{0, 0, 0, 0, 3}) {
		t.Fatalf("second run = %v, want three skipped", got)
	}
}
