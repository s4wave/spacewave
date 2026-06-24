package unixfs

import "os"

// FlagIsCreate reports whether the open flag requests file creation.
func FlagIsCreate(flag int) bool {
	return flag&os.O_CREATE != 0
}

// FlagIsExclusive reports whether the open flag requires exclusive creation.
func FlagIsExclusive(flag int) bool {
	return flag&os.O_EXCL != 0
}

// FlagIsReadOnly reports whether the open flag is read-only.
func FlagIsReadOnly(flag int) bool {
	return flag == os.O_RDONLY
}

// FlagIsAppend reports whether the open flag requests append mode.
func FlagIsAppend(flag int) bool {
	return flag&os.O_APPEND != 0
}

// FlagIsTruncate reports whether the open flag requests truncation.
func FlagIsTruncate(flag int) bool {
	return flag&os.O_TRUNC != 0
}

// FlagIsReadAndWrite reports whether the open flag requests read-write access.
func FlagIsReadAndWrite(flag int) bool {
	return flag&os.O_RDWR != 0
}

// FlagIsWriteOnly reports whether the open flag requests write-only access.
func FlagIsWriteOnly(flag int) bool {
	return flag&os.O_WRONLY != 0
}
