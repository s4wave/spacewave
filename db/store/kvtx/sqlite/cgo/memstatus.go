//go:build cgo

package cgo

/*
// sqlite3_config is defined by go-sqlite3's SQLite, linked into the same
// binary; it is variadic, so Go calls it through memstatusOff.
int sqlite3_config(int, ...);

// memstatusOff turns off SQLite's memory statistics (SQLITE_CONFIG_MEMSTATUS
// is 9 in every SQLite 3 release).
static int memstatusOff(void) { return sqlite3_config(9, 0); }
*/
import "C"

// init turns off SQLite's memory statistics for the process before any
// connection opens. go-sqlite3's own init only registers the driver. SQLite
// refuses configuration once initialized, which leaves the statistics on and
// costs only speed.
//
// With statistics on, every SQLite allocation and free holds one process-wide
// mutex. A read on a new connection allocates a page header for each mapped
// page it fetches, so after a restart, when every connection is new and each
// read turn runs on its own core, all reads queue on that mutex: one measured
// host spent most of 35 busy cores spinning on its futex. Nothing reads the
// statistics.
func init() {
	C.memstatusOff()
}
