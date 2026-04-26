//go:build !js

package spacewave_cli

import (
	"os"
	"strconv"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	git_world "github.com/s4wave/spacewave/db/git/world"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
)

// newObjectCommand builds the object command group as a subcommand of space.
// It inherits statePath, outputFormat, sessionIdx from the parent space command
// and adds a --space-id flag for all object subcommands.
func newObjectCommand(statePath *string, sessionIdx *uint) *cli.Command {
	var spaceID string
	return &cli.Command{
		Name:    "object",
		Aliases: []string{"objects"},
		Usage:   "manage world objects",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "space-id",
				Aliases:     []string{"space"},
				Usage:       "space ID (auto-detected if only one space)",
				EnvVars:     []string{"SPACEWAVE_SPACE"},
				Destination: &spaceID,
			},
		},
		Subcommands: []*cli.Command{
			buildObjectListCommand(statePath, sessionIdx, &spaceID),
			buildObjectInfoCommand(statePath, sessionIdx, &spaceID),
			buildObjectCreateCommand(statePath, sessionIdx, &spaceID),
			buildObjectDeleteCommand(statePath, sessionIdx, &spaceID),
		},
	}
}

// buildObjectListCommand builds the object list subcommand.
func buildObjectListCommand(statePath *string, sessionIdx *uint, spaceID *string) *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "list objects in a space (key + type)",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "watch",
				Usage:   "watch for changes (append mode)",
				EnvVars: []string{"SPACEWAVE_WATCH"},
			},
		},
		Action: func(c *cli.Context) error {
			ctx := c.Context
			watch := c.Bool("watch")
			client, err := connectDaemonFromContext(ctx, c, *statePath)
			if err != nil {
				return err
			}
			defer client.close()

			sess, err := client.mountSession(ctx, uint32(*sessionIdx))
			if err != nil {
				return err
			}
			defer sess.Release()

			sid, err := client.resolveSpaceID(ctx, sess, *spaceID)
			if err != nil {
				return err
			}

			spaceSvc, spaceCleanup, err := client.mountSpace(ctx, sess, sid)
			if err != nil {
				return err
			}
			defer spaceCleanup()

			strm, err := spaceSvc.WatchSpaceState(ctx, &s4wave_space.WatchSpaceStateRequest{})
			if err != nil {
				return errors.Wrap(err, "watch space state")
			}
			defer strm.Close()

			w := os.Stdout
			for {
				state, err := strm.Recv()
				if err != nil {
					return errors.Wrap(err, "recv space state")
				}

				wc := state.GetWorldContents()
				if wc == nil {
					w.WriteString("no objects\n")
				} else {
					objs := wc.GetObjects()
					if len(objs) == 0 {
						w.WriteString("no objects\n")
					} else {
						rows := [][]string{{"KEY", "TYPE"}}
						for _, obj := range objs {
							rows = append(rows, []string{obj.GetObjectKey(), obj.GetObjectType()})
						}
						writeTable(w, "", rows)
					}
				}

				if !watch {
					return nil
				}
				w.WriteString("--- " + time.Now().Format(time.RFC3339) + " ---\n")
			}
		},
	}
}

