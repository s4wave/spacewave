---
title: Serve CDN Space Pointers
section: sync
order: 2
summary: Serve a public CDN Space's root pointer from your own server while its packs stay on the CDN.
---

A public CDN Space is read from two kinds of objects. Its packs hold the
blocks; a pack is named by its content, so its bytes never change. Its root
pointer, `root.packedmsg`, names the signed checkpoint and lists the packs; it
changes on every commit. The two cannot share a cache rule, so a reader can
fetch them from different hosts.

## Where a reader looks

A reader is given a CDN base URL for packs and, optionally, a pointer base URL.
`cdn_bstore.RootPointerBaseURL` picks the pointer base when it is set and the
CDN base otherwise. The CDN World controller and the CDN block store take it as
`rootPointerBaseUrl`:

```yaml
engineId: my-app-content
spaceId: 01kpn3x0y79yr94ps1yae206vp
cdnBaseUrl: https://cdn.example.com
rootPointerBaseUrl: https://app.example.com/cdn
```

The reader fetches `{pointer base}/{spaceId}/root.packedmsg` and the packs
under `{cdn base}/{spaceId}/packs/`.

## Cache rules

| Object         | Cache-Control                         | Why                                  |
|---|---|---|
| `root.packedmsg` | `no-cache` with a strong `ETag`       | A commit must reach the next poll.   |
| `packs/...`    | `public, max-age=31536000, immutable` | A pack's ID names its bytes.         |

A reader revalidating the pointer with `If-None-Match` gets `304 Not Modified`
until the pointer moves, so a poll costs one small request.

## Serve the pointer from your server

`core/cdn/rootserve` serves pointers with those rules. Mount it under the
pointer base and tell it each new pointer after its commit is durable:

```go
roots := cdn_rootserve.NewHandler("https://cdn.example.com", nil)
mux.Handle("/cdn/", http.StripPrefix("/cdn", roots))

// After the commit that produced ptr is durable:
if err := roots.SetOwned(ptr); err != nil {
	return err
}
```

A Space you write is served from memory. Any other Space is fetched from the
origin given to `NewHandler`, revalidated with the copy the handler holds. When
the origin fails, the handler keeps serving its last copy, so a reader does
not lose a Space when the origin blinks. When the origin reports the Space
gone, the handler drops it.

To give packs the year-long rule on a host you serve yourself, wrap that host's
handler with `cdn_rootserve.ImmutablePacks`. A bucket behind a CDN sets the
same `Cache-Control` as object metadata when the pack is written.
