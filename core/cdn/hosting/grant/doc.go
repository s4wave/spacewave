// Package cdn_grant issues CdnSpaceGrants for the private CDN Spaces a
// writer owns. A grant pairs a bucket download authorization scoped to one
// Space's prefix with, when the holder may write, a writer admission for the
// same span, so the holder reads and publishes straight to the bucket and the
// server never relays pack bytes.
//
// A volume serves any block it holds to anyone who supplies its hash, so a
// server holding public and private content keeps them on separate volumes.
package cdn_grant
