//go:build !js && !wasip1 && windows

package spacewave_cli

import "golang.org/x/sys/windows"

func statePathLeaseProcessAlive(pid int) bool {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err == nil {
		_ = windows.CloseHandle(handle)
		return true
	}
	return err == windows.ERROR_ACCESS_DENIED
}
