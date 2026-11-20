//go:build linux

// Command spacewave-init boots the immutable UnixFS experiment or runs its
// probe. On a build host it also seeds an image store and writes its card.
package main

import (
	"context"
	"errors"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/os/boot"
	os_image "github.com/s4wave/spacewave/os/image"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

// main selects bootstrap, adapter helper or separately executed workload mode.
func main() {
	// Dispatch the helper before changing namespaces or bootstrap mounts.
	if filepath.Base(os.Args[0]) == "fusermount3" {
		finish(boot.RunMountHelper(os.Args[1:]))
		return
	}

	// Enter a served root only from an independent bootstrap exec.
	if len(os.Args) == 4 && os.Args[1] == "enter" {
		finish(boot.EnterRoot(os.Args[2], os.Args[3]))
		return
	}
	if len(os.Args) == 3 && os.Args[1] == "enter-console" {
		finish(boot.EnterConsole(os.Args[2]))
		return
	}
	if (len(os.Args) == 3 || len(os.Args) == 4 && os.Args[3] == "grant") && os.Args[1] == "enter-daemon" {
		finish(boot.EnterDaemon(os.Args[2], len(os.Args) == 4))
		return
	}
	if len(os.Args) == 2 && (os.Args[1] == "workload" || os.Args[1] == "workload-fail" || os.Args[1] == "workload-wait") {
		finish(boot.RunWorkload(os.Args[1]))
		return
	}

	// Run a store command, which neither boots nor mounts.
	if len(os.Args) >= 2 {
		if cmd, ok := storeCommands[os.Args[1]]; ok {
			args := os.Args[2:]
			if len(args) < cmd.min || len(args) > cmd.max {
				finish(errors.New("usage: spacewave-init " + os.Args[1] + " " + cmd.usage))
				return
			}
			finish(cmd.run(args))
			return
		}
	}

	// A real PID1 prepares only the independent bootstrap's kernel interfaces.
	pid1 := os.Getpid() == 1
	console := os.Getenv("SPACEWAVE_OS_MODE") == "console" || (len(os.Args) > 1 && os.Args[1] == "console")
	if console && !pid1 {
		finish(errors.New("console requires actual guest PID1; use probe for hosted checks"))
		return
	}
	store := os.Getenv("SPACEWAVE_OS_STORE")
	if pid1 {
		finish(prepareBootstrap(store != "", os.Getenv("SPACEWAVE_OS_CONFIG") != ""))
		println("SPACEWAVE_OS_PID1 gomaxprocs=" + strconv.Itoa(runtime.GOMAXPROCS(0)))
	} else {
		// Mount namespaces are per-thread. Keep this goroutine on its private
		// namespace through helper exec, workload wait and all deferred unmounts.
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		finish(unix.Unshare(unix.CLONE_NEWNS))
		finish(unix.Mount("", "/", "", unix.MS_REC|unix.MS_PRIVATE, ""))
		if len(os.Args) == 3 {
			store = os.Args[2]
		}
	}

	// The console takes BusyBox poweroff (SIGUSR2) and reboot (SIGTERM) as
	// requests, separate from cancellation that disconnects failed filesystem
	// service. A hosted probe stops on SIGTERM or SIGINT.
	ctx := context.Background()
	requests := make(chan os.Signal, 1)
	if console {
		signal.Notify(requests, syscall.SIGUSR2, syscall.SIGTERM, syscall.SIGINT)
	} else {
		var cancel context.CancelFunc
		ctx, cancel = signal.NotifyContext(ctx, syscall.SIGTERM, syscall.SIGINT)
		defer cancel()
	}

	// Select the bounded workload behavior, then run its ordinary supervisor.
	workload := "workload"
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "probe-fail":
			workload = "workload-fail"
		case "probe-wait":
			workload = "workload-wait"
		}
	}

	// Interactive mode is guest-only; hosted probes retain their bounded lifecycle.
	var err error
	stop := boot.PowerOff
	if console {
		terminal := os.Getenv("SPACEWAVE_OS_TTY")
		if terminal == "" {
			terminal = "/dev/ttyS0"
		}
		stop, err = boot.RunConsole(ctx, requests, store, os.Getenv("SPACEWAVE_OS_DATA"), os.Getenv("SPACEWAVE_OS_CONFIG"), terminal,
			os.Getenv("SPACEWAVE_OS_NATIVE_BENCH") == "1")
	} else {
		err = boot.Run(ctx, workload, store)
	}
	if pid1 && store != "" {
		// Console cleanup joined users and closed every store before backing unmount.
		paths := []string{"/store"}
		if os.Getenv("SPACEWAVE_OS_CONFIG") != "" {
			paths = []string{"/config-store", "/store"}
		}
		for _, path := range paths {
			unmountErr := unix.Unmount(path, 0)
			err = errors.Join(err, unmountErr)
			if unmountErr == nil {
				println("SPACEWAVE_OS_BACKING_UNMOUNT_OK path=" + path)
			}
		}
	}
	if err != nil {
		println("SPACEWAVE_OS_FAILED: " + err.Error())
		if console && stop != boot.Reboot {
			finish(err)
		}
	} else {
		println("SPACEWAVE_OS_PASS")
	}
	if pid1 {
		unix.Sync()
		if stop == boot.Reboot {
			println("SPACEWAVE_OS_REBOOT")
			finish(unix.Reboot(unix.LINUX_REBOOT_CMD_RESTART))
		} else {
			println("SPACEWAVE_OS_POWEROFF")
			finish(unix.Reboot(unix.LINUX_REBOOT_CMD_POWER_OFF))
		}
		select {}
	}
	finish(err)
}

