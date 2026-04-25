//go:build test_git_clone

package git_test

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v6/plumbing/protocol"
	"github.com/go-git/go-git/v6/plumbing/transport"
	_ "github.com/go-git/go-git/v6/plumbing/transport/file"
	"github.com/go-git/go-git/v6/storage"
	git_block "github.com/s4wave/spacewave/db/git/block"
	git_world "github.com/s4wave/spacewave/db/git/world"
	"github.com/s4wave/spacewave/db/world"

	alpha_testbed "github.com/s4wave/spacewave/testbed"
)

// localTransport intercepts remote git URLs and redirects them to local
// filesystem paths based on a .gitmodules URL-to-path mapping. Falls back
// to the original transport for URLs not in the map (e.g. sub-submodules).
type localTransport struct {
	// urlToPath maps normalized URL keys to absolute local paths.
	// Key format: "github.com/s4wave/spacewave/db"
	urlToPath map[string]string
	// fallback is the original transport for URLs not in the map.
	fallback transport.Transport
}

// _ is a type assertion
var _ transport.Transport = (*localTransport)(nil)

// newLocalTransport parses .gitmodules at repoRoot and builds a transport
// that resolves all submodule URLs to their local checkout paths.
func newLocalTransport(repoRoot string) *localTransport {
	lt := &localTransport{urlToPath: make(map[string]string)}

	path := filepath.Join(repoRoot, ".gitmodules")
	f, err := os.Open(path)
	if err != nil {
		return lt
	}
	defer f.Close()

	var curPath, curURL string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "path = ") {
			curPath = strings.TrimPrefix(line, "path = ")
		}
		if strings.HasPrefix(line, "url = ") {
			curURL = strings.TrimPrefix(line, "url = ")
		}
		if curPath != "" && curURL != "" {
			key := normalizeURL(curURL)
			if key != "" {
				lt.urlToPath[key] = filepath.Join(repoRoot, curPath)
			}
			curPath, curURL = "", ""
		}
	}
	return lt
}

// normalizeURL converts a git URL to a lookup key.
// "git@github.com:aperturerobotics/hydra" -> "github.com/s4wave/spacewave/db"
// "https://github.com/skiffos/skiffos" -> "github.com/skiffos/skiffos"
func normalizeURL(url string) string {
	// SCP-like: git@github.com:org/repo or git@github.com:org/repo.git
	if idx := strings.Index(url, "@"); idx >= 0 {
		rest := url[idx+1:]
		rest = strings.Replace(rest, ":", "/", 1)
		rest = strings.TrimSuffix(rest, ".git")
		return rest
	}
	// HTTPS: https://github.com/org/repo
	for _, scheme := range []string{"https://", "http://"} {
		if strings.HasPrefix(url, scheme) {
			rest := strings.TrimPrefix(url, scheme)
			rest = strings.TrimSuffix(rest, ".git")
			return rest
		}
	}
	return ""
}

func (lt *localTransport) resolve(ep *transport.Endpoint) (string, bool) {
	// Build key from endpoint: host + path
	path := strings.TrimPrefix(ep.Path, "/")
	path = strings.TrimSuffix(path, ".git")
	key := ep.Host + "/" + path
	localPath, ok := lt.urlToPath[key]
	return localPath, ok
}

// NewSession resolves the endpoint to a local path if found in the URL map,
// otherwise delegates to the fallback transport.
func (lt *localTransport) NewSession(st storage.Storer, ep *transport.Endpoint, auth transport.AuthMethod) (transport.Session, error) {
	localPath, ok := lt.resolve(ep)
	if !ok {
		if lt.fallback != nil {
			return lt.fallback.NewSession(st, ep, auth)
		}
		return nil, transport.ErrRepositoryNotFound
	}
	localEP, err := transport.NewEndpoint(localPath)
	if err != nil {
		return nil, err
	}
	fileTr, err := transport.Get("file")
	if err != nil {
		return nil, err
	}
	return fileTr.NewSession(st, localEP, nil)
}

// SupportedProtocols returns the protocols supported by this transport.
func (lt *localTransport) SupportedProtocols() []protocol.Version {
	return []protocol.Version{protocol.V0, protocol.V1}
}

