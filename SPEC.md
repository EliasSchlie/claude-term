# claude-term Specification

> ⛔ **Protected.** Do not edit without explicit user permission. If you believe the spec needs a change, propose it to the user first. Implementation details not covered here are free to change — but anything that contradicts the spec must be flagged.

Persistent terminal management. Daemon owns PTY processes, exposes them via Unix socket API. CLI and Go client library wrap the API.

## Core Concepts

- **Terminal** — A PTY-backed process with buffered output. Survives client disconnects.
- **Owner** — Opaque string identifying who spawned a terminal. Enables filtering (e.g., "show me only my terminals"). Auto-discovered for Claude sessions via hook; can be overridden manually.
- **Shared access** — Any number of clients can connect to the same terminal simultaneously. All attached clients see the same output, and any client can write input. An agent, a UI, and a human can all interact with the same terminal at the same time.

## Operations

| Operation | Description |
|-----------|-------------|
| `spawn`   | Create a terminal (cmd, cwd, env, owner, dimensions) |
| `write`   | Send input to a terminal |
| `read`    | Get buffered output snapshot |
| `attach`  | Subscribe to live output + replay buffer |
| `detach`  | Unsubscribe from live output |
| `list`    | List terminals, optionally filtered by owner |
| `resize`  | Change terminal dimensions |
| `set-owner`| Change the owner of an existing terminal |
| `kill`    | Terminate a terminal and remove it |
| `ping`    | Health check |

## Interaction Modes

- **Stream mode** (`attach` + `write`) — Full interactive terminal access. Client receives live output and can type input. For UIs and humans.
- **Programmatic mode** (`read` + `write`) — Snapshot-based. Read the buffer, send a command, read again. For agents and scripts.

Both modes work on the same terminal at the same time. A human can be attached interactively while an agent reads and writes programmatically.

## Key Behaviors

- **Persistence** — Terminals survive client disconnects. Killing a client does NOT kill its terminals.
- **Inactivity timeout** — Terminals auto-close after 24 hours with no activity (no writes, no buffer changes). Only other way to close: explicit `kill`.
- **Idle shutdown** — Daemon exits after 30 min with zero terminals and zero clients.
- **Independence** — Works standalone. Does not depend on Open Cockpit, claude-pool, or any other external system. Ships its own hooks and skill.

## Layers

1. **Daemon** — Owns PTYs, manages buffers, broadcasts output. Listens on `~/.claude-term/daemon.sock`.
2. **Client** — Go library. Typed API over the socket. Import this to build apps.
3. **CLI** — `claude-term` binary. Thin wrapper over the client library.
4. **Skill** — Teaches Claude sessions when and how to use persistent terminals.
