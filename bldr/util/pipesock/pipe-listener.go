package pipesock

import (
	"net"
	"sync"
)

// PipeListener owns a local IPC listener and its socket directory.
type PipeListener struct {
	net.Listener

	rootDir string
	path    string
	cleanup func() error

	closeOnce sync.Once
	closeErr  error
}

// GetRootDir returns the directory containing the local IPC socket.
func (l *PipeListener) GetRootDir() string {
	return l.rootDir
}

// GetPath returns the local IPC socket path when the platform uses one.
func (l *PipeListener) GetPath() string {
	return l.path
}

// Close closes the listener and removes its owned socket directory.
func (l *PipeListener) Close() error {
	l.closeOnce.Do(func() {
		l.closeErr = l.Listener.Close()
		if err := l.cleanup(); l.closeErr == nil {
			l.closeErr = err
		}
	})
	return l.closeErr
}
