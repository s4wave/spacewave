//go:build !windows

package pipesock

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/bldr/util/bun"
	"github.com/sirupsen/logrus"
)

func newTestLogger() *logrus.Entry {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	logger.SetOutput(io.Discard)
	return logger.WithField("test", true)
}

// shortTempDir creates a short temp directory in /tmp to avoid Unix socket path length limits.
// Unix sockets are limited to ~104 characters, and macOS t.TempDir() paths can be 100+ chars.
func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "ps")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func TestBuildPipeListener(t *testing.T) {
	le := newTestLogger()

	t.Run("creates listener in temp directory", func(t *testing.T) {
		tmpDir := shortTempDir(t)
		pipeUuid := "u1"

		listener, err := BuildPipeListener(le, tmpDir, pipeUuid)
		if err != nil {
			t.Fatalf("BuildPipeListener failed: %v", err)
		}
		defer listener.Close()

		// Verify the socket file was created
		expectedPath := filepath.Join(tmpDir, ".pipe-"+pipeUuid)
		if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
			t.Errorf("socket file not created at expected path: %s", expectedPath)
		}
	})

	t.Run("creates parent directory if not exists", func(t *testing.T) {
		tmpDir := shortTempDir(t)
		nestedDir := filepath.Join(tmpDir, "a", "b")
		pipeUuid := "u2"

		// nestedDir doesn't exist yet
		if _, err := os.Stat(nestedDir); !os.IsNotExist(err) {
			t.Fatal("nested dir should not exist before test")
		}

		listener, err := BuildPipeListener(le, nestedDir, pipeUuid)
		if err != nil {
			t.Fatalf("BuildPipeListener failed: %v", err)
		}
		defer listener.Close()

		// Verify the directory was created
		if _, err := os.Stat(nestedDir); os.IsNotExist(err) {
			t.Errorf("parent directory was not created: %s", nestedDir)
		}
	})

	t.Run("removes old pipe file if exists", func(t *testing.T) {
		tmpDir := shortTempDir(t)
		pipeUuid := "u3"
		pipePath := filepath.Join(tmpDir, ".pipe-"+pipeUuid)

		// Create an old file at the socket path
		if err := os.WriteFile(pipePath, []byte("old data"), 0o644); err != nil {
			t.Fatalf("failed to create old file: %v", err)
		}

		listener, err := BuildPipeListener(le, tmpDir, pipeUuid)
		if err != nil {
			t.Fatalf("BuildPipeListener failed: %v", err)
		}
		defer listener.Close()

		// The old file should have been replaced with a socket
		fi, err := os.Stat(pipePath)
		if err != nil {
			t.Fatalf("failed to stat pipe path: %v", err)
		}

		// Socket files have ModeSocket bit set
		if fi.Mode()&os.ModeSocket == 0 {
			t.Errorf("expected socket file, got mode: %v", fi.Mode())
		}
	})

	t.Run("multiple listeners with different uuids", func(t *testing.T) {
		tmpDir := shortTempDir(t)

		listener1, err := BuildPipeListener(le, tmpDir, "a")
		if err != nil {
			t.Fatalf("BuildPipeListener 1 failed: %v", err)
		}
		defer listener1.Close()

		listener2, err := BuildPipeListener(le, tmpDir, "b")
		if err != nil {
			t.Fatalf("BuildPipeListener 2 failed: %v", err)
		}
		defer listener2.Close()

		// Both should exist
		if _, err := os.Stat(filepath.Join(tmpDir, ".pipe-a")); os.IsNotExist(err) {
			t.Error("pipe-a not found")
		}
		if _, err := os.Stat(filepath.Join(tmpDir, ".pipe-b")); os.IsNotExist(err) {
			t.Error("pipe-b not found")
		}
	})
}

