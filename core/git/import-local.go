package s4wave_git

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	stderrors "errors"
	"io"
	"path/filepath"
	"slices"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/format/packfile"
	"github.com/go-git/go-git/v6/plumbing/revlist"
	"github.com/go-git/go-git/v6/plumbing/storer"
	"github.com/go-git/go-git/v6/storage/memory"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	git_block "github.com/s4wave/spacewave/db/git/block"
	"github.com/s4wave/spacewave/db/world"
)

// ImportLocalRepoToRef imports the committed graph of a local Git repository
// into an immutable World repo ref without copying configuration or contacting remotes.
// The World must be writable. It retains a verified repo for the canonical source
// path so subsequent imports reuse its packs and encode only missing objects.
// Previously returned refs keep their captured HEAD and references as the source advances.
func ImportLocalRepoToRef(
	ctx context.Context,
	ws world.WorldState,
	localPath string,
) (repoRef *bucket.ObjectRef, headHash, branchName string, err error) {
	// Open the source and bind its file handles to this import.
	repo, err := git.PlainOpen(localPath)
	if err != nil {
		return nil, "", "", errors.Wrap(err, "open local repository")
	}
	closer, ok := repo.Storer.(io.Closer)
	if !ok {
		return nil, "", "", errors.New("local repository storage is not closeable")
	}
	defer func() {
		if closeErr := closer.Close(); closeErr != nil {
			err = stderrors.Join(err, errors.Wrap(closeErr, "close local repository"))
		}
	}()

	// Identify the source without publishing its host path in the World.
	key, err := localRepoImportKey(localPath)
	if err != nil {
		return nil, "", "", err
	}

	// Import only committed objects and references into the retained repository.
	return importOpenedLocalRepo(ctx, ws, repo, key, nil)
}

// localRepoImportKey identifies the retained import of a canonical source directory.
// The key contains a hash of the absolute, symlink-resolved path, never the path itself.
func localRepoImportKey(localPath string) (string, error) {
	// Resolve equivalent spellings of the same source directory.
	abs, err := filepath.Abs(localPath)
	if err != nil {
		return "", errors.Wrap(err, "resolve local repository path")
	}
	canonical, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", errors.Wrap(err, "resolve local repository symlinks")
	}

	// Keep source identity separate from the returned immutable repository snapshot.
	digest := sha256.Sum256([]byte(canonical))
	return "git/local-import/" + hex.EncodeToString(digest[:]), nil
}

