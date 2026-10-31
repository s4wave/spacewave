// Package filelock takes byte-range write locks on a file that processes
// share. Locks name a byte offset, usually one far past the end of the file
// where it never covers data, and the system drops them when the holding
// process dies. The engine's lock bytes and the previous format's reader both
// use it, so one handle can exclude the other.
package filelock
