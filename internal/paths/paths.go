package paths

import (
	"os"
	"path/filepath"
)

// Dir returns the claude-term directory (~/.claude-term).
func Dir() string {
	if v := os.Getenv("CLAUDE_TERM_DIR"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude-term")
}

// SocketPath returns the daemon socket path.
func SocketPath() string {
	if v := os.Getenv("CLAUDE_TERM_SOCKET"); v != "" {
		return v
	}
	return filepath.Join(Dir(), "daemon.sock")
}

// PIDPath returns the daemon PID file path.
func PIDPath() string {
	return filepath.Join(Dir(), "daemon.pid")
}

// EnsureDir creates the claude-term directory if it doesn't exist.
func EnsureDir() error {
	return os.MkdirAll(Dir(), 0o700)
}
