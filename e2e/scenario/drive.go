package scenario

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/e2e/runtime"
)

const starterFile = "getting-started.md"

const (
	crashRecoveryUploadName = "e2e-upload-recovery.bin"
	crashRecoveryUploadSize = 8 * 1024 * 1024
)

// DriveScenarios returns the Drive scenario catalog in execution order.
func DriveScenarios() []Scenario {
	return []Scenario{
		{
			Name:    "drive.first-use.landing",
			Tags:    []string{"drive", "first-use"},
			Session: runtime.SessionFreshInstall,
			Run:     driveFirstUse("/"),
		},
		{
			Name:    "drive.first-use.direct",
			Tags:    []string{"drive", "first-use"},
			Session: runtime.SessionFreshInstall,
			Run:     driveFirstUse("/quickstart/drive"),
		},
		{
			Name:    "drive.navigation.nested-home",
			Tags:    []string{"drive", "navigation"},
			Session: runtime.SessionAny,
			Run:     driveNestedHome,
		},
		{
			Name:    "drive.navigation.history",
			Tags:    []string{"drive", "navigation"},
			Session: runtime.SessionAny,
			Run:     driveNavigationHistory,
		},
		{
			Name:    "drive.upload",
			Tags:    []string{"drive", "upload"},
			Session: runtime.SessionAny,
			Run:     driveUpload,
		},
		{
			Name:    "drive.upload-crash-recovery",
			Tags:    []string{"drive", "upload", "recovery"},
			Session: runtime.SessionFresh,
			Run:     driveUploadCrashRecovery,
		},
		{
			Name:    "drive.row-move",
			Tags:    []string{"drive", "row-move"},
			Session: runtime.SessionFreshInstall,
			Run:     driveRowMove,
		},
		{
			Name:    "drive.space-delete",
			Tags:    []string{"drive", "space-lifecycle"},
			Session: runtime.SessionAny,
			Run:     driveSpaceDelete,
		},
	}
}

func driveUploadCrashRecovery(_ context.Context, rt runtime.Runtime) error {
	// Open a ready Drive before interrupting an upload.
	if err := prepareDrive(rt); err != nil {
		return err
	}

	// Start a large Drive upload and require its active progress state.
	file := runtime.File{
		Name:     crashRecoveryUploadName,
		MIMEType: "application/octet-stream",
		Contents: crashRecoveryUploadContents(),
	}
	if err := rt.UploadFile("input[type='file']", file); err != nil {
		return err
	}
	if err := rt.ExpectVisible("Uploading 1/1"); err != nil {
		return errors.Wrap(err, "upload did not enter active state")
	}

	// Reload the interrupted session and await the app and Drive readiness events.
	if err := rt.ReloadPage(); err != nil {
		return err
	}
	if err := rt.WaitForEvent(runtime.EventAppReady); err != nil {
		return err
	}
	if err := rt.WaitForEvent(runtime.EventDriveReady); err != nil {
		return err
	}

	// Retry the interrupted upload and require its completed Drive entry.
	if err := rt.UploadFile("input[type='file']", file); err != nil {
		return errors.Wrap(err, "retry interrupted upload")
	}
	if err := rt.ExpectVisible("1/1 uploaded"); err != nil {
		return errors.Wrap(err, "recovered upload completion")
	}
	if err := rt.ExpectVisible(crashRecoveryUploadName); err != nil {
		return errors.Wrap(err, "recovered upload entry")
	}

	// Verify that recovery leaves no active or failed upload status.
	if err := rt.ExpectAbsent("Uploading"); err != nil {
		return errors.Wrap(err, "recovered upload still active")
	}
	if err := rt.ExpectAbsent("Queued"); err != nil {
		return errors.Wrap(err, "recovered upload still queued")
	}
	return rt.ExpectAbsent("Failed")
}

func crashRecoveryUploadContents() []byte {
	contents := make([]byte, crashRecoveryUploadSize)
	for index := range contents {
		contents[index] = byte((index * 31) ^ (index >> 7) ^ (index >> 15)) //nolint:gosec // the fixture intentionally keeps the low byte of its deterministic pattern.
	}
	return contents
}

