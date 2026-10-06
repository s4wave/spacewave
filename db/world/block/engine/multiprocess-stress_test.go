package world_block_engine_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/object"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/volume"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
	"github.com/sirupsen/logrus"
)

const (
	multiProcessStressEnv        = "SPACEWAVE_WORLD_ENGINE_STRESS_ROLE"
	multiProcessStressPathEnv    = "SPACEWAVE_WORLD_ENGINE_STRESS_PATH"
	multiProcessStressIDEnv      = "SPACEWAVE_WORLD_ENGINE_STRESS_ID"
	multiProcessStressItersEnv   = "SPACEWAVE_WORLD_ENGINE_STRESS_ITERS"
	multiProcessStressWritersEnv = "SPACEWAVE_WORLD_ENGINE_STRESS_WRITERS"
	multiProcessStressReadersEnv = "SPACEWAVE_WORLD_ENGINE_STRESS_READERS"

	multiProcessStressObjectStoreID = "multi-process-world-head"
	multiProcessStressBucketID      = "test-bucket"

	objectStoreCASEnv        = "SPACEWAVE_OBJECTSTORE_CAS_ROLE"
	objectStoreCASPathEnv    = "SPACEWAVE_OBJECTSTORE_CAS_PATH"
	objectStoreCASIDEnv      = "SPACEWAVE_OBJECTSTORE_CAS_ID"
	objectStoreCASItersEnv   = "SPACEWAVE_OBJECTSTORE_CAS_ITERS"
	objectStoreCASWritersEnv = "SPACEWAVE_OBJECTSTORE_CAS_WRITERS"
	objectStoreCASStoreID    = "multi-process-objectstore-cas"
)

func TestWorldEngineMultiProcessStress(t *testing.T) {
	// Dispatch a World stress child, or run the parent harness.
	if role := os.Getenv(multiProcessStressEnv); role != "" {
		runWorldEngineStressWorker(t, role)
		return
	}
	if testing.Short() {
		t.Skip("skipping multi-process World stress in short mode")
	}

	// Initialize the shared s4db World before starting stress processes.
	path := filepath.Join(t.TempDir(), "world-engine-stress.s4wave")
	initWorldEngineStressVolume(t, path)

	// Read the World stress writer, reader, and iteration counts.
	writers := stressEnvInt(multiProcessStressWritersEnv, 3)
	readers := stressEnvInt(multiProcessStressReadersEnv, 3)
	iterations := stressEnvInt(multiProcessStressItersEnv, 12)

	// Construct the World stress writer and reader processes.
	var cmds []*exec.Cmd
	for i := range writers {
		cmds = append(cmds, worldEngineStressCommand(t, path, "writer", i, iterations))
	}
	for i := range readers {
		cmds = append(cmds, worldEngineStressCommand(t, path, "reader", i, iterations*writers))
	}

	// Start and join all World stress processes.
	for _, cmd := range cmds {
		if err := cmd.Start(); err != nil {
			t.Fatalf("start %s: %v", strings.Join(cmd.Args, " "), err)
		}
	}
	for _, cmd := range cmds {
		if err := waitWorldEngineStressCommand(cmd, 90*time.Second); err != nil {
			t.Fatalf("stress child failed: %v\n%s", err, cmdOutput(cmd))
		}
	}

	// Verify the shared World after all stress processes complete.
	verifier := worldEngineStressCommand(t, path, "verifier", 0, iterations)
	verifier.Env = append(verifier.Env, multiProcessStressWritersEnv+"="+strconv.Itoa(writers))
	if err := verifier.Start(); err != nil {
		t.Fatalf("start verifier: %v", err)
	}
	if err := waitWorldEngineStressCommand(verifier, 90*time.Second); err != nil {
		t.Fatalf("stress verifier failed: %v\n%s", err, cmdOutput(verifier))
	}
}