// importOpenedLocalRepo extends a verified source import and returns its immutable snapshot.
func importOpenedLocalRepo(
	ctx context.Context,
	ws world.WorldState,
	repo *git.Repository,
	sourceKey string,
	afterObject func(),
) (repoRef *bucket.ObjectRef, headHash, branchName string, err error) {
	// Require publication so the next import can reuse the verified object graph.
	if ws.GetReadOnly() {
		return nil, "", "", errors.New("local repository import requires a writable World")
	}

	// Freeze the reference roots and HEAD identity before reading objects.
	snapshot, err := captureLocalRepo(repo)
	if err != nil {
		return nil, "", "", err
	}
	headHash = snapshot.resolvedHead.Hash().String()
	if snapshot.rawHead.Type() == plumbing.SymbolicReference && snapshot.resolvedHead.Name().IsBranch() {
		branchName = snapshot.resolvedHead.Name().Short()
	}

	// Extend the retained source only after its objects and references verify.
	repoRef, _, err = world.AccessWorldObject(ctx, ws, sourceKey, true, func(bcs *block.Cursor) (cbErr error) {
		// Open the retained packs or initialize the source's first import.
		fresh := bcs.GetRef().GetEmpty()
		if fresh {
			bcs.SetBlock(git_block.NewRepo(), true)
		}
		store, err := git_block.NewStore(ctx, nil, bcs, &memory.IndexStorage{}, nil)
		if err != nil {
			return err
		}
		defer func() {
			if closeErr := store.Close(); closeErr != nil {
				cbErr = stderrors.Join(cbErr, errors.Wrap(closeErr, "close imported repository"))
			}
		}()

		// Reuse only closures verified for the prior roots and shallow boundary.
		// This case reads indexes, never source or destination pack data.
		shallow, err := store.Shallow()
		if err != nil {
			return err
		}
		known := !fresh && slices.Equal(shallow, snapshot.shallow)
		if known {
			// Only the prior captured roots promise a complete closure under this
			// shallow boundary; older retained objects can come from another boundary.
			prior, err := captureLocalRepo(&git.Repository{Storer: store})
			if err != nil {
				return err
			}
			verified := makeHashSet(prior.wants)
			for _, hash := range snapshot.wants {
				if _, ok := verified[hash]; !ok {
					known = false
					break
				}
			}
		}

		// Confirm captured roots through the object index without opening packs.
		for _, hash := range snapshot.wants {
			if err := store.HasEncodedObject(hash); err != nil {
				if !errors.Is(err, plumbing.ErrObjectNotFound) {
					return err
				}
				known = false
				break
			}
		}

		// Walk changed source graphs and select only objects absent from the index.
		var wantedSet map[plumbing.Hash]struct{}
		var missing []plumbing.Hash
		if !known {
			// Capture the new closure before extending the retained pack set.
			wanted, err := revlist.Objects(repo.Storer, snapshot.wants, nil)
			if err != nil {
				return errors.Wrap(err, "walk local repository objects")
			}
			wantedSet = makeHashSet(wanted)
			missing = make([]plumbing.Hash, 0, len(wanted))

			// Preserve every indexed object, including objects from deleted refs.
			for _, hash := range wanted {
				if err := ctx.Err(); err != nil {
					return err
				}
				if err := store.HasEncodedObject(hash); err != nil {
					if !errors.Is(err, plumbing.ErrObjectNotFound) {
						return err
					}
					missing = append(missing, hash)
				}
			}
		}

		// Retain source deltas when their bases are also missing. The encoder
		// expands deltas against indexed bases, keeping the new pack self-contained.
		if len(missing) != 0 {
			// Encode only the missing graph; an unchanged import creates no pack.
			writer, err := store.PackfileWriter()
			if err != nil {
				return err
			}
			source := &importObjectStore{Storer: repo.Storer, ctx: ctx, afterObject: afterObject}
			if _, err := packfile.NewEncoder(writer, source, false).Encode(missing, 1); err != nil {
				return errors.Wrap(err, "encode local repository pack")
			}
			if err := writer.Close(); err != nil {
				return errors.Wrap(err, "store local repository pack")
			}
		}

		// Replace source references, including names deleted since the prior import.
		refs := make(map[plumbing.ReferenceName]*plumbing.Reference, len(snapshot.refs))
		for _, ref := range snapshot.refs {
			refs[ref.Name()] = ref
		}
		iter, err := store.IterReferences()
		if err != nil {
			return err
		}
		defer iter.Close()
		if err := iter.ForEach(func(ref *plumbing.Reference) error {
			if current := refs[ref.Name()]; sameReference(ref, current) {
				delete(refs, ref.Name())
				return nil
			}
			if refs[ref.Name()] == nil {
				return store.RemoveReference(ref.Name())
			}
			return nil
		}); err != nil {
			return errors.Wrap(err, "remove obsolete local Git references")
		}

		// Publish changed reference values and shallow history boundaries.
		for _, ref := range refs {
			if err := store.SetReference(ref); err != nil {
				return errors.Wrapf(err, "store local Git reference %s", ref.Name())
			}
		}
		if !slices.Equal(shallow, snapshot.shallow) {
			if err := store.SetShallow(snapshot.shallow); err != nil {
				return errors.Wrap(err, "store local shallow boundary")
			}
		}

		// Reject incomplete graphs, source HEAD drift, and canceled imports.
		if err := verifyImportedRepo(repo, store, snapshot, wantedSet); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return store.Commit()
	})

	// Concurrent first imports can finish before either creates the retained
	// object. AccessWorldObject still returns the completed immutable snapshot
	// when another importer wins creation; that snapshot is valid for this caller.
	if err != nil && !errors.Is(err, world.ErrObjectExists) {
		return nil, "", "", errors.Wrap(err, "import local repository")
	}
	return repoRef, headHash, branchName, nil
}

