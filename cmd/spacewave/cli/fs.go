//go:build !js

package spacewave_cli

import (
	"io"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	s4wave_unixfs "github.com/s4wave/spacewave/sdk/unixfs"
)

// readChunkSize is the size of chunks when reading file data.
const readChunkSize = 32 * 1024

// writeChunkSize is the size of chunks when writing file data.
const writeChunkSize = 32 * 1024

// newFsCommand builds the fs command group for filesystem operations.
func newFsCommand(_ func() cli_entrypoint.CliBus) *cli.Command {
	return &cli.Command{
		Name:  "fs",
		Usage: "filesystem operations on UnixFS objects",
		Subcommands: []*cli.Command{
			buildFsLsCommand(),
			buildFsCatCommand(),
			buildFsMkdirCommand(),
			buildFsRmCommand(),
			buildFsWriteCommand(),
			buildFsMvCommand(),
			buildFsStatCommand(),
		},
	}
}

// fsURIGrammar is the shared URI argument documentation included in each fs
// subcommand's --help description. Newlines are preserved in cli help output
// (each line is reindented under DESCRIPTION:).
const fsURIGrammar = `Each URI argument selects the session, space, object, and (optionally) a
path inside the object. Three forms are accepted:

  object key only          my-object
  object key + path        my-object/-/some/dir
  full URI                 /u/1/so/my-space/-/my-object/-/some/dir

The "/-/" sequence delimits URI segments. The first segment is the object
key (the entity within a space). Later segments form the UnixFS path inside
that object. In the full form, "/u/{n}" selects the session by index and
"so/{space-id}" selects the space. The full form is identical to the URL
path used by the spacewave web UI, so a path copied from the browser
address bar can be used here directly.

When the short forms are used the session index defaults to 1 and the space
is resolved from the daemon's default space. The --space and --session-index
flags (or SPACEWAVE_SPACE / SPACEWAVE_SESSION_INDEX) override the values
parsed from each URI.`

// fsURIDescription composes a help description from a lead sentence, the
// shared URI grammar, and a subcommand-specific examples block.
func fsURIDescription(lead, examples string) string {
	return lead + "\n\n" + fsURIGrammar + "\n\nEXAMPLES:\n\n" + examples
}

// fsURI holds the parsed components needed for FS operations.
type fsURI struct {
	sessionIdx uint32
	spaceID    string
	objectKey  string
	path       string
}

// parseFsURI parses a URI argument and optional flags into fsURI components.
// If spaceFlag is non-empty, it overrides the space from the URI.
// If sessFlag is non-zero, it overrides the session from the URI.
func parseFsURI(arg string, spaceFlag string, sessFlag int) (fsURI, error) {
	if arg == "" {
		return fsURI{}, errors.New("URI argument required")
	}

	parsed, err := s4wave_space.ParseSpacewaveURI(arg)
	if err != nil {
		return fsURI{}, errors.Wrap(err, "parse URI")
	}

	result := fsURI{
		sessionIdx: parsed.SessionIdx,
		spaceID:    parsed.SpaceID,
	}

	if sessFlag > 0 {
		result.sessionIdx = uint32(sessFlag)
	}
	if spaceFlag != "" {
		result.spaceID = spaceFlag
	}

	// segments: [0]=objectKey, [1]=path within object, [2+]=nested
	if len(parsed.Segments) > 0 {
		result.objectKey = parsed.Segments[0]
	}
	if len(parsed.Segments) > 1 {
		result.path = parsed.Segments[1]
	}

	if result.objectKey == "" {
		return fsURI{}, errors.New("object key required in URI")
	}

	return result, nil
}

// fsContext holds the mounted resources for FS operations.
type fsContext struct {
	fsSvc s4wave_unixfs.SRPCFSHandleResourceServiceClient
	// resClient is the resource client for creating sub-references.
	resClient *resource_client.Client
}

// mountFsContext connects to the daemon and mounts the full chain to get
// an FSHandleResourceService client for the given URI.
func mountFsContext(c *cli.Context, statePath string, uri fsURI) (*fsContext, func(), error) {
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

	// resolve space ID
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

	_, engineRef, engineCleanup, err := client.accessWorldEngineWithRef(ctx, spaceSvc)
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

	fsSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(typedClient)

	cleanup := func() {
		typedCleanup()
		engineCleanup()
		spaceCleanup()
		sess.Release()
		client.close()
	}

	return &fsContext{
		fsSvc:     fsSvc,
		resClient: client.resClient,
	}, cleanup, nil
}

