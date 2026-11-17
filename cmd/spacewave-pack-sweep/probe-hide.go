package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
)

// probeArgs configures the hide probe.
type probeArgs struct {
	// Endpoint is the S3 endpoint host of the bucket, reached over HTTPS.
	Endpoint string
	// Region is the bucket's region.
	Region string
	// Bucket is the bucket probed.
	Bucket string
	// StorePrefix is the prefix of the pack store a sweep would hide in; the
	// probe refuses a scratch key under it.
	StorePrefix string
	// ScratchPrefix starts the scratch key the probe writes.
	ScratchPrefix string
	// KeyIDEnv names the variable holding the access key id.
	KeyIDEnv string
	// SecretEnv names the variable holding the secret access key.
	SecretEnv string

	// client reaches the bucket; nil dials Endpoint. Tests set it.
	client *block_store_s3.Client
	// key, when set, is the scratch key instead of a new one. Tests set it.
	key string
}

// probeFlags binds the flags of the hide probe.
func (a *Args) probeFlags() []cli.Flag {
	p := &a.probe
	return []cli.Flag{
		&cli.StringFlag{Name: "s3-endpoint", Usage: "S3 endpoint host of the bucket, reached over HTTPS", Destination: &p.Endpoint, Required: true},
		&cli.StringFlag{Name: "s3-region", Usage: "the bucket's region", Destination: &p.Region},
		&cli.StringFlag{Name: "bucket", Usage: "bucket to probe", Destination: &p.Bucket, Required: true},
		&cli.StringFlag{Name: "store-prefix", Usage: "prefix of the pack store a sweep would hide in; no scratch key may fall under it", Destination: &p.StorePrefix, Required: true},
		&cli.StringFlag{Name: "scratch-prefix", Usage: "prefix of the one scratch key the probe writes and leaves in place", Destination: &p.ScratchPrefix, Required: true},
		&cli.StringFlag{Name: "key-id-env", Usage: "name of the variable holding the access key id", Value: s3KeyIDEnv, Destination: &p.KeyIDEnv},
		&cli.StringFlag{Name: "secret-env", Usage: "name of the variable holding the secret access key", Value: s3SecretEnv, Destination: &p.SecretEnv},
	}
}

// probeStep is one store answer the probe checked against what the hide code
// assumes.
type probeStep struct {
	name string
	want string
	got  string
}

// pass reports whether the store answered as the code assumes.
func (s probeStep) pass() bool {
	return s.want == s.got
}

// runProbeHide writes one new scratch key outside the store prefix and drives
// it through a hide and an unhide, checking each store answer against what
// the hide code assumes: the bucket keeps versions, a listing is newest first
// with the newest alone IsLatest, a hide adds one marker over the version, a
// read by id serves the hidden bytes, and an unhide makes the version current
// again. It prints each answer as a pass or the difference, with the key and
// its version ids. Its only deletes are of the delete markers its own hide
// added, so it deletes no content version, and it leaves the key in place.
func (a *Args) runProbeHide(*cli.Context) error {
	// Run until a signal stops the probe.
	ctx, cancel := signalContext()
	defer cancel()
	p := &a.probe

	// Refuse a scratch key under the store prefix.
	key := p.key
	if key == "" {
		key = p.ScratchPrefix + "probe-hide-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	if strings.HasPrefix(key, p.StorePrefix) {
		return errors.Errorf("the scratch key %s is under the store prefix %q", key, p.StorePrefix)
	}

	// Reach the bucket with the key the named variables hold.
	client := p.client
	if client == nil {
		keyID, secret := os.Getenv(p.KeyIDEnv), os.Getenv(p.SecretEnv)
		if keyID == "" || secret == "" {
			return errors.New("set the variables --key-id-env and --secret-env name")
		}
		var err error
		client, err = block_store_s3.BuildClient(&block_store_s3.ClientConfig{
			Endpoint:    p.Endpoint,
			Region:      p.Region,
			Credentials: &block_store_s3.Credentials{AccessKeyId: keyID, SecretAccessKey: secret},
		})
		if err != nil {
			return err
		}
	}

	// Probe, and print every answer, also when one stopped the probe.
	res := &probeResult{key: key}
	err := probeHide(ctx, client, p.Bucket, key, res)
	if perr := a.print(res.json); perr != nil && err == nil {
		err = perr
	}
	if err == nil && res.failed() != 0 {
		err = errors.Errorf("%d of %d store answers differ from what the hide code assumes", res.failed(), len(res.steps))
	}
	return err
}

// probeResult is what the probe learned: the scratch key, its version ids,
// and each checked answer.
type probeResult struct {
	key     string
	version string
	marker  string
	steps   []probeStep
}

// check records one answer.
func (r *probeResult) check(name, want, got string) bool {
	r.steps = append(r.steps, probeStep{name: name, want: want, got: got})
	return want == got
}

// failed counts the answers that differ.
func (r *probeResult) failed() int {
	n := 0
	for _, s := range r.steps {
		if !s.pass() {
			n++
		}
	}
	return n
}

// json describes the result.
func (r *probeResult) json(a *fastjson.Arena) *fastjson.Value {
	// Name the key and its versions.
	obj := a.NewObject()
	obj.Set("key", a.NewString(r.key))
	obj.Set("version", a.NewString(r.version))
	obj.Set("marker", a.NewString(r.marker))

	// List each answer, with the difference of one that failed.
	steps := a.NewArray()
	for i, s := range r.steps {
		step := a.NewObject()
		step.Set("step", a.NewString(s.name))
		if s.pass() {
			step.Set("pass", a.NewTrue())
		} else {
			step.Set("pass", a.NewFalse())
			step.Set("want", a.NewString(s.want))
			step.Set("got", a.NewString(s.got))
		}
		steps.SetArrayItem(i, step)
	}
	obj.Set("steps", steps)
	obj.Set("failed", a.NewNumberInt(r.failed()))
	return obj
}