func TestDialPipeListener(t *testing.T) {
	le := newTestLogger()

	t.Run("connects to listener", func(t *testing.T) {
		tmpDir := shortTempDir(t)
		pipeUuid := "d1"

		listener, err := BuildPipeListener(le, tmpDir, pipeUuid)
		if err != nil {
			t.Fatalf("BuildPipeListener failed: %v", err)
		}
		defer listener.Close()

		// Accept connections in a goroutine
		acceptDone := make(chan struct{})
		var serverConn io.ReadWriteCloser
		go func() {
			defer close(acceptDone)
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			serverConn = conn
		}()

		// Dial the listener
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		clientConn, err := DialPipeListener(ctx, le, tmpDir, pipeUuid)
		if err != nil {
			t.Fatalf("DialPipeListener failed: %v", err)
		}
		defer clientConn.Close()

		// Wait for accept
		<-acceptDone
		if serverConn != nil {
			defer serverConn.Close()
		}
	})

	t.Run("fails when listener not present", func(t *testing.T) {
		tmpDir := shortTempDir(t)
		pipeUuid := "nx"

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		_, err := DialPipeListener(ctx, le, tmpDir, pipeUuid)
		if err == nil {
			t.Error("expected error when connecting to nonexistent pipe")
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		tmpDir := shortTempDir(t)
		pipeUuid := "cx"

		// Create a listener but don't accept connections
		listener, err := BuildPipeListener(le, tmpDir, pipeUuid)
		if err != nil {
			t.Fatalf("BuildPipeListener failed: %v", err)
		}
		defer listener.Close()

		// Create a context that's already canceled
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err = DialPipeListener(ctx, le, tmpDir, pipeUuid)
		if err == nil {
			t.Error("expected error with canceled context")
		}
	})
}

func TestBidirectionalCommunication(t *testing.T) {
	le := newTestLogger()
	tmpDir := shortTempDir(t)
	pipeUuid := "bd"

	listener, err := BuildPipeListener(le, tmpDir, pipeUuid)
	if err != nil {
		t.Fatalf("BuildPipeListener failed: %v", err)
	}
	defer listener.Close()

	// Server goroutine
	serverDone := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverDone <- err
			return
		}
		defer conn.Close()

		// Read message from client
		buf := make([]byte, 1024)
		n, err := conn.Read(buf)
		if err != nil {
			serverDone <- err
			return
		}

		// Echo it back with prefix
		response := append([]byte("echo: "), buf[:n]...)
		_, err = conn.Write(response)
		serverDone <- err
	}()

	// Client
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	clientConn, err := DialPipeListener(ctx, le, tmpDir, pipeUuid)
	if err != nil {
		t.Fatalf("DialPipeListener failed: %v", err)
	}
	defer clientConn.Close()

	// Send message
	message := []byte("hello pipe")
	if _, err := clientConn.Write(message); err != nil {
		t.Fatalf("client write failed: %v", err)
	}

	// Read response
	buf := make([]byte, 1024)
	n, err := clientConn.Read(buf)
	if err != nil {
		t.Fatalf("client read failed: %v", err)
	}

	expected := "echo: hello pipe"
	if string(buf[:n]) != expected {
		t.Errorf("expected %q, got %q", expected, string(buf[:n]))
	}

	// Check server completed without error
	if err := <-serverDone; err != nil {
		t.Errorf("server error: %v", err)
	}
}

func TestConcurrentConnections(t *testing.T) {
	le := newTestLogger()
	tmpDir := shortTempDir(t)
	pipeUuid := "cc"

	listener, err := BuildPipeListener(le, tmpDir, pipeUuid)
	if err != nil {
		t.Fatalf("BuildPipeListener failed: %v", err)
	}
	defer listener.Close()

	numClients := 5
	var wg sync.WaitGroup

	// Accept connections in goroutine
	go func() {
		for range numClients {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(c io.ReadWriteCloser) {
				defer c.Close()
				io.Copy(c, c) // Echo
			}(conn)
		}
	}()

	// Launch multiple clients
	for i := range numClients {
		wg.Add(1)
		go func(clientID int) {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			conn, err := DialPipeListener(ctx, le, tmpDir, pipeUuid)
			if err != nil {
				t.Errorf("client %d: dial failed: %v", clientID, err)
				return
			}
			defer conn.Close()

			// Send and receive
			msg := []byte("hello from client")
			if _, err := conn.Write(msg); err != nil {
				t.Errorf("client %d: write failed: %v", clientID, err)
				return
			}

			buf := make([]byte, len(msg))
			if _, err := io.ReadFull(conn, buf); err != nil {
				t.Errorf("client %d: read failed: %v", clientID, err)
				return
			}

			if string(buf) != string(msg) {
				t.Errorf("client %d: expected %q, got %q", clientID, msg, buf)
			}
		}(i)
	}

	wg.Wait()
}

func TestListenerCleanup(t *testing.T) {
	le := newTestLogger()
	tmpDir := shortTempDir(t)
	pipeUuid := "cu"

	pipePath := filepath.Join(tmpDir, ".pipe-"+pipeUuid)

	// Create listener
	listener, err := BuildPipeListener(le, tmpDir, pipeUuid)
	if err != nil {
		t.Fatalf("BuildPipeListener failed: %v", err)
	}

	// Verify socket exists
	if _, err := os.Stat(pipePath); os.IsNotExist(err) {
		t.Fatal("socket should exist after listener created")
	}

	// Close listener
	listener.Close()

	// Create a new listener with the same uuid (should work after cleanup)
	listener2, err := BuildPipeListener(le, tmpDir, pipeUuid)
	if err != nil {
		t.Fatalf("second BuildPipeListener failed: %v", err)
	}
	listener2.Close()
}

func TestAbsolutePathFallback(t *testing.T) {
	// This test verifies that the code handles absolute paths correctly
	// when the socket path cannot be made relative to CWD
	le := newTestLogger()

	// Use absolute paths that are definitely not relative to CWD
	tmpDir := shortTempDir(t)
	pipeUuid := "ap"

	listener, err := BuildPipeListener(le, tmpDir, pipeUuid)
	if err != nil {
		t.Fatalf("BuildPipeListener failed: %v", err)
	}
	defer listener.Close()

	// Verify we can connect using the same parameters
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Accept in background
	go func() {
		conn, _ := listener.Accept()
		if conn != nil {
			conn.Close()
		}
	}()

	clientConn, err := DialPipeListener(ctx, le, tmpDir, pipeUuid)
	if err != nil {
		t.Fatalf("DialPipeListener failed: %v", err)
	}
	clientConn.Close()
}

