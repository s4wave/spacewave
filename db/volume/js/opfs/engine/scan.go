package engine

import (
	"bytes"
	"context"
	"sort"
)

// seekEntries copies one bounded partition's live records in scan order.
// Empty partitions are skipped without retaining their records.
func (s *snapshot) seekEntries(ctx context.Context, key []byte, exclusive, reverse bool) ([]*Record, error) {
	return s.seekPage(ctx, s.root.Catalogue, key, exclusive, reverse)
}

// seekPage visits only routing ranges that can follow the requested boundary.
func (s *snapshot) seekPage(ctx context.Context, name string, key []byte, exclusive, reverse bool) ([]*Record, error) {
	// Read the page and count its routing entries.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	page, err := s.engine.readCatalogue(ctx, name)
	if err != nil {
		return nil, err
	}
	count := len(page.Children)
	if count == 0 {
		count = len(page.Partitions)
	}

	// Visit each entry in scan order, skipping ranges beyond the boundary.
	for step := range count {
		i := step
		if reverse {
			i = count - 1 - step
		}
		var lower, upper []byte
		if len(page.Children) != 0 {
			lower = page.Children[i].Lower
			if i+1 < count {
				upper = page.Children[i+1].Lower
			}
		} else {
			lower = page.Partitions[i].Lower
			if i+1 < count {
				upper = page.Partitions[i+1].Lower
			}
		}
		if key != nil && ((!reverse && upper != nil && bytes.Compare(key, upper) >= 0) || (reverse && bytes.Compare(key, lower) < 0)) {
			continue
		}

		// Descend into the child page or read the partition's entries.
		var records []*Record
		if len(page.Children) != 0 {
			records, err = s.seekPage(ctx, page.Children[i].File, key, exclusive, reverse)
		} else {
			records, err = s.partitionEntries(ctx, page.Partitions[i], key, exclusive, reverse)
		}
		if err != nil || len(records) != 0 {
			return records, err
		}
	}
	return nil, nil
}

// partitionEntries resolves newest values before returning bounded live entries.
func (s *snapshot) partitionEntries(ctx context.Context, partition *Partition, key []byte, exclusive, reverse bool) ([]*Record, error) {
	// Overlay the partition's runs by key so the newest record wins.
	latest := make(map[string]*Record)
	for _, file := range partition.Runs {
		run, err := s.engine.readRun(ctx, file.GetName())
		if err != nil {
			return nil, err
		}
		for _, record := range run.Records {
			latest[string(record.Key)] = record
		}
	}

	// Collect live records within the boundary and sort them in scan order.
	records := make([]*Record, 0, len(latest))
	for _, record := range latest {
		if record.Deleted {
			continue
		}
		if key != nil {
			comparison := bytes.Compare(record.Key, key)
			if (exclusive && comparison == 0) || (!reverse && comparison < 0) || (reverse && comparison > 0) {
				continue
			}
		}
		records = append(records, record.CloneVT())
	}
	sort.Slice(records, func(i, j int) bool {
		comparison := bytes.Compare(records[i].Key, records[j].Key)
		if reverse {
			return comparison > 0
		}
		return comparison < 0
	})
	return records, nil
}
