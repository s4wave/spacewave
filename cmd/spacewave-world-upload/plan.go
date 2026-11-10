package main

import (
	"bufio"
	"bytes"
	"crypto/md5" //nolint:gosec // Content-MD5 is the service's body check, not a security hash.
	"crypto/sha256"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/identity"
	"github.com/s4wave/spacewave/db/packfile/writer"
)

// entrySuffix ends the name of an entry file in the adopt layout.
const entrySuffix = ".entry"

// idPattern is a pack id: pfv1_ and base58.
const idPattern = `pfv1_[1-9A-HJ-NP-Za-km-z]+`

// idRE matches a whole pack id.
var idRE = regexp.MustCompile(`^` + idPattern + `$`)

// prefixRE matches a prefix: segments of safe bytes, each followed by /.
var prefixRE = regexp.MustCompile(`^([A-Za-z0-9_-][A-Za-z0-9._-]*/)+$`)

// bucketRE matches a DNS-style bucket name.
var bucketRE = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)

// regionRE matches a region name.
var regionRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// checkPrefix refuses a prefix that is empty, does not end in /, starts with
// /, holds an empty, . or .. segment, or any byte outside [A-Za-z0-9._-/].
func checkPrefix(prefix string) error {
	if !prefixRE.MatchString(prefix) {
		return errors.Errorf("--prefix %q must be segments of [A-Za-z0-9._-] each ending in /", prefix)
	}
	for _, seg := range strings.Split(strings.TrimSuffix(prefix, "/"), "/") {
		if seg == "." || seg == ".." || strings.HasPrefix(seg, ".") {
			return errors.Errorf("--prefix %q holds a dot segment", prefix)
		}
	}
	return nil
}

// keyGuard checks every object key against
// ^{prefix}(packs|entries)/pfv1_[base58]+$ for one checked prefix.
type keyGuard struct {
	re *regexp.Regexp
}

// newKeyGuard checks the prefix and builds its guard.
func newKeyGuard(prefix string) (*keyGuard, error) {
	if err := checkPrefix(prefix); err != nil {
		return nil, err
	}
	return &keyGuard{re: regexp.MustCompile(`^` + regexp.QuoteMeta(prefix) + `(packs|entries)/` + idPattern + `$`)}, nil
}

// check refuses a key that does not match the guard.
func (g *keyGuard) check(key string) error {
	if !g.re.MatchString(key) {
		return errors.Errorf("object key %q does not match %s", key, g.re)
	}
	return nil
}

// planned is one id of the plan: the keys, sizes and digests of its pack and
// entry, checked, without their bytes.
type planned struct {
	id          string
	pack, entry *object
	skip        bool
}

// listDir returns the ids of dir, byte-sorted. It refuses any file that is
// not a pack with its entry or an entry with its pack.
func listDir(dir string) ([]string, error) {
	// List the names, every one a regular file.
	names, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	have := make(map[string]bool, len(names))
	for _, n := range names {
		if !n.Type().IsRegular() {
			return nil, errors.Errorf("%s is not a regular file", filepath.Join(dir, n.Name()))
		}
		have[n.Name()] = true
	}

	// Pair each pack with its entry, refusing anything else.
	var ids []string
	for _, n := range names {
		name := n.Name()
		if id, ok := strings.CutSuffix(name, entrySuffix); ok {
			if !idRE.MatchString(id) || !have[id] {
				return nil, errors.Errorf("entry %s has no pack", filepath.Join(dir, name))
			}
			continue
		}
		if !idRE.MatchString(name) {
			return nil, errors.Errorf("%s is neither a pack nor an entry", filepath.Join(dir, name))
		}
		if !have[name+entrySuffix] {
			return nil, errors.Errorf("pack %s has no entry", filepath.Join(dir, name))
		}
		ids = append(ids, name)
	}
	slices.Sort(ids)
	return ids, nil
}

// readIDList reads a file of ids, one per line, blank lines ignored. It
// refuses a line that is not an id and an id listed twice.
func readIDList(path, flag string) ([]string, error) {
	// Read the file.
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrap(err, flag)
	}

	// Collect each id once, in order.
	var ids []string
	seen := make(map[string]bool)
	sc := bufio.NewScanner(bytes.NewReader(raw))
	for n := 1; sc.Scan(); n++ {
		id := strings.TrimSpace(sc.Text())
		if id == "" {
			continue
		}
		if !idRE.MatchString(id) {
			return nil, errors.Errorf("%s %s line %d is not a pack id", flag, path, n)
		}
		if seen[id] {
			return nil, errors.Errorf("%s %s line %d repeats %s", flag, path, n, id)
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids, sc.Err()
}

// readPair reads the pack and entry of id from dir and checks them: the
// entry names the pack's id and size, and the id is the v1 id of the pack's
// bytes under resourceID.
func readPair(dir, id, resourceID string) (pack, entry *object, err error) {
	// Read both files.
	data, err := os.ReadFile(filepath.Join(dir, id))
	if err != nil {
		return nil, nil, err
	}
	raw, err := os.ReadFile(filepath.Join(dir, id+entrySuffix))
	if err != nil {
		return nil, nil, err
	}

	// The entry names this pack at its size.
	ent := &packfile.PackfileEntry{}
	if err := ent.UnmarshalVT(raw); err != nil {
		return nil, nil, errors.Wrapf(err, "decode entry of %s", id)
	}
	if ent.GetId() != id {
		return nil, nil, errors.Errorf("entry of %s names id %s", id, ent.GetId())
	}
	if ent.GetSizeBytes() != uint64(len(data)) {
		return nil, nil, errors.Errorf("entry of %s names %d bytes, the pack is %d", id, ent.GetSizeBytes(), len(data))
	}

	// The id is the pack's own v1 id, from its sorted keys and bytes.
	if err := identity.ValidatePackID(id); err != nil {
		return nil, nil, errors.Wrapf(err, "pack %s", id)
	}
	want, err := packID(resourceID, data)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "pack %s", id)
	}
	if want != id {
		return nil, nil, errors.Errorf("pack %s: its bytes give id %s", id, want)
	}
	return newObject(data), newObject(raw), nil
}

// packID recomputes the v1 id of a pack from its bytes, as the re-pack
// tool's proof does.
func packID(resourceID string, data []byte) (string, error) {
	// Read the block keys from the pack's index.
	rdr, err := kvfile.BuildReader(bytes.NewReader(data), uint64(len(data)))
	if err != nil {
		return "", errors.Wrap(err, "read index")
	}
	keys := make([][]byte, 0, rdr.Size())
	for i := range rdr.Size() {
		ie, err := rdr.ReadIndexEntry(i)
		if err != nil {
			return "", errors.Wrap(err, "read index entry")
		}
		keys = append(keys, ie.GetKey())
	}

	// Build the id from the keys and the bytes.
	digest := sha256.Sum256(data)
	return identity.BuildPackID(resourceID, &writer.PackResult{
		SortedKeyDigest:  writer.DigestSortedKeys(keys),
		PackBytesDigest:  digest[:],
		PolicyTag:        writer.PolicyTag(writer.DefaultPolicy()),
		ValueOrderPolicy: writer.ValueOrderIterator,
	})
}

// newObject returns data with its digests.
func newObject(data []byte) *object {
	m := md5.Sum(data) //nolint:gosec // see the import.
	return &object{size: int64(len(data)), data: data, sha256: sha256.Sum256(data), md5: m[:]}
}