func TestObjectStoreMultiProcessCoordinationCAS(t *testing.T) {
	// Dispatch an ObjectStore CAS child, or run the parent harness.
	if role := os.Getenv(objectStoreCASEnv); role != "" {
		runObjectStoreCASWorker(t, role)
		return
	}
	if testing.Short() {
		t.Skip("skipping multi-process object store CAS stress in short mode")
	}

	// Initialize the shared s4db ObjectStore before starting CAS writers.
	path := filepath.Join(t.TempDir(), "objectstore-cas.s4wave")
	initObjectStoreCASVolume(t, path)

	// Read the ObjectStore CAS writer and iteration counts.
	writers := stressEnvInt(objectStoreCASWritersEnv, 3)
	iterations := stressEnvInt(objectStoreCASItersEnv, 4)

	// Construct and start the ObjectStore CAS writer processes.
	var cmds []*exec.Cmd
	for i := range writers {
		cmds = append(cmds, objectStoreCASCommand(t, path, "writer", i, iterations))
	}
	for _, cmd := range cmds {
		if err := cmd.Start(); err != nil {
			t.Fatalf("start %s: %v", strings.Join(cmd.Args, " "), err)
		}
	}

	// Join all ObjectStore CAS writers before verifying their results.
	for _, cmd := range cmds {
		if err := waitWorldEngineStressCommand(cmd, 90*time.Second); err != nil {
			t.Fatalf("object store cas child failed: %v\n%s", err, cmdOutput(cmd))
		}
	}

	// Verify the shared ObjectStore after all CAS writers complete.
	verifier := objectStoreCASCommand(t, path, "verifier", 0, iterations)
	verifier.Env = append(verifier.Env, objectStoreCASWritersEnv+"="+strconv.Itoa(writers))
	if err := verifier.Start(); err != nil {
		t.Fatalf("start object store cas verifier: %v", err)
	}
	if err := waitWorldEngineStressCommand(verifier, 90*time.Second); err != nil {
		t.Fatalf("object store cas verifier failed: %v\n%s", err, cmdOutput(verifier))
	}
}