// lookupPath navigates from the root FSHandle to the given path.
// Returns the SRPC client for the handle at that path and a cleanup function.
// If fsPath is empty, returns the root handle's service directly.
func (fc *fsContext) lookupPath(c *cli.Context, fsPath string) (s4wave_unixfs.SRPCFSHandleResourceServiceClient, func(), error) {
	if fsPath == "" {
		return fc.fsSvc, func() {}, nil
	}

	ctx := c.Context
	resp, err := fc.fsSvc.LookupPath(ctx, &s4wave_unixfs.HandleLookupPathRequest{Path: fsPath})
	if err != nil {
		return nil, nil, errors.Wrap(err, "lookup path "+fsPath)
	}

	ref := fc.resClient.CreateResourceReference(resp.GetResourceId())
	childClient, err := ref.GetClient()
	if err != nil {
		ref.Release()
		return nil, nil, errors.Wrap(err, "child handle client")
	}

	childSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(childClient)
	cleanup := func() {
		ref.Release()
	}
	return childSvc, cleanup, nil
}

// lookupParentAndName navigates to the parent directory of fsPath and returns
// the parent's FSHandle service, the base name, and a cleanup function.
// If fsPath has no parent (e.g., it is a top-level name), the root handle is used.
func (fc *fsContext) lookupParentAndName(c *cli.Context, fsPath string) (s4wave_unixfs.SRPCFSHandleResourceServiceClient, string, func(), error) {
	dir := path.Dir(fsPath)
	base := path.Base(fsPath)
	if dir == "." || dir == "/" {
		dir = ""
	}

	svc, cleanup, err := fc.lookupPath(c, dir)
	if err != nil {
		return nil, "", nil, err
	}
	return svc, base, cleanup, nil
}

// commonFsFlags returns the flags shared by all fs subcommands.
func commonFsFlags(statePath *string, spaceID *string, sessIdx *int) []cli.Flag {
	return []cli.Flag{
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
	}
}

// buildFsLsCommand builds the fs ls subcommand.
func buildFsLsCommand() *cli.Command {
	var statePath, spaceID string
	var sessIdx int
	return &cli.Command{
		Name:      "ls",
		Usage:     "list directory contents",
		ArgsUsage: "<uri>",
		Description: fsURIDescription(
			"List entries in a directory inside a UnixFS object.",
			`  # list the root of an object in the default space and session
  spacewave fs ls my-object

  # list a subdirectory inside an object
  spacewave fs ls my-object/-/docs/images

  # list using the full URI form (session 1, space "my-space")
  spacewave fs ls /u/1/so/my-space/-/my-object/-/docs

  # override the space and session via flags
  spacewave fs ls --space my-space --session-index 2 my-object/-/docs`,
		),
		Flags: commonFsFlags(&statePath, &spaceID, &sessIdx),
		Action: func(c *cli.Context) error {
			uri, err := parseFsURI(c.Args().First(), spaceID, sessIdx)
			if err != nil {
				return err
			}

			fc, cleanup, err := mountFsContext(c, statePath, uri)
			if err != nil {
				return err
			}
			defer cleanup()

			svc, pathCleanup, err := fc.lookupPath(c, uri.path)
			if err != nil {
				return err
			}
			defer pathCleanup()

			ctx := c.Context
			strm, err := svc.Readdir(ctx, &s4wave_unixfs.HandleReaddirRequest{})
			if err != nil {
				return errors.Wrap(err, "readdir")
			}

			type entryInfo struct {
				Name    string
				Size    uint64
				IsDir   bool
				Mode    uint32
				ModTime int64
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
					Name:    entry.GetName(),
					Size:    entry.GetSize(),
					IsDir:   entry.GetIsDir(),
					Mode:    entry.GetMode(),
					ModTime: entry.GetModTime(),
				})
			}

			outputFormat := c.String("output")
			if outputFormat == "json" || outputFormat == "yaml" {
				buf, ms := newMarshalBuf()
				ms.WriteArrayStart()
				var af bool
				for _, e := range entries {
					ms.WriteMoreIf(&af)
					ms.WriteObjectStart()
					var f bool
					ms.WriteMoreIf(&f)
					ms.WriteObjectField("name")
					ms.WriteString(e.Name)
					ms.WriteMoreIf(&f)
					ms.WriteObjectField("size")
					ms.WriteUint64(e.Size)
					ms.WriteMoreIf(&f)
					ms.WriteObjectField("isDir")
					ms.WriteBool(e.IsDir)
					ms.WriteMoreIf(&f)
					ms.WriteObjectField("mode")
					ms.WriteUint32(e.Mode)
					if e.ModTime != 0 {
						ms.WriteMoreIf(&f)
						ms.WriteObjectField("modTime")
						ms.WriteInt64(e.ModTime)
					}
					ms.WriteObjectEnd()
				}
				ms.WriteArrayEnd()
				return formatOutput(buf.Bytes(), outputFormat)
			}

			w := os.Stdout
			for _, e := range entries {
				modeStr := os.FileMode(e.Mode).String()
				sizeStr := strconv.FormatUint(e.Size, 10)
				// pad size to 10 chars
				for len(sizeStr) < 10 {
					sizeStr = " " + sizeStr
				}
				typeChar := "-"
				if e.IsDir {
					typeChar = "d"
				}
				w.WriteString(typeChar + " " + modeStr + " " + sizeStr + " " + e.Name + "\n")
			}

			return nil
		},
	}
}

