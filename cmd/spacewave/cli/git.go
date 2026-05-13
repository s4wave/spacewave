//go:build !js

package spacewave_cli

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	s4wave_git_core "github.com/s4wave/spacewave/core/git"
	git_block "github.com/s4wave/spacewave/db/git/block"
	git_world "github.com/s4wave/spacewave/db/git/world"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	s4wave_git "github.com/s4wave/spacewave/sdk/git"
	s4wave_unixfs "github.com/s4wave/spacewave/sdk/unixfs"
	sdk_engine "github.com/s4wave/spacewave/sdk/world/engine"
)

// gitContext holds the mounted resources for git operations.
type gitContext struct {
	gitSvc    s4wave_git.SRPCGitRepoResourceServiceClient
	engine    *sdk_engine.SDKEngine
	objectKey string
	client    *sdkClient
}

// parseGitURI parses the git URI, allowing it to be empty for positional key resolution.
func parseGitURI(arg, spaceFlag string, sessFlag int) (fsURI, error) {
	if arg == "" {
		result := fsURI{sessionIdx: 1, spaceID: spaceFlag}
		if sessFlag > 0 {
			result.sessionIdx = uint32(sessFlag)
		}
		return result, nil
	}
	return parseFsURI(arg, spaceFlag, sessFlag)
}

// mountGitContext connects to the daemon and mounts the full chain to get
// a GitRepoResourceService client for the given URI.
func mountGitContext(c *cli.Context, statePath string, uri fsURI) (*gitContext, func(), error) {
	ctx := c.Context

	client, err := connectDaemonFromContext(ctx, c, statePath)
	if err != nil {
		return nil, nil, err
	}

	sess, err := client.mountSession(ctx, uri.sessionIdx)
	if err != nil {
		client.close()
		return nil, nil, err
	}

	spaceID := uri.spaceID
	if spaceID == "" {
		spaceID, err = client.getSpaceByName(ctx, sess, "")
		if err != nil {
			sess.Release()
			client.close()
			return nil, nil, errors.Wrap(err, "resolve default space")
		}
	}

	spaceSvc, spaceCleanup, err := client.mountSpace(ctx, sess, spaceID)
	if err != nil {
		sess.Release()
		client.close()
		return nil, nil, err
	}

	engine, engineRef, engineCleanup, err := client.accessWorldEngineWithRef(ctx, spaceSvc)
	if err != nil {
		spaceCleanup()
		sess.Release()
		client.close()
		return nil, nil, err
	}

	typedClient, _, _, typedCleanup, err := client.accessTypedObject(ctx, engineRef, uri.objectKey)
	if err != nil {
		engineCleanup()
		spaceCleanup()
		sess.Release()
		client.close()
		return nil, nil, errors.Wrap(err, "access typed object for "+uri.objectKey)
	}

	gitSvc := s4wave_git.NewSRPCGitRepoResourceServiceClient(typedClient)

	cleanup := func() {
		typedCleanup()
		engineCleanup()
		spaceCleanup()
		sess.Release()
		client.close()
	}

	return &gitContext{
		gitSvc:    gitSvc,
		engine:    engine,
		objectKey: uri.objectKey,
		client:    client,
	}, cleanup, nil
}

// mountGitEngine connects to the daemon and mounts the engine without
// accessing a typed object. Used by standalone commands like clone.
func mountGitEngine(c *cli.Context, statePath, spaceID string, sessIdx int) (*sdk_engine.SDKEngine, func(), error) {
	ctx := c.Context

	client, err := connectDaemonFromContext(ctx, c, statePath)
	if err != nil {
		return nil, nil, err
	}

	idx := uint32(1)
	if sessIdx > 0 {
		idx = uint32(sessIdx)
	}

	sess, err := client.mountSession(ctx, idx)
	if err != nil {
		client.close()
		return nil, nil, err
	}

	if spaceID == "" {
		spaceID, err = client.getSpaceByName(ctx, sess, "")
		if err != nil {
			sess.Release()
			client.close()
			return nil, nil, errors.Wrap(err, "resolve default space")
		}
	}

	spaceSvc, spaceCleanup, err := client.mountSpace(ctx, sess, spaceID)
	if err != nil {
		sess.Release()
		client.close()
		return nil, nil, err
	}

	engine, engineCleanup, err := client.accessWorldEngine(ctx, spaceSvc)
	if err != nil {
		spaceCleanup()
		sess.Release()
		client.close()
		return nil, nil, err
	}

	cleanup := func() {
		engineCleanup()
		spaceCleanup()
		sess.Release()
		client.close()
	}
	return engine, cleanup, nil
}

