# claude-term

Persistent terminal management. Daemon + client library + CLI, written in Go.

## Spec

`SPEC.md` is the source of truth for what claude-term does. **Do not edit it without explicit user permission.** Everything else (implementation, docs, tests) can change freely but must stay consistent with the spec. If a change requires a spec update, propose the spec change to the user first.

## Quick Reference

- **Run tests:** `go test ./...`
- **Build:** `go build -o claude-term ./cmd/claude-term`
- **Plugin test:** `claude --plugin-dir .` (loads skill + hook for one session)
- **Standalone install:** `./claude-term install` (fallback — writes directly to `~/.claude/`)
- **Socket:** `~/.claude-term/daemon.sock` (override: `CLAUDE_TERM_SOCKET`)

## Docs

- [Protocol reference](docs/protocol.md) — all message types, fields, examples
- [CLI reference](docs/cli.md) — commands, flags, owner auto-discovery

## Project Structure

```
cmd/claude-term/     CLI entry point
internal/
  daemon/            Socket server, terminal lifecycle, inactivity reaper
  terminal/          PTY management, output buffering
  protocol/          Message types, serialization
  client/            Go client library
  owner/             Owner auto-discovery (PPID walk)
  paths/             Socket/PID file path resolution
.claude-plugin/      Plugin manifest (plugin.json)
skills/claude-term/  Plugin skill (SKILL.md)
hooks/               SessionStart hook + hooks.json (plugin hook config)
cmd/claude-term/embedded/  Embedded copies (go:embed) for standalone install
```

## Conventions

- Work directly on branches (no worktrees needed for this repo)
- Integration tests use real PTYs (spawn `echo`, `sh`, etc.)
- Newline-delimited JSON protocol over Unix socket
- Terminal IDs: sequential `t1`, `t2`, ...
- Owner auto-discovery: SessionStart hook writes `~/.claude-term/owners/<PID>`, CLI walks PPID chain to find it
