// Package git_remote implements a Git remote helper that stores refs and
// objects in a Space's Git Repo object.
//
// Git runs git-remote-spacewave for a remote whose URL uses the spacewave
// scheme, sets GIT_DIR to the local repository, and exchanges line commands on
// the helper's stdin and stdout. See https://git-scm.com/docs/gitremote-helpers.
package git_remote

import (
	"bufio"
	"context"
	"io"
	"strconv"
	"strings"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/bucket"
	git_world "github.com/s4wave/spacewave/db/git/world"
	"github.com/s4wave/spacewave/db/world"
)

// fetchRefPrefix holds the local refs a fetch writes while it transfers
// objects. Git updates its own refs from the listed hashes afterward, so the
// helper removes these before it reports the fetch complete.
const fetchRefPrefix = "refs/spacewave-fetch/"

// localRemoteName names the in-process remote for the local repository.
const localRemoteName = "local"

// Helper serves the Git remote helper protocol for one Space Repo object.
type Helper struct {
	// engine is the Space World holding the Repo object.
	engine world.Engine
	// objectKey is the Repo object key.
	objectKey string
	// gitDir is the local repository's Git directory.
	gitDir string
	// pushed records whether a push changed the Repo.
	pushed bool
}

// NewHelper builds a Helper that moves refs and objects between the local
// repository at gitDir and the Repo object at objectKey.
func NewHelper(engine world.Engine, objectKey, gitDir string) *Helper {
	return &Helper{engine: engine, objectKey: objectKey, gitDir: gitDir}
}

// Pushed reports whether Run changed the Repo, so the caller can wait for the
// Space to sync before Git reports the push complete.
func (h *Helper) Pushed() bool {
	return h.pushed
}

// pushCommand is one ref update Git asks the helper to push.
type pushCommand struct {
	// force permits a non-fast-forward update.
	force bool
	// src is the local ref or object; empty deletes dst.
	src string
	// dst is the Repo ref to update.
	dst string
}

// Run serves commands from in until Git closes it or sends a blank line.
func (h *Helper) Run(ctx context.Context, in io.Reader, out io.Writer) error {
	r := bufio.NewReader(in)
	w := bufio.NewWriter(out)
	for {
		line, err := readLine(r)
		if err == io.EOF || (err == nil && line == "") {
			return nil
		}
		if err != nil {
			return err
		}

		switch {
		case line == "capabilities":
			_, err = w.WriteString("fetch\npush\n\n")
		case line == "list" || line == "list for-push":
			err = h.list(ctx, w)
		case strings.HasPrefix(line, "fetch "):
			err = h.serveFetch(ctx, r, w, line)
		case strings.HasPrefix(line, "push "):
			err = h.servePush(ctx, r, w, line)
		default:
			err = errors.Errorf("unsupported remote helper command %q", line)
		}
		if err != nil {
			return err
		}
		if err := w.Flush(); err != nil {
			return err
		}
	}
}

// readLine reads one command line without its line ending.
func readLine(r *bufio.Reader) (string, error) {
	line, err := r.ReadString('\n')
	if err == io.EOF && line != "" {
		err = nil
	}
	return strings.TrimRight(line, "\r\n"), err
}

// readBatch reads the command lines that follow first up to a blank line.
func readBatch(r *bufio.Reader, first string) ([]string, error) {
	batch := []string{first}
	for {
		line, err := readLine(r)
		if err == io.EOF || (err == nil && line == "") {
			return batch, nil
		}
		if err != nil {
			return nil, err
		}
		batch = append(batch, line)
	}
}

// list writes every Repo ref and HEAD in the helper list format. A missing
// Repo lists no refs, so the first push can create it.
func (h *Helper) list(ctx context.Context, w *bufio.Writer) error {
	_, _, found, err := h.repoRoot(ctx)
	if err != nil {
		return err
	}
	if !found {
		_, err = w.WriteString("\n")
		return err
	}
	err = h.readRepo(ctx, func(repo *git.Repository) error {
		refs, err := repo.Storer.IterReferences()
		if err != nil {
			return err
		}
		var head *plumbing.Reference
		err = refs.ForEach(func(ref *plumbing.Reference) error {
			if ref.Name() == plumbing.HEAD {
				head = ref
				return nil
			}
			return writeListRef(w, ref)
		})
		if err != nil || head == nil {
			return err
		}
		return writeListRef(w, head)
	})
	if err != nil {
		return err
	}
	_, err = w.WriteString("\n")
	return err
}

