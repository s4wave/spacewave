package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/pkg/errors"
)

// options configure one run.
type options struct {
	Dir, Endpoint, Region, Bucket, Prefix string
	CredsPath, Manifest, AllowList, IDs   string
	ResourceID                            string
	Resume, DryRun                        bool

	// Log receives the plan, progress and totals.
	Log io.Writer
	// HTTP sends the requests; nil is a fresh client. Tests set it.
	HTTP *http.Client
	// Now is the clock; nil is time.Now.
	Now func() time.Time
}

// report holds a run's totals.
type report struct {
	// Objects and Bytes count the objects put.
	Objects int
	Bytes   int64
	// Skipped counts the ids the manifest already held.
	Skipped int
}

// run checks the flags and the whole plan, then PUTs each pack and its
// entry in order, stopping at the first failure.
func run(o *options) (*report, error) {
	// Check the target before anything else.
	endpoint, err := checkTarget(o)
	if err != nil {
		return nil, err
	}
	guard, err := newKeyGuard(o.Prefix)
	if err != nil {
		return nil, err
	}

	// Build and check the plan: every key guarded, every pair verified.
	plan, err := buildPlan(o, guard)
	if err != nil {
		return nil, err
	}

	// Print a dry run and stop.
	if o.DryRun {
		for _, p := range plan {
			for _, obj := range []*object{p.pack, p.entry} {
				state := "PUT"
				if p.skip {
					state = "SKIP"
				}
				_, _ = fmt.Fprintf(o.Log, "%s\t%s\t%d\t%s\n", state, obj.key, obj.size, hex.EncodeToString(obj.sha256[:]))
			}
		}
		_, _ = fmt.Fprintf(o.Log, "dry run: %s\n", planTotals(plan))
		return &report{}, nil
	}

	// Read the creds and open the manifest.
	if o.CredsPath == "" {
		return nil, errors.New("--creds is required without --dry-run")
	}
	c, err := readCreds(o.CredsPath)
	if err != nil {
		return nil, err
	}
	man, err := openManifest(o.Manifest, o.Resume)
	if err != nil {
		return nil, err
	}
	defer man.close() //nolint:errcheck // every line is synced.
	p := newPutter(o, endpoint, c)
	if o.Now != nil {
		man.now = o.Now
	}

	// PUT each pack, then its entry, stopping at the first failure.
	rep := &report{}
	ctx := context.Background()
	for i, pl := range plan {
		if pl.skip {
			rep.Skipped++
			continue
		}
		if err := uploadPair(ctx, o, p, man, pl, rep); err != nil {
			_, _ = fmt.Fprintf(o.Log, "stopped at id %d of %d: %d objects, %d bytes put\n", i+1, len(plan), rep.Objects, rep.Bytes)
			return rep, err
		}
		_, _ = fmt.Fprintf(o.Log, "ok %d/%d %s\n", i+1, len(plan), pl.id)
	}
	_, _ = fmt.Fprintf(o.Log, "totals: %d objects, %d bytes put; %d ids skipped as done\n", rep.Objects, rep.Bytes, rep.Skipped)
	return rep, nil
}

// checkTarget checks the endpoint, region and bucket flags and returns the
// endpoint URL.
func checkTarget(o *options) (*url.URL, error) {
	// Require a bare https endpoint.
	u, err := url.Parse(o.Endpoint)
	if err != nil {
		return nil, errors.Wrap(err, "--endpoint")
	}
	if u.Scheme != "https" || u.Host == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.Errorf("--endpoint %q must be a bare https://host URL", o.Endpoint)
	}

	// Require plain region, bucket and resource names.
	if !regionRE.MatchString(o.Region) {
		return nil, errors.Errorf("--region %q is not a region name", o.Region)
	}
	if !bucketRE.MatchString(o.Bucket) {
		return nil, errors.Errorf("--bucket %q is not a bucket name", o.Bucket)
	}
	if o.ResourceID == "" {
		return nil, errors.New("--resource-id is empty")
	}
	return &url.URL{Scheme: u.Scheme, Host: u.Host}, nil
}

