package terminal

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/creack/pty"
)

// Terminal represents a PTY-backed process.
type Terminal struct {
	ID           string
	PID          int
	Cmd          string
	Args         []string
	Cwd          string
	Cols         int
	Rows         int
	Owner        string
	StartedAt    time.Time
	LastActivity time.Time
	alive        bool

	ptmx   *os.File
	proc   *exec.Cmd
	buf    *Buffer
	mu     sync.Mutex
	onData func(termID string, data []byte) // called with raw output
	onExit func(termID string, code int)
}

// SpawnOpts configures a new terminal.
type SpawnOpts struct {
	Cmd    string
	Args   []string
	Cwd    string
	Cols   int
	Rows   int
	Env    map[string]string
	Owner  string
	OnData func(termID string, data []byte)
	OnExit func(termID string, code int)
}

// Spawn creates and starts a new terminal.
func Spawn(id string, opts SpawnOpts) (*Terminal, error) {
	if opts.Cmd == "" {
		opts.Cmd = os.Getenv("SHELL")
		if opts.Cmd == "" {
			opts.Cmd = "/bin/sh"
		}
	}
	if opts.Args == nil {
		opts.Args = []string{"-l"}
	}
	if opts.Cols == 0 {
		opts.Cols = 120
	}
	if opts.Rows == 0 {
		opts.Rows = 40
	}

	cmd := exec.Command(opts.Cmd, opts.Args...)
	cmd.Dir = opts.Cwd

	// Build environment from login shell — gives spawned terminals a clean
	// env identical to opening a fresh terminal, without inheriting daemon state.
	cmd.Env = buildSpawnEnv(opts.Env)

	size := &pty.Winsize{
		Cols: uint16(opts.Cols),
		Rows: uint16(opts.Rows),
	}

	ptmx, err := pty.StartWithSize(cmd, size)
	if err != nil {
		return nil, fmt.Errorf("pty start: %w", err)
	}

	t := &Terminal{
		ID:           id,
		PID:          cmd.Process.Pid,
		Cmd:          opts.Cmd,
		Args:         opts.Args,
		Cwd:          opts.Cwd,
		Cols:         opts.Cols,
		Rows:         opts.Rows,
		Owner:        opts.Owner,
		StartedAt:    time.Now(),
		LastActivity: time.Now(),
		alive:        true,
		ptmx:         ptmx,
		proc:         cmd,
		buf:          NewBuffer(),
		onData:       opts.OnData,
		onExit:       opts.OnExit,
	}

	// Read PTY output in background
	go t.readLoop()

	// Wait for process exit in background
	go t.waitLoop()

	return t, nil
}

func (t *Terminal) readLoop() {
	buf := make([]byte, 4096)
	for {
		n, err := t.ptmx.Read(buf)
		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])

			t.mu.Lock()
			t.buf.Write(data)
			t.LastActivity = time.Now()
			t.mu.Unlock()

			if t.onData != nil {
				t.onData(t.ID, data)
			}
		}
		if err != nil {
			return
		}
	}
}

func (t *Terminal) waitLoop() {
	exitCode := 0
	err := t.proc.Wait()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	t.mu.Lock()
	t.alive = false
	t.mu.Unlock()

	if t.onExit != nil {
		t.onExit(t.ID, exitCode)
	}
}

// Write sends data to the terminal's stdin.
func (t *Terminal) Write(data []byte) error {
	t.mu.Lock()
	t.LastActivity = time.Now()
	t.mu.Unlock()
	_, err := t.ptmx.Write(data)
	return err
}

// ReadBuffer returns a copy of the output buffer.
func (t *Terminal) ReadBuffer() []byte {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.buf.Bytes()
}

// Resize changes the terminal dimensions.
func (t *Terminal) Resize(cols, rows int) error {
	t.mu.Lock()
	t.Cols = cols
	t.Rows = rows
	t.mu.Unlock()

	return pty.Setsize(t.ptmx, &pty.Winsize{
		Cols: uint16(cols),
		Rows: uint16(rows),
	})
}

// Kill terminates the terminal process.
func (t *Terminal) Kill() error {
	if t.proc.Process != nil {
		_ = t.proc.Process.Kill()
	}
	return t.ptmx.Close()
}

// SetOwner changes the terminal's owner.
func (t *Terminal) SetOwner(owner string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Owner = owner
}

// GetLastActivity returns the time of last activity.
func (t *Terminal) GetLastActivity() time.Time {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.LastActivity
}

// IsAlive returns whether the process is still running.
func (t *Terminal) IsAlive() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.alive
}
