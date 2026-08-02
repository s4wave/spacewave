package space_migration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_canvas_world "github.com/s4wave/spacewave/sdk/canvas/world"
)

// PlannerInput describes the source and destination Worlds and read-only
// preview inputs.
type PlannerInput struct {
	SourceSpaceID       string
	DestinationSpaceID  string
	Source              world.WorldState
	Destination         world.WorldState
	Operation           MigrationOperation
	SelectedObjectKeys  []string
	DestinationCapacity uint64
	CapacityKnown       bool
	// CollisionResolutions carries typed choices keyed by source object key.
	CollisionResolutions map[string]MigrationConflictResolution
}

// Planner builds immutable, read-only migration previews.
type Planner struct {
	registry *Registry
}

// NewPlanner constructs a planner using the supplied migration registry.
func NewPlanner(registry *Registry) *Planner {
	return &Planner{registry: registry}
}

// Plan scans both Worlds and returns a deterministic preview.
func (p *Planner) Plan(ctx context.Context, input *PlannerInput) (*MigrationPreview, error) {
	if p == nil || p.registry == nil {
		return nil, errors.New("migration registry is required")
	}
	if input == nil || input.Source == nil || input.Destination == nil {
		return nil, errors.New("source and destination Worlds are required")
	}
	if input.SourceSpaceID == "" || input.DestinationSpaceID == "" {
		return nil, errors.New("source and destination Space IDs are required")
	}
	if input.SourceSpaceID == input.DestinationSpaceID {
		return nil, errors.New("source and destination Space IDs must differ")
	}
	operation := normalizeOperation(input.Operation)
	if !supportedOperation(operation) {
		return nil, errors.Wrapf(ErrUnknownOperation, "%d", input.Operation)
	}

	sourceRevision, err := input.Source.GetSeqno(ctx)
	if err != nil {
		return nil, err
	}
	destinationRevision, err := input.Destination.GetSeqno(ctx)
	if err != nil {
		return nil, err
	}
	source, err := scanWorld(ctx, input.Source)
	if err != nil {
		return nil, err
	}
	destination, err := scanWorld(ctx, input.Destination)
	if err != nil {
		return nil, err
	}
	sourceBlockStoreID, err := owningBlockStoreID(ctx, input.Source, source, "source")
	if err != nil {
		return nil, err
	}
	destinationBlockStoreID, err := owningBlockStoreID(ctx, input.Destination, destination, "destination")
	if err != nil {
		return nil, err
	}
	objects := make(map[string]*ObjectDescriptor, len(source))
	for _, object := range source {
		objects[object.ObjectKey] = object
	}
	selected := selectClosure(input.SelectedObjectKeys, source)
	blockers := make([]*MigrationBlocker, 0)
	conflicts := make([]*MigrationConflict, 0)
	closure := make(map[string]struct{}, len(selected))
	queue := append([]string(nil), selected...)
	for len(queue) > 0 {
		key := queue[0]
		queue = queue[1:]
		if _, exists := closure[key]; exists {
			continue
		}
		object := objects[key]
		if object == nil {
			blockers = append(blockers, &MigrationBlocker{
				Code:       "missing-dependency",
				ObjectKeys: []string{key},
				Detail:     "dependency is not present in the source World",
			})
			continue
		}
		closure[key] = struct{}{}
		handler := p.registry.Lookup(object.ObjectType)
		if handler == nil || handler.Classification() == ClassificationUnclassified {
			detail := "ObjectType has no migration classification"
			if handler != nil {
				detail = "ObjectType is explicitly unclassified"
			}
			blockers = append(blockers, &MigrationBlocker{
				Code:       "unclassified-type",
				ObjectType: object.ObjectType,
				ObjectKeys: []string{object.ObjectKey},
				Detail:     detail,
			})
			continue
		}
		if handler.Classification() == ClassificationNonMigratable {
			blockers = append(blockers, &MigrationBlocker{
				Code:       "non-migratable-type",
				ObjectType: object.ObjectType,
				ObjectKeys: []string{object.ObjectKey},
				Detail:     "ObjectType is not migratable",
			})
			continue
		}
		inspection, err := handler.Inspect(ctx, object)
		if err != nil {
			code := "invalid-references"
			if errors.Is(err, ErrPayloadSchemaRefused) {
				code = "payload-schema-refused"
			}
			blockers = append(blockers, &MigrationBlocker{
				Code:       code,
				ObjectType: object.ObjectType,
				ObjectKeys: []string{object.ObjectKey},
				Detail:     err.Error(),
			})
			continue
		}
		object.Dependencies = append(object.Dependencies, inspection.Dependencies...)
		slices.Sort(object.Dependencies)
		object.Dependencies = slices.Compact(object.Dependencies)
		object.References = append([]TypedReference(nil), inspection.References...)
		object.ExternalReferences = append([]string(nil), inspection.ExternalReferences...)
		for _, dependency := range object.Dependencies {
			if dependency != "" {
				queue = append(queue, dependency)
			}
		}
		for _, external := range inspection.ExternalReferences {
			conflicts = append(conflicts, &MigrationConflict{
				Kind:       MigrationConflictKind_MIGRATION_CONFLICT_KIND_EXTERNAL_REFERENCE,
				ObjectKey:  object.ObjectKey,
				ObjectType: object.ObjectType,
				Detail:     external,
			})
		}
	}

	closureKeys := make([]string, 0, len(closure))
	for key := range closure {
		closureKeys = append(closureKeys, key)
	}
	slices.Sort(closureKeys)
	mapping := NewIdentityMap()
	mapping.SpaceIDs[input.SourceSpaceID] = input.DestinationSpaceID
	mapping.BlockStoreIDs[sourceBlockStoreID] = destinationBlockStoreID
	canvasCombineKeys := make(map[string]bool)
	for _, key := range closureKeys {
		object := objects[key]
		destinationObject := destination[key]
		if operation == MigrationOperation_MIGRATION_OPERATION_MERGE &&
			object.ObjectType == s4wave_canvas_world.CanvasTypeID &&
			destinationObject != nil &&
			destinationObject.ObjectType == s4wave_canvas_world.CanvasTypeID &&
			input.CollisionResolutions[key] == MigrationConflictResolution_MIGRATION_CONFLICT_RESOLUTION_COMBINE {
			canvasCombineKeys[key] = true
		}
	}
	for _, key := range closureKeys {
		mapping.ObjectKeys[key] = key
		object := objects[key]
		for _, reference := range object.References {
			switch reference.Kind {
			case ReferenceNestedSharedObject:
				if reference.Value != "" {
					mapping.NestedSharedObjects[reference.Value] = DeterministicIdentity(input.DestinationSpaceID, "nested-shared-object", reference.Value)
				}
			case ReferenceCanvasNode:
				if canvasCombineKeys[key] && reference.Value != "" {
					mapping.CanvasNodes[reference.Value] = DeterministicIdentity(input.DestinationSpaceID, "canvas-node", reference.Value)
				}
			case ReferenceBlockStore:
				if reference.Value != sourceBlockStoreID {
					blockers = append(blockers, &MigrationBlocker{
						Code:       "block-store-mismatch",
						ObjectType: object.ObjectType,
						ObjectKeys: []string{key},
						Detail:     "typed block-store reference does not belong to the source World owner",
					})
					continue
				}
				mapping.BlockStoreIDs[reference.Value] = destinationBlockStoreID
			}
		}
	}

	for _, key := range closureKeys {
		object := objects[key]
		destinationObject := destination[key]
		resolution := input.CollisionResolutions[key]
		canvasCollision := operation == MigrationOperation_MIGRATION_OPERATION_MERGE &&
			object.ObjectType == s4wave_canvas_world.CanvasTypeID &&
			destinationObject != nil &&
			destinationObject.ObjectType == s4wave_canvas_world.CanvasTypeID
		if destinationObject != nil {
			if destinationObject.ObjectType == object.ObjectType &&
				destinationObject.RootDigest == object.RootDigest &&
				resolution != MigrationConflictResolution_MIGRATION_CONFLICT_RESOLUTION_COMBINE {
				conflicts = append(conflicts, &MigrationConflict{
					Kind:       MigrationConflictKind_MIGRATION_CONFLICT_KIND_OBJECT_KEY,
					ObjectKey:  key,
					ObjectType: object.ObjectType,
					Detail:     "destination object is semantically identical; default is deduplicate",
					Resolution: MigrationConflictResolution_MIGRATION_CONFLICT_RESOLUTION_DEDUPLICATE,
				})
				continue
			}
			suggested := collisionSuggestion(key, input.SourceSpaceID)
			conflicts = append(conflicts, &MigrationConflict{
				Kind:               MigrationConflictKind_MIGRATION_CONFLICT_KIND_OBJECT_KEY,
				ObjectKey:          key,
				ObjectType:         object.ObjectType,
				SuggestedKey:       suggested,
				Detail:             "destination keeps its object; an explicit rename/replace choice is required",
				Resolution:         resolution,
				ResolutionRequired: resolution == MigrationConflictResolution_MIGRATION_CONFLICT_RESOLUTION_UNSPECIFIED && operation != MigrationOperation_MIGRATION_OPERATION_COPY && operation != MigrationOperation_MIGRATION_OPERATION_SPLIT,
			})
			if operation == MigrationOperation_MIGRATION_OPERATION_COPY || operation == MigrationOperation_MIGRATION_OPERATION_SPLIT {
				blockers = append(blockers, &MigrationBlocker{Code: "object-key-collision", ObjectType: object.ObjectType, ObjectKeys: []string{key}, Detail: "Copy preserves Space-local object keys and cannot overwrite destination state"})
				continue
			}
			switch resolution {
			case MigrationConflictResolution_MIGRATION_CONFLICT_RESOLUTION_RENAME:
				mapping.ObjectKeys[key] = suggested
			case MigrationConflictResolution_MIGRATION_CONFLICT_RESOLUTION_REPLACE,
				MigrationConflictResolution_MIGRATION_CONFLICT_RESOLUTION_KEEP_DESTINATION,
				MigrationConflictResolution_MIGRATION_CONFLICT_RESOLUTION_COMBINE:
				if resolution == MigrationConflictResolution_MIGRATION_CONFLICT_RESOLUTION_COMBINE && !canvasCollision {
					blockers = append(blockers, &MigrationBlocker{Code: "invalid-collision-resolution", ObjectType: object.ObjectType, ObjectKeys: []string{key}, Detail: "combine is only valid for a same-key Canvas merge collision"})
				}
			case MigrationConflictResolution_MIGRATION_CONFLICT_RESOLUTION_UNSPECIFIED:
				blockers = append(blockers, &MigrationBlocker{Code: "collision-resolution-required", ObjectType: object.ObjectType, ObjectKeys: []string{key}, Detail: "destination collision requires an explicit typed resolution"})
			default:
				blockers = append(blockers, &MigrationBlocker{Code: "invalid-collision-resolution", ObjectType: object.ObjectType, ObjectKeys: []string{key}, Detail: "collision resolution is not recognized"})
			}
		} else if resolution == MigrationConflictResolution_MIGRATION_CONFLICT_RESOLUTION_COMBINE {
			blockers = append(blockers, &MigrationBlocker{Code: "invalid-collision-resolution", ObjectType: object.ObjectType, ObjectKeys: []string{key}, Detail: "combine is only valid for a same-key Canvas merge collision"})
		}
		if canvasCollision {
			conflicts = append(conflicts, &MigrationConflict{
				Kind:               MigrationConflictKind_MIGRATION_CONFLICT_KIND_OBJECT_KEY,
				ObjectKey:          key,
				ObjectType:         object.ObjectType,
				Detail:             "Canvas node identities may be combined or preserved by the typed object-key resolution",
				Resolution:         resolution,
				ResolutionRequired: resolution == MigrationConflictResolution_MIGRATION_CONFLICT_RESOLUTION_UNSPECIFIED,
			})
		}
	}

	for _, key := range closureKeys {
		if mapping.ObjectKeys[key] != key {
			continue
		}
		var bestSource, bestDestination string
		for _, parent := range closureKeys {
			destinationKey := mapping.ObjectKeys[parent]
			if destinationKey == parent || !strings.HasPrefix(key, parent+"/") || len(parent) <= len(bestSource) {
				continue
			}
			bestSource, bestDestination = parent, destinationKey
		}
		if bestSource != "" {
			mapping.ObjectKeys[key] = bestDestination + key[len(bestSource):]
		}
	}
	seenDestinationKeys := make(map[string]string, len(closureKeys))
	for _, key := range closureKeys {
		destinationKey := mapping.ObjectKeys[key]
		if previous, exists := seenDestinationKeys[destinationKey]; exists && previous != key {
			blockers = append(blockers, &MigrationBlocker{
				Code:       "object-key-collision",
				ObjectKeys: []string{previous, key},
				Detail:     "two source objects map to the same destination key",
			})
		}
		seenDestinationKeys[destinationKey] = key
		if destination[destinationKey] != nil && destinationKey != key {
			blockers = append(blockers, &MigrationBlocker{
				Code:       "object-key-collision",
				ObjectKeys: []string{key},
				Detail:     "mapped destination key is already occupied",
			})
		}
	}
	for source, destinationKey := range mapping.ObjectKeys {
		mapping.GraphIRIs[source] = destinationKey
		mapping.GraphIRIs["<"+source+">"] = "<" + destinationKey + ">"
	}

	identityMappings := make([]*MigrationIdentityMapping, 0)
	for _, key := range closureKeys {
		identityMappings = append(identityMappings, &MigrationIdentityMapping{
			Kind:        MigrationReferenceKind_MIGRATION_REFERENCE_KIND_OBJECT_KEY,
			Source:      key,
			Destination: mapping.ObjectKeys[key],
		})
	}
	appendIdentityMappings(&identityMappings, MigrationReferenceKind_MIGRATION_REFERENCE_KIND_BLOCK_STORE, mapping.BlockStoreIDs)
	appendIdentityMappings(&identityMappings, MigrationReferenceKind_MIGRATION_REFERENCE_KIND_NESTED_SHARED_OBJECT, mapping.NestedSharedObjects)
	appendIdentityMappings(&identityMappings, MigrationReferenceKind_MIGRATION_REFERENCE_KIND_CANVAS_NODE, mapping.CanvasNodes)
	appendIdentityMappings(&identityMappings, MigrationReferenceKind_MIGRATION_REFERENCE_KIND_GRAPH_IRI, mapping.GraphIRIs)
	slices.SortStableFunc(identityMappings, compareIdentityMappings)

	planObjects := make([]*MigrationObject, 0, len(closureKeys))
	for _, key := range closureKeys {
		object := objects[key]
		planObjects = append(planObjects, &MigrationObject{
			ObjectKey:    object.ObjectKey,
			ObjectType:   object.ObjectType,
			Revision:     object.Revision,
			RootDigest:   object.RootDigest,
			LogicalBytes: object.LogicalBytes,
		})
	}
	for _, key := range closureKeys {
		object := objects[key]
		handler := p.registry.Lookup(object.ObjectType)
		if handler == nil || handler.Classification() == ClassificationUnclassified || handler.Classification() == ClassificationNonMigratable {
			continue
		}
		if _, err := handler.Rewrite(ctx, object, mapping); err != nil {
			blockers = append(blockers, &MigrationBlocker{
				Code:       "rewrite-failed",
				ObjectType: object.ObjectType,
				ObjectKeys: []string{object.ObjectKey},
				Detail:     err.Error(),
			})
		}
	}
	logicalBytes := uint64(0)
	logicalOverflow := false
	for _, key := range closureKeys {
		object := objects[key]
		if !object.LogicalBytesKnown {
			blockers = append(blockers, &MigrationBlocker{Code: "logical-size-unavailable", ObjectKeys: []string{key}, Detail: "the immutable block-store DAG size owner could not provide a logical byte total"})
			continue
		}
		size := object.LogicalBytes
		if ^uint64(0)-logicalBytes < size {
			logicalOverflow = true
			break
		}
		logicalBytes += size
	}
	if logicalOverflow {
		blockers = append(blockers, &MigrationBlocker{Code: "logical-size-overflow", Detail: "estimated logical bytes exceed the supported capacity range"})
	}
	if input.CapacityKnown && input.DestinationCapacity < logicalBytes {
		blockers = append(blockers, &MigrationBlocker{Code: "destination-capacity", Detail: "available destination capacity is below the estimated logical bytes"})
	}
	slices.SortStableFunc(blockers, compareBlockers)
	slices.SortStableFunc(conflicts, compareConflicts)
	result := &MigrationTerminalResult{State: MigrationTerminalState_MIGRATION_TERMINAL_STATE_PREVIEW_READY}
	if len(blockers) > 0 {
		result.State = MigrationTerminalState_MIGRATION_TERMINAL_STATE_BLOCKED
		result.Code = "preview-blocked"
		result.Detail = blockerDetail(blockers[0])
	}
	preview := &MigrationPreview{
		Operation:           operation,
		SourceSpaceId:       input.SourceSpaceID,
		DestinationSpaceId:  input.DestinationSpaceID,
		SourceRevision:      sourceRevision,
		DestinationRevision: destinationRevision,
		Objects:             planObjects,
		IdentityMappings:    identityMappings,
		Conflicts:           conflicts,
		Blockers:            blockers,
		Progress:            &MigrationProgress{ObjectsSeen: uint64(len(source)), ObjectsPlanned: uint64(len(closureKeys)), LogicalBytes: logicalBytes},
		Result:              result,
		DestinationCapacity: input.DestinationCapacity,
		CapacityKnown:       input.CapacityKnown,
	}
	preview.Digest = digestPreview(preview)
	if len(blockers) > 0 {
		if input.CapacityKnown && input.DestinationCapacity < logicalBytes {
			return preview, errors.Wrap(ErrCapacityInsufficient, result.Detail)
		}
		return preview, errors.Wrap(ErrPlanBlocked, result.Detail)
	}
	return preview, nil
}

