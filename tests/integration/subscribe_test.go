package integration

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/EliasSchlie/claude-term/internal/client"
	"github.com/EliasSchlie/claude-term/internal/daemon"
	"github.com/EliasSchlie/claude-term/internal/protocol"
)

func TestSubscribeReceivesSpawnEvent(t *testing.T) {
	c := startTestDaemon(t)

	var events []*protocol.Message
	var mu sync.Mutex

	c.SetPushHandler(client.PushHandler{
		OnTermSpawned: func(msg *protocol.Message) {
			mu.Lock()
			events = append(events, msg)
			mu.Unlock()
		},
	})

	if err := c.Subscribe(); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	_, err := c.Spawn(client.SpawnOpts{
		Cmd:   "/bin/sh",
		Args:  []string{"-c", "sleep 10"},
		Owner: "test-owner",
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	waitFor(t, 3*time.Second, "term_spawned event", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(events) >= 1
	})

	mu.Lock()
	defer mu.Unlock()
	e := events[0]
	if e.Type != protocol.TypeTermSpawned {
		t.Errorf("expected type %s, got %s", protocol.TypeTermSpawned, e.Type)
	}
	if e.TermID == "" {
		t.Error("term_spawned should include term_id")
	}
	if e.Owner != "test-owner" {
		t.Errorf("expected owner test-owner, got %s", e.Owner)
	}
	if e.PID == 0 {
		t.Error("term_spawned should include pid")
	}
}

func TestSubscribeReceivesKillEvent(t *testing.T) {
	c := startTestDaemon(t)

	var events []*protocol.Message
	var mu sync.Mutex

	c.SetPushHandler(client.PushHandler{
		OnTermKilled: func(msg *protocol.Message) {
			mu.Lock()
			events = append(events, msg)
			mu.Unlock()
		},
	})

	if err := c.Subscribe(); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	result, _ := c.Spawn(client.SpawnOpts{
		Cmd:  "/bin/sh",
		Args: []string{"-c", "sleep 60"},
	})

	if err := c.Kill(result.TermID); err != nil {
		t.Fatalf("kill: %v", err)
	}

	waitFor(t, 3*time.Second, "term_killed event", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(events) >= 1
	})

	mu.Lock()
	defer mu.Unlock()
	if events[0].TermID != result.TermID {
		t.Errorf("expected term_id %s, got %s", result.TermID, events[0].TermID)
	}
}

func TestSubscribeReceivesExitEvent(t *testing.T) {
	c := startTestDaemon(t)

	var events []*protocol.Message
	var mu sync.Mutex

	c.SetPushHandler(client.PushHandler{
		OnTermExited: func(msg *protocol.Message) {
			mu.Lock()
			events = append(events, msg)
			mu.Unlock()
		},
	})

	if err := c.Subscribe(); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	result, _ := c.Spawn(client.SpawnOpts{
		Cmd:  "/bin/sh",
		Args: []string{"-c", "exit 7"},
	})

	waitFor(t, 5*time.Second, "term_exited event", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(events) >= 1
	})

	mu.Lock()
	defer mu.Unlock()
	e := events[0]
	if e.TermID != result.TermID {
		t.Errorf("expected term_id %s, got %s", result.TermID, e.TermID)
	}
	if e.ExitCode == nil || *e.ExitCode != 7 {
		t.Errorf("expected exit_code 7, got %v", e.ExitCode)
	}
}

func TestSubscribeReceivesOwnerChangedEvent(t *testing.T) {
	c := startTestDaemon(t)

	var events []*protocol.Message
	var mu sync.Mutex

	c.SetPushHandler(client.PushHandler{
		OnTermOwnerChanged: func(msg *protocol.Message) {
			mu.Lock()
			events = append(events, msg)
			mu.Unlock()
		},
	})

	if err := c.Subscribe(); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	result, _ := c.Spawn(client.SpawnOpts{
		Cmd:   "/bin/sh",
		Args:  []string{"-c", "sleep 10"},
		Owner: "old-owner",
	})

	if err := c.SetOwner(result.TermID, "new-owner"); err != nil {
		t.Fatalf("set-owner: %v", err)
	}

	waitFor(t, 3*time.Second, "term_owner_changed event", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(events) >= 1
	})

	mu.Lock()
	defer mu.Unlock()
	e := events[0]
	if e.TermID != result.TermID {
		t.Errorf("expected term_id %s, got %s", result.TermID, e.TermID)
	}
	if e.Owner != "new-owner" {
		t.Errorf("expected owner new-owner, got %s", e.Owner)
	}
}

