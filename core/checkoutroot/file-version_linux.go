package checkoutroot

import "syscall"

// fileDevice returns the device ID in the host file version's field width.
func fileDevice(stat *syscall.Stat_t) uint64 {
	return stat.Dev
}

// changeTime returns the inode change time in nanoseconds.
func changeTime(stat *syscall.Stat_t) int64 {
	return stat.Ctim.Nano()
}
