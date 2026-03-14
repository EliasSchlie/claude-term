package integration

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/EliasSchlie/claude-term/internal/client"
	"github.com/EliasSchlie/claude-term/internal/daemon"
)

// shortTempDir creates a short temp directory to avoid Unix socket path limits (104 chars on macOS).
func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "ct-")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// startTestDaemon starts a daemon with a temporary socket and returns a connected client.
// The daemon and client are cleaned up when the test finishes.
func startTestDaemon(t *testing.T) *client.Client {
	t.Helper()

	tmpDir := shortTempDir(t)
	sockPath := filepath.Join(tmpDir, "d.sock")
	pidPath := filepath.Join(tmpDir, "d.pid")

	// Set env vars for the daemon
	os.Setenv("CLAUDE_TERM_SOCKET", sockPath)
	os.Setenv("CLAUDE_TERM_DIR", tmpDir)
	t.Cleanup(func() {
		os.Unsetenv("CLAUDE_TERM_SOCKET")
		os.Unsetenv("CLAUDE_TERM_DIR")
	})

	// Ensure PID path is set correctly
	_ = pidPath

	d := daemon.New()
	ready := make(chan struct{})

	go func() {
		// Small delay then signal ready
		go func() {
			// Wait for socket to appear
			for i := 0; i < 50; i++ {
				if _, err := os.Stat(sockPath); err == nil {
					close(ready)
					return
				}
				time.Sleep(20 * time.Millisecond)
			}
			close(ready)
		}()
		if err := d.Run(); err != nil {
			// Daemon exited — might be expected during test teardown
		}
	}()

	<-ready

	c, err := client.Connect(sockPath)
	if err != nil {
		t.Fatalf("connect to test daemon: %v", err)
	}

	t.Cleanup(func() {
		c.Close()
		d.Shutdown()
	})

	return c
}

// waitFor polls a condition until it returns true or timeout.
func waitFor(t *testing.T, timeout time.Duration, desc string, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for: %s", desc)
}