// localRepoSnapshot holds the captured references and shallow graph roots.
type localRepoSnapshot struct {
	// resolvedHead identifies the selected commit.
	resolvedHead *plumbing.Reference
	// rawHead preserves detached or symbolic HEAD form.
	rawHead *plumbing.Reference
	// refs contains every captured reference, including HEAD.
	refs []*plumbing.Reference
	// wants contains the distinct commit and tag roots.
	wants []plumbing.Hash
	// shallow marks the permitted history boundaries.
	shallow []plumbing.Hash
}

// captureLocalRepo snapshots HEAD, references, and the shallow boundary.
func captureLocalRepo(repo *git.Repository) (*localRepoSnapshot, error) {
	// Capture both HEAD forms before enumerating other references.
	resolvedHead, err := repo.Head()
	if err != nil {
		return nil, errors.Wrap(err, "resolve local HEAD")
	}
	rawHead, err := repo.Reference(plumbing.HEAD, false)
	if err != nil {
		return nil, errors.Wrap(err, "read local HEAD")
	}

	// Collect references by name so HEAD has exactly one captured value.
	iter, err := repo.References()
	if err != nil {
		return nil, errors.Wrap(err, "list local references")
	}
	defer iter.Close()
	refsByName := make(map[plumbing.ReferenceName]*plumbing.Reference)
	err = iter.ForEach(func(ref *plumbing.Reference) error {
		refsByName[ref.Name()] = ref
		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "list local references")
	}

	// Detect HEAD changes while the reference iterator was running.
	currentRawHead, err := repo.Reference(plumbing.HEAD, false)
	if err != nil {
		return nil, errors.Wrap(err, "reread local HEAD")
	}
	if !sameReference(rawHead, currentRawHead) {
		return nil, errors.New("local HEAD changed during import")
	}
	refsByName[plumbing.HEAD] = rawHead

	// Derive deterministic reference and object-root ordering.
	refs := make([]*plumbing.Reference, 0, len(refsByName))
	wantSet := map[plumbing.Hash]struct{}{resolvedHead.Hash(): {}}
	for _, ref := range refsByName {
		refs = append(refs, ref)
		if ref.Type() == plumbing.HashReference {
			wantSet[ref.Hash()] = struct{}{}
		}
	}
	slices.SortFunc(refs, func(a, b *plumbing.Reference) int {
		return cmp.Compare(a.Name().String(), b.Name().String())
	})
	wants := make([]plumbing.Hash, 0, len(wantSet))
	for hash := range wantSet {
		wants = append(wants, hash)
	}
	slices.SortFunc(wants, func(a, b plumbing.Hash) int {
		return cmp.Compare(a.String(), b.String())
	})

	// Retain shallow boundaries alongside the captured roots.
	shallow, err := repo.Storer.Shallow()
	if err != nil {
		return nil, errors.Wrap(err, "read local shallow boundary")
	}
	return &localRepoSnapshot{
		resolvedHead: resolvedHead,
		rawHead:      rawHead,
		refs:         refs,
		wants:        wants,
		shallow:      shallow,
	}, nil
}

