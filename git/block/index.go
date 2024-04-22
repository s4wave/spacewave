package git_block

import (
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/index"
)

// NewIndex constructs a new Index from a git index.
func NewIndex(i *index.Index) (*Index, error) {
	if i == nil {
		return nil, nil
	}

	entries := make([]*IndexEntry, len(i.Entries))
	for i, e := range i.Entries {
		var err error
		entries[i], err = NewIndexEntry(e)
		if err != nil {
			return nil, err
		}
	}

	cache, err := NewTree(i.Cache)
	if err != nil {
		return nil, err
	}

	ru, err := NewResolveUndo(i.ResolveUndo)
	if err != nil {
		return nil, err
	}

	eoie, err := NewEndOfIndexEntry(i.EndOfIndexEntry)
	if err != nil {
		return nil, err
	}

	return &Index{
		Version:         i.Version,
		Entries:         entries,
		Cache:           cache,
		ResolveUndo:     ru,
		EndOfIndexEntry: eoie,
	}, nil
}

// ToGitIndex converts the index block to a git index.
func (i *Index) ToGitIndex() (*index.Index, error) {
	ents := i.GetEntries()
	cache, err := i.GetCache().ToGitTree()
	if err != nil {
		return nil, err
	}
	rundo, err := i.GetResolveUndo().ToGitResolveUndo()
	if err != nil {
		return nil, err
	}
	eoie, err := i.GetEndOfIndexEntry().ToGitEndOfIndexEntry()
	if err != nil {
		return nil, err
	}
	out := &index.Index{
		Version:         i.GetVersion(),
		Entries:         make([]*index.Entry, len(ents)),
		Cache:           cache,
		ResolveUndo:     rundo,
		EndOfIndexEntry: eoie,
	}
	for i, ent := range ents {
		dataHash, err := FromHash(ent.GetDataHash())
		if err != nil {
			return nil, err
		}
		out.Entries[i] = &index.Entry{
			Hash:         dataHash,
			Name:         ent.GetName(),
			CreatedAt:    ent.GetCreatedAt().AsTime(),
			ModifiedAt:   ent.GetModifiedAt().AsTime(),
			Dev:          ent.GetDev(),
			Inode:        ent.GetInode(),
			Mode:         filemode.FileMode(ent.GetFileMode()),
			UID:          ent.GetUid(),
			GID:          ent.GetGid(),
			Size:         ent.GetSize(),
			Stage:        index.Stage(ent.GetStage()),
			SkipWorktree: ent.GetSkipWorktree(),
			IntentToAdd:  ent.GetIntentToAdd(),
		}
	}
	return out, nil
}
