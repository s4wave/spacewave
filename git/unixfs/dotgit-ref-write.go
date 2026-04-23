package unixfs_git

import (
	"bufio"
	"bytes"
	"slices"
	"strings"

	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/pkg/errors"
)

func dotGitPathIsMetadataFile(path []string) bool {
	return slices.Equal(path, []string{"HEAD"}) ||
		slices.Equal(path, []string{"config"}) ||
		slices.Equal(path, []string{"shallow"}) ||
		slices.Equal(path, []string{"packed-refs"})
}

func dotGitPathIsLooseObject(path []string) bool {
	if len(path) != 3 || path[0] != "objects" {
		return false
	}
	if !dotGitIsLooseObjectPrefix(path[1]) {
		return false
	}
	return plumbing.IsHash(path[1] + path[2])
}

func dotGitPathIsObjectTemp(path []string) bool {
	if len(path) < 2 || path[0] != "objects" {
		return false
	}
	if path[1] == "info" || path[1] == "pack" {
		return false
	}
	if dotGitIsLooseObjectPrefix(path[1]) {
		return !dotGitPathIsLooseObject(path)
	}
	if len(path) == 2 && strings.HasPrefix(path[1], "tmp") {
		return true
	}
	return false
}

func dotGitLooseObjectHash(path []string) plumbing.Hash {
	return plumbing.NewHash(path[1] + path[2])
}

func dotGitPathIsReference(path []string) bool {
	if len(path) < 3 {
		return false
	}
	if dotGitRefsPathKind(path) == "" {
		return false
	}
	return !strings.HasSuffix(path[len(path)-1], ".lock")
}

func dotGitPathIsReferenceLock(path []string) bool {
	if len(path) < 3 {
		return false
	}
	if dotGitRefsPathKind(path) == "" {
		return false
	}
	return strings.HasSuffix(path[len(path)-1], ".lock")
}

func dotGitReferenceLockTarget(path []string) []string {
	if !dotGitPathIsReferenceLock(path) {
		return nil
	}
	out := slices.Clone(path)
	out[len(out)-1] = strings.TrimSuffix(out[len(out)-1], ".lock")
	return out
}

func dotGitReferenceNameFromPath(path []string) plumbing.ReferenceName {
	return plumbing.ReferenceName(strings.Join(path, "/"))
}

func dotGitParseReferenceContent(name plumbing.ReferenceName, content []byte) (*plumbing.Reference, bool, error) {
	data := bytes.TrimSpace(content)
	if len(data) == 0 {
		return nil, true, nil
	}
	if err := name.Validate(); err != nil {
		return nil, false, err
	}
	text := string(data)
	if after, ok := strings.CutPrefix(text, "ref:"); ok {
		target := plumbing.ReferenceName(strings.TrimSpace(after))
		if err := target.Validate(); err != nil {
			return nil, false, err
		}
		return plumbing.NewSymbolicReference(name, target), false, nil
	}
	if !plumbing.IsHash(text) {
		return nil, false, errors.Errorf("invalid reference target %q", text)
	}
	return plumbing.NewHashReference(name, plumbing.NewHash(text)), false, nil
}

func dotGitParseConfigContent(content []byte) (*config.Config, error) {
	cfg := config.NewConfig()
	if err := cfg.Unmarshal(content); err != nil {
		return nil, err
	}
	return cfg, nil
}

func dotGitParseShallowContent(content []byte) ([]plumbing.Hash, error) {
	var hashes []plumbing.Hash
	sc := bufio.NewScanner(bytes.NewReader(content))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if !plumbing.IsHash(line) {
			return nil, errors.Errorf("invalid shallow hash %q", line)
		}
		hashes = append(hashes, plumbing.NewHash(line))
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	plumbing.HashesSort(hashes)
	return hashes, nil
}

func dotGitParsePackedRefsContent(content []byte) ([]*plumbing.Reference, error) {
	var refs []*plumbing.Reference
	sc := bufio.NewScanner(bytes.NewReader(content))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "^") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) != 2 {
			return nil, errors.Errorf("invalid packed-refs line %q", line)
		}
		if !plumbing.IsHash(parts[0]) {
			return nil, errors.Errorf("invalid packed ref hash %q", parts[0])
		}
		name := plumbing.ReferenceName(parts[1])
		if err := name.Validate(); err != nil {
			return nil, err
		}
		refs = append(refs, plumbing.NewHashReference(name, plumbing.NewHash(parts[0])))
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return refs, nil
}
