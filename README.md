# claude-term

Persistent terminal management for Claude Code sessions.

Spawn terminals that survive session disconnects, check back on long-running processes, and let multiple clients (agents, UIs, humans) interact with the same terminal simultaneously.

## Install

```bash
go install github.com/EliasSchlie/claude-term/cmd/claude-term@latest
claude-term install
```

The `install` command sets up a Claude Code skill and a SessionStart hook. No other dependencies required.

To remove:

```bash
claude-term uninstall
```

## Usage

```bash
# Spawn a terminal
claude-term spawn --cwd ~/project bash -c "npm run dev"
# t1

# Check output
claude-term read t1

# Send input
claude-term write t1 "q\n"

# List your terminals
claude-term list

# Kill it
claude-term kill t1
```

The daemon starts automatically on first use.

## How It Works

A daemon process owns PTY instances and exposes them via a Unix socket API (`~/.claude-term/daemon.sock`). The CLI and Go client library talk to the daemon over this socket.

**Owner tracking** is automatic inside Claude Code sessions. A `SessionStart` hook registers the session, and the CLI discovers it by walking the process tree. No manual flags needed — `claude-term spawn` just works.

**Terminals persist** until explicitly killed or inactive for 24 hours. They survive client disconnects, session offloads, and restarts.

**Shared access** — multiple clients can connect to the same terminal simultaneously. An agent can run commands programmatically while a human watches via `claude-term attach`.

## For Apps

Import the Go client library:

```go
import "github.com/EliasSchlie/claude-term/internal/client"

c, _ := client.Connect("")
result, _ := c.Spawn(client.SpawnOpts{
    Cmd: "bash", Args: []string{"-c", "npm run dev"},
    Owner: "my-app",
})
// result.TermID = "t1"

data, _ := c.Read(result.TermID)
```

Or speak the [protocol](docs/protocol.md) directly over the Unix socket (newline-delimited JSON).
