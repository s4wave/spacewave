//go:build !js && !wasip1

package world_recovery

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"strings"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	"github.com/s4wave/spacewave/db/block/byteslice"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_all "github.com/s4wave/spacewave/db/block/transform/all"
	"github.com/s4wave/spacewave/db/bucket"
	iavl "github.com/s4wave/spacewave/db/kvtx/block/iavl"
	okra "github.com/s4wave/spacewave/db/kvtx/block/okra"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/sirupsen/logrus"
)

// construct resolves persisted constructor identities without retaining a corpus map.
func (s *Scanner) construct(ctx context.Context, name string) (block.Block, error) {
	// Resolve native storage constructors independently of application handlers.
	switch strings.Split(name, ":")[0] {
	case "world":
		return &world_block.World{}, nil
	case "iavl":
		return &iavl.Node{}, nil
	case "okra":
		return &okra.Page{}, nil
	case "object":
		return &world_block.Object{}, nil
	case "change":
		return &world_block.ChangeLogLL{}, nil
	case "batch":
		return &world_block.WorldChangeLL{}, nil
	case "blob":
		return &blob.Blob{}, nil
	case "bytes":
		return byteslice.NewByteSliceBlock(), nil
	}

	// Resolve an explicitly typed application body without loading its handler.
	if strings.HasPrefix(name, "type:") && s.opts.Lookup != nil {
		typ, err := s.opts.Lookup(ctx, strings.TrimPrefix(name, "type:"))
		if err != nil {
			return nil, err
		}
		if typ != nil {
			return typ.Constructor(), nil
		}
	}
	return nil, errors.New("unsupported block constructor")
}

// interpret commits one typed node and all bounded child work atomically.
func (s *Scanner) interpret(ctx context.Context) error {
	// Take one queue item outside a writable scratch transaction.
	key, data, err := nextRow(s.db, "queue", nil)
	if err != nil {
		return err
	}
	if key == nil {
		return s.attribute(ctx)
	}

	// Load one durable interpretation with its bounded source-read hook.
	node := &Node{}
	if err := node.UnmarshalVT(data); err != nil {
		return err
	}
	if s.opts.BeforeRead != nil {
		if err := s.opts.BeforeRead(); err != nil {
			return err
		}
	}
	raw, found, err := s.blocks.GetBlock(ctx, node.GetRef())
	if err != nil {
		return err
	}

	// Retain only this block and its bounded outgoing work.
	var decoded block.Block
	var kids []*Node
	var failure string

	// Decode only the constructor established by this parent path and context.
	if !found {
		failure = "missing block"
	} else if contextErr := validateContext(node.GetContext()); contextErr != nil {
		failure = contextErr.Error()
	} else {
		decoded, err = s.construct(ctx, node.GetConstructor())
		if err == nil {
			transformer, xerr := block_transform.NewTransformer(controller.ConstructOpts{Logger: logrus.NewEntry(logrus.New())}, transform_all.BuildFactorySet(), node.GetContext().GetTransformConf())
			if xerr != nil {
				err = xerr
			} else {
				raw, err = transformer.DecodeBlock(raw)
			}
		}
		if s.opts.BeforeDecode != nil {
			if err := s.opts.BeforeDecode(); err != nil {
				return err
			}
		}
		if err == nil && node.GetExpectedSize() != 0 && uint64(len(raw)) != node.GetExpectedSize() {
			err = errors.New("raw chunk size differs from its typed parent")
		}
		if err == nil {
			err = decoded.UnmarshalVT(raw)
		}
		if err == nil {
			if validator, ok := decoded.(interface{ Validate() error }); ok {
				err = validator.Validate()
			}
		}
		if err == nil {
			kids, err = s.children(ctx, node, decoded, "")
		}
		if err != nil {
			failure = err.Error()
		}
	}

	// Record uncertain interpretations without promoting a stored child list.
	return s.db.Update(func(tx *scratchTx) error {
		// Commit the typed node, bounded child work and interpretation status.
		if failure != "" {
			if err := tx.Bucket([]byte("bad")).Put(key, []byte(failure)); err != nil {
				return err
			}
			if err := tx.Bucket([]byte("badqueue")).Put(key, []byte{1}); err != nil {
				return err
			}
		} else {
			encoded, err := node.GetRef().MarshalKey()
			if err != nil {
				return err
			}
			if err := tx.Bucket([]byte("typed")).Put(encoded, []byte{1}); err != nil {
				return err
			}
			if _, ok := decoded.(*world_block.Object); ok && len(node.GetTreeKey()) != 0 {
				if err := tx.Bucket([]byte("bound")).Put(encoded, []byte{1}); err != nil {
					return err
				}
			}
		}

		// Commit bounded outgoing work with this node's completion.
		for _, kid := range kids {
			kid.Parent = key
			if err := putNode(tx, kid); err != nil {
				return err
			}
		}
		if obj, ok := decoded.(*world_block.Object); ok && failure == "" {
			identity := objectIdentity(obj, node.GetContext())
			if err := tx.Bucket([]byte("objectlinks")).Put(append(identity, key...), []byte{1}); err != nil {
				return err
			}
			if obj.GetRootRef().GetRootRef().GetEmpty() {
				if err := tx.Bucket([]byte("objectstatus")).Put(identity, []byte{1}); err != nil {
					return err
				}
			}
		}

		// Preserve native deletion facts beside the decoded interpretation.
		for _, name := range deletedKeys(decoded) {
			if err := tx.Bucket([]byte("deletefacts")).Put(append(bytes.Clone(key), name...), []byte(name)); err != nil {
				return err
			}
		}
		if err := tx.Bucket([]byte("done")).Put(key, []byte{1}); err != nil {
			return err
		}
		if err := tx.Bucket([]byte("queue")).Delete(key); err != nil {
			return err
		}
		return s.save(tx)
	})
}

