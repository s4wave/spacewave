# release.star - the controller configs of a release Deployment.
#
# A project's bldr.star loads this file to mount its release World and select
# its channel from one deployment table. The vendored Spacewave module carries
# it with the publisher package:
#
#   load("@go/github.com/s4wave/spacewave/core/release/publisher/release.star",
#        "release_deployment", "release_world_config_set")
#
# Every identifier derives from the project ID as publisher.Deployment derives
# it, so the launchers built here mount what the publisher releases.

# release_deployment returns the table naming one release Deployment.
#
# space_id is the CDN Space holding the release World, signer_peer_id the peer
# that signs its DistConfig, selector_url the URL serving the signed
# DistConfig, and cdn_base_url the host serving the Space's packs. The root
# pointer is read from root_pointer_base_url, or from the pack host when empty.
# object_key names the object collecting the manifests.
def release_deployment(
        project_id,
        space_id,
        signer_peer_id,
        selector_url,
        cdn_base_url,
        root_pointer_base_url="",
        object_key="",
        channel_key="",
        engine_id="",
        bucket_id="",
        block_store_id="",
        cache_block_store_id="",
        release_authority_plugin_id=""):
    if not project_id or not space_id or not signer_peer_id or not selector_url or not cdn_base_url:
        fail("release_deployment: project_id, space_id, signer_peer_id, selector_url and cdn_base_url are required")
    return {
        "project_id": project_id,
        "space_id": space_id,
        "signer_peer_id": signer_peer_id,
        "selector_url": selector_url,
        "cdn_base_url": cdn_base_url,
        "root_pointer_base_url": root_pointer_base_url,
        "object_key": object_key or project_id + "/release/manifests",
        "channel_key": channel_key,
        "engine_id": engine_id or project_id + "-release-world",
        "bucket_id": bucket_id or project_id + "-release",
        # The CDN World answers for its Space ID.
        "block_store_id": block_store_id or space_id,
        "cache_block_store_id": cache_block_store_id,
        "release_authority_plugin_id": release_authority_plugin_id,
    }

# release_world_mount_config returns the CDN World controller config mounting
# the deployment's Space, given the keys that pick its block store.
def release_world_mount_config(d, store):
    conf = {
        "engineId": d["engine_id"],
        "spaceId": d["space_id"],
        "cdnBaseUrl": d["cdn_base_url"],
    }
    if d["root_pointer_base_url"]:
        conf["rootPointerBaseUrl"] = d["root_pointer_base_url"]
    conf.update(store)
    return config_entry("spacewave/cdn/world", 1, conf)

# release_world_buckets_config_set returns the buckets reading the Space's
# store: manifest refs name the local World's bucket, and refs the Space
# rewrites name the Space.
def release_world_buckets_config_set(d):
    store_id = d["block_store_id"]
    return {
        "release-world-cdn-bucket": config_entry("hydra/block/store/bucket", 1, {
            "blockStoreId": store_id,
            "bucketStoreId": store_id,
            "bucketConfig": {"id": d["bucket_id"], "rev": 1},
        }),
        "release-world-space-bucket": config_entry("hydra/block/store/bucket", 1, {
            "blockStoreId": store_id,
            "bucketStoreId": store_id,
            "bucketConfig": {"id": d["space_id"], "rev": 1},
        }),
    }

# release_world_config_set returns the controllers a launcher's host config
# runs to mount the release World from the CDN Space and fetch its manifests,
# the set publisher.Deployment.HostConfigSet returns. The launcher drops
# release-world controllers from a signed DistConfig, so this set belongs in
# the host config.
def release_world_config_set(d):
    store = {}
    if d["cache_block_store_id"]:
        store["cacheBlockStoreId"] = d["cache_block_store_id"]
    configs = {"release-world": release_world_mount_config(d, store)}
    configs.update(release_world_buckets_config_set(d))
    fetch = {
        "engineId": d["engine_id"],
        "objectKeys": [d["object_key"]],
    }
    if d["release_authority_plugin_id"]:
        fetch["releaseAuthorityPluginId"] = d["release_authority_plugin_id"]
    configs["release-world-fetch"] = config_entry("bldr/manifest/fetch/world", 1, fetch)
    return configs

# release_world_service_id is the block store RPC service serving the release
# World's store to other buses.
def release_world_service_id(d):
    return d["block_store_id"] + "/block.rpc.BlockStore"

# release_world_serve_config_set serves the host bus's release World block
# store to plugin buses, for release_world_reader_config_set.
def release_world_serve_config_set(d):
    return {
        "release-world-cdn-server": config_entry("hydra/block/store/rpc/server", 1, {
            "blockStoreId": d["block_store_id"],
            "serviceId": release_world_service_id(d),
        }),
    }

# release_world_reader_config_set mounts the release World on a plugin bus
# through the plugin host's block store: this bus opens no CDN transport or
# cache of its own, and fetches only the root pointer to build its head.
def release_world_reader_config_set(d):
    store_id = d["block_store_id"]
    configs = {
        "release-world-cdn-store": config_entry("hydra/block/store/rpc", 1, {
            "blockStoreId": store_id,
            "serviceId": "plugin-host/" + release_world_service_id(d),
            "readOnly": True,
            "bucketIds": [d["space_id"], d["bucket_id"]],
            "lookupOnStart": True,
        }),
        "release-world": release_world_mount_config(d, {"suppliedBlockStoreId": store_id}),
    }
    configs.update(release_world_buckets_config_set(d))
    return configs

# release_launcher_controller_config returns the launcher controller config
# that fetches and verifies the deployment's signed DistConfig.
def release_launcher_controller_config(
        d,
        refetch_dur="1h",
        init_dist_config="",
        disable_endpoint_fetch=False):
    conf = {
        "projectId": d["project_id"],
        "distPeerIds": [d["signer_peer_id"]],
    }
    if d["channel_key"]:
        conf["channelKey"] = d["channel_key"]
    if disable_endpoint_fetch:
        conf["disableEndpointFetch"] = True
    if not disable_endpoint_fetch:
        conf["endpoints"] = [{"url": d["selector_url"]}]
    if refetch_dur:
        conf["refetchDur"] = refetch_dur
    if init_dist_config:
        conf["initDistConfig"] = init_dist_config
    return conf
