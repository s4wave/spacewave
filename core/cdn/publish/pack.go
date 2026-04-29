package publish

import (
	"context"
	"crypto/sha256"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/pkg/errors"
	spacewave_provider "github.com/s4wave/spacewave/core/provider/spacewave"
	packfile "github.com/s4wave/spacewave/core/provider/spacewave/packfile"
	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/writer"
	"github.com/s4wave/spacewave/db/block/bloom"
	"github.com/s4wave/spacewave/net/hash"
)

// FetchSourcePackToTempFile downloads one source pack into a temporary file.
func FetchSourcePackToTempFile(
	ctx context.Context,
	opts Options,
	entry *packfile.PackfileEntry,
) (string, error) {
	reqURL, err := url.JoinPath(opts.Endpoint, "/api/bstore", opts.SrcSpaceID, "pack", entry.GetId())
	if err != nil {
		return "", errors.Wrap(err, "build source pack url")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", errors.Wrap(err, "build source pack request")
	}
	req.Header.Set(spacewave_provider.SeedReasonHeader, string(spacewave_provider.SeedReasonColdSeed))
	resp, err := opts.Client.Do(req)
	if err != nil {
		return "", errors.Wrap(err, "request source pack")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1024))
		if readErr != nil {
			return "", errors.Wrap(readErr, "read source pack error body")
		}
		return "", errors.Errorf("source pack status %d: %s", resp.StatusCode, string(body))
	}
	maxBytes := int64(entry.GetSizeBytes()) + 4096
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return "", errors.Wrap(err, "read source pack body")
	}
	if int64(len(body)) > maxBytes {
		return "", errors.New("source pack exceeds declared size budget")
	}
	tmp, err := opts.tempFileFactory()("spacewave-cdn-pack-*.kvf")
	if err != nil {
		return "", errors.Wrap(err, "create temp pack file")
	}
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		_ = os.Remove(tmp.Name())
		return "", errors.Wrap(err, "write temp pack file")
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return "", errors.Wrap(err, "close temp pack file")
	}
	return tmp.Name(), nil
}

// PushSinglePack uploads a local kvfile pack to the destination Space.
func PushSinglePack(
	ctx context.Context,
	opts Options,
	packID string,
	packPath string,
	bloomFilter []byte,
) error {
	packBytes, err := os.ReadFile(packPath)
	if err != nil {
		return errors.Wrap(err, "read pack file")
	}
	blockCount, computedBloom, err := BuildKVFilePushMetadata(ctx, packBytes)
	if err != nil {
		return errors.Wrap(err, "build kvfile metadata")
	}
	if len(bloomFilter) == 0 {
		bloomFilter = computedBloom
	}
	packHash := sha256.Sum256(packBytes)
	if err := opts.Client.SyncPushData(
		ctx,
		opts.DstSpaceID,
		packID,
		blockCount,
		packBytes,
		packHash[:],
		bloomFilter,
		packfile.BloomFormatVersionV1,
	); err != nil {
		return errors.Wrap(err, "sync push")
	}
	_, err = io.WriteString(
		opts.output(),
		"  pushed pack "+packID+
			" size="+strconv.Itoa(len(packBytes))+
			" blocks="+strconv.Itoa(blockCount)+"\n",
	)
	return err
}

// BuildKVFilePushMetadata verifies a kvfile and builds sync/push metadata.
func BuildKVFilePushMetadata(ctx context.Context, data []byte) (int, []byte, error) {
	rdr, err := kvfile.BuildReader(bytesReaderAt(data), uint64(len(data)))
	if err != nil {
		return 0, nil, err
	}
	blockCount := int(rdr.Size())
	policy := writer.DefaultPolicy()
	if uint64(blockCount) > policy.BloomExpectedBlocks {
		policy.BloomExpectedBlocks = uint64(blockCount)
	}
	bf := policy.NewBloomFilter()
	err = rdr.ScanPrefixEntries(nil, func(ie *kvfile.IndexEntry, _ int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		key := ie.GetKey()
		h := &hash.Hash{}
		if err := h.ParseFromB58(string(key)); err != nil {
			return errors.Wrap(err, "parse block hash key")
		}
		block, found, err := rdr.Get(key)
		if err != nil {
			return errors.Wrap(err, "read indexed block")
		}
		if !found {
			return errors.New("indexed block not found")
		}
		if _, err := h.VerifyData(block); err != nil {
			return errors.Wrap(err, "verify indexed block hash")
		}
		bf.Add(key)
		return nil
	})
	if err != nil {
		return 0, nil, errors.Wrap(err, "scan kvfile index")
	}
	bloomBytes, err := bloom.NewBloom(bf).MarshalBlock()
	if err != nil {
		return 0, nil, errors.Wrap(err, "marshal bloom filter")
	}
	return blockCount, bloomBytes, nil
}
