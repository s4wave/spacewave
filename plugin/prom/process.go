package prom_plugin

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	s4wave_plugin "github.com/s4wave/spacewave/sdk/plugin"
	s4wave_process "github.com/s4wave/spacewave/sdk/process"
	s4wave_prom "github.com/s4wave/spacewave/sdk/prom"
	s4wave_root "github.com/s4wave/spacewave/sdk/root"
	"github.com/sirupsen/logrus"
)

// corePluginID is the plugin that serves the root resource of the process
// running core.
const corePluginID = "spacewave-core"

// targetProcess is the scrape process of one target on the Device whose
// ProcessBinding approves it. It is the only writer of the target.
type targetProcess struct {
	// le is the logger.
	le *logrus.Entry
	// b is the plugin's bus, which reaches the core plugin.
	b bus.Bus
	// eng is the Space's World engine.
	eng world.Engine
	// group commits the Device's targets in the Space together.
	group *commitGroup
	// objectKey is the target's object key.
	objectKey string
	// peerID is the Device peer reported in the target's status.
	peerID string
}

// Execute scrapes the target until the Space stops the process. A missing
// target, or one without a scrape config, stops at once.
func (p *targetProcess) Execute(_ *s4wave_process.ExecuteRequest, stream s4wave_process.SRPCPersistentExecutionService_ExecuteStream) error {
	// Report each state change to the Space.
	send := func(state s4wave_process.ExecutionState, err error) error {
		status := &s4wave_process.ExecuteStatus{State: state}
		if err != nil {
			status.Error = err.Error()
		}
		return stream.Send(status)
	}
	ctx := stream.Context()
	if err := send(s4wave_process.ExecutionState_ExecutionState_STARTING, nil); err != nil {
		return err
	}

	// Read the scrape config. Stop when there is nothing to scrape.
	cfg, err := p.readScrapeConfig(ctx)
	if err != nil {
		return send(s4wave_process.ExecutionState_ExecutionState_ERROR, err)
	}
	if cfg == nil {
		return send(s4wave_process.ExecutionState_ExecutionState_STOPPED, nil)
	}

	// Open the source the config selects.
	source, release, err := p.openSource(ctx, cfg)
	if err != nil {
		return send(s4wave_process.ExecutionState_ExecutionState_ERROR, err)
	}
	defer release()

	// Write the target until the process stops, standing by while another
	// Device holds it. A stop is not an error.
	if err := send(s4wave_process.ExecutionState_ExecutionState_RUNNING, nil); err != nil {
		return err
	}
	p.le.WithField("url", cfg.GetUrl()).WithField("interval", cfg.Interval().String()).Info("scraping target")
	for {
		err := p.standBy(ctx)
		if err == nil {
			err = p.scrape(ctx, cfg, source)
		}
		if ctx.Err() != nil {
			return nil
		}
		if errors.Is(err, ErrStoodDown) {
			p.le.Info("another Device took over the target, standing by")
			continue
		}
		return send(s4wave_process.ExecutionState_ExecutionState_ERROR, err)
	}
}

// scrape opens the target's writer after its committed history and scrapes
// source into it in the Device's commit group until ctx ends or another
// Device takes the target over.
func (p *targetProcess) scrape(ctx context.Context, cfg *s4wave_prom.ScrapeConfig, source scrapeSource) error {
	// Open the writer and join the commit group.
	store, err := OpenStore(ctx, p.eng, p.objectKey)
	if err != nil {
		return err
	}
	leave := p.group.join(store, p.eng)
	defer leave()

	// Scrape until stopped or stood down.
	return newScraper(p.le, store, cfg, p.peerID, p.group.window, source).run(ctx)
}

// standBy waits until the target's status is gone, records this Device's peer
// ID, or goes stale, waking on each commit to the World and when the status
// would go stale.
func (p *targetProcess) standBy(ctx context.Context) error {
	for {
		// Read the status as of the current seqno.
		seqno, err := p.eng.GetSeqno(ctx)
		if err != nil {
			return err
		}
		var meta *s4wave_prom.TargetMeta
		var status *s4wave_prom.TargetStatus
		err = world.ExecTransaction(ctx, p.eng, false, func(ctx context.Context, ws world.WorldState) error {
			var err error
			meta, status, err = s4wave_prom.ReadTarget(ctx, ws, p.objectKey)
			return err
		})
		if err != nil {
			return err
		}
		if status == nil || status.GetDevicePeerId() == p.peerID || status.Stale(meta, time.Now()) {
			return nil
		}

		// Wait for the next commit or for the status to go stale.
		staleAt := time.UnixMilli(status.GetLastScrapeMs()).Add(status.StaleAfter(meta))
		waitCtx, cancel := context.WithDeadline(ctx, staleAt)
		_, err = p.eng.WaitSeqno(waitCtx, seqno+1)
		cancel()
		if ctx.Err() != nil {
			return context.Cause(ctx)
		}
		if err != nil && waitCtx.Err() == nil {
			return err
		}
	}
}

// openSource returns the scrape source cfg selects and the function that
// releases it. DeviceMetricsURL gathers the root resource of the core plugin
// this process reaches. Any other URL is a metrics page.
func (p *targetProcess) openSource(ctx context.Context, cfg *s4wave_prom.ScrapeConfig) (scrapeSource, func(), error) {
	// A metrics page needs no connection.
	if cfg.GetUrl() != s4wave_prom.DeviceMetricsURL {
		return httpSource(cfg.GetUrl()), func() {}, nil
	}

	// Reach the core plugin's root resource.
	resources, err := s4wave_plugin.ConnectPluginResources(ctx, p.b, corePluginID)
	if err != nil {
		return nil, nil, errors.Wrap(err, "connect core plugin")
	}
	rootRef := resources.Client.AccessRootResource()
	root, err := s4wave_root.NewRoot(resources.Client, rootRef)
	if err != nil {
		rootRef.Release()
		resources.Release()
		return nil, nil, errors.Wrap(err, "core root resource")
	}

	// Gather it on each scrape.
	return gatherSource(root.GatherMetrics), func() {
		root.Release()
		resources.Release()
	}, nil
}

// readScrapeConfig returns the target's scrape config, or nil when the target
// is missing or scrapes nothing.
func (p *targetProcess) readScrapeConfig(ctx context.Context) (*s4wave_prom.ScrapeConfig, error) {
	// Read the target's meta, if the object still exists.
	tx, err := p.eng.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()
	found, err := tx.HasObject(ctx, p.objectKey)
	if err != nil || !found {
		return nil, err
	}
	meta, _, err := s4wave_prom.ReadTarget(ctx, tx, p.objectKey)
	if err != nil {
		return nil, err
	}

	// Check its scrape config.
	cfg := meta.GetScrape()
	if cfg == nil {
		return nil, nil
	}
	return cfg, errors.Wrap(cfg.Validate(), "target scrape config")
}

// _ is a type assertion
var _ s4wave_process.SRPCPersistentExecutionServiceServer = (*targetProcess)(nil)