// probeData is the scratch key's content, long enough for a range inside it.
const probeData = "spacewave pack sweep hide probe"

// probeHide runs the probe's steps on key, recording each answer in res. It
// stops at the first answer after which the next step would rest on a wrong
// assumption, and returns an error only when a call fails outright.
func probeHide(ctx context.Context, client *block_store_s3.Client, bucket, key string, res *probeResult) error {
	// Refuse a key that holds any version or marker, then write it.
	before, err := listKeyVersions(ctx, client, bucket, key)
	if err != nil {
		return err
	}
	if len(before) != 0 {
		return errors.Errorf("the scratch key %s exists, with %d versions", key, len(before))
	}
	if err := client.PutObject(ctx, bucket, key, []byte(probeData), ""); err != nil {
		return errors.Wrap(err, "put the scratch key")
	}

	// The bucket keeps versions, and lists the one written as latest.
	status, err := client.BucketVersioning(ctx, bucket)
	if err != nil {
		return errors.Wrap(err, "read the bucket's versioning")
	}
	if !res.check("versioning", "Enabled", status) {
		return nil
	}
	versions, err := listKeyVersions(ctx, client, bucket, key)
	if err != nil {
		return err
	}
	if !res.check("listing after the put", "content latest", describeVersions(versions)) {
		return nil
	}
	res.version = versions[0].VersionID

	// A hide adds one marker over the version.
	hidden, err := client.HideObjectVersion(ctx, bucket, key)
	if err != nil {
		res.check("hide", "the version under a marker", err.Error())
		return nil
	}
	res.marker = hidden.Marker
	res.check("hide covers", res.version, hidden.Version)

	// The listing shows the marker over the version, newest first.
	if versions, err = listKeyVersions(ctx, client, bucket, key); err != nil {
		return err
	}
	if !res.check("listing after the hide", "marker latest, content", describeVersions(versions)) {
		return nil
	}
	res.check("marker id", hidden.Marker, versions[0].VersionID)
	res.check("hidden version id", res.version, versions[1].VersionID)

	// The key reads as absent, with the version under its marker.
	_, err = client.GetObjectRange(ctx, bucket, key, 0, 1)
	res.check("read of the hidden key", "not found", errorKind(err))
	version, err := client.HiddenObjectVersion(ctx, bucket, key)
	res.check("hidden version", res.version, version+errorText(err))

	// A ranged read by id serves the hidden bytes.
	data, err := client.GetObjectVersionRange(ctx, bucket, key, res.version, 10, 4)
	res.check("ranged read of the hidden version", probeData[10:14], string(data)+errorText(err))

	// An unhide makes the version current again, with no marker left.
	if err := client.UnhideObjectVersion(ctx, bucket, key, res.version); err != nil {
		res.check("unhide", "the version current", err.Error())
		return nil
	}
	if versions, err = listKeyVersions(ctx, client, bucket, key); err != nil {
		return err
	}
	res.check("listing after the unhide", "content latest", describeVersions(versions))
	if len(versions) != 0 {
		res.check("current version id", res.version, versions[0].VersionID)
	}
	data, err = client.GetObjectRange(ctx, bucket, key, 0, len(probeData))
	res.check("read after the unhide", probeData, string(data)+errorText(err))
	return nil
}

// listKeyVersions lists key's own versions and markers in the store's order,
// unchecked, so the probe sees the order the store gives.
func listKeyVersions(ctx context.Context, client *block_store_s3.Client, bucket, key string) ([]block_store_s3.ObjectVersion, error) {
	var out []block_store_s3.ObjectVersion
	err := client.ListObjectVersions(ctx, bucket, key, func(v block_store_s3.ObjectVersion) error {
		if v.Key == key {
			out = append(out, v)
		}
		return nil
	})
	return out, errors.Wrap(err, "list the versions of "+key)
}

// describeVersions names a key's listing as the hide code reads it: each
// entry's kind, "latest" after the first when it alone is IsLatest, and a
// note on each entry whose time is missing or later than the one before it.
func describeVersions(versions []block_store_s3.ObjectVersion) string {
	parts := make([]string, 0, len(versions))
	for i, v := range versions {
		part := "content"
		if v.DeleteMarker {
			part = "marker"
		}
		if v.IsLatest != (i == 0) {
			part += fmt.Sprintf(" (IsLatest %v)", v.IsLatest)
		} else if i == 0 {
			part += " latest"
		}
		switch {
		case v.LastModified.IsZero():
			part += " (no LastModified)"
		case i > 0 && v.LastModified.After(versions[i-1].LastModified):
			part += " (newer than the entry before it)"
		}
		parts = append(parts, part)
	}
	if len(parts) == 0 {
		return "no versions"
	}
	return strings.Join(parts, ", ")
}

// errorKind names err as the probe compares it: "not found", "no error", or
// its text.
func errorKind(err error) string {
	switch {
	case err == nil:
		return "no error"
	case errors.Is(err, block_store_s3.ErrNotFound):
		return "not found"
	}
	return err.Error()
}

// errorText is " (error: ...)" for an error and "" for nil, appended to an
// answer so a failed call shows as a difference.
func errorText(err error) string {
	if err == nil {
		return ""
	}
	return " (error: " + err.Error() + ")"
}