// writeListRef writes one ref as "<hash> <name>" or "@<target> <name>".
func writeListRef(w *bufio.Writer, ref *plumbing.Reference) error {
	value := ref.Hash().String()
	if ref.Type() == plumbing.SymbolicReference {
		value = "@" + ref.Target().String()
	}
	_, err := w.WriteString(value + " " + ref.Name().String() + "\n")
	return err
}

// serveFetch copies the objects for a batch of "fetch <hash> <name>" lines
// into the local repository.
func (h *Helper) serveFetch(ctx context.Context, r *bufio.Reader, w *bufio.Writer, first string) error {
	batch, err := readBatch(r, first)
	if err != nil {
		return err
	}
	hashes := make([]string, 0, len(batch))
	for _, line := range batch {
		fields := strings.Fields(line)
		if len(fields) != 3 || fields[0] != "fetch" {
			return errors.Errorf("malformed fetch command %q", line)
		}
		hashes = append(hashes, fields[1])
	}
	if err := h.fetch(ctx, hashes); err != nil {
		return err
	}
	_, err = w.WriteString("\n")
	return err
}

// fetch pushes the objects reachable from hashes from the Repo into the local
// repository under temporary refs, then removes those refs.
func (h *Helper) fetch(ctx context.Context, hashes []string) error {
	specs := make([]config.RefSpec, len(hashes))
	for i, hash := range hashes {
		specs[i] = config.RefSpec("+" + hash + ":" + fetchRefPrefix + strconv.Itoa(i))
	}
	err := h.readRepo(ctx, func(repo *git.Repository) error {
		remote := git.NewRemote(repo.Storer, &config.RemoteConfig{Name: localRemoteName, URLs: []string{h.gitDir}})
		err := remote.PushContext(ctx, &git.PushOptions{RemoteName: localRemoteName, RefSpecs: specs})
		if errors.Is(err, git.NoErrAlreadyUpToDate) {
			return nil
		}
		return err
	})
	if err != nil {
		return errors.Wrap(err, "copy objects to the local repository")
	}

	local, err := git.PlainOpen(h.gitDir)
	if err != nil {
		return errors.Wrap(err, "open local repository")
	}
	for i := range hashes {
		name := plumbing.ReferenceName(fetchRefPrefix + strconv.Itoa(i))
		if err := local.Storer.RemoveReference(name); err != nil {
			return errors.Wrapf(err, "remove %s", name)
		}
	}
	return nil
}

// servePush applies a batch of "push [+]<src>:<dst>" lines and reports each
// ref's result.
func (h *Helper) servePush(ctx context.Context, r *bufio.Reader, w *bufio.Writer, first string) error {
	batch, err := readBatch(r, first)
	if err != nil {
		return err
	}
	cmds := make([]pushCommand, 0, len(batch))
	for _, line := range batch {
		spec, ok := strings.CutPrefix(line, "push ")
		if !ok {
			return errors.Errorf("malformed push command %q", line)
		}
		var cmd pushCommand
		spec, cmd.force = strings.CutPrefix(spec, "+")
		cmd.src, cmd.dst, ok = strings.Cut(spec, ":")
		if !ok || cmd.dst == "" {
			return errors.Errorf("malformed push command %q", line)
		}
		cmds = append(cmds, cmd)
	}

	results, err := h.push(ctx, cmds)
	if err != nil {
		return err
	}
	for i, cmd := range cmds {
		status := "ok " + cmd.dst
		if results[i] != "" {
			status = "error " + cmd.dst + " " + results[i]
		}
		if _, err := w.WriteString(status + "\n"); err != nil {
			return err
		}
	}
	_, err = w.WriteString("\n")
	return err
}

