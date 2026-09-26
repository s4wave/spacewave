package spacewave

import "embed"

// DistSources contains the TypeScript source closure that downstream apps use
// to resolve Spacewave package imports without a sibling checkout.
//
//go:embed app/canvas/GraphLinkPill.tsx app/canvas/types.ts app/creator-visibility.ts
//go:embed app/device/add-device-wizard.ts app/quickstart/create.ts app/quickstart/drive-starter-guide.ts
//go:embed app/quickstart/options.ts
//go:embed app/quickstart/perf-test.ts app/quickstart/startup-boundary.ts app/space/create-op-builders.ts
//go:embed app/space/space-settings.ts app/space/space.ts app/urls.ts app/vm/v86-wizard-config.ts
//go:embed bldr/manifest/manifest.pb.ts
//go:embed app/wizard/intro.ts core/account/settings/settings.pb.ts core/changelog/changelog.pb.ts
//go:embed core/pairing/pairing.pb.ts
//go:embed core/forge/dashboard/dashboard.pb.ts core/forge/job/job.pb.ts core/forge/task/task.pb.ts
//go:embed core/git/git.pb.ts core/provider/provider.pb.ts core/provider/spacewave/api/api.pb.ts
//go:embed core/provider/spacewave/cacheseed/cacheseed.pb.ts
//go:embed core/provider/spacewave/cacheseed/cacheseed_srpc.pb.ts
//go:embed core/provider/spacewave/launcher/launcher.pb.ts
//go:embed core/provider/spacewave/launcher/launcher_srpc.pb.ts core/provider/transfer/transfer.pb.ts
//go:embed core/session/handoff/handoff.pb.ts core/session/session.pb.ts core/sobject/sobject.pb.ts
//go:embed core/space/space.pb.ts core/space/world/ops/init-canvas-demo.ts
//go:embed core/space/world/ops/init-object-layout.ts core/space/world/ops/init-unixfs.ts
//go:embed core/space/world/ops/ops.pb.ts core/space/world/ops/set-space-settings.ts
//go:embed core/space/world/world.pb.ts core/space/world/world.ts db/block/blob/blob.pb.ts
//go:embed db/block/block.pb.ts db/block/quad/quad.pb.ts db/block/transform/transform.pb.ts
//go:embed db/bucket/bucket.pb.ts db/git/block/git.pb.ts db/kvtx/block/iavl/iavl.pb.ts
//go:embed db/kvtx/block/kvtx.pb.ts db/kvtx/block/okra/okra.pb.ts db/kvtx/rpc/kvtx.pb.ts
//go:embed db/kvtx/rpc/kvtx_srpc.pb.ts net/hash/hash.pb.ts net/peer/peer.pb.ts sdk/account/account.pb.ts
//go:embed sdk/account/account.ts sdk/account/account_srpc.pb.ts sdk/block/cursor/cursor.pb.ts
//go:embed sdk/block/cursor/cursor.ts sdk/block/cursor/cursor_srpc.pb.ts
//go:embed sdk/block/transaction/transaction.pb.ts sdk/block/transaction/transaction.ts
//go:embed sdk/block/transaction/transaction_srpc.pb.ts sdk/bucket/lookup/lookup.pb.ts
//go:embed sdk/bucket/lookup/lookup.ts sdk/bucket/lookup/lookup_srpc.pb.ts sdk/canvas/canvas.pb.ts
//go:embed sdk/cdn/cdn-resource.pb.ts sdk/cdn/cdn-resource_srpc.pb.ts sdk/cdn/cdn.ts sdk/chat/chat.pb.ts
//go:embed sdk/chat/content/content.pb.ts sdk/chat/state/state.pb.ts
//go:embed sdk/chat/create-channel.ts sdk/chat/init-chat-demo.ts sdk/command/command.pb.ts
//go:embed sdk/command/registry/registry.pb.ts sdk/command/registry/registry_srpc.pb.ts
//go:embed sdk/configtype/registry/registry.pb.ts sdk/configtype/registry/registry_srpc.pb.ts
//go:embed sdk/debug/context.ts sdk/debug/debug-service.ts sdk/debug/debug.pb.ts sdk/debug/debug_srpc.pb.ts
//go:embed sdk/debugdb/benchmark.ts sdk/debugdb/debugdb.pb.ts sdk/debugdb/debugdb.ts
//go:embed sdk/debugdb/debugdb_srpc.pb.ts sdk/deploy/deploy.pb.ts
//go:embed sdk/device/computers/create-computers-dashboard.ts sdk/device/device.pb.ts sdk/device/device.ts
//go:embed sdk/device/device_srpc.pb.ts sdk/forge/dashboard/create-forge-dashboard.ts sdk/forge/execution.ts
//go:embed sdk/forge/dashboard/init-forge-quickstart.ts sdk/kv/index.ts sdk/kv/kv.ts sdk/layout/layout-host.ts
//go:embed sdk/layout/layout.pb.ts sdk/layout/layout.ts sdk/layout/layout_srpc.pb.ts
//go:embed sdk/layout/world/object-layout.ts sdk/layout/world/world.pb.ts
//go:embed sdk/objecttype/registry/registry.pb.ts sdk/objecttype/registry/registry_srpc.pb.ts
//go:embed sdk/provider/index.ts sdk/provider/local/local.pb.ts sdk/provider/local/local.ts
//go:embed sdk/provider/local/local_srpc.pb.ts sdk/provider/provider.pb.ts sdk/provider/provider.ts
//go:embed sdk/provider/provider_srpc.pb.ts sdk/provider/spacewave/spacewave.pb.ts
//go:embed sdk/provider/spacewave/spacewave.ts sdk/provider/spacewave/spacewave_srpc.pb.ts
//go:embed sdk/quickstart/registry/registry.pb.ts sdk/quickstart/registry/registry_srpc.pb.ts
//go:embed sdk/root/index.ts sdk/root/root.pb.ts sdk/root/root.ts sdk/root/root_srpc.pb.ts
//go:embed sdk/root/app.ts
//go:embed sdk/secret/secret.pb.ts sdk/session/index.ts sdk/session/local-session.pb.ts
//go:embed sdk/session/local-session.ts sdk/session/local-session_srpc.pb.ts sdk/session/session.pb.ts
//go:embed sdk/session/session.ts sdk/session/session_srpc.pb.ts
//go:embed sdk/session/shared-object-self-enrollment.pb.ts sdk/session/shared-object-self-enrollment.ts
//go:embed sdk/session/shared-object-self-enrollment_srpc.pb.ts sdk/session/spacewave-session.pb.ts
//go:embed sdk/session/spacewave-session.ts sdk/session/spacewave-session_srpc.pb.ts sdk/sobject/sobject.pb.ts
//go:embed sdk/sobject/sobject.ts sdk/sobject/sobject_srpc.pb.ts sdk/space/contents.ts sdk/space/object-uri.ts
//go:embed sdk/space/space.pb.ts sdk/space/space.ts sdk/space/space_srpc.pb.ts sdk/space/plugin-frontend.ts sdk/status/status.pb.ts
//go:embed sdk/status/status.ts sdk/status/status_srpc.pb.ts sdk/unixfs/file-kind.ts sdk/unixfs/fs-cursor.ts
//go:embed sdk/unixfs/handle.pb.ts sdk/unixfs/handle.ts sdk/unixfs/handle_srpc.pb.ts sdk/unixfs/index.ts
//go:embed sdk/unixfs/path.ts sdk/unixfs/type.ts sdk/viewer/registry/registry.pb.ts
//go:embed sdk/viewer/registry/registry_srpc.pb.ts sdk/vm/v86-wizard.pb.ts sdk/vm/v86.pb.ts
//go:embed sdk/world/engine-state.ts sdk/world/engine.ts sdk/world/errors.ts sdk/world/graph-utils.ts
//go:embed sdk/world/object-ref.ts sdk/world/object-state.ts sdk/world/object_iterator.ts
//go:embed sdk/world/types/errors.ts
//go:embed sdk/world/types/types.ts sdk/world/utils.ts sdk/world/wizard/create-wizard.ts
//go:embed sdk/world/wizard/wizard.pb.ts sdk/world/wizard/wizard_srpc.pb.ts sdk/world/world-state.ts
//go:embed sdk/world/world.pb.ts sdk/world/world_srpc.pb.ts
//go:embed sdk/sync/index.ts net/peer/base58.ts
//go:embed sdk/sync/query.ts sdk/sync/live-query.ts sdk/sync/attachment.ts sdk/sync/instance.ts sdk/sync/plugin.ts
//go:embed sdk/sync/upgrade.ts sdk/sync/plugin-operations.ts sdk/sync/object-query.ts
//go:embed sdk/plugin/registration/*.ts
//go:embed sdk/sync/app.ts sdk/sync/schema.ts sdk/sync/transaction.ts sdk/sync/keys.ts
//go:embed sdk/sync/errors.ts sdk/sync/json.ts sdk/sync/operation.ts sdk/kv/world/store.ts
//go:embed sdk/sync/client.ts sdk/sync/access.ts sdk/sync/wire.ts sdk/sync/sync.pb.ts sdk/sync/sync_srpc.pb.ts
//go:embed db/block/file/file.pb.ts
//go:embed db/unixfs/block/fstree.pb.ts
//go:embed db/unixfs/errors/errors.pb.ts
//go:embed db/unixfs/errors/errors.ts
//go:embed db/unixfs/fs-cursor.ts
//go:embed db/unixfs/fs-handle.ts
//go:embed db/unixfs/fs-inode.ts
//go:embed db/unixfs/fs-node-type.ts
//go:embed db/unixfs/path.ts
//go:embed db/unixfs/rpc/client/fs-cursor-client.ts
//go:embed db/unixfs/rpc/client/fs-cursor-ops-remote.ts
//go:embed db/unixfs/rpc/client/fs-cursor-remote.ts
//go:embed db/unixfs/rpc/client/fs-cursor.ts
//go:embed db/unixfs/rpc/client/fs-handle.ts
//go:embed db/unixfs/rpc/rpc.pb.ts
//go:embed db/unixfs/rpc/rpc_srpc.pb.ts
//go:embed db/volume/volume.pb.ts
//go:embed sdk/worldop/registry/registry.pb.ts
//go:embed sdk/worldop/registry/registry_srpc.pb.ts
var DistSources embed.FS