// verifyImportedRepo checks graph closure and stable source and destination HEADs.
func verifyImportedRepo(
	source *git.Repository,
	destination storer.Storer,
	snapshot *localRepoSnapshot,
	wantedSet map[plumbing.Hash]struct{},
) error {
	// Verify new closures in full; indexed roots retain the prior verification.
	if wantedSet != nil {
		got, err := revlist.Objects(destination, snapshot.wants, nil)
		if err != nil {
			return errors.Wrap(err, "walk imported repository objects")
		}
		if !sameHashSet(wantedSet, got) {
			return errors.New("imported repository object closure differs from source")
		}
	}

	// Verify the destination preserves resolved and symbolic HEAD identity.
	destinationHead, err := storer.ResolveReference(destination, plumbing.HEAD)
	if err != nil {
		return errors.Wrap(err, "resolve imported HEAD")
	}
	if destinationHead.Hash() != snapshot.resolvedHead.Hash() {
		return errors.New("imported HEAD differs from captured source HEAD")
	}
	destinationRawHead, err := destination.Reference(plumbing.HEAD)
	if err != nil {
		return errors.Wrap(err, "read imported HEAD")
	}
	if !sameReference(destinationRawHead, snapshot.rawHead) {
		return errors.New("imported raw HEAD differs from captured source HEAD")
	}

	// Fence publication against a source checkout change during the import.
	currentHead, err := source.Head()
	if err != nil {
		return errors.Wrap(err, "reresolve local HEAD")
	}
	currentRawHead, err := source.Reference(plumbing.HEAD, false)
	if err != nil {
		return errors.Wrap(err, "reread local HEAD")
	}
	if currentHead.Hash() != snapshot.resolvedHead.Hash() || !sameReference(currentRawHead, snapshot.rawHead) {
		return errors.New("local HEAD changed during import")
	}
	return nil
}

// sameReference compares complete reference identity without resolving symbols.
func sameReference(a, b *plumbing.Reference) bool {
	return a != nil && b != nil && a.Name() == b.Name() && a.Type() == b.Type() &&
		a.Hash() == b.Hash() && a.Target() == b.Target()
}

// makeHashSet indexes hashes for closure comparison.
func makeHashSet(hashes []plumbing.Hash) map[plumbing.Hash]struct{} {
	set := make(map[plumbing.Hash]struct{}, len(hashes))
	for _, hash := range hashes {
		set[hash] = struct{}{}
	}
	return set
}

// sameHashSet checks whether a traversal matches the captured unique closure.
func sameHashSet(want map[plumbing.Hash]struct{}, got []plumbing.Hash) bool {
	if len(want) != len(got) {
		return false
	}
	for _, hash := range got {
		if _, ok := want[hash]; !ok {
			return false
		}
	}
	return true
}

// importObjectStore binds object reads during pack selection to the import lifetime.
type importObjectStore struct {
	// Storer supplies repository metadata and object operations to the encoder.
	storer.Storer

	// ctx bounds object selection to the enclosing import operation.
	ctx context.Context
	// afterObject observes successful reads for cancellation regression tests.
	afterObject func()
}

// EncodedObject reads a complete object while the import remains active.
func (s *importObjectStore) EncodedObject(typ plumbing.ObjectType, hash plumbing.Hash) (plumbing.EncodedObject, error) {
	// Stop selection before another source object is read.
	if err := s.ctx.Err(); err != nil {
		return nil, err
	}

	// Read the complete object and expose successful progress to the test seam.
	obj, err := s.Storer.EncodedObject(typ, hash)
	if err == nil && s.afterObject != nil {
		s.afterObject()
	}
	return obj, err
}

// DeltaObject retains a source delta when available, including alternate lookup.
func (s *importObjectStore) DeltaObject(typ plumbing.ObjectType, hash plumbing.Hash) (plumbing.EncodedObject, error) {
	// Stop selection before another source object is read.
	if err := s.ctx.Err(); err != nil {
		return nil, err
	}

	// Prefer existing deltas and use complete objects for alternate databases.
	if delta, ok := s.Storer.(storer.DeltaObjectStorer); ok {
		// Read the stored delta representation when the source supports it.
		obj, err := delta.DeltaObject(typ, hash)

		// Delta lookup may exclude alternate object databases; the ordinary
		// lookup still resolves their complete objects.
		if errors.Is(err, plumbing.ErrObjectNotFound) {
			return s.EncodedObject(typ, hash)
		}

		// Report a successful object read to the test seam.
		if err == nil && s.afterObject != nil {
			s.afterObject()
		}
		return obj, err
	}
	return s.EncodedObject(typ, hash)
}

// Compile-time interface assertion.
var _ storer.DeltaObjectStorer = (*importObjectStore)(nil)