// commonGitFlags returns the flags shared by git subcommands that need a URI.
func commonGitFlags(gitURI *string, statePath *string, spaceID *string, sessIdx *int, outputFormat *string) []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:        "uri",
			Aliases:     []string{"git", "repo"},
			Usage:       "git repo URI",
			EnvVars:     []string{"SPACEWAVE_URI", "SPACEWAVE_GIT"},
			Destination: gitURI,
		},
		statePathFlag(statePath),
		&cli.StringFlag{
			Name:        "space",
			Usage:       "space ID (overrides URI)",
			EnvVars:     []string{"SPACEWAVE_SPACE"},
			Destination: spaceID,
		},
		&cli.IntFlag{
			Name:        "session-index",
			Usage:       "session index (overrides URI)",
			EnvVars:     []string{"SPACEWAVE_SESSION_INDEX"},
			Destination: sessIdx,
		},
		&cli.StringFlag{
			Name:        "output",
			Aliases:     []string{"o"},
			Usage:       "output format (text/json/yaml)",
			EnvVars:     []string{"SPACEWAVE_OUTPUT"},
			Value:       "text",
			Destination: outputFormat,
		},
	}
}

// resolveGitURI resolves the URI from either --uri flag or positional arg + --space.
func resolveGitURI(c *cli.Context, gitURI, spaceID string, sessIdx int) (fsURI, error) {
	if gitURI != "" {
		return parseGitURI(gitURI, spaceID, sessIdx)
	}
	arg := c.Args().First()
	if arg != "" {
		return parseGitURI(arg, spaceID, sessIdx)
	}
	return fsURI{}, errors.New("git repo URI or object key required (use --uri or positional arg)")
}

// parseTagMode parses a tag mode string to the proto enum.
func parseTagMode(s string) (git_block.TagMode, error) {
	switch strings.ToLower(s) {
	case "", "default":
		return git_block.TagMode_TagMode_DEFAULT, nil
	case "none":
		return git_block.TagMode_TagMode_NONE, nil
	case "all":
		return git_block.TagMode_TagMode_ALL, nil
	case "following":
		return git_block.TagMode_TagMode_FOLLOWING, nil
	default:
		return 0, errors.Errorf("unknown tag mode: %s (valid: default, none, all, following)", s)
	}
}

// deriveKeyFromURL derives an object key from a git URL.
func deriveKeyFromURL(url string) string {
	url = strings.TrimSuffix(url, ".git")
	url = strings.TrimSuffix(url, "/")
	idx := strings.LastIndex(url, "/")
	if idx >= 0 {
		return url[idx+1:]
	}
	return url
}

// shortHash returns the first 7 characters of a hash string.
func shortHash(h string) string {
	if len(h) > 7 {
		return h[:7]
	}
	return h
}

// formatTimestamp formats a Unix timestamp as a date string.
func formatTimestamp(ts int64) string {
	if ts == 0 {
		return ""
	}
	return time.Unix(ts, 0).Format("2006-01-02 15:04:05")
}

// formatSize formats a byte size as a human-readable string.
func formatSize(size uint64) string {
	if size < 1024 {
		return strconv.FormatUint(size, 10)
	}
	if size < 1024*1024 {
		return strconv.FormatFloat(float64(size)/1024, 'f', 1, 64) + "K"
	}
	return strconv.FormatFloat(float64(size)/(1024*1024), 'f', 1, 64) + "M"
}

// newGitCommand builds the top-level git command group.
func newGitCommand(_ func() cli_entrypoint.CliBus) *cli.Command {
	return &cli.Command{
		Name:  "git",
		Usage: "git repository operations",
		Subcommands: []*cli.Command{
			buildGitShowCommand(),
			buildGitRefsCommand(),
			buildGitLogCommand(),
			buildGitDiffCommand(),
			buildGitCommitCommand(),
			buildGitTreeCommand(),
			buildGitCloneCommand(),
			buildGitFetchCommand(),
			buildGitWorktreeCommand(),
		},
	}
}

