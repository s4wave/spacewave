package unixfs

import "os"

func isCreate(flag int) bool {
	return flag&os.O_CREATE != 0
}

func isExclusive(flag int) bool {
	return flag&os.O_EXCL != 0
}

func isReadOnly(flag int) bool {
	return flag == os.O_RDONLY
}

/*
func isAppend(flag int) bool {
	return flag&os.O_APPEND != 0
}

func isTruncate(flag int) bool {
	return flag&os.O_TRUNC != 0
}

func isReadAndWrite(flag int) bool {
	return flag&os.O_RDWR != 0
}

func isWriteOnly(flag int) bool {
	return flag&os.O_WRONLY != 0
}

func isSymlink(m os.FileMode) bool {
	return m&os.ModeSymlink != 0
}
*/