// buildPlan lists the ids in order, checks each against the allow-list,
// guards its keys, verifies its pack and entry, and marks those the
// manifest holds as done with --resume.
func buildPlan(o *options, guard *keyGuard) ([]*planned, error) {
	// List the directory, every file in a complete pair.
	all, err := listDir(o.Dir)
	if err != nil {
		return nil, err
	}
	inDir := make(map[string]bool, len(all))
	for _, id := range all {
		inDir[id] = true
	}

	// Choose the ids: the --ids list in its order, or all.
	ids := all
	if o.IDs != "" {
		if ids, err = readIDList(o.IDs, "--ids"); err != nil {
			return nil, err
		}
		for _, id := range ids {
			if !inDir[id] {
				return nil, errors.Errorf("--ids names %s, which --dir does not hold", id)
			}
		}
	}
	if len(ids) == 0 {
		return nil, errors.New("no ids to upload")
	}

	// Refuse any id the allow-list lacks.
	allowed, err := readIDList(o.AllowList, "--allow-list")
	if err != nil {
		return nil, err
	}
	allow := make(map[string]bool, len(allowed))
	for _, id := range allowed {
		allow[id] = true
	}
	for _, id := range ids {
		if !allow[id] {
			return nil, errors.Errorf("id %s is not in --allow-list %s", id, o.AllowList)
		}
	}

	// Guard every key before reading any pack.
	plan := make([]*planned, len(ids))
	for i, id := range ids {
		pl := &planned{id: id, pack: &object{key: o.Prefix + "packs/" + id}, entry: &object{key: o.Prefix + "entries/" + id}}
		for _, key := range []string{pl.pack.key, pl.entry.key} {
			if err := guard.check(key); err != nil {
				return nil, err
			}
		}
		plan[i] = pl
	}

	// Read what the manifest holds as done.
	var have map[string]done
	if o.Resume {
		if have, err = readManifest(o.Manifest); err != nil {
			return nil, err
		}
	}

	// Verify each pair, keeping its digests, and mark the done ones.
	for _, pl := range plan {
		pack, entry, err := readPair(o.Dir, pl.id, o.ResourceID)
		if err != nil {
			return nil, err
		}
		pack.key, entry.key = pl.pack.key, pl.entry.key
		pack.data, entry.data = nil, nil
		pl.pack, pl.entry = pack, entry
		ndone := 0
		for _, obj := range []*object{pack, entry} {
			d, ok := have[obj.key]
			if !ok {
				continue
			}
			if d.size != obj.size || d.sha256 != hex.EncodeToString(obj.sha256[:]) {
				return nil, errors.Errorf("--manifest records %s put with size %d sha256 %s, the local file differs", obj.key, d.size, d.sha256)
			}
			ndone++
		}
		pl.skip = ndone == 2
	}
	return plan, nil
}

// uploadPair rereads one planned pair, checks it is the bytes planned, and
// PUTs the pack, then the entry once the pack PUT returned 200.
func uploadPair(ctx context.Context, o *options, p *putter, man *manifest, pl *planned, rep *report) error {
	// Reread and reverify the pair; it must not have changed since the plan.
	pack, entry, err := readPair(o.Dir, pl.id, o.ResourceID)
	if err != nil {
		return err
	}
	pack.key, entry.key = pl.pack.key, pl.entry.key
	if pack.sha256 != pl.pack.sha256 || entry.sha256 != pl.entry.sha256 {
		return errors.Errorf("%s changed since the plan was checked", pl.id)
	}

	// PUT the pack, then the entry, recording each response.
	for _, obj := range []*object{pack, entry} {
		status, err := p.put(ctx, obj)
		if status != 0 {
			if merr := man.record(obj, status); merr != nil {
				return merr
			}
		}
		if err != nil {
			return err
		}
		rep.Objects++
		rep.Bytes += obj.size
	}
	return nil
}

// newPutter builds the putter for the run. It follows no redirect, so a
// PUT goes only to its own key.
func newPutter(o *options, endpoint *url.URL, c *creds) *putter {
	// Copy the client and refuse every redirect.
	hc := &http.Client{}
	if o.HTTP != nil {
		cp := *o.HTTP
		hc = &cp
	}
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	// Build the putter on the clock.
	now := time.Now
	if o.Now != nil {
		now = o.Now
	}
	return &putter{http: hc, endpoint: endpoint, region: o.Region, bucket: o.Bucket, creds: c, now: now}
}

// planTotals describes the objects and bytes a plan puts.
func planTotals(plan []*planned) string {
	var objs, skip int
	var n int64
	for _, p := range plan {
		if p.skip {
			skip++
			continue
		}
		objs += 2
		n += p.pack.size + p.entry.size
	}
	return fmt.Sprintf("%d objects, %d bytes to put; %d ids skipped as done", objs, n, skip)
}