func TestUnsubscribeStopsEvents(t *testing.T) {
	c := startTestDaemon(t)

	var events []*protocol.Message
	var mu sync.Mutex

	c.SetPushHandler(client.PushHandler{
		OnTermSpawned: func(msg *protocol.Message) {
			mu.Lock()
			events = append(events, msg)
			mu.Unlock()
		},
	})

	if err := c.Subscribe(); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	// Spawn one terminal — should get event
	c.Spawn(client.SpawnOpts{Cmd: "/bin/sh", Args: []string{"-c", "sleep 10"}})
	waitFor(t, 3*time.Second, "first spawn event", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(events) >= 1
	})

	// Unsubscribe
	if err := c.Unsubscribe(); err != nil {
		t.Fatalf("unsubscribe: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	// Spawn another — should NOT get event
	c.Spawn(client.SpawnOpts{Cmd: "/bin/sh", Args: []string{"-c", "sleep 10"}})
	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(events) != 1 {
		t.Errorf("expected 1 event after unsubscribe, got %d", len(events))
	}
}

func TestSubscribeMultipleClients(t *testing.T) {
	c1 := startTestDaemon(t)
	sockPath := c1.SocketPath()

	c2, err := client.Connect(sockPath)
	if err != nil {
		t.Fatalf("connect c2: %v", err)
	}
	defer c2.Close()

	var events1, events2 []*protocol.Message
	var mu1, mu2 sync.Mutex

	c1.SetPushHandler(client.PushHandler{
		OnTermSpawned: func(msg *protocol.Message) {
			mu1.Lock()
			events1 = append(events1, msg)
			mu1.Unlock()
		},
	})
	c2.SetPushHandler(client.PushHandler{
		OnTermSpawned: func(msg *protocol.Message) {
			mu2.Lock()
			events2 = append(events2, msg)
			mu2.Unlock()
		},
	})

	c1.Subscribe()
	c2.Subscribe()

	// Spawn from c1 — both should get the event
	c1.Spawn(client.SpawnOpts{Cmd: "/bin/sh", Args: []string{"-c", "sleep 10"}})

	waitFor(t, 3*time.Second, "c1 receives event", func() bool {
		mu1.Lock()
		defer mu1.Unlock()
		return len(events1) >= 1
	})
	waitFor(t, 3*time.Second, "c2 receives event", func() bool {
		mu2.Lock()
		defer mu2.Unlock()
		return len(events2) >= 1
	})
}

func TestClientDoneClosesOnDaemonShutdown(t *testing.T) {
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

	waitFor(t, 2*time.Second, "daemon socket", func() bool {
		_, err := os.Stat(sockPath)
		return err == nil
	})

	c, err := client.Connect(sockPath)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer c.Close()

	if err := c.Subscribe(); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	// Shutdown daemon — client.Done() should close
	d.Shutdown()

	select {
	case <-c.Done():
		// success
	case <-time.After(3 * time.Second):
		t.Fatal("client.Done() was not closed after daemon shutdown")
	}
}

func TestNonSubscriberDoesNotReceiveEvents(t *testing.T) {
	c := startTestDaemon(t)

	var events []*protocol.Message
	var mu sync.Mutex

	c.SetPushHandler(client.PushHandler{
		OnTermSpawned: func(msg *protocol.Message) {
			mu.Lock()
			events = append(events, msg)
			mu.Unlock()
		},
	})

	// Do NOT subscribe — just spawn
	c.Spawn(client.SpawnOpts{Cmd: "/bin/sh", Args: []string{"-c", "sleep 10"}})
	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(events) != 0 {
		t.Errorf("non-subscriber should not receive events, got %d", len(events))
	}
}