// VerifyFresh rejects a preview if either World changed after planning.
func (p *Planner) VerifyFresh(ctx context.Context, input *PlannerInput, preview *MigrationPreview) error {
	if input == nil || input.Source == nil || input.Destination == nil || preview == nil {
		return errors.New("source, destination, and preview are required")
	}
	sourceRevision, err := input.Source.GetSeqno(ctx)
	if err != nil {
		return err
	}
	destinationRevision, err := input.Destination.GetSeqno(ctx)
	if err != nil {
		return err
	}
	if input.SourceSpaceID != preview.SourceSpaceId || input.DestinationSpaceID != preview.DestinationSpaceId {
		return errors.Wrap(ErrStalePlan, "preview Space identities do not match the current request")
	}
	if normalizeOperation(input.Operation) != preview.Operation {
		return errors.Wrap(ErrStalePlan, "preview operation does not match the current request")
	}
	if input.CapacityKnown != preview.CapacityKnown || input.DestinationCapacity != preview.DestinationCapacity {
		return errors.Wrap(ErrStalePlan, "preview capacity preflight does not match the current request")
	}
	sourceObjects, err := scanWorld(ctx, input.Source)
	if err != nil {
		return err
	}
	destinationObjects, err := scanWorld(ctx, input.Destination)
	if err != nil {
		return err
	}
	sourceStore, err := owningBlockStoreID(ctx, input.Source, sourceObjects, "source")
	if err != nil {
		return errors.Wrap(ErrStalePlan, err.Error())
	}
	destinationStore, err := owningBlockStoreID(ctx, input.Destination, destinationObjects, "destination")
	if err != nil {
		return errors.Wrap(ErrStalePlan, err.Error())
	}
	foundStoreMapping := false
	for _, identity := range preview.IdentityMappings {
		if identity != nil && identity.Kind == MigrationReferenceKind_MIGRATION_REFERENCE_KIND_BLOCK_STORE && identity.Source == sourceStore {
			foundStoreMapping = identity.Destination == destinationStore
			break
		}
	}
	if !foundStoreMapping {
		return errors.Wrap(ErrStalePlan, "block-store identities do not match the preview")
	}
	if sourceRevision != preview.SourceRevision || destinationRevision != preview.DestinationRevision {
		return errors.Wrapf(ErrStalePlan, "source revision %d/%d destination revision %d/%d", sourceRevision, preview.SourceRevision, destinationRevision, preview.DestinationRevision)
	}
	if preview.Digest != digestPreview(preview) {
		return errors.Wrap(ErrStalePlan, "preview digest does not match its contents")
	}
	return nil
}