// storeCommand is a command on image and data stores, run on a build host or,
// for stage and accept-trial, in a booted guest: its argument usage, its
// argument count bounds and its body.
type storeCommand struct {
	usage    string
	min, max int
	run      func(args []string) error
}

// storeCommands seeds, cards and stages image stores and seeds data stores.
var storeCommands = map[string]storeCommand{
	"seed": {"ABSENT-DIRECTORY STATIC-BUSYBOX [ASSETS.tar]", 2, 3, func(args []string) error {
		archive := ""
		if len(args) == 3 {
			archive = args[2]
		}
		return seedImage(args[0], args[1], archive)
	}},
	"rehearsal-card": {"IMAGE-DIRECTORY KEY", 2, 2, func(args []string) error {
		return rehearsalCard(args[0], args[1])
	}},
	"rehearsal-sign": {"IMAGE-DIRECTORY KEY AUTHORIZATION", 3, 3, func(args []string) error {
		return rehearsalSign(args[0], args[1], args[2])
	}},
	"install-card": {"IMAGE-DIRECTORY SIGNERS AUTHORIZATION GRANT", 4, 4, func(args []string) error {
		return installCard(args[0], args[1], args[2], args[3])
	}},
	"stage": {"IMAGE-DIRECTORY SOURCE-IMAGE-DIRECTORY AUTHORIZATION", 3, 3, func(args []string) error {
		return stageImage(args[0], args[1], args[2])
	}},
	"accept-trial": {"IMAGE-DIRECTORY ROOT ACCEPTED TRIAL", 4, 4, boot.RunAcceptTrial},
	"seed-data": {"ABSENT-DIRECTORY", 1, 1, func(args []string) error {
		return seedData(args[0])
	}},
}

// prepareBootstrap mounts independent kernel interfaces and optional backing data.
func prepareBootstrap(persistent, configuration bool) error {
	// Keep all guest mounts private, including kernel mounts nested under UnixFS.
	if err := unix.Mount("", "/", "", unix.MS_REC|unix.MS_PRIVATE, ""); err != nil {
		return err
	}

	// Create kernel mountpoints on the independent initramfs.
	for _, dir := range []string{"/proc", "/dev", "/tmp"} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	if err := unix.Mount("proc", "/proc", "proc", 0, ""); err != nil {
		return err
	}
	if err := unix.Mount("devtmpfs", "/dev", "devtmpfs", 0, ""); err != nil {
		return err
	}

	// Load only kernel content packaged for this matching kernel.
	return boot.PrepareBackingStore(persistent, configuration)
}

// seedData initializes only an absent disposable data fixture through CreateData.
func seedData(dir string) error {
	// Complete the normal Data close fence before reporting fixture creation.
	d, err := boot.CreateData(context.Background(), logrus.NewEntry(logrus.New()), dir)
	if err != nil {
		return err
	}
	if err := d.Close(); err != nil {
		return err
	}
	println("SPACEWAVE_OS_DATA_SEEDED")
	return nil
}

// seedImage imports the bootstrap probe and static tools and verifies a
// reopen. The image carries no card: a rehearsal or an install writes one.
func seedImage(dir, busyboxPath, archivePath string) (retErr error) {
	// Keep the seed input outside the image it will later execute.
	path, err := os.Executable()
	if err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, file.Close()) }()

	// Stream the caller's independent tool file without executing or collecting it.
	busybox, err := os.Open(busyboxPath) //nolint:gosec // The operator selects the tool input, outside any served filesystem.
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, busybox.Close()) }()

	// Import only the caller's explicit application archive into this fresh image.
	var archive *os.File
	var assets io.Reader
	if archivePath != "" {
		archive, err = os.Open(archivePath) //nolint:gosec // The operator selects the archive input, outside any served filesystem.
		if err != nil {
			return err
		}
		defer func() { retErr = errors.Join(retErr, archive.Close()) }()
		assets = archive
	}
	le := logrus.NewEntry(logrus.New())
	if err := boot.SeedRoot(context.Background(), le, dir, file, busybox, assets); err != nil {
		return err
	}

	// Reopen rather than verify the seed's in-memory snapshot.
	built, err := boot.ReadImageRoot(context.Background(), dir)
	if err != nil {
		return err
	}
	root, err := boot.OpenRoot(context.Background(), le, dir, built)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, root.Close()) }()
	if _, err := file.Seek(0, 0); err != nil {
		return err
	}
	return root.VerifyImage(context.Background(), file)
}

