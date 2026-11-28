//go:build !js && !wasip1

package volume_s4db

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/s4wave/spacewave/db/volume"
)

// claimHeader starts the first line of a conversion's claim.
const claimHeader = "s4db-convert"

// convertClaim is a conversion's claim on the files it keeps beside a Volume
// file, stored in the marker <path>.converting.ok. A conversion writes the
// claim durably before it creates <path>.converting, so a file of either
// name that no claim accounts for belongs to someone else, and a conversion
// refuses to start rather than remove it. The claim's first line names the
// source file by its identity. A line appended once every pair of the new
// file read back records their digest.
type convertClaim struct {
	// src is the identity of the source file.
	src string
	// verified is set once the digest line is complete.
	verified bool
	// want is the digest of the source's pairs when verified is set.
	want digest
}

// claimState is what a marker file holds.
type claimState int

const (
	// claimAbsent is no marker file.
	claimAbsent claimState = iota
	// claimTorn is an empty marker, which a crash during the claim's first
	// write leaves before any other conversion file exists.
	claimTorn
	// claimUnknown is a marker that does not parse as a claim.
	claimUnknown
	// claimHeld is a parsed claim.
	claimHeld
)

// readClaim reads the marker and returns its state, the claim when it parses,
// and the marker's description for identity checks before removal. A digest
// line a crash cut short leaves the claim unverified.
func readClaim(marker string) (claimState, convertClaim, os.FileInfo, error) {
	// Read the marker.
	fi, err := os.Lstat(marker)
	if errors.Is(err, fs.ErrNotExist) {
		return claimAbsent, convertClaim{}, nil, nil
	}
	if err != nil {
		return 0, convertClaim{}, nil, err
	}
	if !fi.Mode().IsRegular() {
		return claimUnknown, convertClaim{}, fi, nil
	}
	b, err := os.ReadFile(marker) // #nosec G304 -- beside the Volume file.
	if err != nil {
		return 0, convertClaim{}, nil, err
	}
	if len(b) == 0 {
		return claimTorn, convertClaim{}, fi, nil
	}

	// Parse the claim line, then a complete digest line.
	lines := strings.SplitAfter(string(b), "\n")
	f := strings.Fields(lines[0])
	if len(f) != 2 || f[0] != claimHeader || !strings.HasSuffix(lines[0], "\n") {
		return claimUnknown, convertClaim{}, fi, nil
	}
	c := convertClaim{src: f[1]}
	if len(lines) > 1 && strings.HasSuffix(lines[1], "\n") {
		c.want, c.verified = parseDigest(lines[1])
	}
	return claimHeld, c, fi, nil
}

// parseDigest parses a digest line, reporting false when it does not parse.
func parseDigest(line string) (digest, bool) {
	// Split the count and the hash.
	f := strings.Fields(line)
	if len(f) != 3 || f[0] != "verified" {
		return digest{}, false
	}
	n, err := strconv.ParseInt(f[1], 10, 64)
	if err != nil {
		return digest{}, false
	}
	x, err := hex.DecodeString(f[2])
	if err != nil || len(x) != 32 {
		return digest{}, false
	}
	return digest{n: n, x: [32]byte(x)}, true
}

// writeClaim creates the marker naming the source identity src and makes it
// durable, name included, before any other conversion file exists. It fails
// when the marker exists.
func writeClaim(marker, src string) error {
	// Create the marker exclusively and write the claim line.
	f, err := os.OpenFile(marker, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) // #nosec G304 -- beside the Volume file.
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(f, "%s %s\n", claimHeader, src)
	if err := errors.Join(err, f.Sync(), f.Close()); err != nil {
		return err
	}
	return syncDir(filepath.Dir(marker))
}

// appendDigest records d in the marker durably once the new file verified.
func appendDigest(marker string, d digest) error {
	f, err := os.OpenFile(marker, os.O_WRONLY|os.O_APPEND, 0) // #nosec G304 -- beside the Volume file.
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(f, "verified %d %s\n", d.n, hex.EncodeToString(d.x[:]))
	return errors.Join(err, f.Sync(), f.Close())
}

// claimFiles are the conversion files beside a Volume file that a claim of
// one source accounts for, each nil when absent.
type claimFiles struct {
	// claim is the parsed claim when the marker holds one.
	claim convertClaim
	// marker describes the marker file.
	marker os.FileInfo
	// tmp describes the new file the claim accounts for.
	tmp os.FileInfo
}

// inspectClaim describes the conversion files tmp and marker beside the
// source file whose identity is id. It refuses with a permanent error a file
// that no claim of id accounts for: a tmp file without a marker, or a marker
// that is not a claim of id. An empty marker alone is a claim a crash cut
// short before tmp existed.
func inspectClaim(id, tmp, marker string) (claimFiles, error) {
	// Read the claim and describe the new file.
	state, c, markerInfo, err := readClaim(marker)
	if err != nil {
		return claimFiles{}, err
	}
	tmpInfo, err := os.Lstat(tmp)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return claimFiles{}, err
	}

	// Accept only files a claim of id accounts for.
	switch {
	case state == claimAbsent && tmpInfo == nil:
		return claimFiles{}, nil
	case state == claimAbsent:
		return claimFiles{}, foreignFile(tmp)
	case state == claimTorn && tmpInfo == nil:
		return claimFiles{marker: markerInfo}, nil
	case state != claimHeld || c.src != id:
		return claimFiles{}, foreignFile(marker)
	}
	return claimFiles{claim: c, marker: markerInfo, tmp: tmpInfo}, nil
}

// clear removes the claimed files, the new file before the claim, each only
// while it is still the file inspectClaim described.
func (f claimFiles) clear(tmp, marker string) error {
	if f.tmp != nil {
		if err := removeSame(tmp, f.tmp); err != nil {
			return err
		}
	}
	if f.marker != nil {
		return removeSame(marker, f.marker)
	}
	return nil
}

// removeSame removes name only when it still is the file fi describes, so a
// file someone put in its place is kept. A missing name is not an error.
func removeSame(name string, fi os.FileInfo) error {
	// Compare the current file with fi, then remove it.
	cur, err := os.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !os.SameFile(cur, fi) {
		return foreignFile(name)
	}
	return os.Remove(name)
}

// foreignFile is the permanent refusal to convert beside a file that no
// claim of this conversion accounts for. A person decides what it is.
func foreignFile(name string) error {
	return volume.Permanent(fmt.Errorf("%s does not belong to a conversion of this volume; move it aside to convert", name))
}
