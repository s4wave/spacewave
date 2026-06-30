package plugin_host_scheduler

import (
	"context"
	"strconv"

	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	"github.com/s4wave/spacewave/db/bucket"
	trace "github.com/s4wave/spacewave/db/traceutil"
)

func (t *pluginInstance) logPluginAccountingFields(ctx context.Context) {
	trace.Log(ctx, "plugin-id", t.pluginID)
	trace.Log(ctx, "instance-key", t.instanceKey)
}

func logManifestSnapshotAccountingFields(ctx context.Context, prefix string, manifest *bldr_manifest.ManifestSnapshot) {
	if manifest == nil {
		trace.Log(ctx, prefix+"-manifest-ref", "none")
		return
	}
	logObjectRefAccountingFields(ctx, prefix+"-manifest", manifest.GetManifestRef())
	if m := manifest.GetManifest(); m != nil {
		meta := m.GetMeta()
		logManifestMetaAccountingFields(ctx, prefix, meta)
	}
}

func logManifestRefAccountingFields(ctx context.Context, prefix string, ref *bldr_manifest.ManifestRef) {
	if ref == nil {
		trace.Log(ctx, prefix+"-manifest-ref", "none")
		return
	}
	logObjectRefAccountingFields(ctx, prefix+"-manifest", ref.GetManifestRef())
	logManifestMetaAccountingFields(ctx, prefix, ref.GetMeta())
}

func logObjectRefAccountingFields(ctx context.Context, prefix string, ref *bucket.ObjectRef) {
	if ref == nil {
		trace.Log(ctx, prefix+"-ref", "none")
		return
	}
	trace.Log(ctx, prefix+"-ref", ref.MarshalString())
	if rootRef := ref.GetRootRef(); rootRef != nil {
		trace.Log(ctx, prefix+"-root-ref", rootRef.MarshalString())
	}
	trace.Log(ctx, prefix+"-bucket-id", ref.GetBucketId())
}

func logManifestMetaAccountingFields(ctx context.Context, prefix string, meta *bldr_manifest.ManifestMeta) {
	if meta == nil {
		return
	}
	trace.Log(ctx, prefix+"-manifest-id", meta.GetManifestId())
	trace.Log(ctx, prefix+"-platform-id", meta.GetPlatformId())
	trace.Log(ctx, prefix+"-rev", strconv.FormatUint(meta.GetRev(), 10))
}