// rehearsalCard authorizes the image in dir with the throwaway key in
// keyPath, created when absent, so a rehearsal boot runs the same pin and
// check as an installed card signed by the Space owner, and later rehearsal
// revisions can be signed by the same key.
func rehearsalCard(dir, keyPath string) error {
	// Sign the built root with a key no other image trusts.
	ctx := context.Background()
	root, err := boot.ReadImageRoot(ctx, dir)
	if err != nil {
		return err
	}
	key, err := rehearsalKey(keyPath)
	if err != nil {
		return err
	}
	signed, err := os_image.SignImageRoot(key, root)
	if err != nil {
		return err
	}

	// Write the signers, then publish the authorization they verify.
	signer, err := peer.IDFromPrivateKey(key)
	if err != nil {
		return err
	}
	if err := os_image.WriteSigners(filepath.Join(dir, os_image.CardSigners), []peer.ID{signer}); err != nil {
		return err
	}
	return boot.AuthorizeImage(ctx, logrus.NewEntry(logrus.New()), dir, signed, root)
}

// rehearsalSign writes to authPath the rehearsal key's authorization of the
// root the image in dir was built with, as the sign Step writes the owner's.
func rehearsalSign(dir, keyPath, authPath string) error {
	// Read the built root and the existing rehearsal key.
	root, err := boot.ReadImageRoot(context.Background(), dir)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(keyPath) //nolint:gosec // The operator names the rehearsal key.
	if err != nil {
		return err
	}
	key, err := crypto.UnmarshalPrivateKey(data)
	if err != nil {
		return err
	}

	// Sign the root and write the authorization.
	signed, err := os_image.SignImageRoot(key, root)
	if err != nil {
		return err
	}
	return os_image.WriteAuthorization(authPath, signed)
}

// rehearsalKey reads the rehearsal key at path, first generating and writing
// one when the file is absent.
func rehearsalKey(path string) (crypto.PrivKey, error) {
	// Reuse an existing key.
	data, err := os.ReadFile(path) //nolint:gosec // The operator names the rehearsal key.
	if err == nil {
		return crypto.UnmarshalPrivateKey(data)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	// Generate and keep a new one.
	_, key, _, err := peer.NewPeerWithGenerateED25519()
	if err != nil {
		return nil, err
	}
	data, err = crypto.MarshalPrivateKey(key)
	if err != nil {
		return nil, err
	}
	return key, os.WriteFile(path, data, 0o600) //nolint:gosec // The operator names the rehearsal key.
}

// stageImage stages the image a seed built in source into the image store in
// dir as its trial, with the authorization in authPath, copying only the
// blocks dir lacks. The next boot tries it once.
func stageImage(dir, source, authPath string) (retErr error) {
	// Read the authorization and open the source at its built root.
	signed, err := os_image.ReadAuthorization(authPath)
	if err != nil {
		return err
	}
	ctx, le := context.Background(), logrus.NewEntry(logrus.New())
	src, root, err := boot.OpenImageStore(ctx, le, source)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, src.Close()) }()

	// Copy what the store lacks and publish the trial.
	copied, err := boot.StageRevision(ctx, le, dir, src, signed, root)
	println("SPACEWAVE_OS_STAGE_COPIED blocks=" + strconv.Itoa(copied))
	return err
}

// installCard writes the installable card for the image in dir, the Space's
// signers and its bootstrap grant, then publishes the owner's authorization,
// which must name the image's built root, as the accepted revision.
func installCard(dir, signersPath, authorizationPath, grantPath string) error {
	// Read the card inputs and the root they must authorize.
	signers, err := os_image.ReadSigners(signersPath)
	if err != nil {
		return err
	}
	signed, err := os_image.ReadAuthorization(authorizationPath)
	if err != nil {
		return err
	}
	grant, err := os_image.ReadGrant(grantPath)
	if err != nil {
		return err
	}

	// Read the built root the authorization must name.
	ctx := context.Background()
	root, err := boot.ReadImageRoot(ctx, dir)
	if err != nil {
		return err
	}

	// Verify and write the card, then authorize the image last.
	if err := os_image.InstallCard(dir, signers, signed, grant, root, time.Now()); err != nil {
		return err
	}
	if err := boot.AuthorizeImage(ctx, logrus.NewEntry(logrus.New()), dir, signed, root); err != nil {
		return err
	}
	println("SPACEWAVE_OS_CARD_INSTALLED root=" + root.MarshalString() + " space=" + grant.GetSharedObjectId())
	return nil
}

// finish reports a command failure and prevents a failed bootstrap continuing.
func finish(err error) {
	if err != nil {
		println("SPACEWAVE_OS_ERROR: " + err.Error())
		if os.Getpid() == 1 {
			// Preserve the stopped bootstrap and serial evidence for recovery.
			println("Recovery: stores remain in place; this bootstrap will not reset or reseed them.")
			println("Exit QEMU with Ctrl-a x. Check the error and fixture inputs, then rerun the same run-console.sh command to retry.")
			for {
				unix.Pause()
			}
		}
		os.Exit(1)
	}
}