// buildGitShowCommand builds the git show subcommand.
func buildGitShowCommand() *cli.Command {
	var gitURI, statePath, spaceID, outputFormat string
	var sessIdx int
	return &cli.Command{
		Name:  "show",
		Usage: "show repository overview",
		Flags: commonGitFlags(&gitURI, &statePath, &spaceID, &sessIdx, &outputFormat),
		Action: func(c *cli.Context) error {
			uri, err := resolveGitURI(c, gitURI, spaceID, sessIdx)
			if err != nil {
				return err
			}

			gc, cleanup, err := mountGitContext(c, statePath, uri)
			if err != nil {
				return err
			}
			defer cleanup()

			ctx := c.Context
			resp, err := gc.gitSvc.GetRepoInfo(ctx, &s4wave_git.GetRepoInfoRequest{})
			if err != nil {
				return errors.Wrap(err, "get repo info")
			}

			if outputFormat == "json" || outputFormat == "yaml" {
				data, err := resp.MarshalJSON()
				if err != nil {
					return errors.Wrap(err, "marshal repo info")
				}
				return formatOutput(data, outputFormat)
			}

			w := os.Stdout
			fields := [][2]string{
				{"Repo", gc.objectKey},
				{"HEAD", resp.GetHeadRef() + " (" + shortHash(resp.GetHeadCommitHash()) + ")"},
				{"Empty", strconv.FormatBool(resp.GetIsEmpty())},
			}
			if resp.GetReadmePath() != "" {
				fields = append(fields, [2]string{"README", resp.GetReadmePath()})
			}
			writeFields(w, fields)

			lc := resp.GetLastCommit()
			if lc != nil {
				w.WriteString("\nLast Commit\n")
				writeFields(w, [][2]string{
					{"  Hash", lc.GetHash()},
					{"  Author", lc.GetAuthorName() + " <" + lc.GetAuthorEmail() + ">"},
					{"  Date", formatTimestamp(lc.GetAuthorTimestamp())},
					{"  Message", strings.TrimSpace(lc.GetMessage())},
				})
			}

			return nil
		},
	}
}

// buildGitRefsCommand builds the git refs subcommand.
func buildGitRefsCommand() *cli.Command {
	var gitURI, statePath, spaceID, outputFormat string
	var sessIdx int
	return &cli.Command{
		Name:  "refs",
		Usage: "list branches and tags",
		Flags: commonGitFlags(&gitURI, &statePath, &spaceID, &sessIdx, &outputFormat),
		Action: func(c *cli.Context) error {
			uri, err := resolveGitURI(c, gitURI, spaceID, sessIdx)
			if err != nil {
				return err
			}

			gc, cleanup, err := mountGitContext(c, statePath, uri)
			if err != nil {
				return err
			}
			defer cleanup()

			ctx := c.Context
			resp, err := gc.gitSvc.ListRefs(ctx, &s4wave_git.ListRefsRequest{})
			if err != nil {
				return errors.Wrap(err, "list refs")
			}

			if outputFormat == "json" || outputFormat == "yaml" {
				data, err := resp.MarshalJSON()
				if err != nil {
					return errors.Wrap(err, "marshal refs")
				}
				return formatOutput(data, outputFormat)
			}

			w := os.Stdout
			writeFields(w, [][2]string{{"HEAD", resp.GetHeadRef()}})

			branches := resp.GetBranches()
			if len(branches) > 0 {
				w.WriteString("\nBranches (" + strconv.Itoa(len(branches)) + ")\n")
				rows := [][]string{{"NAME", "COMMIT", "HEAD"}}
				for _, b := range branches {
					head := ""
					if b.GetIsHead() {
						head = "*"
					}
					rows = append(rows, []string{b.GetName(), shortHash(b.GetCommitHash()), head})
				}
				writeTable(w, "  ", rows)
			}

			tags := resp.GetTags()
			if len(tags) > 0 {
				w.WriteString("\nTags (" + strconv.Itoa(len(tags)) + ")\n")
				rows := [][]string{{"NAME", "COMMIT"}}
				for _, t := range tags {
					rows = append(rows, []string{t.GetName(), shortHash(t.GetCommitHash())})
				}
				writeTable(w, "  ", rows)
			}

			return nil
		},
	}
}

// buildGitLogCommand builds the git log subcommand.
func buildGitLogCommand() *cli.Command {
	var gitURI, statePath, spaceID, outputFormat string
	var sessIdx int
	return &cli.Command{
		Name:  "log",
		Usage: "show commit history",
		Flags: append(commonGitFlags(&gitURI, &statePath, &spaceID, &sessIdx, &outputFormat),
			&cli.StringFlag{
				Name:  "ref",
				Usage: "ref to start from (default HEAD)",
			},
			&cli.StringFlag{
				Name:  "since",
				Usage: "filter to commits not reachable from this ref",
			},
			&cli.UintFlag{
				Name:  "limit",
				Usage: "max commits to return",
				Value: 50,
			},
			&cli.UintFlag{
				Name:  "offset",
				Usage: "number of commits to skip",
			},
		),
		Action: func(c *cli.Context) error {
			uri, err := resolveGitURI(c, gitURI, spaceID, sessIdx)
			if err != nil {
				return err
			}

			gc, cleanup, err := mountGitContext(c, statePath, uri)
			if err != nil {
				return err
			}
			defer cleanup()

			ctx := c.Context
			req := &s4wave_git.LogRequest{
				RefName:  c.String("ref"),
				Limit:    uint32(c.Uint("limit")),
				Offset:   uint32(c.Uint("offset")),
				SinceRef: c.String("since"),
			}

			resp, err := gc.gitSvc.Log(ctx, req)
			if err != nil {
				return errors.Wrap(err, "log")
			}

			if outputFormat == "json" || outputFormat == "yaml" {
				data, err := resp.MarshalJSON()
				if err != nil {
					return errors.Wrap(err, "marshal log")
				}
				return formatOutput(data, outputFormat)
			}

			w := os.Stdout
			commits := resp.GetCommits()
			rows := [][]string{{"HASH", "AUTHOR", "DATE", "MESSAGE"}}
			for _, cm := range commits {
				msg := strings.TrimSpace(cm.GetMessage())
				if idx := strings.IndexByte(msg, '\n'); idx >= 0 {
					msg = msg[:idx]
				}
				rows = append(rows, []string{
					shortHash(cm.GetHash()),
					cm.GetAuthorName(),
					formatTimestamp(cm.GetAuthorTimestamp()),
					msg,
				})
			}
			writeTable(w, "", rows)

			if resp.GetHasMore() {
				nextOffset := req.GetOffset() + uint32(len(commits))
				w.WriteString("\nUse --offset " + strconv.FormatUint(uint64(nextOffset), 10) + " to see more\n")
			}

			return nil
		},
	}
}