// getTestScriptPath returns the absolute path to pipesock_test.ts.
func getTestScriptPath() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "pipesock_test.ts")
}

// TestTypeScriptClient tests Go server with TypeScript client using bun.
func TestTypeScriptClient(t *testing.T) {
	le := newTestLogger()
	ctx := context.Background()

	// Use temp directory for bun download
	stateDir := t.TempDir()

	// Check if we can resolve bun
	bunPath, err := bun.ResolveBunPath(ctx, le, stateDir)
	if err != nil {
		t.Skipf("bun not available: %v", err)
	}
	t.Logf("using bun at: %s", bunPath)

	// Get the test script path
	testScript := getTestScriptPath()
	if _, err := os.Stat(testScript); os.IsNotExist(err) {
		t.Fatalf("test script not found: %s", testScript)
	}

	t.Run("typescript client connects and echoes", func(t *testing.T) {
		// Use /tmp directly to avoid long paths (Unix socket paths are limited to ~104 chars)
		tmpDir, err := os.MkdirTemp("/tmp", "ps")
		if err != nil {
			t.Fatalf("failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)
		pipeUuid := "t1"

		// Create Go listener
		listener, err := BuildPipeListener(le, tmpDir, pipeUuid)
		if err != nil {
			t.Fatalf("BuildPipeListener failed: %v", err)
		}
		defer listener.Close()

		// Server goroutine - echo with prefix
		serverDone := make(chan error, 1)
		go func() {
			conn, err := listener.Accept()
			if err != nil {
				serverDone <- err
				return
			}
			defer conn.Close()

			// Read message from TypeScript client
			buf := make([]byte, 1024)
			n, err := conn.Read(buf)
			if err != nil {
				serverDone <- err
				return
			}

			t.Logf("server received: %s", string(buf[:n]))

			// Echo back with prefix (matching what pipesock_test.ts expects)
			response := append([]byte("echo: "), buf[:n]...)
			_, err = conn.Write(response)
			serverDone <- err
		}()

		// Run TypeScript client via bun
		cmd, err := bun.BunExec(ctx, le, stateDir, testScript, tmpDir, pipeUuid)
		if err != nil {
			t.Fatalf("BunExec failed: %v", err)
		}

		// Reset stdout/stderr to capture output (BunExec sets them to os.Stdout/Stderr)
		cmd.Stdout = nil
		cmd.Stderr = nil

		// Capture output
		output, err := cmd.CombinedOutput()
		t.Logf("typescript output:\n%s", string(output))

		if err != nil {
			t.Fatalf("typescript client failed: %v", err)
		}

		// Check server completed without error
		select {
		case err := <-serverDone:
			if err != nil {
				t.Errorf("server error: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("timeout waiting for server to complete")
		}
	})

	t.Run("typescript client with nested directory", func(t *testing.T) {
		// Use /tmp directly to avoid long paths (Unix socket paths are limited to ~104 chars)
		tmpDir, err := os.MkdirTemp("/tmp", "ps")
		if err != nil {
			t.Fatalf("failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)
		nestedDir := filepath.Join(tmpDir, "a", "b")
		pipeUuid := "t2"

		// Create Go listener in nested directory (tests directory creation)
		listener, err := BuildPipeListener(le, nestedDir, pipeUuid)
		if err != nil {
			t.Fatalf("BuildPipeListener failed: %v", err)
		}
		defer listener.Close()

		// Verify directory was created
		if _, err := os.Stat(nestedDir); os.IsNotExist(err) {
			t.Fatalf("nested directory was not created")
		}

		// Server goroutine
		serverDone := make(chan error, 1)
		go func() {
			conn, err := listener.Accept()
			if err != nil {
				serverDone <- err
				return
			}
			defer conn.Close()

			buf := make([]byte, 1024)
			n, err := conn.Read(buf)
			if err != nil {
				serverDone <- err
				return
			}

			response := append([]byte("echo: "), buf[:n]...)
			_, err = conn.Write(response)
			serverDone <- err
		}()

		// Run TypeScript client
		cmd, err := bun.BunExec(ctx, le, stateDir, testScript, nestedDir, pipeUuid)
		if err != nil {
			t.Fatalf("BunExec failed: %v", err)
		}

		// Reset stdout/stderr to capture output (BunExec sets them to os.Stdout/Stderr)
		cmd.Stdout = nil
		cmd.Stderr = nil

		output, err := cmd.CombinedOutput()
		t.Logf("typescript output:\n%s", string(output))

		if err != nil {
			t.Fatalf("typescript client failed: %v", err)
		}

		select {
		case err := <-serverDone:
			if err != nil {
				t.Errorf("server error: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("timeout waiting for server to complete")
		}
	})
}
