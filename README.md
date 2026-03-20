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

# Send input (supports \n \r \t \\ \xNN escape sequences)
claude-term write t1 "q\n"

# Interactive attach (Ctrl+] to detach)
claude-term attach t1

# List your terminals
claude-term list

# Kill it
claude-term kill t1
```

The daemon starts automatically on first use.

## CLI Reference

```
claude-term start                         Start daemon (foreground)
claude-term stop                          Stop daemon
claude-term spawn [cmd] [args...]         Spawn terminal, print term_id
claude-term list                          List your terminals (JSON)
claude-term list --all                    List all terminals
claude-term write <id> <input>            Write input to terminal
claude-term read <id>                     Read buffered output
claude-term attach <id>                   Interactive bidirectional attach
claude-term resize <id> <cols> <rows>     Resize terminal
claude-term set-owner <id> <owner>        Change terminal owner
claude-term kill <id>                     Kill terminal
claude-term subscribe                     Stream lifecycle events as JSON lines
claude-term ping                          Health check
claude-term install                       Install hooks and skill into Claude Code
claude-term uninstall                     Remove hooks and skill from Claude Code
```

### Flags

| Flag | Commands | Description |
|------|----------|-------------|
| `--owner <id>` | spawn, list | Override auto-discovered owner |
| `--all` | list | Show all terminals, not just yours |
| `--cwd <dir>` | spawn | Working directory |
| `--cols <n>` | spawn | Terminal width (default: 120) |
| `--rows <n>` | spawn | Terminal height (default: 40) |

`write` interprets C-style escape sequences: `\n`, `\r`, `\t`, `\\`, `\xNN`.

### Lifecycle events

`subscribe` streams JSON lines to stdout until interrupted:

```bash
claude-term subscribe
# {"type":"term_spawned","term_id":"t1","owner":"session-abc","cmd":"/bin/zsh","cwd":"/home/user","pid":12345}
# {"type":"term_exited","term_id":"t1","exit_code":0}
```

Event types: `term_spawned`, `term_killed`, `term_exited`, `term_owner_changed`.

## How It Works

A daemon process owns PTY instances and exposes them via a Unix socket API (`~/.claude-term/daemon.sock`). The CLI and Go client library talk to the daemon over this socket.

**Owner tracking** is automatic inside Claude Code sessions. A `SessionStart` hook registers the session, and the CLI discovers it by walking the process tree. No manual flags needed — `claude-term spawn` just works.

**Terminals persist** until explicitly killed or inactive for 24 hours. They survive client disconnects, session offloads, and restarts. Killing a client does not kill its terminals.

**Shared access** — multiple clients can connect to the same terminal simultaneously. An agent can run commands programmatically while a human watches via `claude-term attach`.

**Daemon lifecycle** — starts automatically on first use, exits automatically after 30 minutes with no terminals and no clients.

### Architecture

```
Daemon  — owns PTYs, buffers output, broadcasts to attached clients
  └── Client library  — typed Go API over the Unix socket
        └── CLI  — thin wrapper over the client library
              └── Skill  — teaches Claude when and how to use persistent terminals
```

## Go Client Library

```go
import "github.com/EliasSchlie/claude-term/internal/client"

c, _ := client.Connect("")
defer c.Close()

// Spawn a terminal
result, _ := c.Spawn(client.SpawnOpts{
    Cmd:   "bash",
    Args:  []string{"-c", "npm run dev"},
    Cwd:   "/home/user/project",
    Owner: "my-app",
})
// result.TermID = "t1", result.PID = 12345

// Read buffered output
data, _ := c.Read(result.TermID)

// Send input
c.Write(result.TermID, "q\n")

// Stream live output
c.SetPushHandler(client.PushHandler{
    OnReplay: func(_ string, data []byte) { os.Stdout.Write(data) }, // buffer snapshot on attach
    OnData:   func(_ string, data []byte) { os.Stdout.Write(data) }, // live output
    OnExit:   func(_ string, code int)    { fmt.Printf("exited: %d\n", code) },
})
c.Attach(result.TermID)

// Subscribe to lifecycle events
c.SetPushHandler(client.PushHandler{
    OnTermSpawned: func(msg *protocol.Message) { /* ... */ },
    OnTermKilled:  func(msg *protocol.Message) { /* ... */ },
})
c.Subscribe()
<-c.Done() // block until connection lost

// Other operations
c.List("")                          // all terminals
c.List("my-app")                    // filtered by owner
c.Resize(result.TermID, 200, 50)
c.SetOwner(result.TermID, "other")
c.Kill(result.TermID)
c.Ping()
```

Or speak the [protocol](docs/protocol.md) directly over the Unix socket (newline-delimited JSON).
