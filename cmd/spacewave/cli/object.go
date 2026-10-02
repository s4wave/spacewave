//go:build !js

package spacewave_cli

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	space_world "github.com/s4wave/spacewave/core/space/world"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	git_world "github.com/s4wave/spacewave/db/git/world"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	sdk_engine "github.com/s4wave/spacewave/sdk/world/engine"
)

// newObjectCommand builds the object command group as a subcommand of space.
// It inherits statePath and sessionIdx from the parent space command, reads
// the root output flag, and adds a --space-id flag for all object subcommands.
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
			buildObjectGraphCommand(statePath, sessionIdx, &spaceID),
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
			&cli.StringFlag{
				Name:  "prefix",
				Usage: "list only object keys that start with this prefix",
			},
			&cli.BoolFlag{
				Name:    "watch",
				Usage:   "watch for changes (append mode)",
				EnvVars: []string{"SPACEWAVE_WATCH"},
			},
		},
		Action: func(c *cli.Context) error {
			// Mount the session, Space and World engine.
			ctx := c.Context
			client, err := connectDaemonFromContext(ctx, c, *statePath)
			if err != nil {
				return err
			}
			defer client.close()
			sess, err := client.mountSession(ctx, sessionIndex32(*sessionIdx))
			if err != nil {
				return err
			}
			defer sess.Release()

			// Resolve the Space and access its World engine.
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

			// List once, or again after each World change when watching.
			prefix, watch, outputFormat := c.String("prefix"), c.Bool("watch"), c.String("output")
			for {
				seqno, err := engine.GetSeqno(ctx)
				if err != nil {
					return errors.Wrap(err, "get world seqno")
				}
				wc, err := listWorldContents(ctx, engine, prefix)
				if err != nil {
					return err
				}
				if err := writeWorldContents(wc, outputFormat); err != nil {
					return err
				}
				if !watch {
					return nil
				}

				// Wait for the World to advance past the listed state.
				if _, err := engine.WaitSeqno(ctx, seqno+1); err != nil {
					return errors.Wrap(err, "wait world seqno")
				}
				if outputFormat != "json" && outputFormat != "yaml" {
					os.Stdout.WriteString("--- " + time.Now().Format(time.RFC3339) + " ---\n")
				}
			}
		},
	}
}

// listObjectsPageSize is the number of objects requested per ListObjects page.
const listObjectsPageSize = 500

// listWorldContents reads the objects under prefix one bounded page at a time,
// so a large Space never needs one oversized response. Object type
// registrations are listed as types, as the Space state reports them.
func listWorldContents(ctx context.Context, engine *sdk_engine.SDKEngine, prefix string) (*space_world.WorldContents, error) {
	// Read every page from one World transaction.
	tx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		return nil, errors.Wrap(err, "new transaction")
	}
	defer tx.Discard()
	sdkTx, ok := tx.(*sdk_engine.SDKTx)
	if !ok {
		return nil, errors.Errorf("unexpected world transaction type %T", tx)
	}

	// Page through the objects, splitting out the type registrations.
	wc := &space_world.WorldContents{}
	var startAfter string
	for {
		objects, _, more, err := sdkTx.ListObjects(ctx, prefix, "", startAfter, listObjectsPageSize)
		if err != nil {
			return nil, errors.Wrap(err, "list objects")
		}
		for _, obj := range objects {
			if typeID, ok := strings.CutPrefix(obj.ObjectKey, world_types.TypesPrefix); ok {
				wc.ObjectTypes = append(wc.ObjectTypes, &space_world.WorldContentsObjectType{ObjectType: typeID})
				continue
			}
			wc.Objects = append(wc.Objects, &space_world.WorldContentsObject{
				ObjectKey:       obj.ObjectKey,
				ParentObjectKey: obj.ParentObjectKey,
				ObjectType:      obj.TypeID,
			})
		}
		if !more || len(objects) == 0 {
			return wc, nil
		}
		startAfter = objects[len(objects)-1].ObjectKey
	}
}

// writeWorldContents prints one listing as a document or a key and type table.
func writeWorldContents(wc *space_world.WorldContents, outputFormat string) error {
	// Each listing is one document: one JSON line when watching.
	if outputFormat == "json" || outputFormat == "yaml" {
		data, err := wc.MarshalJSON()
		if err != nil {
			return err
		}
		return formatOutput(data, outputFormat)
	}

	// Print the objects as a table.
	objs := wc.GetObjects()
	if len(objs) == 0 {
		_, err := os.Stdout.WriteString("no objects\n")
		return err
	}
	rows := [][]string{{"KEY", "TYPE"}}
	for _, obj := range objs {
		rows = append(rows, []string{obj.GetObjectKey(), obj.GetObjectType()})
	}
	writeTable(os.Stdout, "", rows)
	return nil
}