// buildObjectInfoCommand builds the object info subcommand.
func buildObjectInfoCommand(statePath *string, sessionIdx *uint, spaceID *string) *cli.Command {
	return &cli.Command{
		Name:      "info",
		Usage:     "show object state and root ref",
		ArgsUsage: "<object-key-or-uri>",
		Action: func(c *cli.Context) error {
			arg := c.Args().First()
			if arg == "" {
				return errors.New("object key or URI required")
			}

			ctx := c.Context
			sid := *spaceID

			// Simple URI parsing: if arg contains /u/ and /so/, extract components.
			objectKey := arg
			objectKey, sid = parseObjectArg(objectKey, sid)
			client, err := connectDaemonFromContext(ctx, c, *statePath)
			if err != nil {
				return err
			}
			defer client.close()

			sess, err := client.mountSession(ctx, uint32(*sessionIdx))
			if err != nil {
				return err
			}
			defer sess.Release()

			sid, err = client.resolveSpaceID(ctx, sess, sid)
			if err != nil {
				return err
			}

			spaceSvc, spaceCleanup, err := client.mountSpace(ctx, sess, sid)
			if err != nil {
				return err
			}
			defer spaceCleanup()

			engine, engineCleanup, err := client.accessWorldEngine(ctx, spaceSvc)
			if err != nil {
				return err
			}
			defer engineCleanup()

			tx, err := engine.NewTransaction(ctx, false)
			if err != nil {
				return errors.Wrap(err, "new transaction")
			}
			defer tx.Discard()

			obj, found, err := tx.GetObject(ctx, objectKey)
			if err != nil {
				return errors.Wrap(err, "get object")
			}
			if !found {
				return errors.Errorf("object %q not found", objectKey)
			}

			w := os.Stdout
			fields := [][2]string{{"Key", obj.GetKey()}}

			rootRef, rev, err := obj.GetRootRef(ctx)
			if err != nil {
				fields = append(fields, [2]string{"Root Ref", "error: " + err.Error()})
			} else {
				fields = append(fields, [2]string{"Rev", strconv.FormatUint(rev, 10)})
				if rootRef != nil {
					if bucketID := rootRef.GetBucketId(); bucketID != "" {
						fields = append(fields, [2]string{"Bucket", bucketID})
					}
					if blockRef := rootRef.GetRootRef(); blockRef != nil {
						if h := blockRef.GetHash(); h != nil && len(h.GetHash()) > 0 {
							fields = append(fields, [2]string{"Hash", h.GetHashType().String() + " (" + strconv.Itoa(len(h.GetHash())) + " bytes)"})
						}
					}
				}
			}
			writeFields(w, fields)
			return nil
		},
	}
}

// parseObjectArg parses an object argument that may be a full URI or plain key.
// If the arg starts with /u/, it extracts the space ID and object key.
// If the arg contains /-/, the first segment is the object key.
// Returns the object key and the space ID (which may be unchanged).
func parseObjectArg(arg, spaceID string) (string, string) {
	// Full URI: /u/{idx}/so/{space_id}/-/{objectKey}
	if len(arg) > 3 && arg[0] == '/' && arg[1] == 'u' && arg[2] == '/' {
		rest := arg[3:]
		// skip session index
		idx := 0
		for idx < len(rest) && rest[idx] != '/' {
			idx++
		}
		if idx < len(rest) {
			rest = rest[idx+1:]
		}
		// expect "so/{space_id}"
		if len(rest) > 3 && rest[:3] == "so/" {
			rest = rest[3:]
			idx = 0
			for idx < len(rest) && rest[idx] != '/' {
				idx++
			}
			spaceID = rest[:idx]
			if idx < len(rest) {
				rest = rest[idx+1:]
			} else {
				rest = ""
			}
			// expect "/-/{objectKey}..."
			if len(rest) >= 2 && rest[:2] == "-/" {
				rest = rest[2:]
			} else if rest == "-" {
				rest = ""
			}
			// The remaining part up to the next /-/ is the object key.
			delimIdx := findSubpathDelimiter(rest)
			if delimIdx >= 0 {
				return rest[:delimIdx], spaceID
			}
			return rest, spaceID
		}
	}

	// Arg contains /-/ delimiter: first segment is object key.
	delimIdx := findSubpathDelimiter(arg)
	if delimIdx >= 0 {
		return arg[:delimIdx], spaceID
	}
	return arg, spaceID
}

// findSubpathDelimiter finds the index of /-/ in s. Returns -1 if not found.
func findSubpathDelimiter(s string) int {
	for i := 0; i+2 < len(s); i++ {
		if s[i] == '/' && s[i+1] == '-' && s[i+2] == '/' {
			return i
		}
	}
	return -1
}