// buildGitDiffCommand builds the git diff subcommand.
func buildGitDiffCommand() *cli.Command {
	var gitURI, statePath, spaceID, outputFormat string
	var sessIdx int
	return &cli.Command{
		Name:      "diff",
		Usage:     "show diff stats between two refs",
		ArgsUsage: "<refA> [refB]",
		Flags:     commonGitFlags(&gitURI, &statePath, &spaceID, &sessIdx, &outputFormat),
		Action: func(c *cli.Context) error {
			refA := c.Args().Get(0)
			if refA == "" {
				return errors.New("refA required (branch, tag, or commit hash)")
			}
			refB := c.Args().Get(1)

			uri, err := resolveGitURI(c, gitURI, spaceID, sessIdx)
			if err != nil {
				return err
			}

			gc, cleanup, err := mountGitContext(c, statePath, uri)
			if err != nil {
				return err
			}
			defer cleanup()

			ctx := c.Context
			resp, err := gc.gitSvc.GetDiffStat(ctx, &s4wave_git.GetDiffStatRequest{
				RefA: refA,
				RefB: refB,
			})
			if err != nil {
				return errors.Wrap(err, "get diff stat")
			}

			if outputFormat == "json" || outputFormat == "yaml" {
				data, err := resp.MarshalJSON()
				if err != nil {
					return errors.Wrap(err, "marshal diff stat")
				}
				return formatOutput(data, outputFormat)
			}

			w := os.Stdout
			label := refA
			if refB != "" {
				label = refA + ".." + refB
			}
			writeFields(w, [][2]string{
				{"Diff", label},
				{"Files Changed", strconv.Itoa(len(resp.GetFiles()))},
				{"Additions", strconv.FormatUint(uint64(resp.GetTotalAdditions()), 10)},
				{"Deletions", strconv.FormatUint(uint64(resp.GetTotalDeletions()), 10)},
			})

			if len(resp.GetFiles()) > 0 {
				w.WriteString("\n")
				rows := [][]string{{"PATH", "+", "-"}}
				for _, f := range resp.GetFiles() {
					rows = append(rows, []string{
						f.GetPath(),
						strconv.FormatUint(uint64(f.GetAdditions()), 10),
						strconv.FormatUint(uint64(f.GetDeletions()), 10),
					})
				}
				writeTable(w, "", rows)
			}

			return nil
		},
	}
}

