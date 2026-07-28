package artifact

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

const identitySchemaVersion = 1

// Identity binds a release artifact to every source and build input that can
// determine its contents.
type Identity struct {
	SchemaVersion         int
	Digest                string
	Compiler              string
	Mode                  string
	SourceDigest          string
	BuildDigest           string
	LockfileDigest        string
	BldrDigest            string
	PrerenderInputsDigest string
}

// ComputeIdentity computes the release artifact identity from the current Git
// working tree, including tracked edits, staged edits, deletions, and untracked
// non-ignored files.
func ComputeIdentity(repoRoot string, inputs *BuildInputs) (*Identity, error) {
	buildDigest, err := inputs.digest()
	if err != nil {
		return nil, err
	}

	cmd := exec.Command("git", "ls-files", "--cached", "--others", "--exclude-standard", "-z")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, errors.Wrap(err, "list release artifact source files")
	}

	paths := make([]string, 0)
	for raw := range bytes.SplitSeq(out, []byte{0}) {
		if len(raw) != 0 {
			paths = append(paths, string(raw))
		}
	}
	slices.Sort(paths)

	sourceHash := sha256.New()
	lockfileHash := sha256.New()
	bldrHash := sha256.New()
	prerenderHash := sha256.New()
	for _, path := range paths {
		h := sourceHash
		switch {
		case isLockfile(path):
			h = lockfileHash
		case path == "bldr" || strings.HasPrefix(path, "bldr/"):
			h = bldrHash
		case path == "app/prerender" || strings.HasPrefix(path, "app/prerender/"):
			h = prerenderHash
		}
		if err := hashFile(h, repoRoot, path); err != nil {
			return nil, err
		}
	}

	identity := &Identity{
		SchemaVersion:         identitySchemaVersion,
		Compiler:              inputs.Compiler,
		Mode:                  inputs.Mode,
		SourceDigest:          hex.EncodeToString(sourceHash.Sum(nil)),
		BuildDigest:           buildDigest,
		LockfileDigest:        hex.EncodeToString(lockfileHash.Sum(nil)),
		BldrDigest:            hex.EncodeToString(bldrHash.Sum(nil)),
		PrerenderInputsDigest: hex.EncodeToString(prerenderHash.Sum(nil)),
	}
	identity.Digest = identity.contentDigest()
	return identity, nil
}

// Differences returns the current inputs that do not match the artifact.
func (i *Identity) Differences(artifact *Identity) []string {
	if artifact == nil {
		return []string{"missing identity"}
	}
	var differences []string
	if artifact.SchemaVersion != identitySchemaVersion {
		differences = append(differences, "identity schema")
	}
	if artifact.Compiler != i.Compiler {
		differences = append(differences, "compiler")
	}
	if artifact.Mode != i.Mode {
		differences = append(differences, "build mode")
	}
	if artifact.BuildDigest != i.BuildDigest {
		differences = append(differences, "build environment")
	}
	if artifact.SourceDigest != i.SourceDigest {
		differences = append(differences, "source content")
	}
	if artifact.LockfileDigest != i.LockfileDigest {
		differences = append(differences, "lockfiles")
	}
	if artifact.BldrDigest != i.BldrDigest {
		differences = append(differences, "Bldr cache format inputs")
	}
	if artifact.PrerenderInputsDigest != i.PrerenderInputsDigest {
		differences = append(differences, "prerender inputs")
	}
	if artifact.Digest != artifact.contentDigest() {
		differences = append(differences, "identity digest")
	}
	return differences
}

func (i *Identity) contentDigest() string {
	h := sha256.New()
	writeDigestField(h, "schema", strconv.Itoa(i.SchemaVersion))
	writeDigestField(h, "compiler", i.Compiler)
	writeDigestField(h, "mode", i.Mode)
	writeDigestField(h, "source", i.SourceDigest)
	writeDigestField(h, "build", i.BuildDigest)
	writeDigestField(h, "lockfiles", i.LockfileDigest)
	writeDigestField(h, "bldr", i.BldrDigest)
	writeDigestField(h, "prerender", i.PrerenderInputsDigest)
	return hex.EncodeToString(h.Sum(nil))
}

func isLockfile(path string) bool {
	switch filepath.Base(path) {
	case "bun.lock", "bun.lockb", "go.mod", "go.sum", "package-lock.json", "pnpm-lock.yaml", "yarn.lock":
		return true
	default:
		return false
	}
}

func hashFile(h hash.Hash, repoRoot, path string) error {
	fullPath := filepath.Join(repoRoot, filepath.FromSlash(path))
	info, err := os.Lstat(fullPath)
	if os.IsNotExist(err) {
		writeDigestField(h, path, "deleted")
		return nil
	}
	if err != nil {
		return errors.Wrapf(err, "stat release artifact input %s", path)
	}

	writeDigestField(h, "path", path)
	writeDigestField(h, "mode", strconv.FormatUint(uint64(info.Mode()), 8))
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(fullPath)
		if err != nil {
			return errors.Wrapf(err, "read release artifact input symlink %s", path)
		}
		writeDigestField(h, "symlink", target)
		return nil
	}
	if !info.Mode().IsRegular() {
		return errors.Errorf("release artifact input %s has unsupported mode %s", path, info.Mode())
	}

	writeDigestField(h, "size", strconv.FormatInt(info.Size(), 10))
	f, err := os.Open(fullPath)
	if err != nil {
		return errors.Wrapf(err, "open release artifact input %s", path)
	}
	if _, err := io.Copy(h, f); err != nil {
		f.Close()
		return errors.Wrapf(err, "hash release artifact input %s", path)
	}
	if err := f.Close(); err != nil {
		return errors.Wrapf(err, "close release artifact input %s", path)
	}
	h.Write([]byte{0})
	return nil
}