func initWorldEngineStressVolume(t *testing.T, path string) {
	// Bound the initial World stress setup with a context deadline.
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Open the s4db testbed and its World engine controller.
	tb := newWorldEngineStressTestbed(t, ctx, path, "init")
	defer tb.Release()
	ctrl, ref := startWorldEngineStressController(t, ctx, tb, "stress-init")
	defer ref.Release()
	eng, err := ctrl.GetWorldEngine(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Commit the initial World object before concurrent stress begins.
	tx, err := eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	{
		createdObject, err := tx.CreateObject(ctx, "stress/init", nil)
		world.ReleaseObjectState(createdObject)
		if err != nil {
			tx.Discard()
			t.Fatal(err.Error())
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}
}

func initObjectStoreCASVolume(t *testing.T, path string) {
	// Bound the initial ObjectStore CAS setup with a context deadline.
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Open the s4db testbed and coordinated ObjectStore.
	tb := newWorldEngineStressTestbed(t, ctx, path, "objectstore-cas-init")
	defer tb.Release()
	store, release := openObjectStoreCASStore(t, ctx, tb)
	defer release()

	// Initialize and commit the shared ObjectStore sequence.
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tx.Discard()
	seq := make([]byte, 8)
	if err := tx.Set(ctx, []byte("seq"), seq); err != nil {
		t.Fatal(err.Error())
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}
}

func objectStoreCASCommand(t *testing.T, path, role string, id, iterations int) *exec.Cmd {
	// Attribute ObjectStore CAS command failures to the calling test.
	t.Helper()

	// Re-exec the current test binary with a fixed selector; role data is passed
	// through test-owned environment variables.
	cmd := exec.Command( //nolint:gosec
		os.Args[0],
		"-test.run=^TestObjectStoreMultiProcessCoordinationCAS$",
		"-test.v",
	)
	cmd.Env = append(os.Environ(),
		objectStoreCASEnv+"="+role,
		objectStoreCASPathEnv+"="+path,
		objectStoreCASIDEnv+"="+strconv.Itoa(id),
		objectStoreCASItersEnv+"="+strconv.Itoa(iterations),
	)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	return cmd
}

func worldEngineStressCommand(t *testing.T, path, role string, id, iterations int) *exec.Cmd {
	// Attribute World stress command failures to the calling test.
	t.Helper()

	// Re-exec the current test binary with a fixed selector; role data is passed
	// through test-owned environment variables.
	cmd := exec.Command( //nolint:gosec
		os.Args[0],
		"-test.run=^TestWorldEngineMultiProcessStress$",
		"-test.v",
	)
	cmd.Env = append(os.Environ(),
		multiProcessStressEnv+"="+role,
		multiProcessStressPathEnv+"="+path,
		multiProcessStressIDEnv+"="+strconv.Itoa(id),
		multiProcessStressItersEnv+"="+strconv.Itoa(iterations),
	)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	return cmd
}

func cmdOutput(cmd *exec.Cmd) string {
	if buf, ok := cmd.Stdout.(*bytes.Buffer); ok {
		return buf.String()
	}
	return ""
}

func waitWorldEngineStressCommand(cmd *exec.Cmd, timeout time.Duration) error {
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		err := <-done
		if err != nil {
			return fmt.Errorf("timeout after %s: %w", timeout, err)
		}
		return fmt.Errorf("timeout after %s", timeout)
	}
}

func runWorldEngineStressWorker(t *testing.T, role string) {
	// Read and validate the World stress child configuration.
	path := os.Getenv(multiProcessStressPathEnv)
	if path == "" {
		t.Fatal("stress volume path env is empty")
	}
	id, err := strconv.Atoi(os.Getenv(multiProcessStressIDEnv))
	if err != nil {
		t.Fatalf("parse stress id: %v", err)
	}
	iterations, err := strconv.Atoi(os.Getenv(multiProcessStressItersEnv))
	if err != nil {
		t.Fatalf("parse stress iterations: %v", err)
	}

	// Bound the World stress child execution with a context deadline.
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	// Open the shared s4db World through the stress controller.
	tb := newWorldEngineStressTestbed(t, ctx, path, fmt.Sprintf("%s-%d", role, id))
	defer tb.Release()
	ctrl, ref := startWorldEngineStressController(t, ctx, tb, fmt.Sprintf("stress-%s-%d", role, id))
	defer ref.Release()
	eng, err := ctrl.GetWorldEngine(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Execute the configured World stress writer, reader, or verifier.
	switch role {
	case "writer":
		runWorldEngineStressWriter(t, ctx, eng, id, iterations)
	case "reader":
		runWorldEngineStressReader(t, ctx, eng, id, iterations)
	case "verifier":
		writers, err := strconv.Atoi(os.Getenv("SPACEWAVE_WORLD_ENGINE_STRESS_WRITERS"))
		if err != nil {
			t.Fatalf("parse stress writer count: %v", err)
		}
		runWorldEngineStressVerifier(t, ctx, eng, writers, iterations)
	default:
		t.Fatalf("unknown stress role %q", role)
	}
}

func runObjectStoreCASWorker(t *testing.T, role string) {
	// Read and validate the ObjectStore CAS child configuration.
	path := os.Getenv(objectStoreCASPathEnv)
	if path == "" {
		t.Fatal("object store CAS volume path env is empty")
	}
	id, err := strconv.Atoi(os.Getenv(objectStoreCASIDEnv))
	if err != nil {
		t.Fatalf("parse object store CAS id: %v", err)
	}
	iterations, err := strconv.Atoi(os.Getenv(objectStoreCASItersEnv))
	if err != nil {
		t.Fatalf("parse object store CAS iterations: %v", err)
	}

	// Bound the ObjectStore CAS child execution with a context deadline.
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	// Open the shared s4db ObjectStore for the CAS child.
	tb := newWorldEngineStressTestbed(t, ctx, path, fmt.Sprintf("objectstore-cas-%s-%d", role, id))
	defer tb.Release()
	store, release := openObjectStoreCASStore(t, ctx, tb)
	defer release()

	// Execute the configured ObjectStore CAS writer or verifier.
	switch role {
	case "writer":
		runObjectStoreCASWriter(t, ctx, tb, store, id, iterations)
	case "verifier":
		writers, err := strconv.Atoi(os.Getenv(objectStoreCASWritersEnv))
		if err != nil {
			t.Fatalf("parse object store CAS writer count: %v", err)
		}
		runObjectStoreCASVerifier(t, ctx, store, writers, iterations)
	default:
		t.Fatalf("unknown object store CAS role %q", role)
	}
}

func newWorldEngineStressTestbed(
	t *testing.T,
	ctx context.Context,
	path string,
	id string,
) *testbed.Testbed {
	// Open the shared s4db testbed and register the World engine factory.
	t.Helper()
	log := logrus.New()
	log.SetLevel(logrus.WarnLevel)
	le := logrus.NewEntry(log).WithField("stress", id)
	tb, err := testbed.NewTestbed(ctx, le, testbed.WithVolumeConfig(&volume_s4db.Config{
		Path: path,
		VolumeConfig: &volume_controller.Config{
			GcIntervalDur: "0",
		},
	}))
	if err != nil {
		t.Fatal(err.Error())
	}
	tb.StaticResolver.AddFactory(world_block_engine.NewFactory(tb.Bus))
	return tb
}

func openObjectStoreCASStore(
	t *testing.T,
	ctx context.Context,
	tb *testbed.Testbed,
) (object.ObjectStore, func()) {
	t.Helper()
	storeVal, _, storeRef, err := volume.ExBuildObjectStoreAPI(ctx, tb.Bus, false, objectStoreCASStoreID, tb.Volume.GetID(), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	return storeVal.GetObjectStore(), storeRef.Release
}

func runObjectStoreCASWriter(
	t *testing.T,
	ctx context.Context,
	tb *testbed.Testbed,
	store object.ObjectStore,
	writerID int,
	iterations int,
) {
	t.Helper()
	scope := coord.Scope{
		VolumeID:      tb.Volume.GetID(),
		ObjectStoreID: objectStoreCASStoreID,
		ParticipantID: fmt.Sprintf("objectstore-cas-writer-%d", writerID),
	}
	for i := range iterations {
		lease, err := tb.Volume.WaitAcquireWriteLease(ctx, scope)
		if err != nil {
			t.Fatalf("acquire object store CAS lease: %v", err)
		}
		if _, err := lease.Refresh(ctx); err != nil {
			_ = lease.Release(ctx)
			t.Fatalf("refresh object store CAS lease: %v", err)
		}
		key := fmt.Sprintf("writer-%d-%03d", writerID, i)
		seq, err := commitObjectStoreCASKey(ctx, store, key)
		if err == nil {
			_, err = lease.Publish(ctx, coord.Event{
				KeyPrefixChanged: []byte("seq"),
			})
		}
		releaseErr := lease.Release(ctx)
		if err != nil {
			t.Fatalf("commit object store CAS %s: %v", key, err)
		}
		if releaseErr != nil {
			t.Fatalf("release object store CAS lease: %v", releaseErr)
		}
		if !objectStoreCASKeyVisible(ctx, store, key) {
			t.Fatalf("object store CAS key %s missing after accepted seq %d", key, seq)
		}
	}
}

func commitObjectStoreCASKey(ctx context.Context, store object.ObjectStore, key string) (uint64, error) {
	// Refresh coordinated ObjectStore state and open its writer transaction.
	if err := refreshObjectStoreForCoordination(store); err != nil {
		return 0, err
	}
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		return 0, err
	}
	defer tx.Discard()

	// Read the current ObjectStore sequence before applying the CAS update.
	data, found, err := tx.Get(ctx, []byte("seq"))
	if err != nil {
		return 0, err
	}
	if !found {
		return 0, fmt.Errorf("seq missing")
	}
	seq := binary.BigEndian.Uint64(data)

	// Commit the ObjectStore key and increment its sequence together.
	if err := tx.Set(ctx, []byte("key/"+key), []byte("1")); err != nil {
		return 0, err
	}
	next := make([]byte, 8)
	binary.BigEndian.PutUint64(next, seq+1)
	if err := tx.Set(ctx, []byte("seq"), next); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return seq + 1, nil
}

func runObjectStoreCASVerifier(
	t *testing.T,
	ctx context.Context,
	store object.ObjectStore,
	writers int,
	iterations int,
) {
	// Refresh coordinated ObjectStore state and open a read snapshot.
	t.Helper()
	if err := refreshObjectStoreForCoordination(store); err != nil {
		t.Fatal(err.Error())
	}
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tx.Discard()

	// Require the shared ObjectStore sequence to exist.
	data, found, err := tx.Get(ctx, []byte("seq"))
	if err != nil {
		t.Fatal(err.Error())
	}
	if !found {
		t.Fatal("seq missing")
	}

	// Verify every CAS writer key against the shared ObjectStore sequence.
	seq := binary.BigEndian.Uint64(data)
	wantSeq := uint64(writers * iterations)
	var missing []string
	for writerID := range writers {
		for i := range iterations {
			key := fmt.Sprintf("writer-%d-%03d", writerID, i)
			_, found, err := tx.Get(ctx, []byte("key/"+key))
			if err != nil {
				t.Fatal(err.Error())
			}
			if !found {
				missing = append(missing, key)
			}
		}
	}
	if seq != wantSeq || len(missing) != 0 {
		t.Fatalf("seq=%d want=%d missing=%s", seq, wantSeq, strings.Join(missing, ","))
	}
}

func objectStoreCASKeyVisible(ctx context.Context, store object.ObjectStore, key string) bool {
	// Refresh coordinated ObjectStore state and open a visibility snapshot.
	if err := refreshObjectStoreForCoordination(store); err != nil {
		return false
	}
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return false
	}
	defer tx.Discard()

	// Check whether the committed CAS key is visible in the snapshot.
	_, found, err := tx.Get(ctx, []byte("key/"+key))
	return err == nil && found
}

func refreshObjectStoreForCoordination(store object.ObjectStore) error {
	refreshable, ok := store.(kvtx.CoordinationRefreshStore)
	if !ok {
		return nil
	}
	return refreshable.RefreshForCoordinationLock()
}

func startWorldEngineStressController(
	t *testing.T,
	ctx context.Context,
	tb *testbed.Testbed,
	engineID string,
) (*world_block_engine.Controller, interface{ Release() }) {
	// Configure and start the World stress controller on the shared s4db volume.
	t.Helper()
	transformConf, err := block_transform.NewConfig(nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	conf := world_block_engine.NewConfig(
		engineID,
		tb.Volume.GetID(),
		multiProcessStressBucketID,
		multiProcessStressObjectStoreID,
		&bucket.ObjectRef{
			BucketId:      multiProcessStressBucketID,
			TransformConf: transformConf,
		},
		nil,
		false,
	)
	ctrl, ref, err := world_block_engine.StartEngineWithConfig(ctx, tb.Bus, conf)
	if err != nil {
		t.Fatal(err.Error())
	}
	if _, err := ctrl.GetWorldEngine(ctx); err != nil {
		ref.Release()
		t.Fatal(err.Error())
	}
	return ctrl, ref
}

func runWorldEngineStressWriter(
	t *testing.T,
	ctx context.Context,
	eng world.Engine,
	writerID int,
	iterations int,
) {
	t.Helper()
	for i := range iterations {
		key := worldEngineStressKey(writerID, i)
		if err := commitWorldEngineStressKey(ctx, eng, key); err != nil {
			t.Fatalf("commit %s: %v", key, err)
		}
		if err := waitWorldEngineStressKey(ctx, eng, key); err != nil {
			t.Fatalf("read own key %s: %v", key, err)
		}
	}
}

func commitWorldEngineStressKey(ctx context.Context, eng world.Engine, key string) error {
	var lastErr error
	for attempt := range 200 {
		tx, err := eng.NewTransaction(ctx, true)
		if err != nil {
			lastErr = fmt.Errorf("new write transaction: %w", err)
			if errors.Is(err, coord.ErrStaleGeneration) {
				select {
				case <-ctx.Done():
					return fmt.Errorf("after %d attempts: %w; context: %v", attempt+1, lastErr, ctx.Err())
				case <-time.After(min(time.Duration(attempt+1)*5*time.Millisecond, 25*time.Millisecond)):
				}
				continue
			}
			return lastErr
		}
		{
			createdObject, err := tx.CreateObject(ctx, key, nil)
			world.ReleaseObjectState(createdObject)
			if err != nil {
				tx.Discard()
				lastErr = fmt.Errorf("create object: %w", err)
				if errors.Is(err, coord.ErrStaleGeneration) {
					select {
					case <-ctx.Done():
						return fmt.Errorf("after %d attempts: %w; context: %v", attempt+1, lastErr, ctx.Err())
					case <-time.After(time.Duration(attempt+1) * 5 * time.Millisecond):
					}
					continue
				}
				return lastErr
			}
		}
		err = tx.Commit(ctx)
		if err == nil {
			return nil
		}
		lastErr = fmt.Errorf("commit transaction: %w", err)
		if !errors.Is(err, coord.ErrStaleGeneration) {
			return lastErr
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("after %d attempts: %w; context: %v", attempt+1, lastErr, ctx.Err())
		case <-time.After(min(time.Duration(attempt+1)*5*time.Millisecond, 25*time.Millisecond)):
		}
	}
	return lastErr
}

func runWorldEngineStressReader(
	t *testing.T,
	ctx context.Context,
	eng world.Engine,
	readerID int,
	iterations int,
) {
	t.Helper()
	blockEng := eng.(*world_block.Engine)
	var lastSeqno uint64
	for i := range iterations {
		seqno, err := blockEng.GetSeqno(ctx)
		if err != nil {
			t.Fatalf("reader %d seqno: %v", readerID, err)
		}
		if seqno < lastSeqno {
			t.Fatalf("reader %d seqno decreased from %d to %d", readerID, lastSeqno, seqno)
		}
		lastSeqno = seqno
		key := worldEngineStressKey(i%3, i/3)
		if err := readWorldEngineStressKey(ctx, eng, key, false); err != nil {
			t.Fatalf("reader %d lookup %s: %v", readerID, key, err)
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(2 * time.Millisecond):
		}
	}
}

func runWorldEngineStressVerifier(
	t *testing.T,
	ctx context.Context,
	eng world.Engine,
	writers int,
	iterations int,
) {
	t.Helper()
	for writerID := range writers {
		for i := range iterations {
			key := worldEngineStressKey(writerID, i)
			if err := waitWorldEngineStressKey(ctx, eng, key); err != nil {
				t.Fatalf("verify %s: %v; %s", key, err, describeWorldEngineStressState(ctx, eng, writers, iterations))
			}
		}
	}
}

func describeWorldEngineStressState(ctx context.Context, eng world.Engine, writers, iterations int) string {
	// Capture the World stress sequence for failure diagnostics.
	var parts []string
	if blockEng, ok := eng.(*world_block.Engine); ok {
		if seqno, err := blockEng.GetSeqno(ctx); err == nil {
			parts = append(parts, fmt.Sprintf("seqno=%d", seqno))
		} else {
			parts = append(parts, fmt.Sprintf("seqnoErr=%v", err))
		}
	}

	// Count present and missing stress objects through read transactions.
	var missing []string
	var present int
	for writerID := range writers {
		for i := range iterations {
			key := worldEngineStressKey(writerID, i)
			tx, err := eng.NewTransaction(ctx, false)
			if err != nil {
				parts = append(parts, fmt.Sprintf("probeErr[%s]=new read transaction: %v", key, err))
				continue
			}
			objectState, found, err := tx.GetObject(ctx, key)
			world.ReleaseObjectState(objectState)
			tx.Discard()
			if err != nil {
				parts = append(parts, fmt.Sprintf("probeErr[%s]=%v", key, err))
				continue
			}
			if found {
				present++
			} else {
				missing = append(missing, key)
			}
		}
	}

	// Summarize the World stress object count and missing keys.
	parts = append(parts, fmt.Sprintf("present=%d/%d", present, writers*iterations))
	if len(missing) != 0 {
		parts = append(parts, "missing="+strings.Join(missing, ","))
	}
	return strings.Join(parts, " ")
}

func waitWorldEngineStressKey(ctx context.Context, eng world.Engine, key string) error {
	for {
		err := readWorldEngineStressKey(ctx, eng, key, true)
		if err == nil {
			return nil
		}
		if !strings.Contains(err.Error(), "missing stress key") && !errors.Is(err, block.ErrNotFound) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func readWorldEngineStressKey(ctx context.Context, eng world.Engine, key string, requireFound bool) error {
	// Open a World read transaction for the requested stress key.
	tx, err := eng.NewTransaction(ctx, false)
	if err != nil {
		return fmt.Errorf("new read transaction: %w", err)
	}
	defer tx.Discard()

	// Read and release the stress object and require its presence when requested.
	objectState, found, err := tx.GetObject(ctx, key)
	world.ReleaseObjectState(objectState)
	if err != nil {
		return fmt.Errorf("get object: %w", err)
	}
	if requireFound && !found {
		return fmt.Errorf("missing stress key %s", key)
	}
	return nil
}

func worldEngineStressKey(writerID int, iteration int) string {
	return fmt.Sprintf("stress/writer/%d/%03d", writerID, iteration)
}

func stressEnvInt(key string, fallback int) int {
	// Read the configured stress count or retain the fallback.
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}

	// Parse the configured stress count or retain the fallback.
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}
