// Package s4wave_unixfs is the client contract for a handle into a UnixFS file
// tree stored in a World object.
//
// FSHandleResourceService looks up paths into child handles, reads, writes and
// truncates files, makes, removes and renames entries, and uploads a file or a
// whole tree in one call. WatchReaddir and WatchFiles report changes as they
// happen, so a viewer watches a directory instead of reading it again. The world
// subpackage registers the UnixFS ObjectType that serves a handle for an object.
package s4wave_unixfs
