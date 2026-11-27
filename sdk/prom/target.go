// Package s4wave_prom is the contract of Spacewave's Prometheus metrics: the
// prom/target ObjectType, the parts of its key-value tree that clients read,
// and the PromQL query service of the spacewave-prom plugin.
package s4wave_prom

import (
	"context"
	"net/url"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_kvtx "github.com/s4wave/spacewave/db/kvtx/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
)

// TargetTypeID is the ObjectType of a scrape target.
const TargetTypeID = "prom/target"

// Key prefixes of a target's key-value tree that clients read. The plugin's
// segment store owns the other prefixes: 0x01 for series pages and 0x02 for
// segments.
const (
	// MetaKeyPrefix is the key of the target's TargetMeta.
	MetaKeyPrefix byte = 0x00
	// StatusKeyPrefix is the key of the target's TargetStatus.
	StatusKeyPrefix byte = 0x03
)

// DefaultScrapeInterval is the scrape interval of a config that sets none.
const DefaultScrapeInterval = 15 * time.Second

// DefaultRetention is the retention of a target that sets none.
const DefaultRetention = 15 * 24 * time.Hour

// MetaKey returns the key of the target's TargetMeta.
func MetaKey() []byte {
	return []byte{MetaKeyPrefix}
}

// StatusKey returns the key of the target's TargetStatus.
func StatusKey() []byte {
	return []byte{StatusKeyPrefix}
}

// DeviceMetricsURL is the scrape URL of the metrics of the Spacewave process
// on the Device that runs the target, gathered without HTTP.
const DeviceMetricsURL = "device:metrics"

// Validate checks that the config references an http or https URL, or
// DeviceMetricsURL.
func (c *ScrapeConfig) Validate() error {
	// The device source needs no address.
	if c.GetUrl() == DeviceMetricsURL {
		return nil
	}

	// Any other URL is a metrics page.
	u, err := url.Parse(c.GetUrl())
	if err != nil {
		return errors.Wrap(err, "scrape url")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.Errorf("scrape url %q: want http or https", c.GetUrl())
	}
	return nil
}

// Interval returns the time between scrapes.
func (c *ScrapeConfig) Interval() time.Duration {
	if ms := c.GetIntervalMs(); ms != 0 {
		return time.Duration(ms) * time.Millisecond
	}
	return DefaultScrapeInterval
}

// Timeout returns the bound of one scrape.
func (c *ScrapeConfig) Timeout() time.Duration {
	timeout := time.Duration(c.GetTimeoutMs()) * time.Millisecond
	if interval := c.Interval(); timeout == 0 || timeout > interval {
		return interval
	}
	return timeout
}

// Retention returns how long the target keeps samples.
func (m *TargetMeta) Retention() time.Duration {
	if ms := m.GetRetentionMs(); ms != 0 {
		return time.Duration(ms) * time.Millisecond //nolint:gosec // a retention past 292 years wraps.
	}
	return DefaultRetention
}

// StaleAfter returns how long after its last scrape the status goes stale:
// two scrape intervals of meta plus one commit window of the scraping Device.
func (s *TargetStatus) StaleAfter(meta *TargetMeta) time.Duration {
	window := time.Duration(s.GetCommitWindowMs()) * time.Millisecond
	return 2*meta.GetScrape().Interval() + window
}

// Stale reports whether the scraping Device has stopped committing the target
// by now, judged by StaleAfter.
func (s *TargetStatus) Stale(meta *TargetMeta, now time.Time) bool {
	return now.Sub(time.UnixMilli(s.GetLastScrapeMs())) > s.StaleAfter(meta)
}

// CreateTarget creates the World object key as an empty prom/target holding
// meta. A target without a scrape config only stores samples written to it.
func CreateTarget(ctx context.Context, ws world.WorldState, key string, meta *TargetMeta) error {
	// Check and encode the meta.
	if meta.GetScrape() != nil {
		if err := meta.GetScrape().Validate(); err != nil {
			return err
		}
	}
	metaData, err := meta.MarshalVT()
	if err != nil {
		return err
	}

	// Create the object with an empty tree holding the meta.
	obj, _, err := world.CreateWorldObject(ctx, ws, key, func(bcs *block.Cursor) error {
		// Start an empty tree and write the meta into it.
		bcs.SetBlock(block_kvtx.NewKeyValueStore(0), true)
		kv, err := block_kvtx.BuildKvTransaction(ctx, bcs, true)
		if err != nil {
			return err
		}
		defer kv.Discard()
		if err := kv.Set(ctx, MetaKey(), metaData); err != nil {
			return err
		}
		return kv.Commit(ctx)
	})
	world.ReleaseObjectState(obj)
	if err != nil {
		return errors.Wrapf(err, "create target %q", key)
	}
	return world_types.SetObjectType(ctx, ws, key, TargetTypeID)
}

// ReadTarget reads the meta and the latest status of the target key. The
// status is nil until the target's process has committed a scrape.
func ReadTarget(ctx context.Context, ws world.WorldState, key string) (*TargetMeta, *TargetStatus, error) {
	// Open the target's tree for reading.
	obj, err := world.MustGetObject(ctx, ws, key)
	if err != nil {
		return nil, nil, err
	}
	defer world.ReleaseObjectState(obj)
	var meta *TargetMeta
	var status *TargetStatus
	_, _, err = world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		// Open the tree read only.
		kv, err := block_kvtx.BuildKvTransaction(ctx, bcs, false)
		if err != nil {
			return err
		}
		defer kv.Discard()

		// Decode the meta, which every target holds.
		data, found, err := kv.Get(ctx, MetaKey())
		if err != nil {
			return err
		}
		if !found {
			return errors.New("missing target meta")
		}
		meta = &TargetMeta{}
		if err := meta.UnmarshalVT(data); err != nil {
			return errors.Wrap(err, "decode target meta")
		}

		// Decode the status when a scrape has committed one.
		data, found, err = kv.Get(ctx, StatusKey())
		if err != nil || !found {
			return err
		}
		status = &TargetStatus{}
		return errors.Wrap(status.UnmarshalVT(data), "decode target status")
	})
	if err != nil {
		return nil, nil, errors.Wrapf(err, "read target %q", key)
	}
	return meta, status, nil
}

// ListTargets returns the keys of every target in the World.
func ListTargets(ctx context.Context, ws world.WorldState) ([]string, error) {
	return world_types.ListObjectsWithType(ctx, ws, TargetTypeID)
}