func scanWorld(ctx context.Context, ws world.WorldState) (map[string]*ObjectDescriptor, error) {
	objects := make(map[string]*ObjectDescriptor)
	iterator := ws.IterateObjects(ctx, "", false)
	defer iterator.Close()
	for iterator.Next() {
		key := iterator.Key()
		if strings.HasPrefix(key, world_types.TypesPrefix) {
			continue
		}
		typeID, err := world_types.GetObjectType(ctx, ws, key)
		if err != nil {
			return nil, err
		}
		objectState, found, err := ws.GetObject(ctx, key)
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		root, revision, err := objectState.GetRootRef(ctx)
		world.ReleaseObjectState(objectState)
		if err != nil {
			return nil, err
		}
		rootDigest := ""
		blockStoreID := ""
		if root != nil {
			rootDigest = root.MarshalString()
			blockStoreID = root.GetBucketId()
		}
		logicalBytes, logicalBytesKnown := logicalSizeForRef(ctx, ws, root)
		objects[key] = &ObjectDescriptor{
			ObjectKey: key, ObjectType: typeID, Revision: revision,
			RootDigest: rootDigest, LogicalBytes: logicalBytes,
			LogicalBytesKnown: logicalBytesKnown, World: ws, BlockStoreID: blockStoreID,
		}
	}
	if err := iterator.Err(); err != nil {
		return nil, err
	}
	known := make(map[string]struct{}, len(objects))
	for key := range objects {
		known[key] = struct{}{}
	}
	for key, object := range objects {
		quads, err := ws.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys(key, "", "", ""), 0)
		if err != nil {
			return nil, errors.Wrapf(err, "scan graph for %s", key)
		}
		for _, quad := range quads {
			if quad == nil {
				continue
			}
			values := []string{quad.GetSubject(), quad.GetPredicate(), quad.GetObj(), quad.GetLabel()}
			object.GraphReferences = append(object.GraphReferences, values...)
			for _, value := range []string{quad.GetSubject(), quad.GetObj(), quad.GetLabel()} {
				objKey := strings.Trim(value, "<>")
				if _, exists := known[objKey]; exists {
					object.Dependencies = append(object.Dependencies, objKey)
				}
			}
		}
	}
	return objects, nil
}