// buildGitCommitCommand builds the git commit subcommand.
func buildGitCommitCommand() *cli.Command {
	var gitURI, statePath, spaceID, outputFormat string
	var sessIdx int
	return &cli.Command{
		Name:      "commit",
		Usage:     "show commit details with diff stats",
		ArgsUsage: "<ref>",
		Flags:     commonGitFlags(&gitURI, &statePath, &spaceID, &sessIdx, &outputFormat),
		Action: func(c *cli.Context) error {
			ref := c.Args().First()
			if ref == "" {
				return errors.New("commit ref or hash required")
			}

			uri, err := resolveGitURI(c, gitURI, spaceID, sessIdx)
			if err != nil {
				return err
			}

			gc, cleanup, err := mountGitContext(c, statePath, uri)
			if err != nil {
				return err
			}
			defer cleanup()

			ctx := c.Context

			// Parallel calls: GetCommit + GetDiffStat
			// Both RPCs resolve refs/short hashes/rev-parse internally.
			type commitResult struct {
				resp *s4wave_git.GetCommitResponse
				err  error
			}
			type diffResult struct {
				resp *s4wave_git.GetDiffStatResponse
				err  error
			}

			commitCh := make(chan commitResult, 1)
			diffCh := make(chan diffResult, 1)

			go func() {
				resp, err := gc.gitSvc.GetCommit(ctx, &s4wave_git.GetCommitRequest{Hash: ref})
				commitCh <- commitResult{resp, err}
			}()
			go func() {
				resp, err := gc.gitSvc.GetDiffStat(ctx, &s4wave_git.GetDiffStatRequest{RefA: ref})
				diffCh <- diffResult{resp, err}
			}()

			cr := <-commitCh
			if cr.err != nil {
				return errors.Wrap(cr.err, "get commit")
			}
			dr := <-diffCh
			if dr.err != nil {
				return errors.Wrap(dr.err, "get diff stat")
			}

			if outputFormat == "json" || outputFormat == "yaml" {
				commitJSON, err := cr.resp.MarshalJSON()
				if err != nil {
					return errors.Wrap(err, "marshal commit")
				}
				diffJSON, err := dr.resp.MarshalJSON()
				if err != nil {
					return errors.Wrap(err, "marshal diff stat")
				}
				combined := []byte(`{"commit":`)
				combined = append(combined, commitJSON...)
				combined = append(combined, `,"diffStat":`...)
				combined = append(combined, diffJSON...)
				combined = append(combined, '}')
				return formatOutput(combined, outputFormat)
			}

			cm := cr.resp.GetCommit()
			w := os.Stdout
			fields := [][2]string{
				{"Commit", cm.GetHash()},
				{"Author", cm.GetAuthorName() + " <" + cm.GetAuthorEmail() + ">"},
				{"Date", formatTimestamp(cm.GetAuthorTimestamp())},
			}
			if parents := cm.GetParentHashes(); len(parents) > 0 {
				fields = append(fields, [2]string{"Parents", strings.Join(parents, ", ")})
			}
			writeFields(w, fields)
			w.WriteString("\n    " + strings.TrimSpace(cm.GetMessage()) + "\n")

			diffResp := dr.resp
			w.WriteString("\n")
			writeFields(w, [][2]string{
				{"Files Changed", strconv.Itoa(len(diffResp.GetFiles()))},
				{"Additions", strconv.FormatUint(uint64(diffResp.GetTotalAdditions()), 10)},
				{"Deletions", strconv.FormatUint(uint64(diffResp.GetTotalDeletions()), 10)},
			})

			if len(diffResp.GetFiles()) > 0 {
				w.WriteString("\n")
				rows := [][]string{{"PATH", "+", "-"}}
				for _, fi := range diffResp.GetFiles() {
					rows = append(rows, []string{
						fi.GetPath(),
						strconv.FormatUint(uint64(fi.GetAdditions()), 10),
						strconv.FormatUint(uint64(fi.GetDeletions()), 10),
					})
				}
				writeTable(w, "", rows)
			}

			return nil
		},
	}
}

