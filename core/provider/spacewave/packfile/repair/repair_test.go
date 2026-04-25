package repair

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"slices"
	"testing"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	bbloom "github.com/bits-and-blooms/bloom/v3"
	packfile "github.com/s4wave/spacewave/core/provider/spacewave/packfile"
	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/writer"
	"github.com/s4wave/spacewave/db/block/bloom"
	"github.com/s4wave/spacewave/net/hash"
)

func testPack(t *testing.T, blocks ...[]byte) ([]byte, []byte, []*hash.Hash) {
	t.Helper()
	hashes := make([]*hash.Hash, 0, len(blocks))
	var buf bytes.Buffer
	idx := 0
	result, err := writer.PackBlocks(&buf, func() (*hash.Hash, []byte, error) {
		if idx >= len(blocks) {
			return nil, nil, nil
		}
		data := blocks[idx]
		h, err := hash.Sum(hash.HashType_HashType_SHA256, data)
		if err != nil {
			return nil, nil, err
		}
		hashes = append(hashes, h)
		idx++
		return h, data, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes(), result.BloomFilter, hashes
}

func testBloom(t *testing.T, bf *bbloom.BloomFilter) []byte {
	t.Helper()
	out, err := bloom.NewBloom(bf).MarshalBlock()
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func hasReason(f *Finding, reason Reason) bool {
	return slices.Contains(f.Reasons, reason)
}

func TestAuditIdentifiesBadPackMetadata(t *testing.T) {
	_, validBloom, _ := testPack(t, []byte("alpha"))
	incompatible := testBloom(t, bbloom.New(128, 3))
	createdAt := timestamppb.Now()

	report := Audit([]*packfile.PackfileEntry{
		{Id: "missing-bloom", BlockCount: 1, SizeBytes: 10, CreatedAt: createdAt},
		{
			Id:          "malformed-bloom",
			BloomFilter: []byte("not-a-bloom"),
			BlockCount:  1,
			SizeBytes:   10,
			CreatedAt:   createdAt,
		},
		{
			Id:          "incompatible-bloom",
			BloomFilter: incompatible,
			BlockCount:  1,
			SizeBytes:   10,
			CreatedAt:   createdAt,
		},
		{
			Id:          "under-capacity",
			BloomFilter: validBloom,
			BlockCount:  writer.DefaultMaxBlocksPerPack + 1,
			SizeBytes:   10,
			CreatedAt:   createdAt,
		},
		{Id: "missing-required", BloomFilter: validBloom},
	}, writer.DefaultPolicy())

	if report.PacksScanned != 5 {
		t.Fatalf("PacksScanned = %d, want 5", report.PacksScanned)
	}
	if len(report.Findings) != 5 {
		t.Fatalf("Findings = %d, want 5", len(report.Findings))
	}
	checks := map[string]Reason{
		"missing-bloom":      ReasonMissingBloom,
		"malformed-bloom":    ReasonMalformedBloom,
		"incompatible-bloom": ReasonIncompatibleBloom,
		"under-capacity":     ReasonUnderCapacity,
		"missing-required":   ReasonMissingBlockCount,
	}
	for _, finding := range report.Findings {
		reason := checks[finding.Entry.GetId()]
		if !hasReason(finding, reason) {
			t.Fatalf("finding %s missing reason %s: %v", finding.Entry.GetId(), reason, finding.Reasons)
		}
	}
}

func TestRepairRecomputesBloomWithoutChangingPackBytes(t *testing.T) {
	ctx := context.Background()
	packBytes, validBloom, hashes := testPack(t, []byte("alpha"), []byte("beta"))
	originalPack := bytes.Clone(packBytes)
	createdAt := timestamppb.Now()
	entry := &packfile.PackfileEntry{
		Id:          "pack-1",
		BloomFilter: testBloom(t, bbloom.New(128, 3)),
		BlockCount:  999,
		SizeBytes:   uint64(len(packBytes)),
		CreatedAt:   createdAt,
	}
	entries := []*packfile.PackfileEntry{
		{Id: "pack-0", BloomFilter: validBloom, BlockCount: 1, SizeBytes: 1, CreatedAt: createdAt},
		entry,
		{Id: "pack-2", BloomFilter: validBloom, BlockCount: 1, SizeBytes: 1, CreatedAt: createdAt},
	}

	report, err := Repair(ctx, entries, writer.DefaultPolicy(), func(
		_ context.Context,
		entry *packfile.PackfileEntry,
	) (io.ReaderAt, error) {
		if entry.GetId() != "pack-1" {
			t.Fatalf("unexpected repair open for %s", entry.GetId())
		}
		return bytes.NewReader(packBytes), nil
	})
	if err != nil {
		t.Fatalf("Repair: %v", err)
	}
	if !bytes.Equal(packBytes, originalPack) {
		t.Fatal("repair changed pack bytes")
	}
	if report.PacksChanged != 1 || len(report.UpdatedEntries) != 1 {
		t.Fatalf("changed/updates = %d/%d, want 1/1", report.PacksChanged, len(report.UpdatedEntries))
	}
	sum := sha256.Sum256(packBytes)
	if report.Findings[0].PackSha256Hex != hex.EncodeToString(sum[:]) {
		t.Fatalf("PackSha256Hex = %q, want %q", report.Findings[0].PackSha256Hex, hex.EncodeToString(sum[:]))
	}

	updated := report.UpdatedEntries[0]
	if updated.GetId() != entry.GetId() || updated.GetSizeBytes() != entry.GetSizeBytes() {
		t.Fatalf("updated identity/size changed: %#v", updated)
	}
	if !updated.GetCreatedAt().EqualVT(createdAt) {
		t.Fatal("repair should preserve created-at metadata")
	}
	if updated.GetBlockCount() != uint64(len(hashes)) {
		t.Fatalf("BlockCount = %d, want %d", updated.GetBlockCount(), len(hashes))
	}

	var pbf bloom.BloomFilter
	if err := pbf.UnmarshalBlock(updated.GetBloomFilter()); err != nil {
		t.Fatal(err)
	}
	bf := pbf.ToBloomFilter()
	if bf == nil {
		t.Fatal("repaired bloom decoded to nil")
	}
	policyBloom := writer.DefaultPolicy().NewBloomFilter()
	if bf.Cap() != policyBloom.Cap() || bf.K() != policyBloom.K() {
		t.Fatalf("repaired bloom shape = %d/%d, want %d/%d", bf.Cap(), bf.K(), policyBloom.Cap(), policyBloom.K())
	}
	for _, h := range hashes {
		if !bf.Test([]byte(h.MarshalString())) {
			t.Fatalf("repaired bloom missing %s", h.MarshalString())
		}
	}

	merged := ApplyUpdates(entries, report.UpdatedEntries)
	if len(merged) != len(entries) {
		t.Fatalf("merged entries = %d, want %d", len(merged), len(entries))
	}
	if merged[0].GetId() != "pack-0" || merged[1].GetId() != "pack-1" || merged[2].GetId() != "pack-2" {
		t.Fatalf("manifest order changed: %s, %s, %s", merged[0].GetId(), merged[1].GetId(), merged[2].GetId())
	}
	if merged[1].GetBlockCount() != uint64(len(hashes)) {
		t.Fatalf("merged repaired block count = %d, want %d", merged[1].GetBlockCount(), len(hashes))
	}

	req, err := NewPackMetadataRepairRequest(report, true)
	if err != nil {
		t.Fatalf("NewPackMetadataRepairRequest: %v", err)
	}
	if !req.GetDryRun() || len(req.GetEntries()) != 1 {
		t.Fatalf("repair request shape = dryRun:%v entries:%d", req.GetDryRun(), len(req.GetEntries()))
	}
	reqEntry := req.GetEntries()[0]
	if reqEntry.GetId() != entry.GetId() ||
		reqEntry.GetBlockCount() != uint64(len(hashes)) ||
		reqEntry.GetSizeBytes() != uint64(len(packBytes)) ||
		reqEntry.GetSha256Hex() != hex.EncodeToString(sum[:]) {
		t.Fatalf("unexpected repair request entry: %#v", reqEntry)
	}
}
