package order

import (
	"bytes"
	"testing"

	"github.com/s4wave/spacewave/db/block"
)

func TestAccessOrderRecordBinaryRoundTripPreservesEntries(t *testing.T) {
	refA := testRef(t, "entry-a")
	refB := testRef(t, "entry-b")
	record := &AccessOrderRecord{
		ManifestId:      "manifest-1",
		PlatformId:      "darwin-arm64",
		BuildType:       "release",
		ManifestRootRef: testRef(t, "manifest-root"),
		ManifestRev:     7,
		GenerationId:    "profile-run-1",
		Entries: []*AccessOrderEntry{
			{
				Ordinal:      0,
				Filesystem:   AccessOrderFilesystem_ACCESS_ORDER_FILESYSTEM_DIST,
				Path:         "app/main.js",
				Reason:       AccessOrderReason_ACCESS_ORDER_REASON_ENTRYPOINT,
				ReasonDetail: "startup",
				AccessCount:  1,
				ResolvedRefs: []*block.BlockRef{refA, refB},
			},
			{
				Ordinal:      1,
				Filesystem:   AccessOrderFilesystem_ACCESS_ORDER_FILESYSTEM_DIST,
				Path:         "app/main.js",
				Reason:       AccessOrderReason_ACCESS_ORDER_REASON_DYNAMIC_IMPORT,
				ReasonDetail: "./chunk.js",
				AccessCount:  2,
				ResolvedRefs: nil,
			},
			{
				Ordinal:      2,
				Filesystem:   AccessOrderFilesystem_ACCESS_ORDER_FILESYSTEM_ASSETS,
				Path:         "assets/logo.svg",
				Reason:       AccessOrderReason_ACCESS_ORDER_REASON_ASSET,
				ReasonDetail: "img[src]",
				AccessCount:  1,
				ResolvedRefs: []*block.BlockRef{},
			},
		},
	}

	var buf bytes.Buffer
	if err := EncodeAccessOrderRecord(&buf, record); err != nil {
		t.Fatalf("EncodeAccessOrderRecord: %v", err)
	}
	got, err := DecodeAccessOrderRecord(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("DecodeAccessOrderRecord: %v", err)
	}

	entries := got.GetEntries()
	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3", len(entries))
	}

	if entries[0].GetPath() != "app/main.js" {
		t.Fatalf("entry 0 path = %q, want app/main.js", entries[0].GetPath())
	}
	if entries[0].GetOrdinal() != 0 {
		t.Fatalf("entry 0 ordinal = %d, want 0", entries[0].GetOrdinal())
	}
	assertRefOrder(t, entries[0].GetResolvedRefs(), []*block.BlockRef{refA, refB})

	if entries[1].GetPath() != "app/main.js" {
		t.Fatalf("entry 1 path = %q, want duplicate app/main.js", entries[1].GetPath())
	}
	if entries[1].GetOrdinal() != 1 {
		t.Fatalf("entry 1 ordinal = %d, want 1", entries[1].GetOrdinal())
	}
	if entries[1].GetReason() != AccessOrderReason_ACCESS_ORDER_REASON_DYNAMIC_IMPORT {
		t.Fatalf("entry 1 reason = %s, want %s", entries[1].GetReason(), AccessOrderReason_ACCESS_ORDER_REASON_DYNAMIC_IMPORT)
	}
	if len(entries[1].GetResolvedRefs()) != 0 {
		t.Fatalf("entry 1 resolved refs = %v, want none", refKeys(entries[1].GetResolvedRefs()))
	}

	if entries[2].GetPath() != "assets/logo.svg" {
		t.Fatalf("entry 2 path = %q, want assets/logo.svg", entries[2].GetPath())
	}
	if entries[2].GetOrdinal() != 2 {
		t.Fatalf("entry 2 ordinal = %d, want 2", entries[2].GetOrdinal())
	}
	if entries[2].GetFilesystem() != AccessOrderFilesystem_ACCESS_ORDER_FILESYSTEM_ASSETS {
		t.Fatalf("entry 2 filesystem = %s, want %s", entries[2].GetFilesystem(), AccessOrderFilesystem_ACCESS_ORDER_FILESYSTEM_ASSETS)
	}
	if len(entries[2].GetResolvedRefs()) != 0 {
		t.Fatalf("entry 2 resolved refs = %v, want none", refKeys(entries[2].GetResolvedRefs()))
	}
}

func TestAccessOrderManifestIdentityMatchesRecordMetadata(t *testing.T) {
	record := &AccessOrderRecord{
		ManifestId:      "manifest-1",
		PlatformId:      "darwin-arm64",
		BuildType:       "release",
		ManifestRootRef: testRef(t, "manifest-root"),
		ManifestRev:     7,
	}

	identity := AccessOrderManifestIdentityFromRecord(record)
	if !identity.MatchesRecord(record) {
		t.Fatal("exact manifest identity did not match record")
	}

	cases := []struct {
		name     string
		identity AccessOrderManifestIdentity
	}{
		{
			name: "manifest id",
			identity: AccessOrderManifestIdentity{
				ManifestID:      "manifest-2",
				PlatformID:      identity.PlatformID,
				BuildType:       identity.BuildType,
				ManifestRootRef: identity.ManifestRootRef,
				ManifestRev:     identity.ManifestRev,
			},
		},
		{
			name: "platform id",
			identity: AccessOrderManifestIdentity{
				ManifestID:      identity.ManifestID,
				PlatformID:      "linux-amd64",
				BuildType:       identity.BuildType,
				ManifestRootRef: identity.ManifestRootRef,
				ManifestRev:     identity.ManifestRev,
			},
		},
		{
			name: "build type",
			identity: AccessOrderManifestIdentity{
				ManifestID:      identity.ManifestID,
				PlatformID:      identity.PlatformID,
				BuildType:       "debug",
				ManifestRootRef: identity.ManifestRootRef,
				ManifestRev:     identity.ManifestRev,
			},
		},
		{
			name: "manifest root ref",
			identity: AccessOrderManifestIdentity{
				ManifestID:      identity.ManifestID,
				PlatformID:      identity.PlatformID,
				BuildType:       identity.BuildType,
				ManifestRootRef: testRef(t, "stale-manifest-root"),
				ManifestRev:     identity.ManifestRev,
			},
		},
		{
			name: "manifest rev",
			identity: AccessOrderManifestIdentity{
				ManifestID:      identity.ManifestID,
				PlatformID:      identity.PlatformID,
				BuildType:       identity.BuildType,
				ManifestRootRef: identity.ManifestRootRef,
				ManifestRev:     8,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.identity.MatchesRecord(record) {
				t.Fatal("stale manifest identity matched record")
			}
		})
	}
}