// push applies cmds to the Repo, creating it when missing, and returns a
// failure reason per command, empty on success. Git transfer runs outside the
// World writer; the new Repo root is adopted only if no other writer moved it
// meanwhile, otherwise the push repeats against the current root.
func (h *Helper) push(ctx context.Context, cmds []pushCommand) ([]string, error) {
	for {
		prevRef, prevRev, found, err := h.repoRoot(ctx)
		if err != nil {
			return nil, err
		}
		if !found {
			if err := h.createRepo(ctx); err != nil {
				return nil, err
			}
			continue
		}

		results := make([]string, len(cmds))
		nextRef, err := git_world.AccessRepo(ctx, h.engine.AccessWorldState, prevRef, nil, nil, nil, func(repo *git.Repository) error {
			remote := git.NewRemote(repo.Storer, &config.RemoteConfig{Name: localRemoteName, URLs: []string{h.gitDir}})
			for i, cmd := range cmds {
				reason, err := h.pushOne(ctx, repo, remote, cmd)
				if err != nil {
					return err
				}
				results[i] = reason
			}
			return nil
		})
		if err != nil {
			return nil, errors.Wrap(err, "update the Space repository")
		}

		applied, err := h.adoptRoot(ctx, prevRev, nextRef)
		if err != nil || applied {
			h.pushed = applied
			return results, err
		}
	}
}

// pushOne applies one ref update inside the Repo and returns the reason it was
// refused, or an error when the Repo itself failed.
func (h *Helper) pushOne(ctx context.Context, repo *git.Repository, remote *git.Remote, cmd pushCommand) (string, error) {
	dst := plumbing.ReferenceName(cmd.dst)
	if cmd.src == "" {
		return "", repo.Storer.RemoveReference(dst)
	}
	spec := cmd.src + ":" + cmd.dst
	if cmd.force {
		spec = "+" + spec
	}
	err := remote.FetchContext(ctx, &git.FetchOptions{
		RemoteName: localRemoteName,
		RefSpecs:   []config.RefSpec{config.RefSpec(spec)},
		Tags:       git.NoTags,
	})
	switch {
	case err == nil || errors.Is(err, git.NoErrAlreadyUpToDate):
		return "", nil
	case errors.Is(err, git.ErrForceNeeded):
		return "non-fast-forward", nil
	case ctx.Err() != nil:
		return "", ctx.Err()
	default:
		return strings.ReplaceAll(err.Error(), "\n", " "), nil
	}
}

// readRepo opens the Repo read-only in one World read transaction.
func (h *Helper) readRepo(ctx context.Context, cb func(repo *git.Repository) error) error {
	return world.ExecTransaction(ctx, h.engine, false, func(ctx context.Context, ws world.WorldState) error {
		_, _, err := git_world.AccessWorldObjectRepo(ctx, ws, h.objectKey, false, nil, nil, nil, cb)
		return err
	})
}

// repoRoot returns the Repo object's root ref and revision, or found false
// when the object does not exist.
func (h *Helper) repoRoot(ctx context.Context) (ref *bucket.ObjectRef, rev uint64, found bool, err error) {
	err = world.ExecTransaction(ctx, h.engine, false, func(ctx context.Context, ws world.WorldState) error {
		obj, ok, err := ws.GetObject(ctx, h.objectKey)
		defer world.ReleaseObjectState(obj)
		if err != nil || !ok {
			return err
		}
		found = true
		ref, rev, err = obj.GetRootRef(ctx)
		return err
	})
	return ref, rev, found, err
}

// createRepo creates an empty Repo object without a worktree.
func (h *Helper) createRepo(ctx context.Context) error {
	err := world.ExecTransaction(ctx, h.engine, true, func(ctx context.Context, ws world.WorldState) error {
		op := git_world.NewGitInitOp(h.objectKey, nil, true, nil, nil)
		_, _, err := ws.ApplyWorldOp(ctx, op, "")
		return err
	})
	return errors.Wrap(err, "create the Space repository")
}

// adoptRoot sets the Repo root to next if the object is still at prevRev.
func (h *Helper) adoptRoot(ctx context.Context, prevRev uint64, next *bucket.ObjectRef) (bool, error) {
	var applied bool
	err := world.ExecTransaction(ctx, h.engine, true, func(ctx context.Context, ws world.WorldState) error {
		obj, err := world.MustGetObject(ctx, ws, h.objectKey)
		if err != nil {
			return err
		}
		defer world.ReleaseObjectState(obj)
		_, rev, err := obj.GetRootRef(ctx)
		if err != nil || rev != prevRev {
			return err
		}
		if _, err := obj.SetRootRef(ctx, next); err != nil {
			return err
		}
		applied = true
		return nil
	})
	return applied, err
}