// buildGitTreeCommand builds the git tree subcommand.
func buildGitTreeCommand() *cli.Command {
	var gitURI, statePath, spaceID, outputFormat string
	var sessIdx int
	return &cli.Command{
		Name:      "tree",
		Usage:     "browse files in a ref's tree (read-only)",
		ArgsUsage: "[path]",
		Flags: append(commonGitFlags(&gitURI, &statePath, &spaceID, &sessIdx, &outputFormat),
			&cli.StringFlag{
				Name:  "ref",
				Usage: "ref to browse (default HEAD)",
			},
		),
		Action: func(c *cli.Context) error {
			uri, err := resolveGitURI(c, gitURI, spaceID, sessIdx)
			if err != nil {
				return err
			}

			gc, cleanup, err := mountGitContext(c, statePath, uri)
			if err != nil {
				return err
			}
			defer cleanup()

			ctx := c.Context
			refName := c.String("ref")
			fsPath := c.Args().First()

			// Get the tree resource (FSHandle sub-resource).
			treeResp, err := gc.gitSvc.GetTreeResource(ctx, &s4wave_git.GetTreeResourceRequest{
				RefName: refName,
			})
			if err != nil {
				return errors.Wrap(err, "get tree resource")
			}

			// Create a resource reference for the FSHandle.
			ref := gc.client.resClient.CreateResourceReference(treeResp.GetResourceId())
			defer ref.Release()

			childClient, err := ref.GetClient()
			if err != nil {
				return errors.Wrap(err, "tree handle client")
			}

			fsSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(childClient)

			// Navigate to the requested path if specified.
			var pathSvc s4wave_unixfs.SRPCFSHandleResourceServiceClient
			var pathCleanup func()
			if fsPath != "" {
				lookupResp, err := fsSvc.LookupPath(ctx, &s4wave_unixfs.HandleLookupPathRequest{Path: fsPath})
				if err != nil {
					return errors.Wrap(err, "lookup path "+fsPath)
				}
				pathRef := gc.client.resClient.CreateResourceReference(lookupResp.GetResourceId())
				pc, err := pathRef.GetClient()
				if err != nil {
					pathRef.Release()
					return errors.Wrap(err, "path handle client")
				}
				pathSvc = s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(pc)
				pathCleanup = func() { pathRef.Release() }
			} else {
				pathSvc = fsSvc
				pathCleanup = func() {}
			}
			defer pathCleanup()

			// Check if it's a directory or file.
			ntResp, err := pathSvc.GetNodeType(ctx, &s4wave_unixfs.HandleGetNodeTypeRequest{})
			if err != nil {
				return errors.Wrap(err, "get node type")
			}

			nt := ntResp.GetNodeType()
			if nt != nil && nt.GetIsDir() {
				// Directory listing.
				strm, err := pathSvc.Readdir(ctx, &s4wave_unixfs.HandleReaddirRequest{})
				if err != nil {
					return errors.Wrap(err, "readdir")
				}

				type entryInfo struct {
					name  string
					isDir bool
					size  uint64
				}
				var entries []entryInfo
				for {
					resp, err := strm.Recv()
					if err != nil {
						return errors.Wrap(err, "recv readdir")
					}
					if resp.GetDone() {
						break
					}
					entry := resp.GetEntry()
					if entry == nil {
						continue
					}
					entries = append(entries, entryInfo{
						name:  entry.GetName(),
						isDir: entry.GetIsDir(),
						size:  entry.GetSize(),
					})
				}

				if outputFormat == "json" || outputFormat == "yaml" {
					buf, ms := newMarshalBuf()
					ms.WriteArrayStart()
					var af bool
					for _, e := range entries {
						ms.WriteMoreIf(&af)
						ms.WriteObjectStart()
						var fl bool
						ms.WriteMoreIf(&fl)
						ms.WriteObjectField("name")
						ms.WriteString(e.name)
						ms.WriteMoreIf(&fl)
						ms.WriteObjectField("isDir")
						ms.WriteBool(e.isDir)
						ms.WriteMoreIf(&fl)
						ms.WriteObjectField("size")
						ms.WriteUint64(e.size)
						ms.WriteObjectEnd()
					}
					ms.WriteArrayEnd()
					return formatOutput(buf.Bytes(), outputFormat)
				}

				label := refName
				if label == "" {
					label = "HEAD"
				}
				displayPath := fsPath
				if displayPath == "" {
					displayPath = "/"
				}
				w := os.Stdout
				writeFields(w, [][2]string{{"Tree", label + ":" + displayPath}})
				w.WriteString("\n")
				rows := [][]string{{"TYPE", "NAME", "SIZE"}}
				for _, e := range entries {
					t := "file"
					name := e.name
					if e.isDir {
						t = "dir"
						name += "/"
					}
					rows = append(rows, []string{t, name, formatSize(e.size)})
				}
				writeTable(w, "", rows)
				return nil
			}

			// File: output raw contents.
			var offset int64
			for {
				readResp, err := pathSvc.ReadAt(ctx, &s4wave_unixfs.HandleReadAtRequest{
					Offset: offset,
					Length: readChunkSize,
				})
				if err != nil {
					return errors.Wrap(err, "read file")
				}
				data := readResp.GetData()
				if len(data) > 0 {
					os.Stdout.Write(data)
					offset += int64(len(data))
				}
				if readResp.GetEof() || len(data) == 0 {
					break
				}
			}
			return nil
		},
	}
}

