package auth_method_password

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	cbc "github.com/aperturerobotics/controllerbus/core"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

const testScryptN uint32 = 14

func newTestMethod(t *testing.T) *PasswordMethod {
	t.Helper()
	method := NewPasswordMethod(context.Background())
	t.Cleanup(method.Close)
	return method
}

func buildTestParameters(t *testing.T, method *PasswordMethod, username string, password []byte) (*Parameters, crypto.PrivKey) {
	t.Helper()
	params, priv, err := method.buildParametersWithUsernamePassword(context.Background(), username, password, testScryptN, DefaultScryptR, DefaultScryptP)
	if err != nil {
		t.Fatal(err)
	}
	return params, priv
}

func TestBuildParametersWithUsernamePassword(t *testing.T) {
	params, priv := buildTestParameters(t, newTestMethod(t), "alice", []byte("hunter2"))
	if err := params.Validate(); err != nil {
		t.Fatal(err)
	}
	pid, err := peer.IDFromPrivateKey(priv)
	if err != nil || pid.String() == "" {
		t.Fatalf("invalid private key: %v", err)
	}
}

func TestDeterministic(t *testing.T) {
	method := newTestMethod(t)
	_, priv1 := buildTestParameters(t, method, "bob", []byte("password123"))
	_, priv2 := buildTestParameters(t, method, "bob", []byte("password123"))
	pid1, _ := peer.IDFromPrivateKey(priv1)
	pid2, _ := peer.IDFromPrivateKey(priv2)
	if pid1 != pid2 {
		t.Fatalf("same input produced %s and %s", pid1, pid2)
	}
}

func TestDifferentPasswords(t *testing.T) {
	method := newTestMethod(t)
	_, first := buildTestParameters(t, method, "carol", []byte("pass1"))
	_, second := buildTestParameters(t, method, "carol", []byte("pass2"))
	firstID, _ := peer.IDFromPrivateKey(first)
	secondID, _ := peer.IDFromPrivateKey(second)
	if firstID == secondID {
		t.Fatal("different passwords produced the same key")
	}
}

func TestDifferentUsernames(t *testing.T) {
	method := newTestMethod(t)
	_, first := buildTestParameters(t, method, "dave", []byte("samepass"))
	_, second := buildTestParameters(t, method, "eve", []byte("samepass"))
	firstID, _ := peer.IDFromPrivateKey(first)
	secondID, _ := peer.IDFromPrivateKey(second)
	if firstID == secondID {
		t.Fatal("different usernames produced the same key")
	}
}

func TestAuthenticate(t *testing.T) {
	method := newTestMethod(t)
	params, priv := buildTestParameters(t, method, "frank", []byte("mypassword"))
	paramsBytes, err := params.MarshalBlock()
	if err != nil {
		t.Fatal(err)
	}
	unmarshaled, err := method.UnmarshalParameters(paramsBytes)
	if err != nil {
		t.Fatal(err)
	}
	authPriv, err := method.Authenticate(context.Background(), unmarshaled, []byte("mypassword"))
	if err != nil {
		t.Fatal(err)
	}
	pid1, _ := peer.IDFromPrivateKey(priv)
	pid2, _ := peer.IDFromPrivateKey(authPriv)
	if pid1 != pid2 {
		t.Fatalf("authenticate produced %s, want %s", pid2, pid1)
	}
}

