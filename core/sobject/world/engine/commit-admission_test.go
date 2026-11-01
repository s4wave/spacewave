//go:build !js

package sobject_world_engine_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
)

// TestCommitForksFromAdmittedWorld holds the publication of one write and
// checks that CommitAdmit returns at admission, that readers keep the durable
// World while the next write reads the admitted one and is admitted after it,
// and that WaitDurable and each Commit return once the publication is durable.
func TestCommitForksFromAdmittedWorld(t *testing.T) {
	// Start a World and note its durable and admitted positions.
	w, so := startGatedWorld(t, t.TempDir(), nil)
	before := admittedOps(t, so)
	durable := worldSeqno(t, w)

	// Hold the publication of the first write and admit it.
	applying, release := so.hold(nil)
	defer release()
	first, err := world.CommitAdmit(t.Context(), w.newWrite(t, "admit/1"))
	if err != nil {
		t.Fatal(err)
	}
	defer first.Release()
	waitReached(t, applying)

	// Readers see the durable World without it.
	read := newRead(t, w)
	if worldHas(t, read, "admit/1") {
		t.Fatal("a reader sees the write before it is durable")
	}
	read.Discard()
	if seqno := worldSeqno(t, w); seqno != durable {
		t.Fatalf("durable seqno %d before the publication, want %d", seqno, durable)
	}

	// The second write forks from the first's admitted World.
	tx := w.newWrite(t, "admit/2")
	if found := worldHas(t, tx, "admit/1"); !found {
		t.Fatal("second write does not see the admitted first write")
	}
	second := commitAsync(t, tx)
	waitAdmittedOps(t, so, before+2)

	// The second Commit does not return before the held publication.
	select {
	case err := <-second:
		t.Fatalf("second Commit returned before its receipt: %v", err)
	default:
	}

	// Both finish once it applies, and the World holds both objects.
	release()
	if seqno, err := first.WaitDurable(t.Context()); err != nil || seqno != first.Seqno() || seqno <= durable {
		t.Fatalf("first WaitDurable = %d, %v, want its seqno %d above %d", seqno, err, first.Seqno(), durable)
	}
	if err := <-second; err != nil {
		t.Fatalf("second Commit: %v", err)
	}
	read = newRead(t, w)
	defer read.Discard()
	for _, key := range []string{"admit/1", "admit/2"} {
		if !worldHas(t, read, key) {
			t.Fatalf("%s missing after both Commits", key)
		}
	}
}

// TestCommitReceiptFailure fails the held publication of one write after a
// second write was admitted after it. Both writes fail, the engine reinstalls
// the World from before them and reports its seqno to the reinstall hooks
// once, and the next write builds on that World.
func TestCommitReceiptFailure(t *testing.T) {
	// Watch the reinstalls through a wrapper, as a bus engine's user does.
	w, so := startGatedWorld(t, t.TempDir(), nil)
	reinstalled := make(chan uint64, 8)
	unhook, err := world.OnReinstall(t.Context(), world.NewReadTrackingEngine(w.eng), func(seqno uint64) {
		reinstalled <- seqno
	})
	if err != nil {
		t.Fatal(err)
	}
	defer unhook()

	// Write a durable World and note its position.
	w.create(t, "fail/0")
	durable := worldSeqno(t, w)
	before := admittedOps(t, so)

	// Admit two writes after it, holding the first publication.
	injected := errors.New("injected publication failure")
	applying, release := so.hold(injected)
	defer release()
	first, err := world.CommitAdmit(t.Context(), w.newWrite(t, "fail/1"))
	if err != nil {
		t.Fatal(err)
	}
	defer first.Release()
	waitReached(t, applying)

	// Admit the second after it.
	second := commitAsync(t, w.newWrite(t, "fail/2"))
	waitAdmittedOps(t, so, before+2)

	// Nothing reinstalls before the publication fails.
	select {
	case seqno := <-reinstalled:
		t.Fatalf("reinstall at %d before the publication failed", seqno)
	default:
	}

	// Fail it: both writes fail, and the hooks get the World before them.
	release()
	if seqno, err := first.WaitDurable(t.Context()); !errors.Is(err, injected) || seqno != durable {
		t.Fatalf("first WaitDurable = %d, %v, want %d and the injected failure", seqno, err, durable)
	}
	if err := <-second; !errors.Is(err, injected) || !errors.Is(err, block.ErrPublicationDependency) {
		t.Fatalf("second Commit: %v, want the dependency on the injected failure", err)
	}
	if seqno := <-reinstalled; seqno != durable {
		t.Fatalf("reinstall at %d, want %d", seqno, durable)
	}

	// Readers kept the World from before them.
	read := newRead(t, w)
	for key, found := range map[string]bool{"fail/0": true, "fail/1": false, "fail/2": false} {
		if worldHas(t, read, key) != found {
			t.Fatalf("%s present = %v after the failed Commits, want %v", key, !found, found)
		}
	}
	read.Discard()

	// The next write commits on it, and nothing else was reinstalled.
	w.create(t, "fail/3")
	read = newRead(t, w)
	defer read.Discard()
	for _, key := range []string{"fail/0", "fail/3"} {
		if !worldHas(t, read, key) {
			t.Fatalf("%s missing after the next Commit", key)
		}
	}
	select {
	case seqno := <-reinstalled:
		t.Fatalf("second reinstall at %d", seqno)
	default:
	}
}