// buildObjectInfoCommand builds the object info subcommand.
func buildObjectInfoCommand(statePath *string, sessionIdx *uint, spaceID *string) *cli.Command {
	return &cli.Command{
		Name:      "info",
		Usage:     "show object state and root ref",
		ArgsUsage: "<object-key-or-uri>",
		Action: func(c *cli.Context) error {
			// Require an object key or URI.
			arg := c.Args().First()
			if arg == "" {
				return errors.New("object key or URI required")
			}

			// Take the command context and the Space flag.
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

			// Mount the selected session.
			sess, err := client.mountSession(ctx, sessionIndex32(*sessionIdx))
			if err != nil {
				return err
			}
			defer sess.Release()

			// Resolve the Space ID.
			sid, err = client.resolveSpaceID(ctx, sess, sid)
			if err != nil {
				return err
			}

			// Mount the Space service.
			spaceSvc, spaceCleanup, err := client.mountSpace(ctx, sess, sid)
			if err != nil {
				return err
			}
			defer spaceCleanup()

			// Open the World engine.
			engine, engineCleanup, err := client.accessWorldEngine(ctx, spaceSvc)
			if err != nil {
				return err
			}
			defer engineCleanup()

			// Open a read transaction.
			tx, err := engine.NewTransaction(ctx, false)
			if err != nil {
				return errors.Wrap(err, "new transaction")
			}
			defer tx.Discard()

			// Load the object and require that it exists.
			obj, found, err := tx.GetObject(ctx, objectKey)
			defer world.ReleaseObjectState(obj)
			if err != nil {
				return errors.Wrap(err, "get object")
			}
			if !found {
				return errors.Errorf("object %q not found", objectKey)
			}

			// Start the object field list with its key.
			w := os.Stdout
			fields := [][2]string{{"Key", obj.GetKey()}}

			// Read the root ref and write the object fields.
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

func buildObjectGraphCommand(statePath *string, sessionIdx *uint, spaceID *string) *cli.Command {
	return &cli.Command{
		Name:      "graph",
		Usage:     "show graph quads referencing an object",
		ArgsUsage: "<object-key>",
		Action: func(c *cli.Context) error {
			// Require an object key.
			key := c.Args().First()
			if key == "" {
				return errors.New("object key required")
			}

			// Connect to the daemon.
			ctx := c.Context
			client, err := connectDaemonFromContext(ctx, c, *statePath)
			if err != nil {
				return err
			}
			defer client.close()

			// Mount the selected session.
			sess, err := client.mountSession(ctx, sessionIndex32(*sessionIdx))
			if err != nil {
				return err
			}
			defer sess.Release()

			// Resolve the Space ID.
			sid, err := client.resolveSpaceID(ctx, sess, *spaceID)
			if err != nil {
				return err
			}

			// Mount the Space service.
			spaceSvc, spaceCleanup, err := client.mountSpace(ctx, sess, sid)
			if err != nil {
				return err
			}
			defer spaceCleanup()

			// Open the World engine.
			engine, engineCleanup, err := client.accessWorldEngine(ctx, spaceSvc)
			if err != nil {
				return err
			}
			defer engineCleanup()

			// Open a read transaction.
			tx, err := engine.NewTransaction(ctx, false)
			if err != nil {
				return errors.Wrap(err, "new transaction")
			}
			defer tx.Discard()

			// Look up graph edges where the key is the subject or the object.
			subjQuads, err := tx.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys(key, "", "", ""), 0)
			if err != nil {
				return errors.Wrap(err, "lookup outgoing quads")
			}
			objQuads, err := tx.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys("", "", key, ""), 0)
			if err != nil {
				return errors.Wrap(err, "lookup incoming quads")
			}

			// Write the graph edges, skipping duplicates.
			rows := [][]string{{"DIR", "SUBJECT", "PREDICATE", "OBJECT", "LABEL"}}
			seen := make(map[string]struct{}, len(subjQuads)+len(objQuads))
			appendQuad := func(dir string, q world.GraphQuad) {
				id := dir + "\x00" + q.GetSubject() + "\x00" + q.GetPredicate() + "\x00" + q.GetObj() + "\x00" + q.GetLabel()
				if _, ok := seen[id]; ok {
					return
				}
				seen[id] = struct{}{}
				rows = append(rows, []string{dir, q.GetSubject(), q.GetPredicate(), q.GetObj(), q.GetLabel()})
			}
			for _, q := range subjQuads {
				appendQuad("out", q)
			}
			for _, q := range objQuads {
				appendQuad("in", q)
			}
			if len(rows) == 1 {
				os.Stdout.WriteString("no graph quads\n")
				return nil
			}
			writeTable(os.Stdout, "", rows)
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
				Name:    "type",
				Usage:   "object type: fs, git, canvas, canvas-demo",
				EnvVars: []string{"SPACEWAVE_OBJECT_TYPE"},
			},
			&cli.StringFlag{
				Name:    "object-type",
				Usage:   "generic world ObjectType ID to assign to an empty object",
				EnvVars: []string{"SPACEWAVE_OBJECT_TYPE_ID"},
			},
		},
		Action: func(c *cli.Context) error {
			// Require an object key.
			key := c.Args().First()
			if key == "" {
				return errors.New("object key required")
			}

			// Validate key does not contain /-/ delimiter.
			if findSubpathDelimiter(key) >= 0 {
				return errors.New("object key cannot contain /-/")
			}

			// Require exactly one of the type flags.
			ctx := c.Context
			objType := c.String("type")
			objectTypeID := c.String("object-type")
			if objType == "" && objectTypeID == "" {
				return errors.New("either --type or --object-type is required")
			}
			if objType != "" && objectTypeID != "" {
				return errors.New("--type and --object-type are mutually exclusive")
			}

			// Connect to the daemon.
			client, err := connectDaemonFromContext(ctx, c, *statePath)
			if err != nil {
				return err
			}
			defer client.close()

			// Mount the selected session.
			sess, err := client.mountSession(ctx, sessionIndex32(*sessionIdx))
			if err != nil {
				return err
			}
			defer sess.Release()

			// Resolve the Space ID.
			sid, err := client.resolveSpaceID(ctx, sess, *spaceID)
			if err != nil {
				return err
			}

			// Mount the Space service.
			spaceSvc, spaceCleanup, err := client.mountSpace(ctx, sess, sid)
			if err != nil {
				return err
			}
			defer spaceCleanup()

			// Open the World engine.
			engine, engineCleanup, err := client.accessWorldEngine(ctx, spaceSvc)
			if err != nil {
				return err
			}
			defer engineCleanup()

			// Open a write transaction.
			tx, err := engine.NewTransaction(ctx, true)
			if err != nil {
				return errors.Wrap(err, "new transaction")
			}
			defer tx.Discard()

			// Create the object with the requested type.
			switch objType {
			case "":
				{
					var createdObject world.ObjectState
					createdObject, err = tx.CreateObject(ctx, key, nil)
					world.ReleaseObjectState(createdObject)
					if err != nil {
						return errors.Wrap(err, "create object")
					}
				}
				if err := world_types.SetObjectType(ctx, tx, key, objectTypeID); err != nil {
					return errors.Wrap(err, "set object type")
				}
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

			// Commit the create transaction.
			if err := tx.Commit(ctx); err != nil {
				return errors.Wrap(err, "commit transaction")
			}

			// Print the created object key and type.
			if objectTypeID != "" {
				objType = objectTypeID
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
			// Require an object key.
			key := c.Args().First()
			if key == "" {
				return errors.New("object key required")
			}

			// Connect to the daemon.
			ctx := c.Context
			client, err := connectDaemonFromContext(ctx, c, *statePath)
			if err != nil {
				return err
			}
			defer client.close()

			// Mount the selected session.
			sess, err := client.mountSession(ctx, sessionIndex32(*sessionIdx))
			if err != nil {
				return err
			}
			defer sess.Release()

			// Resolve the Space ID.
			sid, err := client.resolveSpaceID(ctx, sess, *spaceID)
			if err != nil {
				return err
			}

			// Mount the Space service.
			spaceSvc, spaceCleanup, err := client.mountSpace(ctx, sess, sid)
			if err != nil {
				return err
			}
			defer spaceCleanup()

			// Open the World engine.
			engine, engineCleanup, err := client.accessWorldEngine(ctx, spaceSvc)
			if err != nil {
				return err
			}
			defer engineCleanup()

			// Open a write transaction.
			tx, err := engine.NewTransaction(ctx, true)
			if err != nil {
				return errors.Wrap(err, "new transaction")
			}
			defer tx.Discard()

			// Delete the object and require that it existed.
			deleted, err := tx.DeleteObject(ctx, key)
			if err != nil {
				return errors.Wrap(err, "delete object")
			}
			if !deleted {
				return errors.Errorf("object %q not found", key)
			}

			// Commit the delete transaction.
			if err := tx.Commit(ctx); err != nil {
				return errors.Wrap(err, "commit transaction")
			}

			// Report that the object was deleted.
			os.Stdout.WriteString("Deleted object \"" + key + "\".\n")
			return nil
		},
	}
}
