---
name: claude-term
description: Use when you need persistent terminals — long-running processes, dev servers, REPLs, or any command you want to check back on later.
---

# claude-term — Persistent Terminals

Use `claude-term` when you need a terminal that persists beyond a single command — long-running processes, interactive tools, or terminals you want to check back on later.

## When to Use

- **Use claude-term** for: servers, watchers, builds, REPLs, SSH sessions, anything long-running or interactive
- **Use the Bash tool** for: quick one-shot commands (ls, git status, grep)

## Quick Reference

```bash
# Spawn a terminal (returns term_id)
claude-term spawn --cwd /path/to/project

# Spawn with a specific command
claude-term spawn bash -c "npm run dev"

# List your terminals
claude-term list

# Run a command in a terminal
claude-term write t1 "npm test\n"

# Check output
claude-term read t1

# Kill when done
claude-term kill t1
```

## Workflow Example

```bash
# 1. Start a dev server
TERM_ID=$(claude-term spawn --cwd ~/projects/myapp bash -c "npm run dev")

# 2. Wait for it to start, then check output
sleep 2
claude-term read "$TERM_ID"

# 3. Run tests in another terminal
TEST_ID=$(claude-term spawn --cwd ~/projects/myapp bash -c "npm test")

# 4. Check test results
sleep 5
claude-term read "$TEST_ID"

# 5. Clean up
claude-term kill "$TERM_ID"
claude-term kill "$TEST_ID"
```

## Owner Tracking

Terminal ownership is automatic. When you spawn a terminal, it's automatically associated with your session. `claude-term list` shows only your terminals by default. Use `claude-term list --all` to see all terminals.

## Terminal Lifecycle

Terminals persist until explicitly killed or inactive for 24 hours (no reads, no writes, no output). They survive session disconnects and restarts.

## Interactive Attach

For interactive use (humans, not agents), `claude-term attach <id>` provides a live bidirectional connection. Press `Ctrl+]` to detach without killing the terminal.