func TestParametersValidateScryptBounds(t *testing.T) {
	salt := make([]byte, saltLen)
	tests := []struct {
		name    string
		params  *Parameters
		wantErr bool
	}{
		{name: "omitted parameters retain defaults", params: &Parameters{Salt: salt}},
		{name: "repository test cost", params: &Parameters{Salt: salt, ScryptN: testScryptN, ScryptR: DefaultScryptR, ScryptP: DefaultScryptP}},
		{name: "repository production cost", params: &Parameters{Salt: salt, ScryptN: DefaultScryptN, ScryptR: DefaultScryptR, ScryptP: DefaultScryptP}},
		{name: "n below supported range", params: &Parameters{Salt: salt, ScryptN: 13}, wantErr: true},
		{name: "n above resource limit", params: &Parameters{Salt: salt, ScryptN: 21}, wantErr: true},
		{name: "unsupported r", params: &Parameters{Salt: salt, ScryptR: DefaultScryptR + 1}, wantErr: true},
		{name: "unsupported p", params: &Parameters{Salt: salt, ScryptP: DefaultScryptP + 1}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.params.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestUntrustedParametersRejectBeforeDerivation(t *testing.T) {
	method := newTestMethod(t)
	var calls atomic.Int32
	method.derive = func([]byte, []byte, int, int, int, int) ([]byte, error) { calls.Add(1); return make([]byte, 32), nil }
	params := &Parameters{Salt: make([]byte, saltLen), ScryptN: 21, ScryptR: DefaultScryptR, ScryptP: DefaultScryptP}
	if _, err := method.Authenticate(context.Background(), params, []byte("password")); err == nil {
		t.Fatal("accepted oversized record")
	}
	if _, err := method.UnmarshalParameters(make([]byte, maxEncodedParametersLen+1)); err == nil {
		t.Fatal("accepted oversized encoded record")
	}
	if _, err := method.UnmarshalParameters([]byte{0xff}); err == nil {
		t.Fatal("accepted malformed record")
	}
	if calls.Load() != 0 {
		t.Fatalf("derivation ran %d times", calls.Load())
	}
}

func TestConcurrentUntrustedDerivationsAreSerialized(t *testing.T) {
	method := newTestMethod(t)
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	method.derive = func([]byte, []byte, int, int, int, int) ([]byte, error) {
		entered <- struct{}{}
		<-release
		return make([]byte, 32), nil
	}
	params := &Parameters{Salt: make([]byte, saltLen), ScryptN: testScryptN, ScryptR: DefaultScryptR, ScryptP: DefaultScryptP}
	done := make(chan error, 2)
	go func() { _, err := method.Authenticate(context.Background(), params, []byte("one")); done <- err }()
	<-entered
	go func() { _, err := method.Authenticate(context.Background(), params, []byte("two")); done <- err }()
	select {
	case <-entered:
		t.Fatal("second derivation entered concurrently")
	case <-time.After(25 * time.Millisecond):
	}
	release <- struct{}{}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	<-entered
	release <- struct{}{}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestWaitingCancellationReturnsWithoutWork(t *testing.T) {
	method := newTestMethod(t)
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	var calls atomic.Int32
	method.derive = func([]byte, []byte, int, int, int, int) ([]byte, error) {
		calls.Add(1)
		entered <- struct{}{}
		<-release
		return make([]byte, 32), nil
	}
	params := &Parameters{Salt: make([]byte, saltLen), ScryptN: testScryptN, ScryptR: DefaultScryptR, ScryptP: DefaultScryptP}
	first := make(chan error, 1)
	go func() { _, err := method.Authenticate(context.Background(), params, []byte("one")); first <- err }()
	<-entered
	ctx, cancel := context.WithCancel(context.Background())
	canceled := make(chan error, 1)
	go func() { _, err := method.Authenticate(ctx, params, []byte("two")); canceled <- err }()
	cancel()
	if err := <-canceled; err != context.Canceled {
		t.Fatalf("got %v, want context canceled", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("derivation ran %d times", calls.Load())
	}
	release <- struct{}{}
	if err := <-first; err != nil {
		t.Fatal(err)
	}
}

func TestCloseCancelsWaitersAndJoinsActiveWork(t *testing.T) {
	method := NewPasswordMethod(context.Background())
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	method.derive = func([]byte, []byte, int, int, int, int) ([]byte, error) {
		entered <- struct{}{}
		<-release
		return make([]byte, 32), nil
	}
	params := &Parameters{Salt: make([]byte, saltLen), ScryptN: testScryptN, ScryptR: DefaultScryptR, ScryptP: DefaultScryptP}
	active := make(chan error, 1)
	go func() { _, err := method.Authenticate(context.Background(), params, []byte("one")); active <- err }()
	<-entered
	waiting := make(chan error, 1)
	go func() { _, err := method.Authenticate(context.Background(), params, []byte("two")); waiting <- err }()
	closed := make(chan struct{})
	go func() { method.Close(); close(closed) }()
	if err := <-waiting; err != context.Canceled {
		t.Fatalf("waiter got %v", err)
	}
	select {
	case <-closed:
		t.Fatal("Close returned before active derivation")
	case <-time.After(25 * time.Millisecond):
	}
	release <- struct{}{}
	if err := <-active; err != nil {
		t.Fatal(err)
	}
	<-closed
}

func TestRuntimeLookupPublishesOneAdmissionOwner(t *testing.T) {
	ctx := t.Context()
	b, sr, err := cbc.NewCoreBus(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err)
	}
	sr.AddFactory(NewFactory(b))

	first, releaseFirst, err := ExAcquirePasswordMethod(ctx, b)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseFirst()
	second, releaseSecond, err := ExAcquirePasswordMethod(ctx, b)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseSecond()
	if first != second {
		t.Fatal("runtime lookups returned different password admission owners")
	}
	if first.admission != second.admission {
		t.Fatal("runtime lookups returned different admission budgets")
	}
}

func TestDefaultScryptKeyUnchanged(t *testing.T) {
	if testing.Short() {
		t.Skip("default scrypt derivation uses the full memory budget")
	}
	method := newTestMethod(t)
	_, priv, err := method.BuildParametersWithUsernamePassword(context.Background(), "compatibility@example.com", []byte("spacewave-default-scrypt-vector"))
	if err != nil {
		t.Fatal(err)
	}
	pid, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	const expected = "12D3KooWAhjC69jGfUQovhfZAExcv3om5Bfs4hnJEq4Dp8Y6D45x"
	if pid.String() != expected {
		t.Fatalf("peer ID = %s, want %s", pid, expected)
	}
}

func TestMemoryBudgetAdmitsOneDefaultDerivation(t *testing.T) {
	method := newTestMethod(t)
	if cap(method.admission) != 1 {
		t.Fatal("default derivation must consume the complete memory budget")
	}
}

func TestProductionDerivationsUseRuntimeMethod(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate source root")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
	var productionCalls int
	var runtimeFactories int
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == "vendor" || entry.Name() == ".bldr" || entry.Name() == ".git" || entry.Name() == ".tmp" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		source := string(data)
		calls := strings.Count(source, "BuildParametersWithUsernamePassword(")
		productionCalls += calls
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == filepath.Join("cmd", "spacewave", "main.go") {
			runtimeFactories = strings.Count(source, "auth_method_password.NewFactory(")
		}
		productionDerivation := strings.HasPrefix(rel, "cmd"+string(filepath.Separator)) ||
			strings.HasPrefix(rel, "core"+string(filepath.Separator))
		if productionDerivation && calls != strings.Count(source, "ExBuildParametersWithUsernamePassword(") {
			t.Errorf("%s bypasses the runtime password method", rel)
		}
		if productionDerivation && strings.Contains(source, "NewPasswordMethod(") {
			t.Errorf("%s constructs an independent password method", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if productionCalls < 10 {
		t.Fatalf("password derivation census found only %d production calls", productionCalls)
	}
	if runtimeFactories != 1 {
		t.Fatalf("spacewave runtime constructs %d password method factories, want 1", runtimeFactories)
	}
}
