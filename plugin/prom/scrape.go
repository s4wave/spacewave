package prom_plugin

import (
	"context"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	s4wave_prom "github.com/s4wave/spacewave/sdk/prom"
	"github.com/sirupsen/logrus"
)

// scrapeSample is one parsed sample of a scrape, before it is appended.
type scrapeSample struct {
	// lset is the series' labels as the source reports them.
	lset labels.Labels
	// v is the sample value.
	v float64
}

// scrapeSource reads one scrape of a target. It returns every sample or an
// error, never part of a scrape.
type scrapeSource func(ctx context.Context) ([]scrapeSample, error)

// scraper scrapes one target's source into the target's Store on the
// target's interval. It appends samples only. A commitGroup commits them.
type scraper struct {
	// le is the logger.
	le *logrus.Entry
	// store is the target's writer.
	store *Store
	// cfg configures the scrape.
	cfg *s4wave_prom.ScrapeConfig
	// peerID is the Device peer reported in the status.
	peerID string
	// window is the commit window reported in the status.
	window time.Duration
	// source reads each scrape.
	source scrapeSource
	// last holds the series the previous successful scrape returned, by hash.
	last map[uint64]labels.Labels
}

// newScraper constructs a scraper that reads source into the target store
// writes, reporting the Device peerID that commits every window.
func newScraper(le *logrus.Entry, store *Store, cfg *s4wave_prom.ScrapeConfig, peerID string, window time.Duration, source scrapeSource) *scraper {
	return &scraper{
		le:     le,
		store:  store,
		cfg:    cfg,
		peerID: peerID,
		window: window,
		source: source,
		last:   make(map[uint64]labels.Labels),
	}
}

// run scrapes now and then once per interval until ctx ends or the store is
// stood down, in which case run returns ErrStoodDown.
func (s *scraper) run(ctx context.Context) error {
	//gostyle:timer deadline
	t := time.NewTicker(s.cfg.Interval())
	defer t.Stop()
	for {
		s.scrape(ctx, time.Now())
		if err := s.store.WaitScrape(ctx, t.C); err != nil {
			return err
		}
	}
}

// scrape fetches and appends one scrape stamped at start. A failed scrape
// appends up as 0 and marks every series of the last successful scrape stale.
// A successful scrape marks stale the series that vanished since then.
func (s *scraper) scrape(ctx context.Context, start time.Time) {
	// Read the whole scrape within the timeout before appending any of it.
	ts := start.UnixMilli()
	readCtx, cancel := context.WithTimeout(ctx, s.cfg.Timeout())
	samples, err := s.source(readCtx)
	cancel()
	duration := time.Since(start)
	if ctx.Err() != nil {
		return
	}

	// Append the scraped samples, renaming labels the target sets.
	seen := make(map[uint64]labels.Labels, len(samples))
	var skipped int
	for _, smp := range samples {
		lset := s.exportedLabels(smp.lset)
		if err := s.store.Append(lset, ts, smp.v); err != nil {
			skipped++
			continue
		}
		seen[lset.Hash()] = lset
	}
	if skipped != 0 {
		s.le.WithField("skipped", skipped).Debug("skipped out of order or duplicate samples")
	}

	// Mark stale each series the last successful scrape had and this one lacks.
	for h, lset := range s.last {
		if _, ok := seen[h]; !ok {
			_ = s.store.AppendStale(lset, ts)
		}
	}
	if err == nil {
		s.last = seen
	} else {
		s.last = make(map[uint64]labels.Labels)
	}

	// Append the scrape's own series and record its status.
	up := 1.0
	status := &s4wave_prom.TargetStatus{
		Up:             err == nil,
		LastScrapeMs:   ts,
		LastDurationMs: uint32(min(duration.Milliseconds(), int64(^uint32(0)))), //nolint:gosec // clamped above.
		Series:         uint64(len(seen)),
		DevicePeerId:   s.peerID,
		CommitWindowMs: uint32(s.window.Milliseconds()), //nolint:gosec // a window past 49 days wraps.
	}
	if err != nil {
		up = 0
		status.LastError = err.Error()
		s.le.WithError(err).Debug("scrape failed")
	}
	for name, v := range map[string]float64{
		"up":                      up,
		"scrape_duration_seconds": duration.Seconds(),
		"scrape_samples_scraped":  float64(len(samples)),
	} {
		_ = s.store.Append(labels.FromStrings(model.MetricNameLabel, name), ts, v)
	}
	if err := s.store.SetStatus(status); err != nil {
		s.le.WithError(err).Warn("encode target status")
	}
}

// exportedLabels renames each label of lset that the target also sets to
// exported_<name>, as Prometheus does without honor_labels.
func (s *scraper) exportedLabels(lset labels.Labels) labels.Labels {
	// Keep the set when no name clashes.
	meta := s.store.meta
	clash := false
	lset.Range(func(l labels.Label) { clash = clash || meta.Has(l.Name) })
	if !clash {
		return lset
	}

	// Rebuild it with the clashing names prefixed.
	b := labels.NewScratchBuilder(lset.Len())
	lset.Range(func(l labels.Label) {
		name := l.Name
		if meta.Has(name) {
			name = "exported_" + name
		}
		b.Add(name, l.Value)
	})
	b.Sort()
	return b.Labels()
}
