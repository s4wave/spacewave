//go:build windows

package launcher_install

import (
	"encoding/binary"
	"runtime"
	"unsafe"

	"github.com/pkg/errors"
	"golang.org/x/sys/windows"
)

// procTaskDialogIndirect shows a task dialog. The executable's manifest
// selects Common Controls 6, the first version that exports it.
var procTaskDialogIndirect = windows.NewLazySystemDLL("comctl32.dll").NewProc("TaskDialogIndirect")

// Byte offsets in TASKDIALOGCONFIG, which commctrl.h packs to one byte. The
// offsets hold for 64-bit Windows, the only Windows targets.
const (
	// tdcSize is the size of the whole structure.
	tdcSize = 160
	// tdcFlags is dwFlags.
	tdcFlags = 20
	// tdcCommonButtons is dwCommonButtons.
	tdcCommonButtons = 24
	// tdcWindowTitle is pszWindowTitle.
	tdcWindowTitle = 28
	// tdcMainInstruction is pszMainInstruction.
	tdcMainInstruction = 44
	// tdcContent is pszContent.
	tdcContent = 52
	// tdcButtonCount is cButtons.
	tdcButtonCount = 60
	// tdcButtons is pButtons.
	tdcButtons = 64
	// tdcDefaultButton is nDefaultButton.
	tdcDefaultButton = 72
	// tdcVerificationText is pszVerificationText.
	tdcVerificationText = 92
)

// Task dialog flags, buttons and results.
const (
	// tdfAllowDialogCancellation lets Escape and the close box cancel.
	tdfAllowDialogCancellation = 0x0008
	// tdcbfCancelButton adds the Cancel common button.
	tdcbfCancelButton = 0x0008
	// idCancel is the Cancel button's result.
	idCancel = 2
	// idUninstall is the Uninstall button's result.
	idUninstall = 100
)

// ConfirmUninstall asks the person to confirm the uninstall, with an
// unchecked option to delete their local Spaces too.
func ConfirmUninstall() (confirmed, deleteData bool, err error) {
	// Encode the dialog's text.
	texts := make([][]uint16, 0, 5)
	text := func(s string) uint64 {
		encoded := windows.StringToUTF16(s)
		texts = append(texts, encoded)
		return uint64(uintptr(unsafe.Pointer(&encoded[0])))
	}
	defer runtime.KeepAlive(&texts)

	// Describe one Uninstall button in a packed TASKDIALOG_BUTTON.
	button := make([]byte, 12)
	binary.LittleEndian.PutUint32(button[0:], idUninstall)
	binary.LittleEndian.PutUint64(button[4:], text("Uninstall"))
	defer runtime.KeepAlive(button)

	// Describe the dialog frame, with Cancel as the default button.
	config := make([]byte, tdcSize)
	binary.LittleEndian.PutUint32(config[0:], tdcSize)
	binary.LittleEndian.PutUint32(config[tdcFlags:], tdfAllowDialogCancellation)
	binary.LittleEndian.PutUint32(config[tdcCommonButtons:], tdcbfCancelButton)
	binary.LittleEndian.PutUint32(config[tdcDefaultButton:], idCancel)
	binary.LittleEndian.PutUint64(config[tdcWindowTitle:], text("Uninstall Spacewave"))

	// Describe the question, the Uninstall button and the data option.
	binary.LittleEndian.PutUint64(config[tdcMainInstruction:], text("Uninstall Spacewave?"))
	binary.LittleEndian.PutUint64(config[tdcContent:], text("This removes the Spacewave app from this PC. Your local Spaces stay unless you also delete them."))
	binary.LittleEndian.PutUint32(config[tdcButtonCount:], 1)
	binary.LittleEndian.PutUint64(config[tdcButtons:], uint64(uintptr(unsafe.Pointer(&button[0]))))
	binary.LittleEndian.PutUint64(config[tdcVerificationText:], text("Also delete my local Spaces"))

	// Show the dialog and read the pressed button and the option.
	var pressed int32
	var checked int32
	hr, _, _ := procTaskDialogIndirect.Call(
		uintptr(unsafe.Pointer(&config[0])),
		uintptr(unsafe.Pointer(&pressed)),
		0,
		uintptr(unsafe.Pointer(&checked)),
	)
	if int32(hr) < 0 {
		return false, false, errors.Errorf("show uninstall dialog: HRESULT 0x%08x", uint32(hr))
	}
	return pressed == idUninstall, checked != 0, nil
}

// ShowError reports a failed operation in a message box.
func ShowError(title string, err error) {
	// mbIconError shows the MB_ICONERROR error icon with an OK button.
	const mbIconError = 0x10
	caption, _ := windows.UTF16PtrFromString(title)
	message, _ := windows.UTF16PtrFromString(err.Error())
	_, _ = windows.MessageBox(0, message, caption, mbIconError)
}
