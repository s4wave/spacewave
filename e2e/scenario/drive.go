package scenario

import (
	"context"
	"fmt"

	"github.com/s4wave/spacewave/e2e/runtime"
)

const starterFile = "getting-started.md"

const crashRecoveryUploadName = "e2e-upload-recovery.bin"
const crashRecoveryUploadSize = 8 * 1024 * 1024

func init() {
	Register(Scenario{Name: "drive.first-use.landing", Tags: []string{"drive", "first-use"}, Session: runtime.SessionFreshInstall, Run: driveFirstUse("/")})
	Register(Scenario{Name: "drive.first-use.direct", Tags: []string{"drive", "first-use"}, Session: runtime.SessionFreshInstall, Run: driveFirstUse("/quickstart/drive")})
	Register(Scenario{Name: "drive.navigation.nested-home", Tags: []string{"drive", "navigation"}, Session: runtime.SessionAny, Run: driveNestedHome})
	Register(Scenario{Name: "drive.navigation.history", Tags: []string{"drive", "navigation"}, Session: runtime.SessionAny, Run: driveNavigationHistory})
	Register(Scenario{Name: "drive.upload", Tags: []string{"drive", "upload"}, Session: runtime.SessionAny, Run: driveUpload})
	Register(Scenario{Name: "drive.upload-crash-recovery", Tags: []string{"drive", "upload", "recovery"}, Session: runtime.SessionFresh, Run: driveUploadCrashRecovery})
	Register(Scenario{Name: "drive.row-move", Tags: []string{"drive", "row-move"}, Session: runtime.SessionAny, Run: driveRowMove})
	Register(Scenario{Name: "drive.space-delete", Tags: []string{"drive", "space-lifecycle"}, Session: runtime.SessionAny, Run: driveSpaceDelete})
}

func driveUploadCrashRecovery(_ context.Context, rt runtime.Runtime) error {
	if err := prepareDrive(rt); err != nil {
		return err
	}
	file := runtime.File{
		Name:     crashRecoveryUploadName,
		MIMEType: "application/octet-stream",
		Contents: crashRecoveryUploadContents(),
	}
	if err := rt.UploadFile("input[type='file']", file); err != nil {
		return err
	}
	if err := rt.ExpectVisible("Uploading 1/1"); err != nil {
		return fmt.Errorf("upload did not enter active state: %w", err)
	}
	if err := rt.ReloadPage(); err != nil {
		return err
	}
	if err := rt.WaitForEvent(runtime.EventAppReady); err != nil {
		return err
	}
	if err := rt.WaitForEvent(runtime.EventDriveReady); err != nil {
		return err
	}
	if err := rt.UploadFile("input[type='file']", file); err != nil {
		return fmt.Errorf("retry interrupted upload: %w", err)
	}
	if err := rt.ExpectVisible("1/1 uploaded"); err != nil {
		return fmt.Errorf("recovered upload completion: %w", err)
	}
	if err := rt.ExpectVisible(crashRecoveryUploadName); err != nil {
		return fmt.Errorf("recovered upload entry: %w", err)
	}
	if err := rt.ExpectAbsent("Uploading"); err != nil {
		return fmt.Errorf("recovered upload still active: %w", err)
	}
	if err := rt.ExpectAbsent("Queued"); err != nil {
		return fmt.Errorf("recovered upload still queued: %w", err)
	}
	return rt.ExpectAbsent("Failed")
}

func crashRecoveryUploadContents() []byte {
	contents := make([]byte, crashRecoveryUploadSize)
	for index := range contents {
		contents[index] = byte((index * 31) ^ (index >> 7) ^ (index >> 15))
	}
	return contents
}

func driveFirstUse(route string) func(context.Context, runtime.Runtime) error {
	return func(_ context.Context, rt runtime.Runtime) error {
		if err := rt.OpenRoute(route); err != nil {
			return err
		}
		if route == "/" {
			if err := rt.ClickControl("drive"); err != nil {
				return err
			}
		}
		if err := rt.WaitForEvent(runtime.EventDriveReady); err != nil {
			return err
		}
		if err := rt.ExpectVisible(starterFile); err != nil {
			return fmt.Errorf("starter file: %w", err)
		}
		return rt.ExpectRoute("/u/")
	}
}

