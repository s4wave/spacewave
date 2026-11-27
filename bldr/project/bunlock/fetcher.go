package bunlock

import (
	"context"
	"crypto/sha512"
	"encoding/base64"
	"io"
	"math"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
)

// Fetcher downloads pinned npm tarballs without running package scripts.
// The caller supplies the HTTP policy, default registry, download bound and
// dependency tree. Fetcher retains no package cache of its own.
type Fetcher struct {
	// client applies the caller's transport, authentication and redirect policy.
	client *http.Client
	// registry is the default registry for lock entries with an empty URL.
	registry string
	// maxTarballSize bounds the memory used to verify one downloaded tarball.
	maxTarballSize int64
}

// NewFetcher constructs a fetcher with an explicit registry and download bound.
// The client must be non-nil. registry must be an HTTP(S) base URL.
func NewFetcher(client *http.Client, registry string, maxTarballSize int64) (*Fetcher, error) {
	// Require a transport and a bound before reading untrusted tarballs.
	if client == nil {
		return nil, errors.New("npm fetcher requires an HTTP client")
	}
	// The bound leaves room for the one extra byte read to detect overflow.
	if maxTarballSize <= 0 || maxTarballSize == math.MaxInt64 {
		return nil, errors.New("npm fetcher requires a positive tarball size bound")
	}
	if err := checkURL(registry); err != nil {
		return nil, errors.Wrap(err, "npm registry")
	}
	return &Fetcher{
		client:         client,
		registry:       strings.TrimRight(registry, "/"),
		maxTarballSize: maxTarballSize,
	}, nil
}

// Fetch parses bun.lock and stores missing tarballs in dependencies, keyed by
// their canonical integrity strings. KVTX chunks the bytes into addressed blocks.
// dependencies must be the caller's writable, dedicated tarball tree, populated
// only with verified bytes. The caller commits or discards it and retains its root.
// On failure, earlier verified tarballs may remain staged. The failing tarball
// is never stored. A new Fetcher reuses the same tree without fetching known pins.
func (f *Fetcher) Fetch(ctx context.Context, data []byte, dependencies kvtx.BlockTx) ([]Package, error) {
	// Validate the complete lockfile before writing any package.
	packages, err := Parse(data)
	if err != nil {
		return nil, err
	}

	// Reuse the stored integrity keys and fetch only the missing tarballs.
	for _, pkg := range packages {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		key := []byte(pkg.Integrity)
		found, err := dependencies.Exists(ctx, key)
		if err != nil {
			return nil, err
		}
		if found {
			continue
		}
		tarball, err := f.fetchTarball(ctx, pkg)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, errors.Wrap(err, pkg.Key)
		}
		if err := dependencies.Set(ctx, key, tarball); err != nil {
			return nil, err
		}
	}
	return packages, nil
}

// fetchTarball reads and verifies one tarball before exposing bytes for storage.
func (f *Fetcher) fetchTarball(ctx context.Context, pkg Package) ([]byte, error) {
	// Resolve Bun's explicit tarball URL or its default-registry convention.
	tarballURL := pkg.URL
	if tarballURL == "" {
		tarballURL = f.registry + "/" + pkg.Name + "/-/" + path.Base(pkg.Name) + "-" + pkg.Version + ".tgz"
	}
	if err := checkURL(tarballURL); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tarballURL, nil)
	if err != nil {
		return nil, err
	}

	// Close each response before moving to the next dependency.
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("fetch npm tarball: HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > f.maxTarballSize {
		return nil, errors.New("npm tarball exceeds size bound")
	}

	// Bound unknown-length responses as well as declared download sizes.
	data, err := io.ReadAll(io.LimitReader(resp.Body, f.maxTarballSize+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > f.maxTarballSize {
		return nil, errors.New("npm tarball exceeds size bound")
	}
	sum := sha512.Sum512(data)
	if "sha512-"+base64.StdEncoding.EncodeToString(sum[:]) != pkg.Integrity {
		return nil, ErrIntegrity
	}
	return data, nil
}

// checkURL requires an HTTP(S) URL. HTTP policy belongs to the supplied client.
func checkURL(raw string) error {
	// Parse the address before issuing an HTTP request.
	addr, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if (addr.Scheme != "http" && addr.Scheme != "https") || addr.Host == "" {
		return errors.New("npm URL requires an HTTP(S) address")
	}
	return nil
}
