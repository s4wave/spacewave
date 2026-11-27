// Package cdn_writer is the writer of self-hosted CDN Spaces. It serves the
// session API routes the publisher speaks, keeps each Space's root pointer
// in a Store, and has writers upload packs straight to the bucket on a
// presigned PUT. A pack joins a Space only after the bucket holds its exact
// bytes. A reclaim's replacement pack supersedes others, which stay in the
// bucket for a grace period, so a reader holding an old pointer keeps
// working.
package cdn_writer