func prepareDrive(rt runtime.Runtime) error {
	if err := rt.OpenRoute("/quickstart/drive"); err != nil {
		return err
	}
	if err := rt.WaitForEvent(runtime.EventDriveReady); err != nil {
		return err
	}
	return rt.ExpectVisible(starterFile)
}

func driveNavigationHistory(_ context.Context, rt runtime.Runtime) error {
	step := func(name string, fn func() error) error {
		if err := fn(); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		return nil
	}
	if err := step("prepare drive", func() error { return prepareDrive(rt) }); err != nil {
		return err
	}
	if err := step("open starter file", func() error { return rt.DoubleClickContent(starterFile) }); err != nil {
		return err
	}
	if err := step("wait for file content", func() error { return rt.ExpectVisible("Welcome to your new drive") }); err != nil {
		return err
	}
	if err := step("navigate up", func() error { return rt.ClickControl("up") }); err != nil {
		return err
	}
	if err := step("wait for starter row", func() error { return rt.ExpectVisible(starterFile) }); err != nil {
		return err
	}
	if err := step("navigate back", func() error { return rt.ClickControl("back") }); err != nil {
		return err
	}
	if err := step("wait for file after back", func() error { return rt.ExpectVisible("Welcome to your new drive") }); err != nil {
		return err
	}
	if err := step("navigate forward", func() error { return rt.ClickControl("forward") }); err != nil {
		return err
	}
	return step("wait for starter row after forward", func() error { return rt.ExpectVisible(starterFile) })
}

func driveNestedHome(_ context.Context, rt runtime.Runtime) error {
	step := func(name string, fn func() error) error {
		if err := fn(); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		return nil
	}
	steps := []struct {
		name string
		run  func() error
	}{
		{"prepare drive", func() error { return prepareDrive(rt) }},
		{"open new folder", func() error { return rt.ClickControl("new-folder") }},
		{"type nested folder", func() error { return rt.Type("Folder name", "e2e-nested") }},
		{"confirm nested folder", func() error { return rt.ClickControl("confirm") }},
		{"wait for nested folder", func() error { return rt.ExpectVisible("e2e-nested") }},
		{"open nested folder", func() error { return rt.DoubleClickContent("e2e-nested") }},
		{"wait for nested empty folder", func() error { return rt.ExpectVisible("This folder is empty") }},
		{"open child folder", func() error { return rt.ClickControl("new-folder") }},
		{"type child folder", func() error { return rt.Type("Folder name", "e2e-child") }},
		{"confirm child folder", func() error { return rt.ClickControl("confirm") }},
		{"wait for child folder", func() error { return rt.ExpectVisible("e2e-child") }},
		{"navigate home", func() error { return rt.ClickControl("home") }},
		{"wait for root starter", func() error { return rt.ExpectVisible(starterFile) }},
	}
	for _, item := range steps {
		if err := step(item.name, item.run); err != nil {
			return err
		}
	}
	return nil
}

func driveUpload(_ context.Context, rt runtime.Runtime) error {
	if err := prepareDrive(rt); err != nil {
		return err
	}
	if err := rt.UploadFile("input[type='file']", runtime.File{Name: "e2e-upload.txt", MIMEType: "text/plain", Contents: []byte("e2e upload contents")}); err != nil {
		return err
	}
	if err := rt.ExpectVisible("1/1 uploaded"); err != nil {
		return fmt.Errorf("upload completion: %w", err)
	}
	return rt.ExpectVisible("e2e-upload.txt")
}

func driveRowMove(_ context.Context, rt runtime.Runtime) error {
	if err := driveUpload(context.Background(), rt); err != nil {
		return err
	}
	if err := rt.ClickControl("new-folder"); err != nil {
		return err
	}
	if err := rt.Type("Folder name", "e2e-move-target"); err != nil {
		return err
	}
	if err := rt.ClickControl("confirm"); err != nil {
		return err
	}
	if err := rt.ExpectVisible("e2e-move-target"); err != nil {
		return err
	}
	if err := rt.MoveContent("e2e-upload.txt", "e2e-move-target"); err != nil {
		return err
	}
	if err := rt.ExpectAbsent("e2e-upload.txt"); err != nil {
		return err
	}
	if err := rt.DoubleClickContent("e2e-move-target"); err != nil {
		return err
	}
	return rt.ExpectVisible("e2e-upload.txt")
}

func driveSpaceDelete(_ context.Context, rt runtime.Runtime) error {
	if err := prepareDrive(rt); err != nil {
		return err
	}
	return rt.DeleteSpace()
}
