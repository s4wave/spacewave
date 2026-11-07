package block_store_nodepush

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/packfile"
)

// Adopter registers a pack another writer uploaded: block_store_s3.PackStore
// does, once the pack's own index shows the report is true.
type Adopter interface {
	// AdoptPack makes the pack's blocks readable through the local index. An
	// error has the node report the pack again, so it must be safe to repeat.
	AdoptPack(ctx context.Context, entry *packfile.PackfileEntry, keys [][]byte) error
}

// PackApplier applies reports to an Adopter. It drops no queue entry itself: a
// queue entry goes only once the index holds its block, which the upload checks
// and so does any clear the Adopter runs after adopting.
type PackApplier struct {
	adopter Adopter
}

// NewPackApplier builds a PackApplier of adopter.
func NewPackApplier(adopter Adopter) *PackApplier {
	return &PackApplier{adopter: adopter}
}

// ApplyPack decodes report and adopts its pack.
func (a *PackApplier) ApplyPack(ctx context.Context, report *PackReport) error {
	// The report's name must be the entry's.
	entry := &packfile.PackfileEntry{}
	if err := entry.UnmarshalVT(report.GetEntry()); err != nil {
		return errors.Wrap(err, "decode pack entry")
	}
	if entry.GetId() != report.GetPackId() {
		return errors.Errorf("report for pack %s carries the entry of %s", report.GetPackId(), entry.GetId())
	}

	// Adopt it.
	return a.adopter.AdoptPack(ctx, entry, report.GetKeys())
}

// _ is a type assertion
var _ Applier = (*PackApplier)(nil)