// buildFsCatCommand builds the fs cat subcommand.
func buildFsCatCommand() *cli.Command {
	var statePath, spaceID string
	var sessIdx int
	var offset, limit uint64
	return &cli.Command{
		Name:      "cat",
		Usage:     "read file contents to stdout",
		ArgsUsage: "<uri>",
		Description: fsURIDescription(
			"Read the contents of a file inside a UnixFS object to stdout.",
			`  # print a whole file
  spacewave fs cat my-object/-/notes.txt

  # print a 1KiB window starting at byte 4096
  spacewave fs cat --offset 4096 --limit 1024 my-object/-/big.bin

  # full URI form
  spacewave fs cat /u/1/so/my-space/-/my-object/-/notes.txt`,
		),
		Flags: append(commonFsFlags(&statePath, &spaceID, &sessIdx),
			&cli.Uint64Flag{
				Name:        "offset",
				Usage:       "byte offset to start reading from",
				EnvVars:     []string{"SPACEWAVE_OFFSET"},
				Value:       0,
				Destination: &offset,
			},
			&cli.Uint64Flag{
				Name:        "limit",
				Usage:       "maximum bytes to read (0 = read all)",
				EnvVars:     []string{"SPACEWAVE_LIMIT"},
				Value:       0,
				Destination: &limit,
			},
		),
		Action: func(c *cli.Context) error {
			uri, err := parseFsURI(c.Args().First(), spaceID, sessIdx)
			if err != nil {
				return err
			}

			fc, cleanup, err := mountFsContext(c, statePath, uri)
			if err != nil {
				return err
			}
			defer cleanup()

			svc, pathCleanup, err := fc.lookupPath(c, uri.path)
			if err != nil {
				return err
			}
			defer pathCleanup()

			ctx := c.Context
			pos := int64(offset)
			var totalRead uint64

			for {
				chunkLen := int64(readChunkSize)
				if limit > 0 {
					remaining := int64(limit) - int64(totalRead)
					if remaining <= 0 {
						break
					}
					if chunkLen > remaining {
						chunkLen = remaining
					}
				}

				resp, err := svc.ReadAt(ctx, &s4wave_unixfs.HandleReadAtRequest{
					Offset: pos,
					Length: chunkLen,
				})
				if err != nil {
					return errors.Wrap(err, "read at offset "+strconv.FormatInt(pos, 10))
				}

				data := resp.GetData()
				if len(data) > 0 {
					_, err = os.Stdout.Write(data)
					if err != nil {
						return errors.Wrap(err, "write stdout")
					}
					pos += int64(len(data))
					totalRead += uint64(len(data))
				}

				if resp.GetEof() || len(data) == 0 {
					break
				}
			}

			return nil
		},
	}
}