func logicalSizeForRef(ctx context.Context, ws world.WorldState, root *bucket.ObjectRef) (uint64, bool) {
	if root == nil || root.GetRootRef() == nil || root.GetRootRef().GetEmpty() {
		return 0, false
	}
	var total uint64
	err := ws.AccessWorldState(ctx, root, func(cursor *bucket_lookup.Cursor) error {
		return bucket_lookup.WalkObjectBlocks(ctx, bucket_lookup.NewWalkObjectBlocksWithRef(root.GetRootRef(), nil), func(entry *bucket_lookup.WalkObjectBlocksEntry) (bool, error) {
			if entry == nil {
				return true, nil
			}
			if entry.Err != nil {
				return false, entry.Err
			}
			if !entry.Found || entry.IsSubBlock {
				return true, nil
			}
			dataLen := uint64(len(entry.Data))
			if dataLen == 0 {
				dataLen = uint64(len(entry.XfrmData))
			}
			if ^uint64(0)-total < dataLen {
				return false, errors.New("logical DAG size overflow")
			}
			total += dataLen
			return true, nil
		}, cursor.GetBucket(), cursor.GetTransformer(), 0, false)
	})
	return total, err == nil
}
func owningBlockStoreID(ctx context.Context, ws world.WorldState, objects map[string]*ObjectDescriptor, label string) (string, error) {
	var ownerID string
	if err := ws.AccessWorldState(ctx, nil, func(cursor *bucket_lookup.Cursor) error {
		if cursor != nil && cursor.GetRef() != nil {
			ownerID = cursor.GetRef().GetBucketId()
		}
		return nil
	}); err != nil {
		return "", errors.Wrapf(err, "read %s World block-store identity", label)
	}
	if ownerID == "" {
		return "", errors.Errorf("%s World has no block-store identity", label)
	}
	for key, object := range objects {
		if object == nil || object.BlockStoreID == "" {
			continue
		}
		if object.BlockStoreID != ownerID {
			return "", errors.Errorf("%s World object %s belongs to block store %s, not owning store %s", label, key, object.BlockStoreID, ownerID)
		}
	}
	return ownerID, nil
}

