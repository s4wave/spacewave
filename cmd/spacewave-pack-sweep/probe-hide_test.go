package main

import (
	"bytes"
	"slices"
	"testing"

	"github.com/aperturerobotics/fastjson"
	"github.com/s4wave/spacewave/db/block/store/s3/s3test"
)

// TestCommandProbeHide checks the hide probe on the in-memory bucket: every
// answer passes, the scratch key stays in place with its one version current,
// and no content version is deleted. A listing oldest first shows as a
// difference, deleting nothing, and the probe refuses a key under the store
// prefix and a key that exists before any write.
func TestCommandProbeHide(t *testing.T) {
	// Run the probe on one key of the bucket, under the world/ store.
	bucket, client := s3test.New(t)
	run := func(key string) (*fastjson.Value, error) {
		var out bytes.Buffer
		args := &Args{out: &out}
		args.probe = probeArgs{Bucket: s3test.BucketName, StorePrefix: "world/", ScratchPrefix: "probe/", client: client, key: key}
		runErr := args.runProbeHide(nil)
		if out.Len() == 0 {
			return nil, runErr
		}
		res, err := fastjson.ParseBytes(out.Bytes())
		if err != nil {
			t.Fatalf("probe printed %q: %v", out.String(), err)
		}
		return res, runErr
	}

	// Every answer passes, and the key holds its one version, current.
	res, err := run("probe/a")
	t.Logf("probe: %s", res)
	if err != nil || res.GetInt("failed") != 0 || len(res.GetArray("steps")) < 10 {
		t.Fatalf("probe = %s, %v", res, err)
	}
	if len(res.GetStringBytes("version")) == 0 || len(res.GetStringBytes("marker")) == 0 {
		t.Fatalf("probe named no version ids: %s", res)
	}
	if got := bucket.Versions("probe/"); !slices.Equal(got, []string{"probe/a " + probeData}) {
		t.Fatalf("versions after the probe = %v", got)
	}

	// A listing oldest first is a difference: the hide's check after its
	// marker refuses it, and the content version stays.
	bucket.ListOldestFirst(true)
	res, err = run("probe/b")
	bucket.ListOldestFirst(false)
	t.Logf("oldest-first probe: %s, %v", res, err)
	if err == nil || res.GetInt("failed") == 0 {
		t.Fatalf("oldest-first probe = %s, %v, want a difference", res, err)
	}
	if got := bucket.Versions("probe/b"); !slices.Contains(got, "probe/b "+probeData) || bucket.ContentDeleted("probe/") != 0 {
		t.Fatalf("versions after the oldest-first probe = %v", got)
	}

	// A key under the store prefix, or one that exists, is refused before
	// any write.
	puts := bucket.Puts()
	if _, err := run("world/packs/probe"); err == nil {
		t.Fatal("probed a key under the store prefix")
	}
	if _, err := run("probe/a"); err == nil {
		t.Fatal("probed a key that exists")
	}
	if bucket.Puts() != puts || bucket.ContentDeleted("") != 0 {
		t.Fatalf("refused probes wrote %d objects and deleted %d versions", bucket.Puts()-puts, bucket.ContentDeleted(""))
	}
}