// buildFsMkdirCommand builds the fs mkdir subcommand.
func buildFsMkdirCommand() *cli.Command {
	var statePath, spaceID string
	var sessIdx int
	return &cli.Command{
		Name:      "mkdir",
		Usage:     "create a directory (and parents)",
		ArgsUsage: "<uri>",
		Description: fsURIDescription(
			"Create a directory inside a UnixFS object, including any missing parents.",
			`  # create a/b/c under the object root
  spacewave fs mkdir my-object/-/a/b/c

  # full URI form
  spacewave fs mkdir /u/1/so/my-space/-/my-object/-/a/b/c`,
		),
		Flags: commonFsFlags(&statePath, &spaceID, &sessIdx),
		Action: func(c *cli.Context) error {
			uri, err := parseFsURI(c.Args().First(), spaceID, sessIdx)
			if err != nil {
				return err
			}

			if uri.path == "" {
				return errors.New("path required for mkdir")
			}

			fc, cleanup, err := mountFsContext(c, statePath, uri)
			if err != nil {
				return err
			}
			defer cleanup()

			ctx := c.Context
			parts := strings.Split(uri.path, "/")
			_, err = fc.fsSvc.MkdirAll(ctx, &s4wave_unixfs.HandleMkdirAllRequest{
				PathParts: parts,
				Mode:      0o755,
			})
			if err != nil {
				return errors.Wrap(err, "mkdir")
			}

			return nil
		},
	}
}

// buildFsRmCommand builds the fs rm subcommand.
func buildFsRmCommand() *cli.Command {
	var statePath, spaceID string
	var sessIdx int
	return &cli.Command{
		Name:      "rm",
		Usage:     "remove a file or directory",
		ArgsUsage: "<uri>",
		Description: fsURIDescription(
			"Remove a file or directory inside a UnixFS object. The URI must include a path; removing an object root is not supported.",
			`  # delete a file
  spacewave fs rm my-object/-/old/notes.txt

  # delete a directory
  spacewave fs rm my-object/-/old`,
		),
		Flags: commonFsFlags(&statePath, &spaceID, &sessIdx),
		Action: func(c *cli.Context) error {
			uri, err := parseFsURI(c.Args().First(), spaceID, sessIdx)
			if err != nil {
				return err
			}

			if uri.path == "" {
				return errors.New("path required for rm")
			}

			fc, cleanup, err := mountFsContext(c, statePath, uri)
			if err != nil {
				return err
			}
			defer cleanup()

			parentSvc, name, parentCleanup, err := fc.lookupParentAndName(c, uri.path)
			if err != nil {
				return err
			}
			defer parentCleanup()

			ctx := c.Context
			_, err = parentSvc.Remove(ctx, &s4wave_unixfs.HandleRemoveRequest{
				Names: []string{name},
			})
			if err != nil {
				return errors.Wrap(err, "remove")
			}

			return nil
		},
	}
}

// buildFsWriteCommand builds the fs write subcommand.
func buildFsWriteCommand() *cli.Command {
	var statePath, spaceID, fromPath string
	var sessIdx int
	return &cli.Command{
		Name:      "write",
		Usage:     "write data to a file from stdin or local file",
		ArgsUsage: "<uri>",
		Description: fsURIDescription(
			"Write data to a file inside a UnixFS object, creating it if needed. The target file is truncated before writing. Input comes from stdin unless --from is given.",
			`  # write a file from stdin
  echo hello | spacewave fs write my-object/-/greeting.txt

  # upload a local file
  spacewave fs write --from ./report.pdf my-object/-/docs/report.pdf

  # full URI form
  spacewave fs write --from ./report.pdf /u/1/so/my-space/-/my-object/-/docs/report.pdf`,
		),
		Flags: append(commonFsFlags(&statePath, &spaceID, &sessIdx),
			&cli.StringFlag{
				Name:        "from",
				Usage:       "local file path to read from (default: stdin)",
				EnvVars:     []string{"SPACEWAVE_FROM"},
				Destination: &fromPath,
			},
		),
		Action: func(c *cli.Context) error {
			uri, err := parseFsURI(c.Args().First(), spaceID, sessIdx)
			if err != nil {
				return err
			}

			if uri.path == "" {
				return errors.New("path required for write")
			}

			fc, cleanup, err := mountFsContext(c, statePath, uri)
			if err != nil {
				return err
			}
			defer cleanup()

			// navigate to the target file, creating it if it doesn't exist
			svc, pathCleanup, err := fc.lookupPath(c, uri.path)
			if err != nil {
				// file doesn't exist — create it via Mknod on the parent, then lookup
				parentSvc, baseName, parentCleanup, parentErr := fc.lookupParentAndName(c, uri.path)
				if parentErr != nil {
					return errors.Wrap(err, "lookup path "+uri.path)
				}
				_, mknodErr := parentSvc.Mknod(c.Context, &s4wave_unixfs.HandleMknodRequest{
					Names: []string{baseName},
					Mode:  0o644,
				})
				parentCleanup()
				if mknodErr != nil {
					return errors.Wrap(mknodErr, "create file "+baseName)
				}

				// now lookup the newly created file
				svc, pathCleanup, err = fc.lookupPath(c, uri.path)
				if err != nil {
					return errors.Wrap(err, "lookup after create "+uri.path)
				}
			}
			defer pathCleanup()

			// determine input source
			var reader io.Reader
			if fromPath != "" {
				f, err := os.Open(fromPath)
				if err != nil {
					return errors.Wrap(err, "open local file")
				}
				defer f.Close()
				reader = f
			} else {
				reader = os.Stdin
			}

			// truncate the file first to ensure clean write
			ctx := c.Context
			_, err = svc.Truncate(ctx, &s4wave_unixfs.HandleTruncateRequest{Size: 0})
			if err != nil {
				return errors.Wrap(err, "truncate before write")
			}

			buf := make([]byte, writeChunkSize)
			var pos int64
			for {
				n, readErr := reader.Read(buf)
				if n > 0 {
					_, err = svc.WriteAt(ctx, &s4wave_unixfs.HandleWriteAtRequest{
						Offset: pos,
						Data:   buf[:n],
					})
					if err != nil {
						return errors.Wrap(err, "write at offset "+strconv.FormatInt(pos, 10))
					}
					pos += int64(n)
				}
				if readErr == io.EOF {
					break
				}
				if readErr != nil {
					return errors.Wrap(readErr, "read input")
				}
			}

			return nil
		},
	}
}

