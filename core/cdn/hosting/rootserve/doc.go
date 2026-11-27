// Package cdn_rootserve serves CDN Space root pointers and the cache rule of
// their packs. A pointer changes on every commit, so it is served with
// no-cache and a strong ETag, and a reader polling it gets 304 until it moves.
// A pack's ID identifies its content, so the pack is cached for a year.
package cdn_rootserve
