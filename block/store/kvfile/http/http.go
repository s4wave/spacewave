package block_store_kvfile_http

import (
	"context"
	"io"

	kvfile "github.com/aperturerobotics/go-kvfile"
	block_store_kvfile "github.com/aperturerobotics/hydra/block/store/kvfile"
	store_kvkey "github.com/aperturerobotics/hydra/store/kvkey"
	buffered_reader_at "github.com/aperturerobotics/hydra/util/buffered-reader-at"
	http_range "github.com/aperturerobotics/hydra/util/http-range"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// KvfileHTTPBlock is a block store on top of a HTTP or Fetch client and base URL prefix.
type KvfileHTTPBlock = block_store_kvfile.KvfileBlock

// NewKvfileHTTPBlock builds a read-only block store on top of a kvfile via. HTTP.
//
// fileURL cannot be nil
// if disableCache is set the browser cache will be disabled (if possible)
// kvkey controls the keys used to access blocks from the kvfile
// httpRangeMinSize sets a minimum size for http requests and enables buffering.
// if httpRangeMinSize is zero, disables buffering.
// verbose logs http requests
func NewKvfileHTTPBlock(
	ctx context.Context,
	le *logrus.Entry,
	fileURL string,
	headers map[string]string,
	disableCache bool,
	kvkey *store_kvkey.KVKey,
	httpRangeMinSize int64,
	verbose bool,
) (*KvfileHTTPBlock, error) {
	if fileURL == "" {
		// this won't work
		return nil, errors.New("file url cannot be empty")
	}

	// construct the range reader
	fetchReader, err := http_range.NewHTTPRangeReader(ctx, le, fileURL, headers, disableCache, verbose)
	if err != nil {
		return nil, err
	}

	// get the total file size
	totalSize, err := fetchReader.Size()
	if err != nil {
		return nil, err
	}

	var reader io.ReaderAt = fetchReader
	if httpRangeMinSize > 0 {
		reader = buffered_reader_at.NewBufferedReaderAt(fetchReader, httpRangeMinSize)
	}

	kvReader, err := kvfile.BuildReader(reader, totalSize)
	if err != nil {
		return nil, err
	}

	// construct the fetch client
	return block_store_kvfile.NewKvfileBlock(ctx, kvkey, kvReader), nil
}