// buildFsMvCommand builds the fs mv subcommand.
func buildFsMvCommand() *cli.Command {
	var statePath, spaceID string
	var sessIdx int
	return &cli.Command{
		Name:      "mv",
		Usage:     "rename/move a file or directory",
		ArgsUsage: "<source-uri> <dest-uri>",
		Description: fsURIDescription(
			"Rename or move a file or directory inside a UnixFS object. The source and destination URIs must point at the same space and the same object; only the path within the object may differ.",
			`  # rename a file in place
  spacewave fs mv my-object/-/old.txt my-object/-/new.txt

  # move a file into a different directory
  spacewave fs mv my-object/-/inbox/note.md my-object/-/archive/2026/note.md`,
		),
		Flags: commonFsFlags(&statePath, &spaceID, &sessIdx),
		Action: func(c *cli.Context) error {
			if c.NArg() < 2 {
				return errors.New("source and destination URIs required")
			}

			srcURI, err := parseFsURI(c.Args().Get(0), spaceID, sessIdx)
			if err != nil {
				return errors.Wrap(err, "parse source URI")
			}
			dstURI, err := parseFsURI(c.Args().Get(1), spaceID, sessIdx)
			if err != nil {
				return errors.Wrap(err, "parse dest URI")
			}

			if srcURI.objectKey != dstURI.objectKey {
				return errors.New("source and destination must be in the same object")
			}
			if srcURI.spaceID != dstURI.spaceID {
				return errors.New("source and destination must be in the same space")
			}
			if srcURI.path == "" {
				return errors.New("source path required for mv")
			}
			if dstURI.path == "" {
				return errors.New("destination path required for mv")
			}

			fc, cleanup, err := mountFsContext(c, statePath, srcURI)
			if err != nil {
				return err
			}
			defer cleanup()

			// look up source: navigate to parent directory and get the entry handle
			srcSvc, _, srcCleanup, err := fc.lookupParentAndName(c, srcURI.path)
			if err != nil {
				return errors.Wrap(err, "lookup source parent")
			}
			defer srcCleanup()

			srcName := path.Base(srcURI.path)

			// look up the source entry to get a handle on it
			ctx := c.Context
			srcEntryResp, err := srcSvc.Lookup(ctx, &s4wave_unixfs.HandleLookupRequest{Name: srcName})
			if err != nil {
				return errors.Wrap(err, "lookup source entry")
			}

			srcEntryRef := fc.resClient.CreateResourceReference(srcEntryResp.GetResourceId())
			srcEntryClient, err := srcEntryRef.GetClient()
			if err != nil {
				srcEntryRef.Release()
				return errors.Wrap(err, "source entry client")
			}
			srcEntrySvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(srcEntryClient)
			defer srcEntryRef.Release()

			// look up dest parent
			dstParentSvc, _, dstCleanup, err := fc.lookupParentAndName(c, dstURI.path)
			if err != nil {
				return errors.Wrap(err, "lookup dest parent")
			}
			defer dstCleanup()

			// clone the dest parent to get a resource ID for it
			dstParentCloneResp, err := dstParentSvc.Clone(ctx, &s4wave_unixfs.HandleCloneRequest{})
			if err != nil {
				return errors.Wrap(err, "clone dest parent")
			}
			dstParentResourceID := dstParentCloneResp.GetResourceId()
			dstParentCloneRef := fc.resClient.CreateResourceReference(dstParentResourceID)
			defer dstParentCloneRef.Release()

			dstName := path.Base(dstURI.path)

			_, err = srcEntrySvc.Rename(ctx, &s4wave_unixfs.HandleRenameRequest{
				DestParentResourceId: dstParentResourceID,
				DestName:             dstName,
			})
			if err != nil {
				return errors.Wrap(err, "rename")
			}

			return nil
		},
	}
}

