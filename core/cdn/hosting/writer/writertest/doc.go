// Package writertest runs a CDN Space writer for tests: an in-process bucket
// behind a counting proxy, a pointer handler, and a writer over a file-backed
// World, each served on loopback, with a new Space owned by a new key.
package writertest
