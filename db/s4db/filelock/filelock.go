// Package filelock takes byte-range write and read locks on a file that
// processes share. Locks reference a byte offset, usually one far past the
// end of the file where it never covers data, and the system drops them when
// the holding process dies. The engine's lock bytes and the previous format's
// reader both use it, so one handle can exclude the other.
package filelock