// buildGitCloneCommand builds the git clone subcommand (standalone, no --uri needed).
func buildGitCloneCommand() *cli.Command {
	var statePath, spaceID string
	var sessIdx int
	return &cli.Command{
		Name:  "clone",
		Usage: "clone a remote repository into the world",
		Flags: []cli.Flag{
			statePathFlag(&statePath),
			&cli.StringFlag{
				Name:        "space",
				Usage:       "space ID (auto-detected if only one space)",
				EnvVars:     []string{"SPACEWAVE_SPACE"},
				Destination: &spaceID,
			},
			&cli.IntFlag{
				Name:        "session-index",
				Usage:       "session index",
				EnvVars:     []string{"SPACEWAVE_SESSION_INDEX"},
				Destination: &sessIdx,
			},
			&cli.StringFlag{
				Name:     "url",
				Usage:    "git URL to clone",
				Required: true,
			},
			&cli.StringFlag{
				Name:  "key",
				Usage: "object key (auto-derived from URL if omitted)",
			},
			&cli.StringFlag{
				Name:  "ref",
				Usage: "reference to clone",
			},
			&cli.StringFlag{
				Name:  "remote",
				Usage: "remote name (default origin)",
			},
			&cli.BoolFlag{
				Name:  "single-branch",
				Usage: "fetch only specified ref",
			},
			&cli.UintFlag{
				Name:  "depth",
				Usage: "shallow clone depth",
			},
			&cli.BoolFlag{
				Name:  "recursive",
				Usage: "fetch submodules",
			},
			&cli.StringFlag{
				Name:  "tag-mode",
				Usage: "tag fetching mode (default/none/all/following)",
			},
			&cli.BoolFlag{
				Name:  "insecure",
				Usage: "skip TLS verification",
			},
			&cli.BoolFlag{
				Name:  "no-checkout",
				Usage: "skip worktree creation",
			},
		},
		Action: func(c *cli.Context) error {
			url := c.String("url")
			key := c.String("key")
			if key == "" {
				key = deriveKeyFromURL(url)
			}

			tagMode, err := parseTagMode(c.String("tag-mode"))
			if err != nil {
				return err
			}

			engine, cleanup, err := mountGitEngine(c, statePath, spaceID, sessIdx)
			if err != nil {
				return err
			}
			defer cleanup()

			cloneOpts := &git_block.CloneOpts{
				Url:          url,
				RemoteName:   c.String("remote"),
				Ref:          c.String("ref"),
				SingleBranch: c.Bool("single-branch"),
				Depth:        uint32(c.Uint("depth")),
				Recursive:    c.Bool("recursive"),
				TagMode:      tagMode,
				Insecure:     c.Bool("insecure"),
			}

			cloneOpts.DisableCheckout = c.Bool("no-checkout")
			repoRef, err := s4wave_git_core.CloneGitRepoToRef(c.Context, engine, cloneOpts, nil, nil)
			if err != nil {
				return err
			}

			tx, err := engine.NewTransaction(c.Context, true)
			if err != nil {
				return errors.Wrap(err, "new transaction")
			}
			defer tx.Discard()
			op := git_world.NewGitInitOp(key, repoRef, c.Bool("no-checkout"), nil, nil)
			_, _, err = tx.ApplyWorldOp(c.Context, op, "")
			if err != nil {
				return errors.Wrap(err, "publish git repo")
			}
			if err := tx.Commit(c.Context); err != nil {
				return errors.Wrap(err, "commit transaction")
			}

			os.Stdout.WriteString(key + "\n")
			return nil
		},
	}
}

// buildGitFetchCommand builds the git fetch subcommand.
func buildGitFetchCommand() *cli.Command {
	var gitURI, statePath, spaceID, outputFormat string
	var sessIdx int
	return &cli.Command{
		Name:  "fetch",
		Usage: "fetch updates from remote",
		Flags: append(commonGitFlags(&gitURI, &statePath, &spaceID, &sessIdx, &outputFormat),
			&cli.StringFlag{
				Name:  "remote",
				Usage: "remote name (default origin)",
			},
			&cli.StringFlag{
				Name:  "remote-url",
				Usage: "override remote URL",
			},
			&cli.StringSliceFlag{
				Name:  "ref-spec",
				Usage: "refspec (repeatable)",
			},
			&cli.UintFlag{
				Name:  "depth",
				Usage: "shallow fetch depth",
			},
			&cli.StringFlag{
				Name:  "tag-mode",
				Usage: "tag fetching mode (default/none/all/following)",
			},
			&cli.BoolFlag{
				Name:  "force",
				Usage: "allow forced updates",
			},
			&cli.BoolFlag{
				Name:  "insecure",
				Usage: "skip TLS verification",
			},
			&cli.BoolFlag{
				Name:  "prune",
				Usage: "remove local refs not on remote",
			},
		),
		Action: func(c *cli.Context) error {
			uri, err := resolveGitURI(c, gitURI, spaceID, sessIdx)
			if err != nil {
				return err
			}

			gc, cleanup, err := mountGitContext(c, statePath, uri)
			if err != nil {
				return err
			}
			defer cleanup()

			tagMode, err := parseTagMode(c.String("tag-mode"))
			if err != nil {
				return err
			}

			fetchOpts := &git_block.FetchOpts{
				RemoteName: c.String("remote"),
				RemoteUrl:  c.String("remote-url"),
				RefSpecs:   c.StringSlice("ref-spec"),
				Depth:      uint32(c.Uint("depth")),
				TagMode:    tagMode,
				Force:      c.Bool("force"),
				Insecure:   c.Bool("insecure"),
				Prune:      c.Bool("prune"),
			}

			op := git_world.NewGitFetchOp(gc.objectKey, fetchOpts)
			if err := applyWorldOp(c, gc.engine, op); err != nil {
				return err
			}

			os.Stdout.WriteString("fetched\n")
			return nil
		},
	}
}

