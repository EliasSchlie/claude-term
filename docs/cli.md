# CLI Reference

```
claude-term start                        Start daemon (foreground)
claude-term stop                         Stop daemon
claude-term spawn [cmd] [args...]        Spawn terminal, print term_id
claude-term list                         List your terminals (JSON)
claude-term list --all                   List all terminals
claude-term write <id> <input>           Write input to terminal (supports \n \r \t \\ \xNN escapes)
claude-term read <id>                    Read buffered output
claude-term attach <id>                  Interactive bidirectional attach (Ctrl+] to detach)
claude-term resize <id> <cols> <rows>    Resize terminal
claude-term kill <id>                    Kill terminal
claude-term subscribe                    Stream lifecycle events as JSON lines (Ctrl+C to stop)
claude-term ping                         Health check
```

## Flags

| Flag | Commands | Description |
|------|----------|-------------|
| `--owner <id>` | spawn, list | Override auto-discovered owner |
| `--all` | list | Show all terminals, not just yours |
| `--cwd <dir>` | spawn | Working directory |
| `--cols <n>` | spawn | Terminal width (default: 120) |
| `--rows <n>` | spawn | Terminal height (default: 40) |

## Owner Auto-Discovery

When run inside a Claude session (with the SessionStart hook installed), `spawn` and `list` automatically detect which session they belong to. No `--owner` flag needed.

Outside of Claude sessions, `--owner` must be passed explicitly, or terminals will have no owner.
