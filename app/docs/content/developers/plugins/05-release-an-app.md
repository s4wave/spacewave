---
title: Release an App Through a CDN Space
section: plugins
order: 5
summary: Publish your app's release manifests to a CDN Space and sign the launcher's selector only once the Space serves them.
---

Your launchers read releases from a CDN Space. A `publisher.Deployment` names
the Space and the identifiers your launchers mount, and `publisher.Release`
publishes a local release World to it:

```go
d := publisher.Deployment{
	ProjectID:  "myapp",
	ChannelKey: "stable",
	SpaceID:    spaceID,
	CdnBaseURL: "https://cdn.example.com",
}
w, err := publisher.OpenLocalWorld(ctx, le, "release.s4wave", "myapp-release-world", "myapp-release")
// ... build your manifests into w's manifest store, myapp/release/manifests ...
released, err := publisher.Release(ctx, w.Engine, publisher.ReleaseOptions{
	Deployment: d,
	Metadata:   &release.ReleaseMetadata{Version: "1.4.0", Rev: 14},
	ReleaseKey: releaseKey,
	DistKey:    distKey,
	Rev:        14,
	Publish:    cdn_publish.Options{Client: session, OwnerKeyPem: "owner.pem"},
})
```

`Release` stages the channel over the newest release manifest of each
platform, publishes the World, waits for the Space to post the checkpoint
naming it, and only then signs the selector. Serve `released.Signed` where
your launchers fetch their DistConfig.

## Why the order matters

A launcher that verifies a new selector fetches the release at once. If the
selector were signed first, a launcher could adopt it while the Space still
served the previous root. So `Release` signs nothing until the root is
posted: a manifest whose content the World lacks stops the release before
any upload, and a failed upload or checkpoint returns an error with no
selector.

## Retention and rollback

Each release keeps the newest three revisions of every platform's manifest in
the release World and deletes the older ones before staging. Set
`ReleaseOptions.Retain` to keep a different count.

A launcher runs the highest revision it sees, so you can't roll back by
publishing an older one again. `publisher.Reissue` copies a retained revision
to a new revision above the platform's newest and releases it:

```go
released, err := publisher.Reissue(ctx, w.Engine, opts, &bldr_manifest.ManifestMeta{
	ManifestId: "myapp-desktop",
	BuildType:  "release",
	PlatformId: "desktop/linux/amd64",
	Rev:        12,
})
```

The copy shares the original's files, so the rollback uploads almost nothing.
Give `opts.Rev` and `opts.Metadata` new revisions, as for any release.

## The launcher's side

Launchers ignore Release World controllers inside a signed DistConfig, so the
mount belongs in your launcher's host config. `d.HostConfigSet()` returns
those controllers: the CDN World following the Space's root pointer, the
buckets its refs name, and the manifest fetcher over your manifest store.

## Name the deployment once in bldr.star

Your `bldr.star` loads the same configs from the release library in the
vendored Spacewave module. `go mod vendor` copies it with the publisher
package your release tool imports:

```python
load(
    "@go/github.com/s4wave/spacewave/core/release/publisher/release.star",
    "release_deployment",
    "release_launcher_controller_config",
    "release_world_config_set",
)

DEPLOYMENT = release_deployment(
    project_id = "myapp",
    space_id = "01kq...",
    signer_peer_id = "12D3KooW...",
    selector_url = "https://example.com/api/release/config",
    object_key = "myapp/release/manifests",
    cdn_base_url = "https://cdn.example.com",
    root_pointer_base_url = "https://example.com/cdn",
)

manifest("myapp-launcher", builder = "bldr/plugin/compiler/go", rev = 1, config = {
    "goPkgs": ["github.com/s4wave/spacewave/core/provider/spacewave/launcher/controller"],
    "configSet": {"myapp-launcher": config_entry(
        "spacewave/launcher/controller", 1,
        release_launcher_controller_config(DEPLOYMENT),
    )},
    "hostConfigSet": release_world_config_set(DEPLOYMENT),
})
```

`release_world_config_set` returns the controllers `d.HostConfigSet()`
returns for the same values, so the launcher you build mounts what your
release tool publishes. Leave `root_pointer_base_url` empty to read the root
pointer from the pack host. A launcher whose plugin buses read the World
through the host uses `release_world_serve_config_set` on the host and
`release_world_reader_config_set` on the plugin bus, so only one bus opens the
CDN.
