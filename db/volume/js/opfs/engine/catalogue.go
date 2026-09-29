package engine

import (
	"bytes"
	"context"
	"slices"
	"sort"
)

// readCatalogue decodes and validates one bounded immutable routing page.
func (e *Engine) readCatalogue(ctx context.Context, name string) (*Catalogue, error) {
	// Return a cached page after checking its concrete type.
	if cached, err := e.cachedMessage(ctx, name); cached != nil || err != nil {
		if err != nil {
			return nil, err
		}
		page, ok := cached.(*Catalogue)
		if !ok {
			return nil, ErrCorrupt
		}
		return page, nil
	}

	// Read and decode the page's encoded bytes.
	data, err := e.readFile(ctx, name)
	if err != nil {
		return nil, err
	}
	page := new(Catalogue)
	if err := decode(data, page); err != nil {
		return nil, err
	}

	// Reject pages without exactly one of children or partitions, or beyond the fanout.
	if (len(page.Children) == 0) == (len(page.Partitions) == 0) || len(page.Children) > pageFanout || len(page.Partitions) > pageFanout {
		return nil, ErrCorrupt
	}

	// Check children carry files and strictly increasing lower bounds.
	var previous []byte
	for i, child := range page.Children {
		if child == nil || child.File == "" || len(child.Lower) > maxKeyBytes || (i > 0 && bytes.Compare(previous, child.Lower) >= 0) {
			return nil, ErrCorrupt
		}
		previous = child.Lower
	}

	// Check partitions carry valid runs and strictly increasing lower bounds.
	for i, partition := range page.Partitions {
		if partition == nil || len(partition.Lower) > maxKeyBytes || len(partition.Runs) > partitionRunLimit || slices.ContainsFunc(partition.Runs, invalidRunFile) || (i > 0 && bytes.Compare(previous, partition.Lower) >= 0) {
			return nil, ErrCorrupt
		}
		previous = partition.Lower
	}

	// Cache the validated page and return it.
	e.cacheMessage(name, page, len(data)*2+(len(page.Children)+len(page.Partitions))*192)
	return page, nil
}

// invalidRunFile reports a run reference without a name or beyond the run size bound.
func invalidRunFile(file *RunFile) bool {
	return file == nil || file.Name == "" || file.Bytes > maxRunBytes || file.Deleted > file.Records
}

// readRun decodes a sorted run with bounded records and encoded size.
func (e *Engine) readRun(ctx context.Context, name string) (*Run, error) {
	// Return a cached run after checking its concrete type.
	if cached, err := e.cachedMessage(ctx, name); cached != nil || err != nil {
		if err != nil {
			return nil, err
		}
		run, ok := cached.(*Run)
		if !ok {
			return nil, ErrCorrupt
		}
		return run, nil
	}

	// Read the run's bytes and reject files beyond the run size bound.
	data, err := e.readFile(ctx, name)
	if err != nil {
		return nil, err
	}
	if len(data) > maxRunBytes {
		return nil, ErrCorrupt
	}

	// Decode the run and validate its records.
	run := new(Run)
	if err := decode(data, run); err != nil {
		return nil, err
	}
	if err := validateRecords(run.Records); err != nil {
		return nil, ErrCorrupt
	}

	// Cache the validated run and return it.
	e.cacheMessage(name, run, len(data)*2+len(run.Records)*192)
	return run, nil
}

// updateCatalogue rewrites only paths containing this sorted mutation batch.
func (p *publication) updateCatalogue(ctx context.Context, name string, records []*Record) ([]*Child, error) {
	// Read the page and retire its cache entry before rewriting it.
	page, err := p.engine.readCatalogue(ctx, name)
	if err != nil {
		return nil, err
	}
	p.retire(name)
	if len(page.Children) != 0 {
		var children []*Child
		for i, child := range page.Children {
			end := len(records)
			if i+1 < len(page.Children) {
				upper := page.Children[i+1].Lower
				end = sort.Search(len(records), func(j int) bool { return bytes.Compare(records[j].Key, upper) >= 0 })
			}
			if end == 0 {
				children = append(children, child)
				continue
			}
			replaced, err := p.updateCatalogue(ctx, child.File, records[:end])
			if err != nil {
				return nil, err
			}
			children = append(children, replaced...)
			records = records[end:]
		}
		return p.writeBranches(children)
	}

	// Each leaf partitions the batch without visiting unrelated run files.
	var partitions []*Partition
	for i, partition := range page.Partitions {
		end := len(records)
		if i+1 < len(page.Partitions) {
			upper := page.Partitions[i+1].Lower
			end = sort.Search(len(records), func(j int) bool { return bytes.Compare(records[j].Key, upper) >= 0 })
		}
		if end == 0 {
			partitions = append(partitions, partition)
			continue
		}
		replaced, err := p.updatePartition(ctx, partition, records[:end])
		if err != nil {
			return nil, err
		}
		partitions = append(partitions, replaced...)
		records = records[end:]
	}
	var children []*Child
	for len(partitions) != 0 {
		count := min(pageFanout, len(partitions))
		name, err := p.add("page", &Catalogue{Partitions: partitions[:count]})
		if err != nil {
			return nil, err
		}
		children = append(children, &Child{Lower: partitions[0].Lower, File: name})
		partitions = partitions[count:]
	}
	return children, nil
}

