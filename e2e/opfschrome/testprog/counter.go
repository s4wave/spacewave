//go:build js

package main

import (
	"context"
	"encoding/binary"
	"syscall/js"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/opfs"
	"github.com/s4wave/spacewave/db/opfs/filelock"
)

// runCounterInit creates and flushes the counter used by cross-worker lock probes.
func runCounterInit(c *config) error {
	// Lock the counter file.
	dir, err := openTestDirectory(c.root, []string{"locks"})
	if err != nil {
		return err
	}
	file, release, err := filelock.AcquireFile(dir, "counter", c.root+"/locks", true)
	if err != nil {
		return err
	}
	defer release()

	// Write and flush a zero value.
	var zero [8]byte
	if err := file.Truncate(int64(len(zero))); err != nil {
		return err
	}
	if _, err := file.WriteAt(zero[:], 0); err != nil {
		return err
	}
	return file.Flush()
}

// runCounterHold holds the counter's exclusive file lock until the harness releases it.
func runCounterHold(c *config) error {
	// Lock and read the counter, then hold it until released.
	dir, err := openTestDirectory(c.root, []string{"locks"})
	if err != nil {
		return err
	}
	file, release, err := filelock.AcquireFile(dir, "counter", c.root+"/locks", true)
	if err != nil {
		return errors.Wrap(err, "acquire held counter")
	}
	defer release()
	var buf [8]byte
	if _, err := file.ReadAt(buf[:], 0); err != nil {
		return errors.Wrap(err, "read held counter")
	}
	return waitCounterRelease(c)
}

// runCounterIncrement flushes each counter increment while holding its exclusive lock.
func runCounterIncrement(c *config) error {
	// Open the lock directory, then increment under the lock each iteration.
	dir, err := openTestDirectory(c.root, []string{"locks"})
	if err != nil {
		return err
	}
	for range c.iterations {
		if err := incrementCounter(dir, c); err != nil {
			return err
		}
	}
	return nil
}

// incrementCounter adds one to the counter in dir while holding its exclusive
// lock, flushing before the release.
func incrementCounter(dir js.Value, c *config) error {
	// Lock and read the counter.
	file, release, err := filelock.AcquireFile(dir, "counter", c.root+"/locks", true)
	if err != nil {
		return errors.Wrap(err, "acquire counter")
	}
	defer release()
	var buf [8]byte
	if _, err := file.ReadAt(buf[:], 0); err != nil {
		return errors.Wrap(err, "read counter")
	}

	// Write and flush the incremented value.
	binary.LittleEndian.PutUint64(buf[:], binary.LittleEndian.Uint64(buf[:])+1)
	if _, err := file.WriteAt(buf[:], 0); err != nil {
		return errors.Wrap(err, "write counter")
	}
	return errors.Wrap(file.Flush(), "flush counter")
}

// runCounterTryLock checks the nonblocking Web Lock acquisition outcome.
func runCounterTryLock(c *config, want bool) error {
	// Try the lock and compare the outcome, releasing it if taken.
	release, acquired, err := filelock.AcquireWebLockIfAvailable(c.root+"/locks/counter", true)
	if err != nil {
		return err
	}
	if acquired != want {
		return errors.Errorf("try counter lock acquired=%v want %v", acquired, want)
	}
	if release != nil {
		release()
	}
	return nil
}

// runCounterTimeoutLock requires a queued Web Lock request to honor cancellation.
func runCounterTimeoutLock(ctx context.Context, c *config) error {
	// Request the held lock with a short deadline. The request must end
	// canceled.
	ctx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	result, err := opfs.DefaultDriver.AcquireWebLock(ctx, c.root+"/locks/counter", true)
	if err == nil {
		if result != nil && result.Release != nil {
			result.Release()
		}
		return errors.New("blocking WebLock unexpectedly acquired before timeout")
	}
	if result == nil || result.Outcome != opfs.WebLockOutcomeCanceled {
		return errors.Errorf("WebLock timeout outcome=%v err=%v", result, err)
	}
	return nil
}

// waitCounterRelease announces readiness and waits for the harness release message.
func waitCounterRelease(c *config) error {
	// Listen for the release message.
	ch := make(chan struct{}, 1)
	bc := js.Global().Get("BroadcastChannel").New(counterReleaseChannel(c.root))
	cb := js.FuncOf(func(this js.Value, args []js.Value) any {
		data := args[0].Get("data")
		if data.Get("type").String() == "release" {
			ch <- struct{}{}
		}
		return nil
	})
	defer cb.Release()
	defer bc.Call("close")
	bc.Set("onmessage", cb)

	// Announce readiness and wait.
	postReady(c)
	<-ch
	return nil
}

// runCounterVerify checks that serialized increments preserved every writer's update.
func runCounterVerify(c *config) error {
	// Lock the counter shared.
	dir, err := openTestDirectory(c.root, []string{"locks"})
	if err != nil {
		return err
	}
	file, release, err := filelock.AcquireFile(dir, "counter", c.root+"/locks", false)
	if err != nil {
		return err
	}
	defer release()

	// Check that every increment landed.
	var buf [8]byte
	if _, err := file.ReadAt(buf[:], 0); err != nil {
		return err
	}
	got := binary.LittleEndian.Uint64(buf[:])
	want := uint64(c.workers * c.iterations)
	if got != want {
		return errors.Errorf("counter=%d want=%d", got, want)
	}
	return nil
}

// openTestDirectory creates the requested descendant under the disposable test root.
func openTestDirectory(rootName string, parts []string) (js.Value, error) {
	// Resolve the path from the OPFS root.
	root, err := opfs.GetRoot()
	if err != nil {
		return js.Undefined(), err
	}
	path := append([]string{rootName}, parts...)
	return opfs.GetDirectoryPath(root, path, true)
}

// counterReleaseChannel derives the lock-holder release channel from the test root.
func counterReleaseChannel(root string) string {
	return "opfs-chrometest-counter-release:" + root
}