// buildObjectCreateCommand builds the object create subcommand.
func buildObjectCreateCommand(statePath *string, sessionIdx *uint, spaceID *string) *cli.Command {
	return &cli.Command{
		Name:      "create",
		Usage:     "create an object via type-specific world op",
		ArgsUsage: "<key>",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "type",
				Usage:    "object type: fs, git, canvas, canvas-demo",
				EnvVars:  []string{"SPACEWAVE_OBJECT_TYPE"},
				Required: true,
			},
		},
		Action: func(c *cli.Context) error {
			key := c.Args().First()
			if key == "" {
				return errors.New("object key required")
			}

			// Validate key does not contain /-/ delimiter.
			if findSubpathDelimiter(key) >= 0 {
				return errors.New("object key cannot contain /-/")
			}

			ctx := c.Context
			objType := c.String("type")
			client, err := connectDaemonFromContext(ctx, c, *statePath)
			if err != nil {
				return err
			}
			defer client.close()

			sess, err := client.mountSession(ctx, uint32(*sessionIdx))
			if err != nil {
				return err
			}
			defer sess.Release()

			sid, err := client.resolveSpaceID(ctx, sess, *spaceID)
			if err != nil {
				return err
			}

			spaceSvc, spaceCleanup, err := client.mountSpace(ctx, sess, sid)
			if err != nil {
				return err
			}
			defer spaceCleanup()

			engine, engineCleanup, err := client.accessWorldEngine(ctx, spaceSvc)
			if err != nil {
				return err
			}
			defer engineCleanup()

			tx, err := engine.NewTransaction(ctx, true)
			if err != nil {
				return errors.Wrap(err, "new transaction")
			}
			defer tx.Discard()

			switch objType {
			case "fs":
				op := unixfs_world.NewFsInitOp(
					key,
					unixfs_world.FSType_FSType_FS_NODE,
					nil,
					false,
					time.Now(),
				)
				_, _, err = tx.ApplyWorldOp(ctx, op, "")
				if err != nil {
					return errors.Wrap(err, "apply fs init op")
				}
			case "git":
				op := git_world.NewGitInitOp(key, nil, true, nil, nil)
				_, _, err = tx.ApplyWorldOp(ctx, op, "")
				if err != nil {
					return errors.Wrap(err, "apply git init op")
				}
			case "canvas":
				op := space_world_ops.NewCanvasInitOp(key, time.Now())
				_, _, err = tx.ApplyWorldOp(ctx, op, "")
				if err != nil {
					return errors.Wrap(err, "apply canvas init op")
				}
			case "canvas-demo":
				op := space_world_ops.NewInitCanvasDemoOp(key, time.Now())
				_, _, err = tx.ApplyWorldOp(ctx, op, "")
				if err != nil {
					return errors.Wrap(err, "apply canvas demo init op")
				}
			default:
				return errors.Errorf("unsupported object type: %s (supported: fs, git, canvas, canvas-demo)", objType)
			}

			if err := tx.Commit(ctx); err != nil {
				return errors.Wrap(err, "commit transaction")
			}

			os.Stdout.WriteString("Created object \"" + key + "\" (type=" + objType + ").\n")
			return nil
		},
	}
}

// buildObjectDeleteCommand builds the object delete subcommand.
func buildObjectDeleteCommand(statePath *string, sessionIdx *uint, spaceID *string) *cli.Command {
	return &cli.Command{
		Name:      "delete",
		Usage:     "delete an object from the world",
		ArgsUsage: "<key>",
		Action: func(c *cli.Context) error {
			key := c.Args().First()
			if key == "" {
				return errors.New("object key required")
			}

			ctx := c.Context
			client, err := connectDaemonFromContext(ctx, c, *statePath)
			if err != nil {
				return err
			}
			defer client.close()

			sess, err := client.mountSession(ctx, uint32(*sessionIdx))
			if err != nil {
				return err
			}
			defer sess.Release()

			sid, err := client.resolveSpaceID(ctx, sess, *spaceID)
			if err != nil {
				return err
			}

			spaceSvc, spaceCleanup, err := client.mountSpace(ctx, sess, sid)
			if err != nil {
				return err
			}
			defer spaceCleanup()

			engine, engineCleanup, err := client.accessWorldEngine(ctx, spaceSvc)
			if err != nil {
				return err
			}
			defer engineCleanup()

			tx, err := engine.NewTransaction(ctx, true)
			if err != nil {
				return errors.Wrap(err, "new transaction")
			}
			defer tx.Discard()

			deleted, err := tx.DeleteObject(ctx, key)
			if err != nil {
				return errors.Wrap(err, "delete object")
			}
			if !deleted {
				return errors.Errorf("object %q not found", key)
			}

			if err := tx.Commit(ctx); err != nil {
				return errors.Wrap(err, "commit transaction")
			}

			os.Stdout.WriteString("Deleted object \"" + key + "\".\n")
			return nil
		},
	}
}
