package repair

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/pkg/errors"
	packfile "github.com/s4wave/spacewave/core/provider/spacewave/packfile"
	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/writer"
	"github.com/s4wave/spacewave/db/block/bloom"
	"github.com/s4wave/spacewave/net/hash"
)

const falsePositiveSlack = 1.01

// ReaderAtFunc opens immutable pack bytes for an entry.
type ReaderAtFunc func(ctx context.Context, entry *packfile.PackfileEntry) (io.ReaderAt, error)

// Audit identifies packfile entries whose metadata violates policy.
func Audit(entries []*packfile.PackfileEntry, policy writer.Policy) *Report {
	policy = normalizePolicy(policy)
	policyBloom := policy.NewBloomFilter()
	report := &Report{}

	for _, entry := range entries {
		report.PacksScanned++
		blockCount := entry.GetBlockCount()
		report.addBlockCount(blockCount)
		finding := &Finding{Entry: entry}

		if blockCount == 0 {
			finding.addReason(ReasonMissingBlockCount)
		}
		if entry.GetCreatedAt() == nil && policy.RequireCreatedAt {
			finding.addReason(ReasonMissingCreatedAt)
		}
		if entry.GetSizeBytes() == 0 {
			finding.addReason(ReasonMissingSize)
		}
		if blockCount > policy.MaxBlocksPerPack {
			finding.addReason(ReasonUnderCapacity)
		}

		bf, malformed := entryBloomFilter(entry)
		if len(entry.GetBloomFilter()) == 0 {
			finding.addReason(ReasonMissingBloom)
		}
		if len(entry.GetBloomFilter()) != 0 && (malformed || bf == nil) {
			finding.addReason(ReasonMalformedBloom)
		}
		if len(entry.GetBloomFilter()) != 0 && !malformed && bf != nil {
			if bf.Cap() != policyBloom.Cap() || bf.K() != policyBloom.K() {
				finding.addReason(ReasonIncompatibleBloom)
			}
			fp := bloom.EstimateFalsePositiveRate(bf.Cap(), bf.K(), uint(blockCount))
			finding.EstimatedFalsePositive = fp
			if fp > report.BeforeMaxFalsePositiveRate {
				report.BeforeMaxFalsePositiveRate = fp
			}
			if fp > policy.BloomFalsePositive*falsePositiveSlack {
				finding.addReason(ReasonUnderCapacity)
			}
		}

		if len(finding.Reasons) != 0 {
			report.Findings = append(report.Findings, finding)
		}
	}

	return report
}

// Repair recomputes metadata for entries that fail Audit.
func Repair(
	ctx context.Context,
	entries []*packfile.PackfileEntry,
	policy writer.Policy,
	open ReaderAtFunc,
) (*Report, error) {
	if open == nil {
		return nil, errors.New("repair reader opener is nil")
	}
	policy = normalizePolicy(policy)
	report := Audit(entries, policy)
	if len(report.Findings) == 0 {
		return report, nil
	}
	policyBloom := policy.NewBloomFilter()

	for _, finding := range report.Findings {
		entry := finding.Entry
		updated, verified, packSha256Hex, err := repairEntry(ctx, entry, policy, open)
		if err != nil {
			return nil, errors.Wrapf(err, "repair pack metadata %s", entry.GetId())
		}
		finding.RepairedBlockCount = updated.GetBlockCount()
		finding.RepairedBloomBytes = len(updated.GetBloomFilter())
		finding.PackSha256Hex = packSha256Hex
		finding.VerifiedIndexedBlockCnt = verified
		report.PacksChanged++
		report.VerifiedIndexedBlockCountSum += verified
		report.UpdatedEntries = append(report.UpdatedEntries, updated)

		fp := bloom.EstimateFalsePositiveRate(
			policyBloom.Cap(),
			policyBloom.K(),
			uint(updated.GetBlockCount()),
		)
		if fp > report.AfterMaxFalsePositiveRate {
			report.AfterMaxFalsePositiveRate = fp
		}
	}

	return report, nil
}

func entryBloomFilter(entry *packfile.PackfileEntry) (*bloom.Filter, bool) {
	bloomData := entry.GetBloomFilter()
	if len(bloomData) == 0 {
		return nil, false
	}
	var pbf bloom.BloomFilter
	if err := pbf.UnmarshalBlock(bloomData); err != nil {
		return nil, true
	}
	return pbf.ToBloomFilter(), false
}

func repairEntry(
	ctx context.Context,
	entry *packfile.PackfileEntry,
	policy writer.Policy,
	open ReaderAtFunc,
) (*packfile.PackfileEntry, uint64, string, error) {
	if entry.GetId() == "" {
		return nil, 0, "", errors.New("pack id is empty")
	}
	size := entry.GetSizeBytes()
	if size == 0 {
		return nil, 0, "", errors.New("pack size is empty")
	}
	ra, err := open(ctx, entry)
	if err != nil {
		return nil, 0, "", err
	}
	packSha256Hex, err := hashPackBytes(ra, size)
	if err != nil {
		return nil, 0, "", err
	}
	reader, err := kvfile.BuildReader(ra, size)
	if err != nil {
		return nil, 0, "", errors.Wrap(err, "build kvfile reader")
	}

	bf := policy.NewBloomFilter()
	var count uint64
	err = reader.ScanPrefixEntries(nil, func(ie *kvfile.IndexEntry, _ int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		key := ie.GetKey()
		h := &hash.Hash{}
		if err := h.ParseFromB58(string(key)); err != nil {
			return errors.Wrap(err, "parse block hash key")
		}
		data, found, err := reader.Get(key)
		if err != nil {
			return errors.Wrap(err, "read indexed block")
		}
		if !found {
			return errors.New("indexed block not found")
		}
		if _, err := h.VerifyData(data); err != nil {
			return errors.Wrap(err, "verify indexed block hash")
		}
		bf.Add(key)
		count++
		return nil
	})
	if err != nil {
		return nil, 0, "", errors.Wrap(err, "scan pack index")
	}
	if count > policy.MaxBlocksPerPack {
		return nil, 0, "", errors.Errorf(
			"pack has %d indexed blocks, exceeds policy limit %d",
			count,
			policy.MaxBlocksPerPack,
		)
	}

	bloomBytes, err := bloom.NewBloom(bf).MarshalBlock()
	if err != nil {
		return nil, 0, "", errors.Wrap(err, "marshal repaired bloom")
	}

	updated := entry.CloneVT()
	updated.BloomFilter = bloomBytes
	updated.BlockCount = count
	updated.SizeBytes = size
	return updated, count, packSha256Hex, nil
}

func normalizePolicy(policy writer.Policy) writer.Policy {
	if policy.MaxBlocksPerPack == 0 || policy.BloomExpectedBlocks == 0 {
		return writer.DefaultPolicy()
	}
	return policy
}

func hashPackBytes(ra io.ReaderAt, size uint64) (string, error) {
	if size > uint64(^uint(0)>>1) {
		return "", errors.New("pack size overflows int")
	}
	r := io.NewSectionReader(ra, 0, int64(size))
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", errors.Wrap(err, "hash pack bytes")
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
