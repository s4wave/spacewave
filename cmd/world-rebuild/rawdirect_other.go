//go:build !linux

package main

// oDirect is 0 where O_DIRECT is not available; reads go through the cache.
const oDirect = 0
