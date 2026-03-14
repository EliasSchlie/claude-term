# Protocol Reference

Newline-delimited JSON over Unix socket. One JSON object per line.

## Requests (Client → Daemon)

### `spawn`
```json
{"type": "spawn", "id": "req-1", "cmd": "/bin/zsh", "args": ["-l"], "cwd": "/home/user/project", "cols": 120, "rows": 40, "env": {"FOO": "bar"}, "owner": "session-abc123"}
```

| Field    | Default              | Description |
|----------|----------------------|-------------|
| `cmd`    | `$SHELL` or `/bin/sh`| Command to run |
| `args`   | `["-l"]`             | Arguments |
| `cwd`    | daemon's cwd         | Working directory |
| `cols`   | 120                  | Terminal width |
| `rows`   | 40                   | Terminal height |
| `env`    | `{}`                 | Extra env vars (merged with daemon's) |
| `owner` | `""`                 | Opaque owner identifier |

Response: `{"type": "spawned", "id": "req-1", "term_id": "t1", "pid": 12345}`

### `write`
Fire-and-forget (no `id`, no response).
```json
{"type": "write", "term_id": "t1", "data": "ls -la\n"}
```

### `read`
```json
{"type": "read", "id": "req-2", "term_id": "t1"}
```
Response: `{"type": "read_result", "id": "req-2", "term_id": "t1", "data": "<base64>"}`

### `attach`
```json
{"type": "attach", "id": "req-3", "term_id": "t1"}
```
Response: `{"type": "attached", "id": "req-3", "term_id": "t1"}`

After attaching, client receives `replay`, then ongoing `data` pushes, then `exit` when process terminates.

### `detach`
Fire-and-forget.
```json
{"type": "detach", "term_id": "t1"}
```

### `list`
```json
{"type": "list", "id": "req-4", "owner": "session-abc123"}
```
`owner` empty → return all. Response:
```json
{"type": "list_result", "id": "req-4", "terminals": [{"term_id": "t1", "pid": 12345, "cmd": "/bin/zsh", "cwd": "/home/user", "cols": 120, "rows": 40, "owner": "session-abc123", "started_at": "2026-03-14T10:00:00Z", "alive": true}]}
```

### `resize`
```json
{"type": "resize", "id": "req-5", "term_id": "t1", "cols": 200, "rows": 50}
```
Response: `{"type": "resized", "id": "req-5", "term_id": "t1"}`

### `kill`
```json
{"type": "kill", "id": "req-6", "term_id": "t1"}
```
Response: `{"type": "killed", "id": "req-6", "term_id": "t1"}`

### `ping`
```json
{"type": "ping", "id": "req-7"}
```
Response: `{"type": "pong", "id": "req-7"}`

## Push Events (Daemon → Client, no request ID)

| Type     | Description | Data encoding |
|----------|-------------|---------------|
| `data`   | Live output from attached terminal | base64 |
| `replay` | Buffer snapshot sent on attach | base64 |
| `exit`   | Process terminated | `exit_code` integer |

## Errors

```json
{"type": "error", "id": "req-1", "error": "terminal not found: t99"}
```