func selectClosure(selected []string, source map[string]*ObjectDescriptor) []string {
	if len(selected) == 0 {
		keys := make([]string, 0, len(source))
		for key := range source {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		return keys
	}
	keys := append([]string(nil), selected...)
	slices.Sort(keys)
	return slices.Compact(keys)
}

func collisionSuggestion(key, sourceSpaceID string) string {
	short := sourceSpaceID
	if len(short) > 8 {
		short = short[len(short)-8:]
	}
	idx := strings.LastIndex(key, "/")
	if idx < 0 {
		return key + "~" + short
	}
	return path.Join(key[:idx], key[idx+1:]+"~"+short)
}

func appendIdentityMappings(dst *[]*MigrationIdentityMapping, kind MigrationReferenceKind, values map[string]string) {
	keys := make([]string, 0, len(values))
	for source := range values {
		keys = append(keys, source)
	}
	slices.Sort(keys)
	for _, source := range keys {
		*dst = append(*dst, &MigrationIdentityMapping{Kind: kind, Source: source, Destination: values[source]})
	}
}

func compareIdentityMappings(a, b *MigrationIdentityMapping) int {
	if a.Kind != b.Kind {
		if a.Kind < b.Kind {
			return -1
		}
		return 1
	}
	return strings.Compare(a.Source, b.Source)
}

func compareBlockers(a, b *MigrationBlocker) int {
	if value := strings.Compare(a.Code, b.Code); value != 0 {
		return value
	}
	if value := strings.Compare(a.ObjectType, b.ObjectType); value != 0 {
		return value
	}
	return strings.Compare(strings.Join(a.ObjectKeys, "\x00"), strings.Join(b.ObjectKeys, "\x00"))
}

func compareConflicts(a, b *MigrationConflict) int {
	if value := strings.Compare(a.ObjectKey, b.ObjectKey); value != 0 {
		return value
	}
	if a.Kind < b.Kind {
		return -1
	}
	if a.Kind > b.Kind {
		return 1
	}
	return strings.Compare(a.Detail, b.Detail)
}

func blockerDetail(blocker *MigrationBlocker) string {
	if blocker == nil {
		return "preview is blocked"
	}
	if blocker.ObjectType == "" {
		return blocker.Detail
	}
	return blocker.ObjectType + ": " + blocker.Detail
}

func digestPreview(preview *MigrationPreview) string {
	if preview == nil {
		return ""
	}
	var b bytes.Buffer
	write := func(values ...string) {
		b.WriteByte(0)
		b.WriteString(strings.Join(values, "\x00"))
	}
	write(strconv.Itoa(int(preview.Operation)), preview.SourceSpaceId, preview.DestinationSpaceId, strconv.FormatUint(preview.SourceRevision, 10), strconv.FormatUint(preview.DestinationRevision, 10))
	for _, object := range preview.Objects {
		if object == nil {
			write("object:nil")
			continue
		}
		write("object", object.ObjectKey, object.ObjectType, strconv.FormatUint(object.Revision, 10), object.RootDigest, strconv.FormatUint(object.LogicalBytes, 10))
	}
	for _, mapping := range preview.IdentityMappings {
		if mapping == nil {
			write("mapping:nil")
			continue
		}
		write("mapping", strconv.Itoa(int(mapping.Kind)), mapping.Source, mapping.Destination)
	}
	for _, conflict := range preview.Conflicts {
		if conflict == nil {
			write("conflict:nil")
			continue
		}
		write("conflict", strconv.Itoa(int(conflict.Kind)), conflict.ObjectKey, conflict.ObjectType, conflict.SuggestedKey, conflict.Detail, strconv.Itoa(int(conflict.Resolution)), strconv.FormatBool(conflict.ResolutionRequired))
	}
	for _, blocker := range preview.Blockers {
		if blocker == nil {
			write("blocker:nil")
			continue
		}
		write("blocker", blocker.Code, blocker.ObjectType, strings.Join(blocker.ObjectKeys, "\x00"), blocker.Detail)
	}
	if preview.Progress == nil {
		write("progress:nil")
	} else {
		write("progress",
			strconv.FormatUint(preview.Progress.ObjectsSeen, 10),
			strconv.FormatUint(preview.Progress.ObjectsPlanned, 10),
			strconv.FormatUint(preview.Progress.LogicalBytes, 10),
			strconv.FormatUint(preview.Progress.BlocksSeen, 10),
			strconv.FormatUint(preview.Progress.BlocksCopied, 10),
			strconv.FormatUint(preview.Progress.BlocksExisting, 10),
			strconv.FormatUint(preview.Progress.BlocksDeduplicated, 10),
			strconv.FormatUint(preview.Progress.BlocksWritten, 10),
			strconv.FormatUint(preview.Progress.SubtreesSkipped, 10),
			strconv.FormatUint(preview.Progress.DestinationBytesWritten, 10),
			strconv.FormatUint(preview.Progress.NestedSharedObjectsPlanned, 10),
			strconv.FormatUint(preview.Progress.NestedSharedObjectsCompleted, 10),
		)
	}
	if preview.Result == nil {
		write("result:nil")
	} else {
		write("result", strconv.Itoa(int(preview.Result.State)), preview.Result.Code, preview.Result.Detail)
	}
	write("capacity", strconv.FormatUint(preview.DestinationCapacity, 10), strconv.FormatBool(preview.CapacityKnown))
	hash := sha256.Sum256(b.Bytes())
	return hex.EncodeToString(hash[:])
}

func supportedOperation(operation MigrationOperation) bool {
	switch operation {
	case MigrationOperation_MIGRATION_OPERATION_COPY,
		MigrationOperation_MIGRATION_OPERATION_MERGE,
		MigrationOperation_MIGRATION_OPERATION_COPY_OBJECTS,
		MigrationOperation_MIGRATION_OPERATION_MOVE_OBJECTS,
		MigrationOperation_MIGRATION_OPERATION_SPLIT:
		return true
	default:
		return false
	}
}

func normalizeOperation(operation MigrationOperation) MigrationOperation {
	if operation == MigrationOperation_MIGRATION_OPERATION_UNSPECIFIED {
		return MigrationOperation_MIGRATION_OPERATION_COPY
	}
	return operation
}
