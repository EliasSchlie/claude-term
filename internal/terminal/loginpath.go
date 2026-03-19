package terminal

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

var (
	loginPATH     string
	loginPATHOnce sync.Once
)

// getLoginShellPATH returns the PATH from a login shell, capturing what the
// user's shell profile (.zprofile, .bash_profile, etc.) would set. This ensures
// spawned terminals inherit the same PATH as a normal terminal, even when the
// daemon is launched from a non-login context (e.g., an Electron app or launchd).
//
// The result is cached — the login shell runs at most once per process.
// On failure (timeout, broken profile), falls back to the current process PATH.
func getLoginShellPATH() string {
	loginPATHOnce.Do(func() {
		loginPATH = resolveLoginPATH()
	})
	return loginPATH
}

func resolveLoginPATH() string {
	if runtime.GOOS == "windows" {
		return os.Getenv("PATH")
	}

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, shell, "-lc", "echo $PATH")
	out, err := cmd.Output()
	if err != nil {
		return os.Getenv("PATH")
	}

	result := strings.TrimSpace(string(out))
	if result == "" {
		return os.Getenv("PATH")
	}
	return result
}

// buildEnvWithLoginPATH returns a copy of os.Environ() with PATH replaced by
// the login shell PATH. Additional env vars from the extras map are appended.
func buildEnvWithLoginPATH(extras map[string]string) []string {
	loginPath := getLoginShellPATH()
	currentPath := os.Getenv("PATH")

	env := os.Environ()

	// Only replace PATH if the login shell gave us something different
	if loginPath != currentPath {
		for i, e := range env {
			if strings.HasPrefix(e, "PATH=") {
				env[i] = "PATH=" + loginPath
				break
			}
		}
	}

	for k, v := range extras {
		env = append(env, k+"="+v)
	}
	return env
}