// buildGitWorktreeCommand builds the git worktree command group.
func buildGitWorktreeCommand() *cli.Command {
	return &cli.Command{
		Name:  "worktree",
		Usage: "worktree operations",
		Subcommands: []*cli.Command{
			buildGitWorktreeCreateCommand(),
			buildGitWorktreeCheckoutCommand(),
		},
	}
}

// buildGitWorktreeCreateCommand builds the git worktree create subcommand.
func buildGitWorktreeCreateCommand() *cli.Command {
	var gitURI, statePath, spaceID, outputFormat string
	var sessIdx int
	return &cli.Command{
		Name:  "create",
		Usage: "create a worktree for a repo",
		Flags: append(commonGitFlags(&gitURI, &statePath, &spaceID, &sessIdx, &outputFormat),
			&cli.StringFlag{
				Name:  "key",
				Usage: "worktree object key (default: <repo-key>/worktree)",
			},
			&cli.BoolFlag{
				Name:  "create-workdir",
				Usage: "create workdir if missing",
			},
			&cli.StringFlag{
				Name:  "branch",
				Usage: "branch to checkout",
			},
			&cli.StringFlag{
				Name:  "commit",
				Usage: "commit hash to checkout",
			},
			&cli.BoolFlag{
				Name:  "no-checkout",
				Usage: "skip checkout step",
			},
		),
		Action: func(c *cli.Context) error {
			uri, err := resolveGitURI(c, gitURI, spaceID, sessIdx)
			if err != nil {
				return err
			}

			gc, cleanup, err := mountGitContext(c, statePath, uri)
			if err != nil {
				return err
			}
			defer cleanup()

			repoKey := gc.objectKey
			wtKey := c.String("key")
			if wtKey == "" {
				wtKey = repoKey + "/worktree"
			}

			disableCheckout := c.Bool("no-checkout")
			var checkoutOpts *git_block.CheckoutOpts
			if !disableCheckout {
				checkoutOpts = &git_block.CheckoutOpts{
					Branch: c.String("branch"),
				}
			}

			// WorkdirRef: use the worktree key as the workdir object key with FS_NODE type.
			workdirRef := &unixfs_world.UnixfsRef{
				ObjectKey: wtKey + "/workdir",
				FsType:    unixfs_world.FSType_FSType_FS_NODE,
			}

			op := git_world.NewGitCreateWorktreeOp(
				wtKey,
				repoKey,
				workdirRef,
				true, // create workdir
				checkoutOpts,
				disableCheckout,
				time.Now(),
			)

			if err := applyWorldOp(c, gc.engine, op); err != nil {
				return err
			}

			os.Stdout.WriteString(wtKey + "\n")
			return nil
		},
	}
}

// buildGitWorktreeCheckoutCommand builds the git worktree checkout subcommand.
func buildGitWorktreeCheckoutCommand() *cli.Command {
	var gitURI, statePath, spaceID, outputFormat string
	var sessIdx int
	return &cli.Command{
		Name:  "checkout",
		Usage: "checkout a revision in an existing worktree",
		Flags: append(commonGitFlags(&gitURI, &statePath, &spaceID, &sessIdx, &outputFormat),
			&cli.StringFlag{
				Name:     "worktree-key",
				Usage:    "worktree object key",
				Required: true,
			},
			&cli.StringFlag{
				Name:  "branch",
				Usage: "branch to checkout",
			},
			&cli.StringFlag{
				Name:  "commit",
				Usage: "commit hash to checkout",
			},
			&cli.BoolFlag{
				Name:  "create",
				Usage: "create branch from commit",
			},
			&cli.BoolFlag{
				Name:  "force",
				Usage: "force checkout even if dirty",
			},
			&cli.BoolFlag{
				Name:  "keep",
				Usage: "keep index/workdir changes",
			},
		),
		Action: func(c *cli.Context) error {
			uri, err := resolveGitURI(c, gitURI, spaceID, sessIdx)
			if err != nil {
				return err
			}

			gc, cleanup, err := mountGitContext(c, statePath, uri)
			if err != nil {
				return err
			}
			defer cleanup()

			wtKey := c.String("worktree-key")
			repoKey := gc.objectKey

			checkoutOpts := &git_block.CheckoutOpts{
				Branch: c.String("branch"),
				Create: c.Bool("create"),
				Force:  c.Bool("force"),
				Keep:   c.Bool("keep"),
			}

			op := git_world.NewGitWorktreeCheckoutOp(wtKey, repoKey, checkoutOpts)
			if err := applyWorldOp(c, gc.engine, op); err != nil {
				return err
			}

			os.Stdout.WriteString("checked out\n")
			return nil
		},
	}
}
