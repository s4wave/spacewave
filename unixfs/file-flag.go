package unixfs

import "os"

func FlagIsCreate(flag int) bool {
	return flag&os.O_CREATE != 0
}

func FlagIsExclusive(flag int) bool {
	return flag&os.O_EXCL != 0
}

func FlagIsReadOnly(flag int) bool {
	return flag == os.O_RDONLY
}

func FlagIsAppend(flag int) bool {
	return flag&os.O_APPEND != 0
}

func FlagIsTruncate(flag int) bool {
	return flag&os.O_TRUNC != 0
}

func FlagIsReadAndWrite(flag int) bool {
	return flag&os.O_RDWR != 0
}

func FlagIsWriteOnly(flag int) bool {
	return flag&os.O_WRONLY != 0
}