// objectIdentity matches exact metadata across typed tree and changelog paths.
func objectIdentity(object *world_block.Object, parent *bucket.ObjectRef) []byte {
	// Match exact metadata after resolving inherited bucket and transform context.
	normalized := object.CloneVT()
	normalized.RootRef = resolveContext(parent, object.GetRootRef())
	data, _ := normalized.MarshalVT()
	key := sha256.Sum256(data)
	return key[:]
}

// children follows native reference and inline-subblock contracts, preserving tree keys.
func (s *Scanner) children(ctx context.Context, node *Node, value any, tree string) ([]*Node, error) {
	// Keep each decoded record's outgoing work bounded by its encoded size.
	var out []*Node
	add := func(ref *block.BlockRef, ctor string, key []byte, context *bucket.ObjectRef) {
		if !ref.GetEmpty() {
			out = append(out, &Node{Ref: ref, Constructor: ctor, World: node.GetWorld(), TreeKey: key, Context: context})
		}
	}

	// Carry only bucket and transform context through the native type graph.
	context := node.GetContext()
	if strings.HasSuffix(node.GetConstructor(), ":objects") {
		tree = "objects"
	}
	if strings.HasSuffix(node.GetConstructor(), ":graph") {
		tree = "graph"
	}

	// Apply native inline-block validation before following its children.
	if validator, ok := value.(interface{ Validate() error }); ok {
		if err := validator.Validate(); err != nil {
			return nil, err
		}
	}
	if chunks, ok := value.(*blob.ChunkIndex); ok {
		for _, chunk := range chunks.GetChunks() {
			if !chunk.GetDataRef().GetEmpty() {
				out = append(out, &Node{Ref: chunk.GetDataRef(), Constructor: "bytes", World: node.GetWorld(), Context: context, ExpectedSize: chunk.GetSize()})
			}
		}
		return out, nil
	}

	// Object membership comes only from a matching typed object-tree key.
	if obj, ok := value.(*world_block.Object); ok {
		if len(node.GetTreeKey()) != 0 {
			if string(node.GetTreeKey()) != "o/"+obj.GetKey() || obj.GetKey() == "" {
				return nil, errors.New("object metadata does not match its typed tree key")
			}
		}
		// Per-World membership attaches typed bodies after the shared tree walk.
		return out, nil
	}

	// Tree values retain the exact object key; graph values are byte leaves.
	if n, ok := value.(*iavl.Node); ok {
		suffix := ""
		if tree != "" {
			suffix = ":" + tree
		}
		add(n.GetLeftChildRef(), "iavl"+suffix, nil, context)
		add(n.GetRightChildRef(), "iavl"+suffix, nil, context)
		ctor := "bytes"
		if tree == "objects" {
			ctor = "object"
		}
		if !n.GetValueRef().GetEmpty() {
			add(n.GetValueRef(), ctor, n.GetKey(), context)
		}
		if n.GetValueBlob() != nil && tree == "objects" {
			return out, errors.New("inline IAVL object values are unsupported")
		}
		if n.GetValueBlob() != nil {
			kids, err := s.children(ctx, node, n.GetValueBlob(), "")
			return append(out, kids...), err
		}
		return out, nil
	}
	if page, ok := value.(*okra.Page); ok {
		suffix := ""
		if tree != "" {
			suffix = ":" + tree
		}
		for _, ent := range page.GetEntries() {
			add(ent.GetChildRef(), "okra"+suffix, nil, context)
			ctor := "bytes"
			if tree == "objects" {
				ctor = "object"
			}
			if ent.GetValueIsBlob() {
				ctor = "blob"
			}
			add(ent.GetValueRef(), ctor, ent.GetKey(), context)
			if ent.GetValueBlob() != nil {
				if tree == "objects" {
					return out, errors.New("inline Okra object values are unsupported")
				}
				kids, err := s.children(ctx, node, ent.GetValueBlob(), "")
				if err != nil {
					return nil, err
				}
				out = append(out, kids...)
			}
		}
		return out, nil
	}

	// Every other native block declares its references and their constructors.
	if refs, ok := value.(block.BlockWithRefs); ok {
		children, err := refs.GetBlockRefs()
		if err != nil {
			return nil, err
		}
		for id, ref := range children {
			ctor := "unknown"
			if build := refs.GetBlockRefCtor(id); build != nil {
				ctor = constructorName(build())
			}
			if tree != "" && (ctor == "iavl" || ctor == "okra") {
				ctor += ":" + tree
			}
			add(ref, ctor, nil, context)
		}
	}
	if subs, ok := value.(block.BlockWithSubBlocks); ok {
		for id, sub := range subs.GetSubBlocks() {
			if sub == nil || sub.IsNil() {
				continue
			}
			childTree := tree
			if _, ok := value.(*world_block.World); ok {
				childTree = ""
				if id == 1 {
					childTree = "objects"
				}
				if id == 2 {
					childTree = "graph"
				}
			}
			kids, err := s.children(ctx, node, sub, childTree)
			if err != nil {
				return nil, err
			}
			out = append(out, kids...)
		}
	}
	return out, nil
}

