package publish

import (
	"context"
	"io"
	"os"
	"strconv"

	"github.com/pkg/errors"
	spacewave_provider "github.com/s4wave/spacewave/core/provider/spacewave"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/packfile"
)

// Promote copies missing packs from the source Space to the destination Space
// and posts the destination checkpoint after all required packs are present.
func Promote(ctx context.Context, opts Options) error {
	// Read the source head and both pack manifests.
	srcHeadRef, err := FetchSourceHeadRef(ctx, opts.Client, opts.SrcSpaceID)
	if err != nil {
		return errors.Wrap(err, "fetch source head ref")
	}
	if srcHeadRef == nil {
		return errors.New("source space has no published World")
	}
	srcPacks, err := FetchPackEntries(ctx, opts.Client, opts.SrcSpaceID)
	if err != nil {
		return errors.Wrap(err, "fetch source pack manifest")
	}
	dstPacks, err := FetchPackEntries(ctx, opts.Client, opts.DstSpaceID)
	if err != nil {
		return errors.Wrap(err, "fetch destination pack manifest")
	}

	// Plan against the destination head; a matching destination needs nothing.
	dstHeadRef, err := FetchDestinationHeadRef(ctx, opts.CdnBaseURL, opts.DstSpaceID)
	if err != nil {
		return errors.Wrap(err, "fetch destination head ref")
	}
	plan := BuildPublishPlan(srcPacks, dstPacks, srcHeadRef, dstHeadRef)
	if len(plan.MissingPackIDs) == 0 && !plan.NeedCheckpoint {
		_, err := io.WriteString(opts.output(), "publish-space: no changes (destination already matches source)\n")
		return err
	}

	// Push each missing pack from the source.
	srcPackMap := make(map[string]*packfile.PackfileEntry, len(srcPacks))
	for _, entry := range srcPacks {
		srcPackMap[entry.GetId()] = entry
	}
	if _, err := io.WriteString(
		opts.output(),
		"publishing "+strconv.Itoa(len(plan.MissingPackIDs))+
			" missing pack(s) from "+opts.SrcSpaceID+
			" to "+opts.DstSpaceID+"\n",
	); err != nil {
		return err
	}
	for _, packID := range plan.MissingPackIDs {
		entry := srcPackMap[packID]
		packPath, err := FetchSourcePackToTempFile(ctx, opts, entry)
		if err != nil {
			return errors.Wrapf(err, "fetch source pack %s", entry.GetId())
		}
		pushErr := PushSinglePack(ctx, opts, packPath, entry.GetBloomFilter())
		rmErr := os.Remove(packPath)
		if pushErr != nil {
			return errors.Wrapf(pushErr, "push destination pack %s", entry.GetId())
		}
		if rmErr != nil {
			return errors.Wrapf(rmErr, "remove temp pack %s", packPath)
		}
	}

	// Post the checkpoint when the World differs.
	if !plan.NeedCheckpoint {
		_, err := io.WriteString(opts.output(), "destination World already matches source; skipped checkpoint post\n")
		return err
	}
	checkpoint, err := PostCheckpoint(ctx, opts, srcHeadRef)
	if err != nil {
		return err
	}
	_, err = io.WriteString(
		opts.output(),
		"published checkpoint height="+strconv.FormatUint(checkpoint.GetHeight(), 10)+
			" to destination="+opts.DstSpaceID+"\n",
	)
	return err
}

// FetchSourceHeadRef reads the source SO snapshot and decodes its World head ref.
func FetchSourceHeadRef(ctx context.Context, client SessionClient, spaceID string) (*bucket.ObjectRef, error) {
	// Read the source Space state through its session client.
	stateData, err := client.GetSOState(ctx, spaceID, 0, spacewave_provider.SeedReasonColdSeed)
	if err != nil {
		return nil, errors.Wrap(err, "get source state")
	}

	// Decode the source state message before inspecting its content.
	state := &api.SOStateMessage{}
	if err := state.UnmarshalVT(stateData); err != nil {
		return nil, errors.Wrap(err, "unmarshal source state")
	}

	// Require the source snapshot before extracting its World head.
	snapshot := state.GetSnapshot()
	if snapshot == nil {
		return nil, errors.Errorf("expected source state snapshot, got %s", getSOStateMessageContentCase(state))
	}
	return DecodeHeadRef(snapshot)
}

func getSOStateMessageContentCase(msg *api.SOStateMessage) string {
	switch msg.GetContent().(type) {
	case *api.SOStateMessage_Snapshot:
		return "snapshot"
	case *api.SOStateMessage_Delta:
		return "delta"
	case *api.SOStateMessage_Error:
		return "error"
	case *api.SOStateMessage_ConfigChanged:
		return "config_changed"
	default:
		return "unknown"
	}
}
