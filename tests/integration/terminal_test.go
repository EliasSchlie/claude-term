package integration

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/EliasSchlie/claude-term/internal/client"
	"github.com/EliasSchlie/claude-term/internal/daemon"
)

func TestPing(t *testing.T) {
	c := startTestDaemon(t)
	if err := c.Ping(); err != nil {
		t.Fatalf("ping failed: %v", err)
	}
}

func TestSpawnAndList(t *testing.T) {
	c := startTestDaemon(t)

	result, err := c.Spawn(client.SpawnOpts{
		Cmd:   "/bin/sh",
		Args:  []string{"-c", "sleep 10"},
		Owner: "test-session",
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if result.TermID == "" {
		t.Fatal("spawn should return a term_id")
	}
	if result.PID == 0 {
		t.Fatal("spawn should return a pid")
	}

	terms, err := c.List("")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(terms) != 1 {
		t.Fatalf("expected 1 terminal, got %d", len(terms))
	}
	if terms[0].TermID != result.TermID {
		t.Errorf("list term_id %s != spawn term_id %s", terms[0].TermID, result.TermID)
	}
	if terms[0].Owner != "test-session" {
		t.Errorf("owner should be test-session, got %s", terms[0].Owner)
	}
	if !terms[0].Alive {
		t.Error("terminal should be alive")
	}
}

func TestListFilterByOwner(t *testing.T) {
	c := startTestDaemon(t)

	c.Spawn(client.SpawnOpts{Cmd: "/bin/sh", Args: []string{"-c", "sleep 10"}, Owner: "session-a"})
	c.Spawn(client.SpawnOpts{Cmd: "/bin/sh", Args: []string{"-c", "sleep 10"}, Owner: "session-b"})
	c.Spawn(client.SpawnOpts{Cmd: "/bin/sh", Args: []string{"-c", "sleep 10"}, Owner: "session-a"})

	termsA, err := c.List("session-a")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(termsA) != 2 {
		t.Errorf("expected 2 terminals for session-a, got %d", len(termsA))
	}

	termsB, err := c.List("session-b")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(termsB) != 1 {
		t.Errorf("expected 1 terminal for session-b, got %d", len(termsB))
	}

	all, err := c.List("")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("expected 3 total terminals, got %d", len(all))
	}
}

func TestWriteAndRead(t *testing.T) {
	c := startTestDaemon(t)

	result, err := c.Spawn(client.SpawnOpts{
		Cmd:  "/bin/sh",
		Args: []string{},
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	// Write a command
	time.Sleep(200 * time.Millisecond) // let shell start
	if err := c.Write(result.TermID, "echo HELLO_CLAUDE_TERM\n"); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Wait for output
	waitFor(t, 3*time.Second, "command output", func() bool {
		data, err := c.Read(result.TermID)
		if err != nil {
			return false
		}
		return strings.Contains(string(data), "HELLO_CLAUDE_TERM")
	})
}

func TestKillTerminal(t *testing.T) {
	c := startTestDaemon(t)

	result, _ := c.Spawn(client.SpawnOpts{
		Cmd:  "/bin/sh",
		Args: []string{"-c", "sleep 60"},
	})

	if err := c.Kill(result.TermID); err != nil {
		t.Fatalf("kill: %v", err)
	}

	// Terminal should be gone from list
	terms, _ := c.List("")
	if len(terms) != 0 {
		t.Errorf("expected 0 terminals after kill, got %d", len(terms))
	}
}

func TestKillNonexistent(t *testing.T) {
	c := startTestDaemon(t)
	err := c.Kill("t999")
	if err == nil {
		t.Error("kill non-existent terminal should error")
	}
}

func TestResize(t *testing.T) {
	c := startTestDaemon(t)

	result, _ := c.Spawn(client.SpawnOpts{
		Cmd:  "/bin/sh",
		Args: []string{"-c", "sleep 10"},
		Cols: 80,
		Rows: 24,
	})

	if err := c.Resize(result.TermID, 200, 50); err != nil {
		t.Fatalf("resize: %v", err)
	}

	// Verify via list
	terms, _ := c.List("")
	if len(terms) != 1 {
		t.Fatal("expected 1 terminal")
	}
	if terms[0].Cols != 200 || terms[0].Rows != 50 {
		t.Errorf("dimensions should be 200x50, got %dx%d", terms[0].Cols, terms[0].Rows)
	}
}

func TestAttachReceivesReplay(t *testing.T) {
	c := startTestDaemon(t)

	result, _ := c.Spawn(client.SpawnOpts{
		Cmd:  "/bin/sh",
		Args: []string{"-c", "echo REPLAY_TEST && sleep 10"},
	})

	// Wait for output to be buffered
	waitFor(t, 3*time.Second, "output buffered", func() bool {
		data, _ := c.Read(result.TermID)
		return strings.Contains(string(data), "REPLAY_TEST")
	})

	// Now attach — should get replay
	var replayData []byte
	var mu sync.Mutex
	c.SetPushHandler(client.PushHandler{
		OnReplay: func(_ string, data []byte) {
			mu.Lock()
			replayData = append(replayData, data...)
			mu.Unlock()
		},
	})

	if err := c.Attach(result.TermID); err != nil {
		t.Fatalf("attach: %v", err)
	}

	// Give replay time to arrive
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(string(replayData), "REPLAY_TEST") {
		t.Errorf("replay should contain REPLAY_TEST, got %q", string(replayData))
	}
}

func TestAttachReceivesLiveData(t *testing.T) {
	c := startTestDaemon(t)

	result, _ := c.Spawn(client.SpawnOpts{
		Cmd:  "/bin/sh",
		Args: []string{},
	})

	var liveData []byte
	var mu sync.Mutex
	c.SetPushHandler(client.PushHandler{
		OnData: func(_ string, data []byte) {
			mu.Lock()
			liveData = append(liveData, data...)
			mu.Unlock()
		},
	})

	time.Sleep(200 * time.Millisecond) // let shell start
	if err := c.Attach(result.TermID); err != nil {
		t.Fatalf("attach: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	// Write after attach
	c.Write(result.TermID, "echo LIVE_DATA_TEST\n")

	waitFor(t, 3*time.Second, "live data", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return strings.Contains(string(liveData), "LIVE_DATA_TEST")
	})
}

func TestMultipleClientsAttach(t *testing.T) {
	c1 := startTestDaemon(t)

	result, _ := c1.Spawn(client.SpawnOpts{
		Cmd:  "/bin/sh",
		Args: []string{},
	})

	// Connect second client
	sockPath := c1.SocketPath()
	c2, err := client.Connect(sockPath)
	if err != nil {
		t.Fatalf("connect c2: %v", err)
	}
	defer c2.Close()

	var data1, data2 []byte
	var mu1, mu2 sync.Mutex

	c1.SetPushHandler(client.PushHandler{
		OnData: func(_ string, data []byte) {
			mu1.Lock()
			data1 = append(data1, data...)
			mu1.Unlock()
		},
	})
	c2.SetPushHandler(client.PushHandler{
		OnData: func(_ string, data []byte) {
			mu2.Lock()
			data2 = append(data2, data...)
			mu2.Unlock()
		},
	})

	time.Sleep(200 * time.Millisecond)
	c1.Attach(result.TermID)
	c2.Attach(result.TermID)
	time.Sleep(100 * time.Millisecond)

	c1.Write(result.TermID, "echo MULTI_CLIENT\n")

	waitFor(t, 3*time.Second, "client 1 receives data", func() bool {
		mu1.Lock()
		defer mu1.Unlock()
		return strings.Contains(string(data1), "MULTI_CLIENT")
	})

	waitFor(t, 3*time.Second, "client 2 receives data", func() bool {
		mu2.Lock()
		defer mu2.Unlock()
		return strings.Contains(string(data2), "MULTI_CLIENT")
	})
}

func TestProcessExitEvent(t *testing.T) {
	c := startTestDaemon(t)

	result, _ := c.Spawn(client.SpawnOpts{
		Cmd:  "/bin/sh",
		Args: []string{"-c", "echo done && exit 42"},
	})

	exitReceived := make(chan int, 1)
	c.SetPushHandler(client.PushHandler{
		OnExit: func(_ string, code int) {
			exitReceived <- code
		},
	})

	c.Attach(result.TermID)

	select {
	case code := <-exitReceived:
		if code != 42 {
			t.Errorf("exit code should be 42, got %d", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for exit event")
	}
}

func TestClientDisconnectDoesNotKillTerminal(t *testing.T) {
	// Start daemon manually so we control multiple client connections
	tmpDir := shortTempDir(t)
	sockPath := filepath.Join(tmpDir, "d.sock")

	os.Setenv("CLAUDE_TERM_SOCKET", sockPath)
	os.Setenv("CLAUDE_TERM_DIR", tmpDir)
	t.Cleanup(func() {
		os.Unsetenv("CLAUDE_TERM_SOCKET")
		os.Unsetenv("CLAUDE_TERM_DIR")
	})

	d := daemon.New()
	go d.Run()
	t.Cleanup(func() { d.Shutdown() })

	// Wait for socket
	waitFor(t, 2*time.Second, "daemon socket", func() bool {
		_, err := os.Stat(sockPath)
		return err == nil
	})

	// Client 1 spawns a terminal then disconnects
	c1, err := client.Connect(sockPath)
	if err != nil {
		t.Fatalf("connect c1: %v", err)
	}

	result, _ := c1.Spawn(client.SpawnOpts{
		Cmd:  "/bin/sh",
		Args: []string{"-c", "sleep 60"},
	})
	termID := result.TermID

	// Disconnect client 1
	c1.Close()
	time.Sleep(100 * time.Millisecond)

	// Client 2 connects — terminal should still be alive
	c2, err := client.Connect(sockPath)
	if err != nil {
		t.Fatalf("connect c2: %v", err)
	}
	defer c2.Close()

	terms, err := c2.List("")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(terms) != 1 {
		t.Fatalf("expected 1 terminal still running, got %d", len(terms))
	}
	if terms[0].TermID != termID {
		t.Errorf("terminal ID mismatch")
	}
	if !terms[0].Alive {
		t.Error("terminal should still be alive after client disconnect")
	}
}

func TestSpawnWithEnv(t *testing.T) {
	c := startTestDaemon(t)

	result, _ := c.Spawn(client.SpawnOpts{
		Cmd:  "/bin/sh",
		Args: []string{"-c", "echo $MY_TEST_VAR"},
		Env:  map[string]string{"MY_TEST_VAR": "CUSTOM_VALUE_123"},
	})

	waitFor(t, 3*time.Second, "env var output", func() bool {
		data, _ := c.Read(result.TermID)
		return strings.Contains(string(data), "CUSTOM_VALUE_123")
	})
}

func TestReadNonexistent(t *testing.T) {
	c := startTestDaemon(t)
	_, err := c.Read("t999")
	if err == nil {
		t.Error("read non-existent terminal should error")
	}
}

func TestResizeNonexistent(t *testing.T) {
	c := startTestDaemon(t)
	err := c.Resize("t999", 80, 24)
	if err == nil {
		t.Error("resize non-existent terminal should error")
	}
}