// buildFsStatCommand builds the fs stat subcommand.
func buildFsStatCommand() *cli.Command {
	var statePath, spaceID string
	var sessIdx int
	return &cli.Command{
		Name:      "stat",
		Usage:     "show file or directory information",
		ArgsUsage: "<uri>",
		Description: fsURIDescription(
			"Show metadata (name, type, size, mode, modification time) for a file or directory inside a UnixFS object. Omit the path to stat the object's UnixFS root.",
			`  # stat the root of an object
  spacewave fs stat my-object

  # stat a file
  spacewave fs stat my-object/-/docs/report.pdf

  # full URI form
  spacewave fs stat /u/1/so/my-space/-/my-object/-/docs/report.pdf`,
		),
		Flags: commonFsFlags(&statePath, &spaceID, &sessIdx),
		Action: func(c *cli.Context) error {
			uri, err := parseFsURI(c.Args().First(), spaceID, sessIdx)
			if err != nil {
				return err
			}

			fc, cleanup, err := mountFsContext(c, statePath, uri)
			if err != nil {
				return err
			}
			defer cleanup()

			svc, pathCleanup, err := fc.lookupPath(c, uri.path)
			if err != nil {
				return err
			}
			defer pathCleanup()

			ctx := c.Context
			resp, err := svc.GetFileInfo(ctx, &s4wave_unixfs.HandleGetFileInfoRequest{})
			if err != nil {
				return errors.Wrap(err, "get file info")
			}

			info := resp.GetInfo()
			if info == nil {
				return errors.New("no file info returned")
			}

			name := info.GetName()
			size := info.GetSize()
			mode := info.GetMode()
			modTime := info.GetModTime()
			isDir := info.GetIsDir()

			outputFormat := c.String("output")
			if outputFormat == "json" || outputFormat == "yaml" {
				buf, ms := newMarshalBuf()
				ms.WriteObjectStart()
				var f bool
				ms.WriteMoreIf(&f)
				ms.WriteObjectField("name")
				ms.WriteString(name)
				ms.WriteMoreIf(&f)
				ms.WriteObjectField("size")
				ms.WriteInt64(size)
				ms.WriteMoreIf(&f)
				ms.WriteObjectField("mode")
				ms.WriteUint32(mode)
				if modTime != 0 {
					ms.WriteMoreIf(&f)
					ms.WriteObjectField("modTime")
					ms.WriteInt64(modTime)
				}
				ms.WriteMoreIf(&f)
				ms.WriteObjectField("isDir")
				ms.WriteBool(isDir)
				ms.WriteObjectEnd()
				return formatOutput(buf.Bytes(), outputFormat)
			}

			w := os.Stdout
			displayName := name
			if displayName == "" {
				displayName = uri.path
				if displayName == "" {
					displayName = "(root)"
				}
			}
			typeStr := "file"
			if isDir {
				typeStr = "directory"
			}
			fields := [][2]string{
				{"Name", displayName},
				{"Type", typeStr},
				{"Size", strconv.FormatInt(size, 10)},
				{"Mode", os.FileMode(mode).String()},
			}
			if modTime > 0 {
				t := time.Unix(modTime, 0)
				fields = append(fields, [2]string{"Modified", t.Format(time.RFC3339)})
			}
			writeFields(w, fields)

			return nil
		},
	}
}
