// Package provider_spacewave_cacheseed provides the dev-only cache-seed
// inspector RPC service. The service streams the provider's recorded
// X-Alpha-Seed-Reason tagged HTTP calls so developers can observe cold-mount
// fan-out in real time. Registration is gated behind the =alphadebug= build
// tag so production binaries do not expose the service.
package provider_spacewave_cacheseed

import (
	"github.com/s4wave/spacewave/core/provider/spacewave/cacheseedbuffer"
)

// Service implements SRPCCacheSeedInspectorServer against a
// cacheseedbuffer.Buffer.
type Service struct {
	buf *cacheseedbuffer.Buffer
}

// NewService constructs a new Service streaming from buf.
func NewService(buf *cacheseedbuffer.Buffer) *Service {
	return &Service{buf: buf}
}

// GetCacheSeedReasons streams the current ring-buffer snapshot and then every
// subsequent recorded entry until the caller cancels the stream.
func (s *Service) GetCacheSeedReasons(
	req *GetCacheSeedReasonsRequest,
	strm SRPCCacheSeedInspector_GetCacheSeedReasonsStream,
) error {
	snap, updates, release := s.buf.Subscribe()
	defer release()

	for _, entry := range snap {
		if err := strm.Send(toProtoEntry(entry)); err != nil {
			return err
		}
	}

	ctx := strm.Context()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case entry, ok := <-updates:
			if !ok {
				return nil
			}
			if err := strm.Send(toProtoEntry(entry)); err != nil {
				return err
			}
		}
	}
}

func toProtoEntry(entry cacheseedbuffer.Entry) *CacheSeedEntry {
	return &CacheSeedEntry{
		TimestampMs: entry.TimestampMs,
		Reason:      string(entry.Reason),
		Path:        entry.Path,
	}
}

// _ is a type assertion
var _ SRPCCacheSeedInspectorServer = (*Service)(nil)
