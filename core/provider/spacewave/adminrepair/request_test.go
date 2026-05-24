package adminrepair

import (
	"testing"

	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

func TestPath(t *testing.T) {
	got := Path("01kny7hn4wp25f7t86xzww6bd6")
	want := "/api/admin/bstore/01kny7hn4wp25f7t86xzww6bd6/pack-metadata-repair"
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}

func TestMarshalRequest(t *testing.T) {
	body, err := MarshalRequest(&api.PackMetadataRepairRequest{
		Entries: []*api.PackMetadataRepairEntry{{
			Id:          "01kny7hn7r5qzaznnsvpqf7p2m",
			BloomFilter: []byte{1, 2, 3},
			BlockCount:  2,
			SizeBytes:   4,
			Sha256Hex:   "sha",
		}},
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	var got api.PackMetadataRepairRequest
	if err := got.UnmarshalVT(body); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}
	entries := got.GetEntries()
	if len(entries) != 1 {
		t.Fatalf("entries = %#v", entries)
	}
	entry := entries[0]
	if entry.GetId() != "01kny7hn7r5qzaznnsvpqf7p2m" {
		t.Fatalf("entry id = %q", entry.GetId())
	}
	if entry.GetBlockCount() != 2 || entry.GetSizeBytes() != 4 {
		t.Fatalf("entry counters = block:%d size:%d", entry.GetBlockCount(), entry.GetSizeBytes())
	}
}

func TestParseResponse(t *testing.T) {
	body, err := (&api.PackMetadataRepairResponse{
		Scanned: 1,
		Changed: 1,
		DryRun:  true,
	}).MarshalVT()
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	resp, err := ParseResponse(body)
	if err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if resp.GetScanned() != 1 || resp.GetChanged() != 1 || !resp.GetDryRun() {
		t.Fatalf("response = %#v", resp)
	}
}
