package unixfs_git

import (
	"slices"
	"strings"
	"sync"

	"github.com/s4wave/spacewave/db/unixfs"
)

type dotGitWriteState struct {
	mtx   sync.Mutex
	dirs  map[string]struct{}
	files map[string][]byte
}

func newDotGitWriteState() *dotGitWriteState {
	return &dotGitWriteState{
		dirs:  make(map[string]struct{}),
		files: make(map[string][]byte),
	}
}

func (s *dotGitWriteState) get(path []string) ([]byte, bool) {
	// Use a missing result when no .git write state exists.
	if s == nil {
		return nil, false
	}

	// Read and clone the staged .git file under the write-state mutex.
	s.mtx.Lock()
	defer s.mtx.Unlock()
	data, ok := s.files[dotGitWriteStateKey(path)]
	if !ok {
		return nil, false
	}
	return slices.Clone(data), true
}

func (s *dotGitWriteState) set(path []string, data []byte) {
	if s == nil {
		return
	}
	s.mtx.Lock()
	s.files[dotGitWriteStateKey(path)] = slices.Clone(data)
	s.mtx.Unlock()
}

func (s *dotGitWriteState) setDir(path []string) {
	if s == nil {
		return
	}
	s.mtx.Lock()
	s.dirs[dotGitWriteStateKey(path)] = struct{}{}
	s.mtx.Unlock()
}

func (s *dotGitWriteState) remove(path []string) {
	// Ignore removal when no .git write state exists.
	if s == nil {
		return
	}

	// Remove staged .git file and directory entries under the write-state mutex.
	s.mtx.Lock()
	delete(s.files, dotGitWriteStateKey(path))
	delete(s.dirs, dotGitWriteStateKey(path))
	s.mtx.Unlock()
}

func (s *dotGitWriteState) lookup(dirPath []string, name string) (*dotGitNode, bool) {
	// Use a missing result when no .git write state exists.
	if s == nil {
		return nil, false
	}

	// Lock the .git write state for a consistent child lookup.
	s.mtx.Lock()
	defer s.mtx.Unlock()

	// Resolve an explicitly staged .git directory.
	if _, ok := s.dirs[dotGitWriteStateKey(append(slices.Clone(dirPath), name))]; ok {
		return newDotGitDirNode(name, append(slices.Clone(dirPath), name)), true
	}

	// Resolve a staged .git file or an implicit parent directory.
	var foundDir bool
	for key, data := range s.files {
		path := dotGitWriteStatePath(key)
		if len(path) <= len(dirPath) || !slices.Equal(path[:len(dirPath)], dirPath) || path[len(dirPath)] != name {
			continue
		}
		nextPath := slices.Clone(path[:len(dirPath)+1])
		if len(path) == len(dirPath)+1 {
			return newDotGitFileNode(name, nextPath, slices.Clone(data)), true
		}
		foundDir = true
	}

	// Expose an implicit .git directory when staged children exist.
	if foundDir {
		return newDotGitDirNode(name, append(slices.Clone(dirPath), name)), true
	}
	return nil, false
}

func (s *dotGitWriteState) overlayDirents(dirPath []string, ents []unixfsDirentInfo) []unixfsDirentInfo {
	// Keep stored .git entries when no write overlay exists.
	if s == nil {
		return ents
	}

	// Lock the .git write state while collecting overlay entries.
	s.mtx.Lock()
	defer s.mtx.Unlock()

	// Index the stored .git directory entries by name.
	seen := make(map[string]unixfsDirentInfo)
	for _, ent := range ents {
		seen[ent.name] = ent
	}

	// Merge staged .git files and their parent directories.
	for key := range s.files {
		path := dotGitWriteStatePath(key)
		if len(path) <= len(dirPath) || !slices.Equal(path[:len(dirPath)], dirPath) {
			continue
		}
		name := path[len(dirPath)]
		ent := seen[name]
		ent.name = name
		if len(path) == len(dirPath)+1 {
			ent.isFile = true
		} else {
			ent.isDir = true
			ent.isFile = false
		}
		seen[name] = ent
	}

	// Merge explicitly staged .git directories.
	for key := range s.dirs {
		path := dotGitWriteStatePath(key)
		if len(path) <= len(dirPath) || !slices.Equal(path[:len(dirPath)], dirPath) {
			continue
		}
		name := path[len(dirPath)]
		ent := seen[name]
		ent.name = name
		ent.isDir = true
		ent.isFile = false
		seen[name] = ent
	}

	// Sort the overlaid .git directory names.
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	slices.Sort(names)

	// Build the ordered .git directory overlay entries.
	out := make([]unixfsDirentInfo, 0, len(names))
	for _, name := range names {
		out = append(out, seen[name])
	}
	return out
}

func dotGitWriteStateKey(path []string) string {
	return strings.Join(path, "/")
}

func dotGitWriteStatePath(key string) []string {
	if key == "" {
		return nil
	}
	return strings.Split(key, "/")
}

type unixfsDirentInfo struct {
	name   string
	isDir  bool
	isFile bool
}

func dotGitDirentsToInfo(ents []unixfs.FSCursorDirent) []unixfsDirentInfo {
	out := make([]unixfsDirentInfo, 0, len(ents))
	for _, ent := range ents {
		out = append(out, unixfsDirentInfo{
			name:   ent.GetName(),
			isDir:  ent.GetIsDirectory(),
			isFile: ent.GetIsFile(),
		})
	}
	return out
}

func dotGitInfoToDirents(infos []unixfsDirentInfo) []unixfs.FSCursorDirent {
	ents := make([]unixfs.FSCursorDirent, 0, len(infos))
	for _, info := range infos {
		ents = append(ents, &gitDirent{
			name:   info.name,
			isDir:  info.isDir,
			isFile: info.isFile,
		})
	}
	return ents
}
