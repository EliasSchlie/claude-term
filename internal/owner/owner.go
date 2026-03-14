package owner

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/EliasSchlie/claude-term/internal/paths"
)

// Discover walks up the process tree to find a registered owner ID.
// Returns empty string if no owner is found.
func Discover() string {
	ownersDir := filepath.Join(paths.Dir(), "owners")
	pid := os.Getppid()

	// Walk up at most 10 levels (claude-term → shell → Claude Code)
	for i := 0; i < 10 && pid > 1; i++ {
		ownerFile := filepath.Join(ownersDir, strconv.Itoa(pid))
		if data, err := os.ReadFile(ownerFile); err == nil {
			return strings.TrimSpace(string(data))
		}
		pid = getParentPID(pid)
	}
	return ""
}

// getParentPID returns the parent PID of a given process.
func getParentPID(pid int) int {
	// Try /proc first (Linux)
	statFile := filepath.Join("/proc", strconv.Itoa(pid), "stat")
	if data, err := os.ReadFile(statFile); err == nil {
		s := string(data)
		idx := strings.LastIndex(s, ")")
		if idx >= 0 && idx+2 < len(s) {
			fields := strings.Fields(s[idx+2:])
			if len(fields) >= 2 {
				if ppid, err := strconv.Atoi(fields[1]); err == nil {
					return ppid
				}
			}
		}
	}

	// Fallback: use ps (works on macOS and Linux)
	out, err := exec.Command("ps", "-o", "ppid=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return 0
	}
	ppid, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0
	}
	return ppid
}
