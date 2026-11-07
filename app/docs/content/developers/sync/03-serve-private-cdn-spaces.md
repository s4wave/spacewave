---
title: Serve Private CDN Spaces
section: sync
order: 3
summary: Let a client read, and optionally write, one private CDN Space straight from the bucket with a short-lived grant.
---

A private CDN Space lives in a bucket no one reads anonymously. Your server
holds the bucket's key and hands each client a `CdnSpaceGrant`: a download
authorization scoped to that one Space's key prefix, good for a few hours. The
client reads packs from the bucket itself, so your server never relays pack
bytes. A grant can also let its holder publish to the Space.

## Keep public and private content on separate volumes

A volume serves any block it holds to anyone who names the block's hash. If
one volume holds both public and private Spaces, a reader allowed one block
can ask for any other block by hash. Put private Spaces in their own bucket,
or at least under a writer prefix that no public grant or public CDN path
covers, and keep them on their own server volume.

## Issue grants

`core/cdn/grant` issues grants for the Spaces a `cdn_writer.Writer` owns.
`cdn_grant.NewB2` mints B2 native download authorizations; its application key
stays on your server.

```go
b2, err := cdn_grant.NewB2(cdn_grant.B2Config{
	KeyID:          keyID,
	ApplicationKey: appKey,
	BucketID:       bucketID,
	BucketName:     "my-private-spaces",
})
if err != nil {
	return err
}
issuer := &cdn_grant.Issuer{Writer: writer, Downloads: b2}

// Your app decides who may hold which grant.
grant, err := issuer.Issue(ctx, spaceID, holderPeerID, mayWrite)
```

A grant lasts `cdn_grant.DefaultTTL`, twelve hours, unless you set `TTL`. With
`mayWrite` set, `Issue` also admits the holder as a writer of the Space until
the grant expires, so the holder signs its own checkpoints.

## Read with a grant

Set `privateSpace` on the CDN World controller. It then reads packs only
through grants, and `cdnBaseUrl` serves just the root pointer:

```yaml
engineId: my-private-content
spaceId: 01kpn3x0y79yr94ps1yae206vp
cdnBaseUrl: https://app.example.com/cdn
privateSpace: true
```

The controller asks the bus for grants with the `LookupCdnSpaceGrants`
directive. Resolve it on the client with a `cdn_bstore.GrantSource` that fetches
a fresh grant from your server:

```go
src := cdn_bstore.GrantSourceFunc(func(ctx context.Context, spaceID string) (*cdn.CdnSpaceGrant, error) {
	return myAPI.SpaceGrant(ctx, spaceID)
})
```

The reader renews the grant after three quarters of its life, without stalling
reads while the old grant is still valid. If a renewal fails, it keeps the
valid grant and asks again a few seconds later. If the bucket refuses a grant
with 401 or 403, the reader drops that grant and fetches a new one on the next
read.

## Publish with a grant

Pass the same `GrantSource` as `Grants` to `cdn_mirror.Mirror`. The mirror
fetches a grant before its first request and renews it before each pack upload
and before the checkpoint, which keeps the holder's admission current. A grant
without `mayWrite` fails the mirror with "the grant does not allow publishing
to the Space".
