package checkoutroot

import (
	"os"
	"strconv"

	"github.com/aperturerobotics/fsnotify"
)

// addDirectoryWatch registers the open directory without re-resolving its name.
func addDirectoryWatch(w *fsnotify.Watcher, _ *os.Root, _ string, dir *os.File) (string, error) {
	// The fd stays open so the inotify watch name remains unique until removal.
	if _, err := checkDirectory(dir); err != nil {
		return "", err
	}
	name := "/proc/self/fd/" + strconv.FormatUint(uint64(dir.Fd()), 10)
	return name, w.Add(name)
}
