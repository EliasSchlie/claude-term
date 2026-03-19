package terminal

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

var (
	loginEnv     []string
	loginEnvOnce sync.Once
	loginEnvErr  error
)

// getLoginShellEnv returns the full environment from a login shell, capturing
// what the user's shell profile (.zprofile, .bash_profile, etc.) would set.
// This gives spawned terminals a clean, reproducible env — identical to opening
// a fresh terminal — without inheriting daemon-specific vars or stale state.
//
// The result is cached — the login shell runs at most once per process.
// Returns an error if the login shell fails (no silent fallback).
func getLoginShellEnv() ([]string, error) {
	loginEnvOnce.Do(func() {
		loginEnv, loginEnvErr = resolveLoginEnv()
		if loginEnvErr != nil {
			log.Printf("WARNING: failed to resolve login shell env: %v (spawned terminals will inherit daemon env)", loginEnvErr)
		} else {
			log.Printf("resolved login shell env: %d vars", len(loginEnv))
		}
	})
	return loginEnv, loginEnvErr
}

func resolveLoginEnv() ([]string, error) {
	if runtime.GOOS == "windows" {
		return os.Environ(), nil
	}

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Use env -0 for null-delimited output — handles values with newlines
	cmd := exec.CommandContext(ctx, shell, "-lc", "env -0")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("login shell (%s -lc 'env -0'): %w", shell, err)
	}

	env := parseNullDelimitedEnv(out)
	if len(env) == 0 {
		return nil, fmt.Errorf("login shell returned empty env")
	}

	// Sanity check: PATH and HOME must be present
	var hasPath, hasHome bool
	for _, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			hasPath = true
		}
		if strings.HasPrefix(e, "HOME=") {
			hasHome = true
		}
	}
	if !hasPath || !hasHome {
		return nil, fmt.Errorf("login shell env missing essential vars (PATH=%v, HOME=%v)", hasPath, hasHome)
	}

	return env, nil
}

func parseNullDelimitedEnv(data []byte) []string {
	var env []string
	for _, entry := range bytes.Split(data, []byte{0}) {
		s := string(entry)
		if s == "" {
			continue
		}
		// Must contain '=' to be a valid env var
		if !strings.Contains(s, "=") {
			continue
		}
		env = append(env, s)
	}
	return env
}

// buildSpawnEnv returns the environment for a spawned terminal. Uses the login
// shell's env as a clean base, then layers extras on top. If login shell env
// resolution failed, falls back to os.Environ() (logged as warning at startup).
func buildSpawnEnv(extras map[string]string) []string {
	base, err := getLoginShellEnv()
	if err != nil {
		// Fallback: use daemon env (already logged warning at init time)
		base = append([]string(nil), os.Environ()...)
	} else {
		// Copy so we don't mutate the cached slice
		base = append([]string(nil), base...)
	}

	for k, v := range extras {
		base = append(base, k+"="+v)
	}
	return base
}