// TestWaitDurableSkipsHooks fails the publication of an admitted write
// while the test holds a consumer lock the reinstall hook blocks on, as a
// store that applies the World under its own lock does. The lock order is the
// engine's writer lock first, so the lock is taken after CommitAdmit.
// WaitDurable returns the failure and the seqno readers kept without waiting
// for the hook, the receipt's watcher runs the hook once the lock is free,
// and the next write commits on the World from before the failed one.
func TestWaitDurableSkipsHooks(t *testing.T) {
	// Register a hook that blocks on the consumer lock, then briefly.
	w, so := startGatedWorld(t, t.TempDir(), nil)
	var consumer sync.Mutex
	reinstalled := make(chan uint64, 8)
	unhook, err := world.OnReinstall(t.Context(), w.eng, func(seqno uint64) {
		consumer.Lock()
		defer consumer.Unlock()
		time.Sleep(20 * time.Millisecond)
		reinstalled <- seqno
	})
	if err != nil {
		t.Fatal(err)
	}
	defer unhook()

	// Write a durable World and note its seqno.
	w.create(t, "hook/0")
	durable := worldSeqno(t, w)

	// Admit a write behind a held publication.
	injected := errors.New("injected publication failure")
	applying, release := so.hold(injected)
	defer release()
	adm, err := world.CommitAdmit(t.Context(), w.newWrite(t, "hook/1"))
	if err != nil {
		t.Fatal(err)
	}
	defer adm.Release()
	waitReached(t, applying)

	// Take the consumer lock.
	consumer.Lock()
	locked := true
	defer func() {
		if locked {
			consumer.Unlock()
		}
	}()

	// Fail it: WaitDurable returns while the hook waits for the lock.
	release()
	waited := make(chan error, 1)
	var kept uint64
	go func() {
		var err error
		kept, err = adm.WaitDurable(t.Context())
		waited <- err
	}()
	select {
	case err := <-waited:
		if !errors.Is(err, injected) || kept != durable {
			t.Fatalf("WaitDurable = %d, %v, want %d and the injected failure", kept, err, durable)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("WaitDurable waited for the reinstall hook")
	}
	if seqno := worldSeqno(t, w); seqno != durable {
		t.Fatalf("readers at %d after the failure, want %d", seqno, durable)
	}
	select {
	case seqno := <-reinstalled:
		t.Fatalf("hook returned at %d while the consumer lock was held", seqno)
	default:
	}

	// Free the lock: the watcher's hook reports the World before the write.
	consumer.Unlock()
	locked = false
	select {
	case seqno := <-reinstalled:
		if seqno != durable {
			t.Fatalf("reinstall at %d, want %d", seqno, durable)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the watcher did not run the hook")
	}

	// The next write commits on it, and the hook ran once.
	w.create(t, "hook/2")
	read := newRead(t, w)
	defer read.Discard()
	for key, found := range map[string]bool{"hook/0": true, "hook/1": false, "hook/2": true} {
		if worldHas(t, read, key) != found {
			t.Fatalf("%s present = %v, want %v", key, !found, found)
		}
	}
	select {
	case seqno := <-reinstalled:
		t.Fatalf("second reinstall at %d", seqno)
	default:
	}
}

// TestCommitAfterFailedBase forks a write on the admitted World of a write
// whose publication then fails, while the second write's body holds a
// consumer lock the reinstall hook blocks on, as Hyperline's SQLite writer
// is. Its CommitAdmit fails at once without running the hook, it is never
// replayed onto the reinstalled World, the hook runs once on the receipt's
// watcher after the body releases the lock, and the next write commits.
func TestCommitAfterFailedBase(t *testing.T) {
	// Register a hook that blocks on the consumer lock.
	w, so := startGatedWorld(t, t.TempDir(), nil)
	var consumer sync.Mutex
	reinstalled := make(chan uint64, 8)
	unhook, err := world.OnReinstall(t.Context(), w.eng, func(seqno uint64) {
		consumer.Lock()
		defer consumer.Unlock()
		reinstalled <- seqno
	})
	if err != nil {
		t.Fatal(err)
	}
	defer unhook()

	// Write a durable World and note its seqno.
	w.create(t, "base/0")
	durable := worldSeqno(t, w)

	// Admit A behind a held publication.
	injected := errors.New("injected publication failure")
	applying, release := so.hold(injected)
	defer release()
	first, err := world.CommitAdmit(t.Context(), w.newWrite(t, "base/1"))
	if err != nil {
		t.Fatal(err)
	}
	defer first.Release()
	waitReached(t, applying)

	// Fork B on A's admitted World.
	tx := w.newWrite(t, "base/2")
	defer tx.Discard()
	if !worldHas(t, tx, "base/1") {
		t.Fatal("B does not see the admitted A")
	}

	// B's body takes the consumer lock, then A fails.
	consumer.Lock()
	locked := true
	defer func() {
		if locked {
			consumer.Unlock()
		}
	}()
	release()
	if _, err := first.WaitDurable(t.Context()); !errors.Is(err, injected) {
		t.Fatalf("A WaitDurable: %v, want the injected failure", err)
	}

	// B's CommitAdmit fails without running the hook.
	committed := make(chan error, 1)
	go func() {
		adm, err := world.CommitAdmit(t.Context(), tx)
		if err == nil {
			_, err = adm.WaitDurable(t.Context())
			adm.Release()
			if err == nil {
				err = errors.New("B committed on a dropped base")
			}
			committed <- errors.Wrap(err, "admitted")
			return
		}
		committed <- err
	}()
	select {
	case err := <-committed:
		if !errors.Is(err, injected) || !errors.Is(err, block.ErrPublicationDependency) {
			t.Fatalf("B CommitAdmit: %v, want the dependency on the injected failure", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("B's CommitAdmit waited for the reinstall hook")
	}
	select {
	case seqno := <-reinstalled:
		t.Fatalf("hook returned at %d while B held the consumer lock", seqno)
	default:
	}

	// B releases the lock: the watcher's hook reports the World before A.
	consumer.Unlock()
	locked = false
	select {
	case seqno := <-reinstalled:
		if seqno != durable {
			t.Fatalf("reinstall at %d, want %d", seqno, durable)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the watcher did not run the hook")
	}

	// The next write commits on that World, without A or B.
	w.create(t, "base/3")
	read := newRead(t, w)
	defer read.Discard()
	for key, found := range map[string]bool{"base/0": true, "base/1": false, "base/2": false, "base/3": true} {
		if worldHas(t, read, key) != found {
			t.Fatalf("%s present = %v, want %v", key, !found, found)
		}
	}
	select {
	case seqno := <-reinstalled:
		t.Fatalf("second reinstall at %d", seqno)
	default:
	}
}

// TestCommitAdmissionCrash cuts the process after two writes are admitted
// while the first publication is held. A group of publications commits
// atomically, so the restarted engine reads the World from before both, which
// holds the durable write of an earlier process, and commits again.
func TestCommitAdmissionCrash(t *testing.T) {
	const roleEnv = "SPACEWAVE_COMMIT_ADMISSION_CRASH_ROLE"
	const dirEnv = "SPACEWAVE_COMMIT_ADMISSION_CRASH_DIR"
	if role := os.Getenv(roleEnv); role != "" {
		commitAdmissionCrashWorker(t, role, os.Getenv(dirEnv))
		return
	}

	// Run each step in its own process.
	dir := t.TempDir()
	for _, step := range []struct {
		role string
		code int
	}{{"seed", 0}, {"admitted", 74}, {"verify", 0}} {
		ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCommitAdmissionCrash$", "-test.timeout=50s")
		cmd.Env = append(os.Environ(), roleEnv+"="+step.role, dirEnv+"="+dir)
		out, err := cmd.CombinedOutput()
		cancel()
		if cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != step.code {
			t.Fatalf("%s: %v\n%s", step.role, err, out)
		}
	}
}

// commitAdmissionCrashWorker runs one step of the admission crash test on the
// s4db storage under dir.
func commitAdmissionCrashWorker(t *testing.T, role, dir string) {
	// Read the SharedObject the seed step created.
	refPath := filepath.Join(dir, "so-ref")
	storage := filepath.Join(dir, "storage")
	var soRef *sobject.SharedObjectRef
	if role != "seed" {
		data, err := os.ReadFile(refPath)
		if err != nil {
			t.Fatal(err)
		}
		soRef = &sobject.SharedObjectRef{}
		if err := soRef.UnmarshalVT(data); err != nil {
			t.Fatal(err)
		}
	}

	switch role {
	// Seed one committed object and save the SharedObject.
	case "seed":
		if err := os.MkdirAll(storage, 0o700); err != nil {
			t.Fatal(err)
		}
		w := startLocalWorld(t, storage, nil)
		w.create(t, "crash/0")
		data, err := w.soRef.MarshalVT()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(refPath, data, 0o600); err != nil {
			t.Fatal(err)
		}

	// Admit two writes behind a held publication and cut.
	case "admitted":
		w, so := startGatedWorld(t, storage, soRef)
		before := admittedOps(t, so)
		applying, _ := so.hold(nil)
		commitAsync(t, w.newWrite(t, "crash/1"))
		waitReached(t, applying)
		commitAsync(t, w.newWrite(t, "crash/2"))
		waitAdmittedOps(t, so, before+2)
		os.Exit(74)

	// Read the World from before both writes, then commit again.
	case "verify":
		w := startLocalWorld(t, storage, soRef)
		read := newRead(t, w)
		want := map[string]bool{"crash/0": true, "crash/1": false, "crash/2": false}
		for key, found := range want {
			if got := worldHas(t, read, key); got != found {
				t.Fatalf("%s: found = %v, want %v", key, got, found)
			}
		}
		read.Discard()
		w.create(t, "crash/next")

	default:
		t.Fatalf("unknown role %q", role)
	}
}

// TestEngineReplacedWithAdmissions stops the engine while two admitted writes
// wait for a held publication, as losing read access does, and serves the World
// again. The new engine starts after the publication fails, reads the World
// from before the writes and commits again.
func TestEngineReplacedWithAdmissions(t *testing.T) {
	// Admit two writes behind a held publication, then stop the engine.
	w, so := startGatedWorld(t, t.TempDir(), nil)
	w.create(t, "replace/0")
	before := admittedOps(t, so)
	injected := errors.New("injected publication failure")
	applying, release := so.hold(injected)
	first := commitAsync(t, w.newWrite(t, "replace/1"))
	waitReached(t, applying)
	second := commitAsync(t, w.newWrite(t, "replace/2"))
	waitAdmittedOps(t, so, before+2)
	so.stop()

	// The replacement waits for the publication, which fails.
	so.serve(t)
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	if _, err := so.ctrl.GetWorldEngine(ctx); err == nil {
		t.Fatal("replacement engine served while the publication was held")
	}
	release()
	eng, err := so.ctrl.GetWorldEngine(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	w.eng = eng
	for _, done := range []<-chan error{first, second} {
		if err := <-done; err == nil {
			t.Fatal("Commit of the stopped engine succeeded")
		}
	}

	// It reads the World from before the writes, and commits on it.
	read := newRead(t, w)
	for key, found := range map[string]bool{"replace/0": true, "replace/1": false, "replace/2": false} {
		if worldHas(t, read, key) != found {
			t.Fatalf("%s present = %v in the replacement engine, want %v", key, !found, found)
		}
	}
	read.Discard()
	w.create(t, "replace/3")
}

// TestCheckpointUnderOverlappingWrites holds the publication of the write that
// makes MinCheckpointOperations operations stable. The engine does not
// checkpoint, or hold the writer lock, while it waits for the receipt, and
// adopts nothing when the publication fails. It then writes so that each
// publication is admitted before the one before it lands, as a busy server
// does, and the engine still checkpoints. The World keeps every committed
// write and none of the failed one.
func TestCheckpointUnderOverlappingWrites(t *testing.T) {
	// Write one operation short of a checkpoint. Every operation of the only
	// roster member is stable.
	w, so := startGatedWorld(t, t.TempDir(), nil)
	var n int
	for ; admittedOps(t, so) < sobject.MinCheckpointOperations-1; n++ {
		w.create(t, "checkpoint/"+strconv.Itoa(n))
	}

	// Hold the write that reaches it, and fail it.
	errHeld := errors.New("held publication failed")
	applying, release := so.hold(errHeld)
	failed := "checkpoint/" + strconv.Itoa(n)
	held := commitAsync(t, w.newWrite(t, failed))
	waitReached(t, applying)

	// The writer lock is free.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	write, err := w.eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("write while the publication is held: %v", err)
	}
	write.Discard()

	// The durable state has no checkpoint while it is held.
	durable, relDurable, err := so.AccessSharedObjectState(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relDurable()
	checkHeight := func(when string) {
		checkpoint, err := durable.GetValue().GetCheckpoint(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if height := checkpoint.GetHeight(); height != 0 {
			t.Fatalf("checkpointed at height %d %s", height, when)
		}
	}
	checkHeight("while the publication is held")

	// Fail it: nothing adopts the World it held.
	release()
	if err := <-held; !errors.Is(err, errHeld) {
		t.Fatalf("held commit returned %v, want the publication's failure", err)
	}
	n++
	w.create(t, "checkpoint/"+strconv.Itoa(n))
	checkHeight("on a failed publication")

	// Overlap the writes: admit each before releasing the one before it. A
	// write the checkpoint's state write blocks is not admitted, so the
	// previous one is released after a pause.
	trimmed := make(chan struct{})
	go func() {
		_, err := durable.WaitValueWithValidator(t.Context(), func(snap sobject.SharedObjectStateSnapshot) (bool, error) {
			return checkpointTrimmed(t.Context(), snap)
		}, nil)
		if err == nil {
			close(trimmed)
		}
	}()
	var commits []<-chan error
	prev := func() {}
	for round := 0; ; round++ {
		select {
		case <-trimmed:
		default:
			if round == 4*sobject.MinCheckpointOperations {
				t.Fatal("no checkpoint under overlapping writes")
			}
			n++
			applying, release := so.hold(nil)
			commits = append(commits, commitAsync(t, w.newWrite(t, "checkpoint/"+strconv.Itoa(n))))
			select {
			case <-applying:
			case <-time.After(200 * time.Millisecond):
			}
			prev()
			prev = release
			continue
		}
		break
	}
	prev()
	for _, done := range commits {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}

	// The World holds every committed write and not the failed one.
	read := newRead(t, w)
	defer read.Discard()
	for i := 0; i <= n; i++ {
		key := "checkpoint/" + strconv.Itoa(i)
		if found := worldHas(t, read, key); found != (key != failed) {
			t.Fatalf("%s present = %v, want %v", key, found, !found)
		}
	}
}

// TestCheckpointWithoutWriterLock holds the host lock while the engine
// checkpoints the stable operations. Writes take the writer lock meanwhile,
// and the checkpoint lands once the host lock is free.
func TestCheckpointWithoutWriterLock(t *testing.T) {
	// Write one operation short of a checkpoint and hold the write that
	// reaches it, so the engine does not checkpoint before the host lock is
	// taken.
	w, so := startGatedWorld(t, t.TempDir(), nil)
	var n int
	for ; admittedOps(t, so) < sobject.MinCheckpointOperations-1; n++ {
		w.create(t, "checkpoint/"+strconv.Itoa(n))
	}
	applying, release := so.hold(nil)
	held := commitAsync(t, w.newWrite(t, "checkpoint/"+strconv.Itoa(n)))
	waitReached(t, applying)

	// Take the host lock, which waits for the receipt, and keep it.
	errKeep := errors.New("keep the host lock")
	entered, gate := make(chan struct{}), make(chan struct{})
	kept := make(chan error, 1)
	go func() {
		kept <- so.GetSOHost().UpdateSOState(t.Context(), func(*sobject.SOState) error {
			close(entered)
			<-gate
			return errKeep
		})
	}()
	release()
	if err := <-held; err != nil {
		t.Fatal(err)
	}
	waitReached(t, entered)

	// Writes take the writer lock while the checkpoint waits for the host.
	for start := time.Now(); time.Since(start) < 300*time.Millisecond; {
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		write, err := w.eng.NewTransaction(ctx, true)
		cancel()
		if err != nil {
			t.Fatalf("write while the checkpoint waits for the host lock: %v", err)
		}
		write.Discard()
		time.Sleep(10 * time.Millisecond)
	}

	// Free the host lock: the engine checkpoints the stable operations.
	close(gate)
	if err := <-kept; !errors.Is(err, errKeep) {
		t.Fatalf("host lock hold returned %v", err)
	}
	durable, relDurable, err := so.AccessSharedObjectState(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relDurable()
	_, err = durable.WaitValueWithValidator(t.Context(), func(snap sobject.SharedObjectStateSnapshot) (bool, error) {
		return checkpointTrimmed(t.Context(), snap)
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
}

// commitAsync commits tx in a goroutine and returns the channel its result
// arrives on.
func commitAsync(t *testing.T, tx world.Tx) <-chan error {
	done := make(chan error, 1)
	go func() {
		done <- tx.Commit(t.Context())
	}()
	return done
}

// waitReached waits until a held publication reaches its state head.
func waitReached(t *testing.T, applying <-chan struct{}) {
	t.Helper()
	select {
	case <-applying:
	case <-t.Context().Done():
		t.Fatal(context.Cause(t.Context()))
	}
}

// admittedOps returns the number of operations the admitted state of so holds.
func admittedOps(t *testing.T, so sobject.StatePublisher) int {
	t.Helper()
	ctr, rel, err := so.AccessAdmittedState(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rel()
	set, err := ctr.GetValue().GetOperationSet(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return set.Len()
}

// waitAdmittedOps waits until the admitted state of so holds n operations.
func waitAdmittedOps(t *testing.T, so sobject.StatePublisher, n int) {
	t.Helper()
	ctr, rel, err := so.AccessAdmittedState(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rel()
	_, err = ctr.WaitValueWithValidator(t.Context(), func(snap sobject.SharedObjectStateSnapshot) (bool, error) {
		if snap == nil {
			return false, nil
		}
		set, err := snap.GetOperationSet(t.Context())
		return err == nil && set.Len() >= n, err
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
}

// newRead opens a read transaction on the installed World of w.
func newRead(t *testing.T, w *localWorld) world.Tx {
	t.Helper()
	tx, err := w.eng.NewTransaction(t.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	return tx
}

// worldSeqno returns the seqno of the World readers of w see.
func worldSeqno(t *testing.T, w *localWorld) uint64 {
	t.Helper()
	seqno, err := w.eng.GetSeqno(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return seqno
}

// worldHas reports whether tx holds the object key.
func worldHas(t *testing.T, tx world.Tx, key string) bool {
	t.Helper()
	obj, found, err := tx.GetObject(t.Context(), key)
	world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err)
	}
	return found
}
