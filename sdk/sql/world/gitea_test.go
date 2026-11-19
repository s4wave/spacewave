//go:build !js && !wasip1

package s4wave_sql_world_test

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/aperturerobotics/fastjson"
	"github.com/s4wave/spacewave/db/block"
	db_testbed "github.com/s4wave/spacewave/db/testbed"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	gitea_testbed "github.com/s4wave/spacewave/internal/testbed/gitea"
	s4wave_sql_world "github.com/s4wave/spacewave/sdk/sql/world"
	"github.com/s4wave/spacewave/testbed"
)

// TestGiteaMySQLWorld runs an unmodified Gitea's real schema migrations.
// GITEA_BINARY opts into the external consumer on the build host.
func TestGiteaMySQLWorld(t *testing.T) {
	// Require an explicit binary rather than downloading dependencies in a test.
	binary := os.Getenv("GITEA_BINARY")
	if binary == "" {
		t.Skip("set GITEA_BINARY to the pinned Gitea executable")
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	if os.Getenv("GITEA_SCALE") == "1" {
		ctx, _ = block.WithReadCounter(ctx)
	}
	dir := t.TempDir()
	tb, err := testbed.WithTestbedOptions(ctx, []db_testbed.Option{db_testbed.WithVolumeConfig(&volume_s4db.Config{Path: filepath.Join(dir, "world.s4wave")})}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if tb != nil {
			tb.Release()
		}
	}()
	key := "sql/gitea"
	createSqlDbObject(t, ctx, tb.WorldState, key, true)
	store, err := s4wave_sql_world.NewWorldBackedSql(ctx, tb.WorldState, key)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	addr, stopSQL := serveMySQLWorld(t, ctx, store, "127.0.0.1:0")
	defer stopSQL()

	// Create the application's database using its normal client protocol.
	db, err := sql.Open("mysql", "root@tcp("+addr+")/")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, "CREATE DATABASE gitea CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci"); err != nil {
		t.Fatal(err)
	}

	// Confine every generated file and key to a fresh disposable fixture.
	conf := filepath.Join(dir, "app.ini")
	httpListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	httpAddr := httpListener.Addr().String()
	port := strconv.Itoa(httpListener.Addr().(*net.TCPAddr).Port)
	_ = httpListener.Close()
	config := "APP_NAME = Spacewave SQL integration\nRUN_USER = " + os.Getenv("USER") + "\nRUN_MODE = prod\n" +
		"[database]\nDB_TYPE = mysql\nHOST = " + addr + "\nNAME = gitea\nUSER = root\nPASSWD =\nSSL_MODE = disable\nCHARSET = utf8mb4\n" +
		"[security]\nINSTALL_LOCK = true\n" +
		"[repository]\nDEFAULT_BRANCH = main\nROOT = " + filepath.Join(dir, "repositories") + "\n" +
		"[server]\nHTTP_ADDR = 127.0.0.1\nHTTP_PORT = " + port + "\nROOT_URL = http://" + httpAddr + "/\nOFFLINE_MODE = true\nDISABLE_SSH = true\nAPP_DATA_PATH = " + filepath.Join(dir, "data") + "\n" +
		"[indexer]\nISSUE_INDEXER_TYPE = db\n[actions]\nENABLED = false\n[log]\nMODE = console\nLEVEL = warn\n"
	if err := os.WriteFile(conf, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}

	// Run the application's migration code against the attached World object.
	command := exec.CommandContext(ctx, binary, "--work-path", dir, "--config", conf, "migrate")
	output, err := command.CombinedOutput()
	if len(output) > 12000 {
		output = output[len(output)-12000:]
	}
	if err != nil {
		t.Fatalf("Gitea migrate: %v\n%s", err, output)
	}
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM gitea.version").Scan(&count); err != nil || count != 1 {
		t.Fatalf("Gitea schema version rows=%d err=%v", count, err)
	}

	// Create a disposable application account through the application's own CLI.
	password := "Spacewave-Fixture-2026!"
	command = exec.CommandContext(ctx, binary, "--work-path", dir, "--config", conf, "admin", "user", "create", "--username", "operator", "--password", password, "--email", "fixture@example.invalid", "--admin", "--must-change-password=false")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("Gitea create user: %v\n%s", err, output)
	}

	// Start the real application and join it on every test exit.
	webCtx, stopWeb := context.WithCancel(ctx)
	command = exec.CommandContext(webCtx, binary, "--work-path", dir, "--config", conf, "web")
	log, err := os.Create(filepath.Join(dir, "gitea.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	command.Stdout, command.Stderr = log, log
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	webDone := make(chan struct{})
	var webErr error
	go func() { webErr = command.Wait(); close(webDone) }()
	defer func() { stopWeb(); <-webDone }()
	base := "http://" + httpAddr

	// Wait for readiness until the server answers or exits.
	readyCtx, readyCancel := context.WithCancelCause(ctx)
	go func() {
		select {
		case <-webDone:
			readyCancel(errors.New("gitea web exited before it answered"))
		case <-readyCtx.Done():
		}
	}()
	gitea_testbed.WaitReady(t, readyCtx, base, filepath.Join(dir, "gitea.log"))
	readyCancel(nil)

	// Create a repository and issue through Gitea's authenticated HTTP API.
	gitea_testbed.Request(t, ctx, base+"/api/v1/user/repos", password, "POST", `{"name":"proof","auto_init":false}`)
	issue := gitea_testbed.Request(t, ctx, base+"/api/v1/repos/operator/proof/issues", password, "POST", `{"title":"Retained state","body":"Original Spacewave SQL issue"}`)
	saved, err := fastjson.ParseBytes(issue)
	if err != nil {
		t.Fatal(err)
	}
	issueNumber := saved.GetInt64("number")
	issueBody := string(saved.GetStringBytes("body"))
	if issueNumber < 1 || issueBody != "Original Spacewave SQL issue" {
		t.Fatalf("issue response=%s", issue)
	}

	// Push and clone a known commit using the real Git clients and HTTP service.
	seed := filepath.Join(dir, "seed")
	if err := os.Mkdir(seed, 0o700); err != nil {
		t.Fatal(err)
	}
	gitea_testbed.Git(t, ctx, seed, password, "init", "--initial-branch=main")
	if err := os.WriteFile(filepath.Join(seed, "proof.txt"), []byte("persistent repository content\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitea_testbed.Git(t, ctx, seed, password, "add", "proof.txt")
	gitea_testbed.Git(t, ctx, seed, password, "commit", "-m", "Retained repository fixture")
	commit := strings.TrimSpace(gitea_testbed.Git(t, ctx, seed, password, "rev-parse", "HEAD"))
	gitea_testbed.Git(t, ctx, seed, password, "push", base+"/operator/proof.git", "main")
	clone := filepath.Join(dir, "clone")
	gitea_testbed.Git(t, ctx, dir, password, "clone", base+"/operator/proof.git", clone)
	if got := strings.TrimSpace(gitea_testbed.Git(t, ctx, clone, password, "rev-parse", "HEAD")); got != commit {
		t.Fatalf("cloned commit=%s want=%s", got, commit)
	}
	readback := gitea_testbed.Request(t, ctx, base+"/api/v1/repos/operator/proof/issues/"+strconv.FormatInt(issueNumber, 10), password, "GET", "")
	if !strings.Contains(string(readback), issueBody) {
		t.Fatalf("issue readback=%s", readback)
	}

	// Run optional production-size profiling against the same real consumer.
	if os.Getenv("GITEA_SCALE") == "1" {
		scaleGitea(t, ctx, db, base, password, issueNumber, tb)
	}

	// Ask Gitea for a clean stop before synchronizing the backing World.
	if err := command.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	<-webDone
	if webErr != nil {
		t.Fatalf("Gitea clean stop: %v", webErr)
	}
	if _, err := tb.Engine.Sync(ctx); err != nil {
		t.Fatal(err)
	}

	// Close every SQL user and reopen the disk Volume and World from their head.
	_ = db.Close()
	stopSQL()
	store.Close()
	tb.Release()
	tb = nil
	reopened, err := testbed.WithTestbedOptions(ctx, []db_testbed.Option{db_testbed.WithVolumeConfig(&volume_s4db.Config{Path: filepath.Join(dir, "world.s4wave")})}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Release()
	reopenedStore, err := s4wave_sql_world.NewWorldBackedSql(ctx, reopened.WorldState, key)
	if err != nil {
		t.Fatal(err)
	}
	defer reopenedStore.Close()
	reopenedAddr, stopReopened := serveMySQLWorld(t, ctx, reopenedStore, "127.0.0.1:0")
	defer stopReopened()
	reader, err := sql.Open("mysql", "root@tcp("+reopenedAddr+")/gitea")
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	var content string
	if err := reader.QueryRowContext(ctx, "SELECT content FROM issue WHERE `index` = ?", issueNumber).Scan(&content); err != nil || content != issueBody {
		t.Fatalf("durable issue readback=%q err=%v", content, err)
	}
	t.Logf("Gitea issue=%d commit=%s SQL object=%s", issueNumber, commit, key)
}
