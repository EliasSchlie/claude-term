#!/usr/bin/env bash
# claude-term SessionStart hook
# Registers this Claude session with the claude-term daemon.
# Reads session_id from stdin JSON, generates a stable owner ID,
# and writes it to ~/.claude-term/owners/<PID> for auto-discovery.

set -euo pipefail

CLAUDE_TERM_DIR="${CLAUDE_TERM_DIR:-$HOME/.claude-term}"
OWNERS_DIR="$CLAUDE_TERM_DIR/owners"
mkdir -p "$OWNERS_DIR"

# Read hook input from stdin (JSON with session_id)
read -t 1 -r input 2>/dev/null || true

# Extract session_id using simple pattern matching (no jq dependency)
session_id=""
if [ -n "${input:-}" ]; then
    session_id=$(echo "$input" | sed -n 's/.*"session_id"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')
fi

# Use session_id as the owner ID (it's already a UUID).
# If we can't get it, generate one.
if [ -n "$session_id" ]; then
    owner_id="$session_id"
else
    owner_id=$(uuidgen 2>/dev/null || cat /proc/sys/kernel/random/uuid 2>/dev/null || echo "unknown-$$")
fi

# Write owner file keyed by Claude Code's PID (our parent process)
claude_pid="$PPID"
echo "$owner_id" > "$OWNERS_DIR/$claude_pid"

# Also write a reverse mapping: owner_id -> pid (for debugging/lookup)
echo "$claude_pid" > "$OWNERS_DIR/by-owner-$owner_id"
