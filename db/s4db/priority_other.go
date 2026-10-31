//go:build !darwin && !linux && !windows

package s4db

// lowerThreadPriority leaves the thread's priority: the platform has no
// thread priority to lower.
func lowerThreadPriority() error {
	return nil
}
