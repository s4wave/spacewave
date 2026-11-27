//go:build windows

package launcher_install

import (
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"

	"github.com/pkg/errors"
	"golang.org/x/sys/windows"
)

// procCoCreateInstance creates COM objects. x/sys/windows does not wrap it.
var procCoCreateInstance = windows.NewLazySystemDLL("ole32.dll").NewProc("CoCreateInstance")

// COM class and interface IDs for writing a shell link.
var (
	// clsidShellLink is CLSID_ShellLink.
	clsidShellLink = windows.GUID{Data1: 0x00021401, Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
	// iidShellLinkW is IID_IShellLinkW.
	iidShellLinkW = windows.GUID{Data1: 0x000214f9, Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
	// iidPersistFile is IID_IPersistFile.
	iidPersistFile = windows.GUID{Data1: 0x0000010b, Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
)

// Vtable slots of the COM methods writeShortcut calls.
const (
	// slotQueryInterface is IUnknown::QueryInterface.
	slotQueryInterface = 0
	// slotRelease is IUnknown::Release.
	slotRelease = 2
	// slotSetDescription is IShellLinkW::SetDescription.
	slotSetDescription = 7
	// slotSetWorkingDirectory is IShellLinkW::SetWorkingDirectory.
	slotSetWorkingDirectory = 9
	// slotSetIconLocation is IShellLinkW::SetIconLocation.
	slotSetIconLocation = 17
	// slotSetPath is IShellLinkW::SetPath.
	slotSetPath = 20
	// slotSave is IPersistFile::Save.
	slotSave = 6
)

// comObject is the memory layout of a COM interface pointer.
type comObject struct {
	// vtbl points to the interface's method table.
	vtbl *[21]uintptr
}

// call invokes the method in slot with args and converts a failed HRESULT.
func (o *comObject) call(slot int, args ...uintptr) error {
	hr, _, _ := syscall.SyscallN(o.vtbl[slot], append([]uintptr{uintptr(unsafe.Pointer(o))}, args...)...)
	if int32(hr) < 0 {
		return errors.Errorf("HRESULT 0x%08x", uint32(hr))
	}
	return nil
}

// release drops the caller's reference.
func (o *comObject) release() {
	_, _, _ = syscall.SyscallN(o.vtbl[slotRelease], uintptr(unsafe.Pointer(o)))
}

// writeShortcut saves a Start menu shortcut at path that starts target, using
// target's own icon.
func writeShortcut(path, target, description string) error {
	// COM state is per thread, so keep this goroutine on one apartment thread.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err != nil && !errors.Is(err, syscall.Errno(1)) {
		return errors.Wrap(err, "initialize COM")
	}
	defer windows.CoUninitialize()

	// Create the shell link and its file persistence interface.
	var link *comObject
	hr, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidShellLink)),
		0,
		windows.CLSCTX_INPROC_SERVER,
		uintptr(unsafe.Pointer(&iidShellLinkW)),
		uintptr(unsafe.Pointer(&link)),
	)
	if int32(hr) < 0 {
		return errors.Errorf("create shell link: HRESULT 0x%08x", uint32(hr))
	}
	defer link.release()
	var file *comObject
	if err := link.call(slotQueryInterface, uintptr(unsafe.Pointer(&iidPersistFile)), uintptr(unsafe.Pointer(&file))); err != nil {
		return errors.Wrap(err, "query shell link file interface")
	}
	defer file.release()

	// Point the link at target and its folder.
	fields := []struct {
		slot  int
		value string
	}{
		{slotSetPath, target},
		{slotSetWorkingDirectory, filepath.Dir(target)},
		{slotSetDescription, description},
	}
	for _, field := range fields {
		ptr, err := windows.UTF16PtrFromString(field.value)
		if err != nil {
			return err
		}
		if err := link.call(field.slot, uintptr(unsafe.Pointer(ptr))); err != nil {
			return errors.Wrap(err, "set shell link field")
		}
	}

	// Show target's own icon.
	icon, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	if err := link.call(slotSetIconLocation, uintptr(unsafe.Pointer(icon)), 0); err != nil {
		return errors.Wrap(err, "set shell link icon")
	}

	// Save the link at path, replacing an earlier one.
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	if err := file.call(slotSave, uintptr(unsafe.Pointer(pathPtr)), 1); err != nil {
		return errors.Wrap(err, "save Start menu shortcut")
	}
	return nil
}