// installLocalTransport installs a local transport override for SSH and HTTPS
// protocols, returning a cleanup function that restores the original transports.
func installLocalTransport(repoRoot string) func() {
	lt := newLocalTransport(repoRoot)
	origSSH, _ := transport.Get("ssh")
	origHTTPS, _ := transport.Get("https")
	origHTTP, _ := transport.Get("http")
	sshLT := &localTransport{urlToPath: lt.urlToPath, fallback: origSSH}
	httpsLT := &localTransport{urlToPath: lt.urlToPath, fallback: origHTTPS}
	httpLT := &localTransport{urlToPath: lt.urlToPath, fallback: origHTTP}
	transport.Register("ssh", sshLT)
	transport.Register("https", httpsLT)
	transport.Register("http", httpLT)
	return func() {
		transport.Register("ssh", origSSH)
		transport.Register("https", origHTTPS)
		transport.Register("http", origHTTP)
	}
}

// TestGitCloneRecursive clones the company repo with recursive submodules
// through a full hydra world engine stack. Used for profiling.
func TestGitCloneRecursive(t *testing.T) {
	ctx := context.Background()

	// resolve company repo root and install local transport
	repoPath := "../../../../.."
	absRepo, err := filepath.Abs(repoPath)
	if err != nil {
		t.Fatal(err)
	}
	cleanup := installLocalTransport(absRepo)
	defer cleanup()

	tb, err := alpha_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	// register git ops on the bus
	opc := world.NewLookupOpController("test-git-ops", tb.EngineID, git_world.LookupGitOp)
	go func() {
		_ = tb.Bus.ExecuteController(ctx, opc)
	}()
	select {
	case <-ctx.Done():
		t.Fatal("context cancelled while waiting for controller startup")
	case <-time.After(100 * time.Millisecond):
	}

	ws := tb.WorldState
	sender := tb.Volume.GetPeerID()

	objKey := "gitrepo/company"
	cloneOp := &git_world.GitCloneOp{
		ObjectKey: objKey,
		CloneOpts: &git_block.CloneOpts{
			Url:             repoPath,
			Recursive:       true,
			RecursionDepth:  1,
			DisableCheckout: false,
		},
	}

	t.Logf("cloning %s as %s (recursive, local transport)...", repoPath, objKey)
	start := time.Now()
	seqno, _, err := ws.ApplyWorldOp(ctx, cloneOp, sender)
	if err != nil {
		t.Fatalf("clone failed after %s: %v", time.Since(start), err)
	}
	t.Logf("clone complete in %s, seqno=%d", time.Since(start), seqno)

	_, exists, err := ws.GetObject(ctx, objKey)
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("expected object to exist after clone")
	}
}

// TestGitCloneNonRecursive clones the company repo without submodules
// as a baseline comparison.
func TestGitCloneNonRecursive(t *testing.T) {
	ctx := context.Background()
	tb, err := alpha_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	opc := world.NewLookupOpController("test-git-ops", tb.EngineID, git_world.LookupGitOp)
	go func() {
		_ = tb.Bus.ExecuteController(ctx, opc)
	}()
	select {
	case <-ctx.Done():
		t.Fatal("context cancelled while waiting for controller startup")
	case <-time.After(100 * time.Millisecond):
	}

	ws := tb.WorldState
	sender := tb.Volume.GetPeerID()

	repoPath := "../../../../.."
	objKey := "gitrepo/company"
	cloneOp := &git_world.GitCloneOp{
		ObjectKey: objKey,
		CloneOpts: &git_block.CloneOpts{
			Url:             repoPath,
			DisableCheckout: true,
		},
	}

	t.Logf("cloning %s as %s (non-recursive)...", repoPath, objKey)
	start := time.Now()
	seqno, _, err := ws.ApplyWorldOp(ctx, cloneOp, sender)
	if err != nil {
		t.Fatalf("clone failed after %s: %v", time.Since(start), err)
	}
	t.Logf("clone complete in %s, seqno=%d", time.Since(start), seqno)

	_, exists, err := ws.GetObject(ctx, objKey)
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("expected object to exist after clone")
	}
}