func driveFirstUse(route string) func(context.Context, runtime.Runtime) error {
	return func(_ context.Context, rt runtime.Runtime) error {
		// Open Drive through the requested first-use route.
		if err := rt.OpenRoute(route); err != nil {
			return err
		}
		if route == "/" {
			if err := rt.ClickControl("drive"); err != nil {
				return err
			}
		}

		// Await Drive readiness and require the starter file in its resolved route.
		if err := rt.WaitForEvent(runtime.EventDriveReady); err != nil {
			return err
		}
		if err := rt.ExpectVisible(starterFile); err != nil {
			return errors.Wrap(err, "starter file")
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
	// Preserve the Drive navigation step name in runtime errors.
	step := func(name string, fn func() error) error {
		if err := fn(); err != nil {
			return errors.Wrap(err, name)
		}
		return nil
	}

	// Open the starter file and wait for its content.
	if err := step("prepare drive", func() error { return prepareDrive(rt) }); err != nil {
		return err
	}
	if err := step("open starter file", func() error { return rt.DoubleClickContent(starterFile) }); err != nil {
		return err
	}
	if err := step("wait for file content", func() error { return rt.ExpectVisible("Welcome to your new drive") }); err != nil {
		return err
	}

	// Return to the Drive directory and require the starter row.
	if err := step("navigate up", func() error { return rt.ClickControl("up") }); err != nil {
		return err
	}
	if err := step("wait for starter row", func() error { return rt.ExpectVisible(starterFile) }); err != nil {
		return err
	}

	// Follow Drive history back to the starter file content.
	if err := step("navigate back", func() error { return rt.ClickControl("back") }); err != nil {
		return err
	}
	if err := step("wait for file after back", func() error { return rt.ExpectVisible("Welcome to your new drive") }); err != nil {
		return err
	}

	// Follow Drive history forward to the directory listing.
	if err := step("navigate forward", func() error { return rt.ClickControl("forward") }); err != nil {
		return err
	}
	return step("wait for starter row after forward", func() error { return rt.ExpectVisible(starterFile) })
}

func driveNestedHome(_ context.Context, rt runtime.Runtime) error {
	step := func(name string, fn func() error) error {
		if err := fn(); err != nil {
			return errors.Wrap(err, name)
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
	return driveUploadFile(rt, "e2e-upload.txt")
}

func driveUploadFile(rt runtime.Runtime, name string) error {
	// Open a ready Drive for the named file upload.
	if err := prepareDrive(rt); err != nil {
		return err
	}

	// Submit the named text file to the Drive upload control.
	file := runtime.File{
		Name:     name,
		MIMEType: "text/plain",
		Contents: []byte("e2e upload contents"),
	}
	if err := rt.UploadFile("input[type='file']", file); err != nil {
		return err
	}

	// Require upload completion and the resulting Drive entry.
	if err := rt.ExpectVisible("1/1 uploaded"); err != nil {
		return errors.Wrap(err, "upload completion")
	}
	return rt.ExpectVisible(name)
}

func driveRowMove(_ context.Context, rt runtime.Runtime) error {
	// Upload the source file for the Drive move scenario.
	source := "e2e-move-source.txt"
	if err := driveUploadFile(rt, source); err != nil {
		return err
	}

	// Create the destination folder in the current Drive directory.
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

	// Move the source file and require its removal from the current directory.
	if err := rt.MoveContent(source, "e2e-move-target"); err != nil {
		return err
	}
	if err := rt.ExpectAbsent(source); err != nil {
		return err
	}

	// Open the destination folder and require the moved source file.
	if err := rt.DoubleClickContent("e2e-move-target"); err != nil {
		return err
	}
	return rt.ExpectVisible(source)
}

func driveSpaceDelete(_ context.Context, rt runtime.Runtime) error {
	if err := prepareDrive(rt); err != nil {
		return err
	}
	return rt.DeleteSpace()
}