// constructorName assigns stable identities to supported native child constructors.
func constructorName(value block.Block) string {
	switch value.(type) {
	case *world_block.World:
		return "world"
	case *world_block.Object:
		return "object"
	case *world_block.ChangeLogLL:
		return "change"
	case *world_block.WorldChangeLL:
		return "batch"
	case *iavl.Node:
		return "iavl"
	case *okra.Page:
		return "okra"
	case *blob.Blob:
		return "blob"
	case *byteslice.ByteSlice:
		return "bytes"
	default:
		return "unknown"
	}
}

// resolveContext makes inherited bucket and transformation identity explicit.
func resolveContext(parent, child *bucket.ObjectRef) *bucket.ObjectRef {
	// Preserve an empty body while resolving its interpretation context.
	result := child.CloneVT()
	if result == nil {
		result = &bucket.ObjectRef{}
	}
	if result.GetBucketId() == "" {
		result.BucketId = parent.GetBucketId()
	}
	if result.GetTransformConf().GetEmpty() && result.GetTransformConfRef().GetEmpty() {
		result.TransformConf = parent.GetTransformConf().CloneVT()
		result.TransformConfRef = parent.GetTransformConfRef().CloneVT()
	}
	return result
}

// versionKey deduplicates exact normalized versions independently of provenance.
func versionKey(version *Version) []byte {
	data, _ := (&Version{SpaceId: version.GetSpaceId(), Object: version.GetObject()}).MarshalVT()
	key := sha256.Sum256(data)
	return key[:]
}

// putVersion stores version identity and per-World membership as separate relations.
func (s *Scanner) putVersion(tx *scratchTx, node *Node, version *Version) error {
	// Retain each exact version once and each World membership independently.
	key := versionKey(version)
	data, err := version.MarshalVT()
	if err != nil {
		return err
	}
	if err := tx.Bucket([]byte("versions")).Put(key, data); err != nil {
		return err
	}
	if err := tx.Bucket([]byte("worldobjects")).Put(worldKey(node.GetWorld(), version.GetObject().GetKey()), key); err != nil {
		return err
	}
	if s.source.GetWorlds()[node.GetWorld()].GetCurrent() {
		if err := tx.Bucket([]byte("current")).Put(append([]byte(version.GetSpaceId()+"\x00"), version.GetObject().GetKey()...), key); err != nil {
			return err
		}
	}
	return nil
}

// deletedKeys reads only typed native changelog entries within this bounded block.
func deletedKeys(value any) []string {
	// Stop at WorldChange metadata, which explicitly distinguishes intentional deletion.
	if change, ok := value.(*world_block.WorldChange); ok {
		if change.GetChangeType() == world_block.WorldChangeType_WorldChange_OBJECT_DELETE {
			return []string{change.GetKey()}
		}
		return nil
	}

	// Follow inline changelog sub-blocks without touching their stored child lists.
	var result []string
	if subs, ok := value.(block.BlockWithSubBlocks); ok {
		for _, sub := range subs.GetSubBlocks() {
			if sub != nil && !sub.IsNil() {
				result = append(result, deletedKeys(sub)...)
			}
		}
	}
	return result
}