// updatePartition appends the batch as a run and merges runs at the run limit.
//
// Merges are size-tiered: the newest runs combine until the next older run
// outweighs them, so random keys spread across many partitions rewrite each
// record a logarithmic number of times rather than once per few batches. Only a
// merge reaching the oldest run discards deletion records and splits the output,
// so one also runs once pending deletions could hide half the oldest run.
func (p *publication) updatePartition(ctx context.Context, partition *Partition, records []*Record) ([]*Partition, error) {
	// Extend the partition's lower bound to cover the batch.
	lower := partition.Lower
	if bytes.Compare(records[0].Key, lower) < 0 {
		lower = records[0].Key
	}

	// Sum the batch's encoded size and deletions plus retained runs' deletions.
	var encodedSize, deleted int
	for _, record := range records {
		encodedSize += record.SizeVT() + 8
		if record.Deleted {
			deleted++
		}
	}
	runs := partition.Runs
	if len(runs) != 0 {
		for _, run := range runs[1:] {
			deleted += int(run.GetDeleted())
		}
	}

	// Choose the oldest run the batch merges with; len(runs) appends a run.
	start := len(runs)
	if len(records) > maxBatchRecords || encodedSize > runTargetBytes || (len(runs) != 0 && deleted*2 >= int(runs[0].GetRecords())) {
		start = 0
	} else if len(runs) == partitionRunLimit {
		start = len(runs) - 1
		merged := int(runs[start].GetBytes()) + encodedSize
		for start > 0 && int(runs[start-1].GetBytes()) <= merged {
			start--
			merged += int(runs[start].GetBytes())
		}
	}
	if start != 0 {
		if start != len(runs) {
			var err error
			records, err = p.mergeRuns(ctx, runs[start:], records, true)
			if err != nil {
				return nil, err
			}
		}
		run, err := p.addRun(records)
		if err != nil {
			return nil, err
		}
		runs = append(slices.Clone(runs[:start]), run)
		return []*Partition{{Lower: lower, Runs: runs}}, nil
	}

	// Merge every run with the batch and drop deletion records.
	all, err := p.mergeRuns(ctx, runs, records, false)
	if err != nil || len(all) == 0 {
		return nil, err
	}

	// Split complete sorted output so no future merge inherits an unbounded range.
	var partitions []*Partition
	for len(all) != 0 {
		count, size := 0, 0
		for count < len(all) && count < maxBatchRecords {
			next := all[count].SizeVT() + 8
			if count > 0 && size+next > runTargetBytes {
				break
			}
			size += next
			count++
		}
		run, err := p.addRun(all[:count])
		if err != nil {
			return nil, err
		}
		boundary := all[0].Key
		if len(partitions) == 0 {
			boundary = lower
		}
		partitions = append(partitions, &Partition{Lower: boundary, Runs: []*RunFile{run}})
		all = all[count:]
	}
	return partitions, nil
}

// mergeRuns retires runs and returns their newest records overlaid by the batch.
//
// Deletion records survive only when keepDeleted is set, because a merge that
// excludes the oldest run must keep hiding the older values it holds.
func (p *publication) mergeRuns(ctx context.Context, runs []*RunFile, records []*Record, keepDeleted bool) ([]*Record, error) {
	// Overlay each run's records with the batch by key.
	merged := make(map[string]*Record)
	for _, file := range runs {
		run, err := p.engine.readRun(ctx, file.GetName())
		if err != nil {
			return nil, err
		}
		for _, record := range run.Records {
			merged[string(record.Key)] = record
		}
		p.retire(file.GetName())
	}
	for _, record := range records {
		merged[string(record.Key)] = record
	}

	// Collect the surviving records in key order.
	all := make([]*Record, 0, len(merged))
	for _, record := range merged {
		if keepDeleted || !record.Deleted {
			all = append(all, record)
		}
	}
	sort.Slice(all, func(i, j int) bool { return bytes.Compare(all[i].Key, all[j].Key) < 0 })
	return all, nil
}

// addRun retains one sorted run and records its encoded size.
func (p *publication) addRun(records []*Record) (*RunFile, error) {
	// Encode the run and add its bytes to the publication.
	data, err := encode(&Run{Records: records})
	if err != nil {
		return nil, err
	}
	name, err := p.addBytes("run", data)
	if err != nil {
		return nil, err
	}

	// Record the run's encoded size and deletion count.
	file := &RunFile{Name: name, Bytes: uint32(len(data)), Records: uint32(len(records))} //nolint:gosec // addBytes bounds data by maxPublicationBytes.
	for _, record := range records {
		if record.Deleted {
			file.Deleted++
		}
	}
	return file, nil
}

// writeBranches splits routing output into bounded immutable parent pages.
func (p *publication) writeBranches(children []*Child) ([]*Child, error) {
	if len(children) == 1 {
		return children, nil
	}
	var parents []*Child
	for len(children) != 0 {
		count := min(pageFanout, len(children))
		name, err := p.add("page", &Catalogue{Children: children[:count]})
		if err != nil {
			return nil, err
		}
		parents = append(parents, &Child{Lower: children[0].Lower, File: name})
		children = children[count:]
	}
	return parents, nil
}

// finishCatalogue adds root levels only when a split requires them.
func (p *publication) finishCatalogue(children []*Child) (string, error) {
	if len(children) == 0 {
		return p.add("page", &Catalogue{Partitions: []*Partition{{}}})
	}
	for len(children) > 1 {
		var err error
		children, err = p.writeBranches(children)
		if err != nil {
			return "", err
		}
	}
	return children[0].File, nil
}
