// Package dex fetches blocks from other peers.
//
// A local read that misses, with network lookup enabled in its bucket's
// lookup configuration, emits a LookupBlockFromNetwork directive for the
// block's ref, and any DEX controller on the bus may resolve it: package
// psecho through want lists gossiped over a pub-sub channel, and package
// solicit through a stream per peer that Bifrost protocol solicitation pairs.
// Each verifies the bytes it receives against the requested ref before it
// emits them, so a peer cannot substitute a block. A DEX serves the block
// and stores nothing itself: the bucket lookup decides whether to write it
// back, and it does so only when the block's outgoing refs are known.
package dex
