package repair

import (
	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

// NewPackMetadataRepairRequest builds the cloud admin repair request.
func NewPackMetadataRepairRequest(
	report *Report,
	dryRun bool,
) (*api.PackMetadataRepairRequest, error) {
	if report == nil {
		return nil, errors.New("repair report is nil")
	}
	shaByID := make(map[string]string, len(report.Findings))
	for _, finding := range report.Findings {
		if finding == nil || finding.Entry == nil {
			continue
		}
		shaByID[finding.Entry.GetId()] = finding.PackSha256Hex
	}

	entries := make([]*api.PackMetadataRepairEntry, 0, len(report.UpdatedEntries))
	for _, entry := range report.UpdatedEntries {
		if entry == nil {
			continue
		}
		sha := shaByID[entry.GetId()]
		if sha == "" {
			return nil, errors.Errorf("repair entry %s missing pack sha256", entry.GetId())
		}
		entries = append(entries, &api.PackMetadataRepairEntry{
			Id:          entry.GetId(),
			BloomFilter: entry.GetBloomFilter(),
			BlockCount:  entry.GetBlockCount(),
			SizeBytes:   entry.GetSizeBytes(),
			Sha256Hex:   sha,
		})
	}
	if len(entries) == 0 {
		return nil, errors.New("repair report has no updated entries")
	}
	return &api.PackMetadataRepairRequest{
		DryRun:  dryRun,
		Entries: entries,
	}, nil
}
